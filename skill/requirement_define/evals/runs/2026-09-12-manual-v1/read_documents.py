"""只给最终需求与问题做模型阅读检查；不向读者提供原始对话或标准答案。"""
import concurrent.futures,json,sys
from run_eval import ROOT
from grade_eval import call


def read(cid):
    reference=(ROOT/f'frozen-suite/reviewer/{cid}.md').read_text()
    section=reference.split('## 文档独立阅读检查\n',1)[1].split('## 判据来源',1)[0]
    questions=[line[2:].split('（',1)[0] for line in section.splitlines() if line.startswith('- ')]
    docs=[]
    for folder in sorted((ROOT/'trials').glob(cid+'-r*')):
        files=sorted(folder.glob('turn-*.assistant.md'))
        if files: docs.append({'run':folder.name,'document':files[-1].read_text()})
    prompt='''你是首次接触这些需求文档的开发者。只依据每份文档回答下列业务问题，不读取任何文件，不使用工具，不推测未说明的规则。无法从文档确定时回答 UNKNOWN 并说明歧义。每个答案引用文档中的短证据。不同文档独立回答，不把另一文档信息补到本份里。
只输出 JSON：{"documents":[{"run":"...","answers":[{"question":"...","answer":"...","quote":"..."}]}]}。
问题：\n'''+json.dumps(questions,ensure_ascii=False)+'\n文档：\n'+json.dumps(docs,ensure_ascii=False)
    call('reader-'+cid,prompt)

if __name__=='__main__':
    with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:list(pool.map(read,sys.argv[1:]))
