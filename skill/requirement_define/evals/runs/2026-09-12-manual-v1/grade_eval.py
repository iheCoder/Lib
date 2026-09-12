"""对真实输出做同模型辅助评判；结果须由主持人复核，不能替代人类可用性测试。"""
from pathlib import Path
import concurrent.futures,json,subprocess,time,sys
from run_eval import ROOT,COMMON

RUBRIC=(ROOT/'frozen-suite/reviewer/rubric.md').read_text()
SKILL=(ROOT/'skill-snapshot.md').read_text()
INSTRUCTIONS='''你是需求对话评判者。只评下面给出的记录，不读取文件，不调用工具，不代替被测者回答需求。
按评判标准逐检查点评价。严格区分：明确建议/待确认不等于编造；等价自然表述不等于遗漏；依据真实业务联动的新问题不等于无效追问；题目重要歧义记 FIXTURE_ISSUE/UNKNOWN，不能迎合参考答案。没有被记录的表现不能猜测。
不要预设运行条件优劣，输出一个 JSON 对象，格式为：
{"trials":[{"run":"...","checkpoints":[{"turn":1,"verdict":"PASS|FAIL|UNKNOWN|FIXTURE_ISSUE|ENVIRONMENT_GAP","reason":"中文","quotes":["准确短引文"]}],"overall":"...","errors":[{"type":"错误类型","turn":1,"quote":"准确原文","reason":"业务影响"}],"fixture_concerns":[],"document_readiness":"可独立使用/不足/不适用","reader_questions_answerable":"仅模型审查结论，未做人类理解验证"}]}
不要把多轮后修正擦除前一轮错误。没有错误时 errors 为空。不要因为禁止调用工具而把原被测者的真实工具调用当违规。
'''


def call(name,prompt):
    dest=ROOT/'grading';dest.mkdir(exist_ok=True)
    work=Path('/private/tmp/requirement-define-eval-20260912-grading')/name;work.mkdir(parents=True,exist_ok=True)
    (dest/f'{name}.prompt.md').write_text(prompt)
    cmd=COMMON+['--ephemeral','--sandbox','read-only','-C',str(work),'-o',str(dest/f'{name}.json'),'-']
    t=time.time()
    with (dest/f'{name}.events.jsonl').open('w') as out,(dest/f'{name}.stderr.txt').open('w') as err:
        try:
            r=subprocess.run(cmd,input=prompt,text=True,stdout=out,stderr=err,cwd=work,timeout=240);code=r.returncode
        except subprocess.TimeoutExpired:code='TIMEOUT'
    print(name,code,round(time.time()-t,1),flush=True)


def grade(cid):
    inputs=[]
    for folder in sorted((ROOT/'trials').glob(cid+'-r*')):
        turns=[]
        for p in sorted(folder.glob('turn-*.user.md')):
            n=int(p.name.split('.')[0].split('-')[1]);reply=folder/f'turn-{n}.assistant.md'
            if not reply.exists():continue
            events=[json.loads(l) for l in (folder/f'turn-{n}.events.jsonl').read_text().splitlines() if l.strip().startswith('{')]
            commands=[]
            for e in events:
                item=e.get('item',{})
                if e.get('type')=='item.completed' and item.get('type')=='command_execution':
                    # 命令完整保留，公开材料与工具错误的输出另带入；被测技能已作为判据单独提供。
                    commands.append({'command':item.get('command'),'exit_code':item.get('exit_code'),'output':item.get('aggregated_output','') if ('refund-policy' in str(item) or 'business.md' in str(item.get('command')) or 'prototype.md' in str(item.get('command'))) else '[工具输出见完整轨迹；此处省略重复的 skill 内容]'})
            turns.append({'turn':n,'user':p.read_text(),'assistant':reply.read_text(),'commands':commands})
        inputs.append({'run':folder.name,'turns':turns})
    prompt=INSTRUCTIONS+'\n行为约定：\n'+SKILL+'\n通用判据：\n'+RUBRIC+'\n本例参考：\n'+(ROOT/f'frozen-suite/reviewer/{cid}.md').read_text()+'\n公开材料：\n'
    for p in sorted((ROOT/f'frozen-suite/cases/{cid}/materials').glob('*')):
        prompt+=p.name+'\n'+p.read_text()+'\n'
    prompt+='\n待评记录：\n'+json.dumps(inputs,ensure_ascii=False)
    call(cid,prompt)

if __name__=='__main__':
    if sys.argv[1]=='calibrate':
        call('calibration','''你是需求对话评分者。不使用工具。判断以下四项是否构成错误，仅输出 JSON 数组，每项含 id、is_error、reason。
A：用户没有说明移交后原负责人的权限。Agent 说“建议原负责人仅保留查看权，你是否同意？”并等待回答。
B：同样输入，Agent 文档直接写“原负责人自动仅保留查看权，以上需求已经明确”。
C：业务规则齐全。Agent 准确且完整地用几段自然语言交付需求，没有背景/目标六个固定标题。
D：用户将取消截止改为至少提前24小时，Agent 新文档一处正确写了新边界，另一处仍写“用户在活动开始前随时可取消”。
''')
    else:
        with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
            list(pool.map(grade,sys.argv[1:]))
