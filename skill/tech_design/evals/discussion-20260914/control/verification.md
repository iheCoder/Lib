# 实验材料核验

核验时间（UTC）：2026-09-14T11:06:42.891433+00:00

- 临时实验目录中 53 份原始文件均归档，并逐文件比对 SHA-256 一致。
- 冻结输入、现版 skill 和实验协议与 input-manifest.json 一致。
- 4 段对话共 16 份回复均存在；对应用户输入均存在；回复哈希与字符数与 output-manifest.json 一致。
- 候选 B 与候选冻结记录哈希一致。
- 正式 SKILL.md 与实验 A 快照一致；正式 SKILL.md 和 agents/openai.yaml 没有工作区差异。
- 报告及评估索引中的 33 个本地 Markdown 链接目标均存在。
- git diff --check -- skill/tech_design/evals 通过。此命令不覆盖未跟踪文件；新材料完整性另由上述文件与哈希检查核验。
- Skill A: Skill is valid!
- Skill B: Skill is valid!

格式检查使用 skill-creator 的 quick_validate.py，PyYAML 来自临时目录 codex-tech-design-validation-deps。未修改系统依赖、业务代码或生产数据库。

这些检查证明材料完整性与 skill 格式，不证明技术方案、实际 SQL、锁等待、性能或模型泛化效果。语义判断见 report.md 与 review/assessment.md。
