"""核对运行完整性、工具轨迹和冻结版本，保存宿主实际消息用于追溯。"""
from pathlib import Path
import hashlib,json,shutil
from run_eval import ROOT,SUITE,SKILL,WORK

sessions={}
for path in Path('/Users/ihewe/.codex/sessions/2026/09/12').glob('*.jsonl'):
    # 文件名末尾即 session id，不读取无关历史。
    sessions[path.name[-42:-6]]=path
summary=[]
for folder in sorted((ROOT/'trials').iterdir()):
    if not folder.is_dir():continue
    row={'run':folder.name,'turns':[],'commands':[],'outside_scope_commands':[],'skill_read_observed':False,'memory_injected':False,'skill_catalog_injected':False}
    session=None
    for p in sorted(folder.glob('turn-*.meta.json')):
        meta=json.loads(p.read_text());row['turns'].append(meta);session=meta['session_id']
        epath=folder/f"turn-{meta['turn']}.events.jsonl"
        for line in epath.read_text().splitlines():
            try:e=json.loads(line)
            except json.JSONDecodeError:continue
            item=e.get('item',{})
            if e.get('type')=='item.completed' and item.get('type')=='command_execution':
                cmd=item.get('command','');row['commands'].append({'turn':meta['turn'],'command':cmd,'exit_code':item.get('exit_code')})
                if 'SKILL.md' in cmd and '需求定义协作者' in item.get('aggregated_output',''):row['skill_read_observed']=True
                # 这是保守的人工审查提示，不声称静态规则能证明文件访问隔离。
                if any(x in cmd for x in ['/Users/', '..', 'curl ', 'wget ', 'http:', 'https:', '/private/tmp/']):row['outside_scope_commands'].append(cmd)
    if session in sessions:
        shutil.copy2(sessions[session],folder/'session.rollout.jsonl')
        for line in (folder/'session.rollout.jsonl').read_text().splitlines():
            e=json.loads(line);p=e.get('payload',{})
            if e.get('type')=='response_item' and p.get('role') in ['developer','user']:
                text='\n'.join(x.get('text','') for x in p.get('content',[]))
                row['memory_injected']|='MEMORY_SUMMARY' in text
                row['skill_catalog_injected']|='### Available skills' in text
    row['snapshot_saved']=(folder/'session.rollout.jsonl').exists()
    summary.append(row)
config=json.loads((ROOT/'run-config.json').read_text())
assert hashlib.sha256(SKILL.read_bytes()).hexdigest()==config['skill_sha256']
assert hashlib.sha256((SUITE/'manifest.json').read_bytes()).hexdigest()==config['case_material_sha256']
(ROOT/'trajectory-audit.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2)+'\n')
print(json.dumps({'trials_seen':len(summary),'completed_turns':sum(t['completed'] for r in summary for t in r['turns']),'errors':[(r['run'],t['turn']) for r in summary for t in r['turns'] if not t['completed']],'skill_read':sum(r['skill_read_observed'] for r in summary),'memories_injected':sum(r['memory_injected'] for r in summary),'catalogs_injected':sum(r['skill_catalog_injected'] for r in summary),'scope_flags':[(r['run'],r['outside_scope_commands']) for r in summary if r['outside_scope_commands']],'rollouts_saved':sum(r['snapshot_saved'] for r in summary)},ensure_ascii=False))
