# 正式匿名评分

先重新阅读 rubric.md 的最终冻结版本。预审已澄清 adaptation：首轮本就采用合格后台刷新时，可以解释影响后保持原流程，不强迫换方案。

每位评分者只读分配的 packet-N.json、rubric.md 和 authoring/interaction-case.json 的业务期望，不读 Skill、condition-map、reader-map、原始运行目录、另一位评分或旧 eval。包里是完整自然对话，编号与条件无关。以题目原句作依据，允许等价实现；不要预设某条件应该更好。评分时不联网，不读记忆。

评分格式为 {"trials":[...]}。每 trial 包含 blind_id、case、dimensions、strengths、concrete_issues；八个 dimensions 的键及内部结构依照 rubric。NA 也要有 reason 与 evidence:[]。每个非 NA 的 reason 说明具体依据，evidence 每项用 turn 数字和逐字 quote；引用尽量短，不要改写、省略号拼接或换用全角符号。

export 还必须有三项 interaction_rows，id 与独立判据一致，字段按 rubric 定义。分别记录第一次已主动解释的交错、只有防护的情形、提示后补充与修正。其他题 interaction_rows 可以为 []。不得把第二轮进步回填给首轮。

重点区分：

- 建议方案写得完整，不必然是未经讨论的最终定稿；看其决定身份和表达能否支持评审。
- 有效期、失败传播、数据迁移等如果改变正确机制，可以属于设计；完整测试计划、部署配置和日常运行指标不自动属于本次设计范围。
- 明确解释来源接口尚缺能力而暂停，不自动等于不会修正或不清楚；但也不能称已交付完整最终文档。
- 关键步骤缺失与常规细节未展开分开；无法只凭文字认定有效时标明不足，不靠自己补代码替作者通过。

对每个 trial 给短而具体的 strengths / concrete_issues，并引用支持性原话。完成后写指定 grades-N.json；可以另写 notes-N.md 说明最重要的差异和判断边界。不要试图猜条件。
