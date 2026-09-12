"""运行已冻结的手工案例，逐轮保存真实 CLI 输出；评分和条件回答由主持人另行处理。"""
from pathlib import Path
import concurrent.futures, hashlib, json, subprocess, shutil, time, sys

ROOT = Path(__file__).resolve().parent
SUITE = ROOT.parent.parent / 'manual-v1'
SKILL = ROOT.parent.parent.parent / 'SKILL.md'
CLI = '/Applications/ChatGPT.app/Contents/Resources/codex'
WORK = Path('/private/tmp/requirement-define-eval-20260912')
# 禁用宿主的其他技能，防止评估目标被额外流程替代；不改变全局配置。
skills=[]
for base in [Path.home()/'.codex/skills', Path.home()/'.agents/skills']:
    if base.exists():
        for p in base.glob('*/SKILL.md'): skills.append({'path':str(p), 'enabled':False})
        for p in base.glob('.system/*/SKILL.md'): skills.append({'path':str(p), 'enabled':False})
# JSON 数组中的对象不适用于 TOML 内联表，显式构造已知路径配置。
skill_config='skills.config=['+','.join('{path='+json.dumps(x['path'])+',enabled=false}' for x in skills)+']'
SCOPE='本次任务只使用当前工作目录中的需求材料和用户后续消息。不要读取其他目录、个人记忆或其他技能，不要联网，不要委派。问题直接写在回复中。工作区只读；在回复中交付内容。'
COMMON=[CLI,'exec','--ignore-user-config','--enable','respect_system_proxy','--disable','memories','--disable','plugins','--disable','apps','--disable','multi_agent','--disable','hooks','--skip-git-repo-check','-m','gpt-6-astra','-c','model_reasoning_effort="medium"','-c','project_doc_max_bytes=0','-c',skill_config,'-c','developer_instructions='+json.dumps(SCOPE,ensure_ascii=False),'--json']


def save(p,v):
    p.parent.mkdir(parents=True,exist_ok=True)
    p.write_text(json.dumps(v,ensure_ascii=False,indent=2)+'\n')


def turn(run, number, prompt, session=None):
    work=WORK/run; dest=ROOT/'trials'/run
    dest.mkdir(parents=True,exist_ok=True)
    (dest/f'turn-{number}.user.md').write_text(prompt)
    cmd=COMMON+(['resume',session] if session else ['--sandbox','read-only','-C',str(work)])+['-o',str(dest/f'turn-{number}.assistant.md'),'-']
    start=time.time()
    with (dest/f'turn-{number}.events.jsonl').open('w') as out,(dest/f'turn-{number}.stderr.txt').open('w') as err:
        try:
            result=subprocess.run(cmd,input=prompt,text=True,stdout=out,stderr=err,cwd=work,timeout=240)
            code=result.returncode
        except subprocess.TimeoutExpired: code='TIMEOUT'
    events=[]
    for line in (dest/f'turn-{number}.events.jsonl').read_text().splitlines():
        try: events.append(json.loads(line))
        except json.JSONDecodeError: pass
    session=next((e['thread_id'] for e in events if e.get('type')=='thread.started'),session)
    completed=any(e.get('type')=='turn.completed' for e in events)
    meta={'run':run,'turn':number,'session_id':session,'exit_code':code,'completed':completed,'elapsed_seconds':round(time.time()-start,2),'usage':[e.get('usage') for e in events if e.get('type')=='turn.completed']}
    save(dest/f'turn-{number}.meta.json',meta)
    print(json.dumps(meta,ensure_ascii=False),flush=True)
    return session,completed


def initial(run):
    cid=run[:3]; work=WORK/run; work.mkdir(parents=True,exist_ok=True)
    shutil.copy2(SKILL,work/'SKILL.md')
    shutil.copy2(SUITE/'cases'/cid/'input.md',work/'input.md')
    materials=SUITE/'cases'/cid/'materials'
    if materials.exists(): shutil.copytree(materials,work/'materials',dirs_exist_ok=True)
    prompt='请使用当前目录 SKILL.md 中的 requirement-define skill 帮我梳理下面的需求。\n\n'+(work/'input.md').read_text()
    session,ok=turn(run,1,prompt)
    if not ok: return
    # 固定时间到达的新信息不依赖评分，错误首轮也会保留。
    for number in [2,3]:
        path=SUITE/'cases'/cid/f'turn-{number}.md'
        if path.exists():
            session,ok=turn(run,number,path.read_text(),session)
            if not ok: break


def conditional(run):
    if (ROOT/'trials'/run/'turn-2.meta.json').exists(): return
    decision=json.loads((ROOT/'conditional-decisions.json').read_text())[run]
    if decision['action']=='STOP': return
    session=json.loads((ROOT/'trials'/run/'turn-1.meta.json').read_text())['session_id']
    name='answer-permission.md' if run.startswith('c01') else 'answer-scope.md'
    turn(run,2,(SUITE/'cases'/run[:3]/name).read_text(),session)

if __name__=='__main__':
    phase=sys.argv[1]
    save(ROOT/'run-config.json',{'model':'gpt-6-astra','reasoning':'medium','trials_per_case':3,'condition':'skill','cli':'0.153.4','sandbox':'read-only','case_material_sha256':hashlib.sha256((SUITE/'manifest.json').read_bytes()).hexdigest(),'skill_sha256':hashlib.sha256(SKILL.read_bytes()).hexdigest(),'developer_scope':SCOPE,'common_args':COMMON,'independence':'new CLI session per case/trial; no memory; grader not in participant workspace; host retains filesystem read capability'})
    runs=[f'c{i:02d}-r{r}' for r in range(1,4) for i in [1,2,3,8,9,10,4,5,6,7,11,12]] if phase=='initial' else list(json.loads((ROOT/'conditional-decisions.json').read_text()))
    with concurrent.futures.ThreadPoolExecutor(max_workers=3 if phase=='initial' else 1) as pool:
        list(pool.map(initial if phase=='initial' else conditional,runs))
