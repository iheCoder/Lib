# Research Basis

这些材料只解释本 skill 的设计取舍。执行具体任务时，当前需求、仓库契约和真实调用方证据优先。

## 关键取舍

### 1. 覆盖率不是充分性证明

[MUTGEN (2025)](https://arxiv.org/abs/2506.02954) 报告了覆盖率很高但变异杀伤能力很弱的测试集，并用 mutation feedback 改进故障检测。因此本 skill 不把 line/branch coverage 当完成条件，而把 plausible fault 是否被场景暴露作为核心审查。

[2026 replication study](https://arxiv.org/abs/2607.22880) 进一步指出 coverage/mutation 的解释依赖语境：当当前代码可能已有 Bug 且目标是暴露它时，这些代理指标并不可靠。故本 skill 先区分 regression 与 buggy-code discovery。

### 2. Specification-first，避免被错误实现诱导

[Misguidance Effect study (2026)](https://arxiv.org/abs/2607.22883) 发现 buggy code 会增加确认错误行为的 tests，并压制真正的 bug-finding tests；基于 specification 的生成可缓解该问题。因此当前代码只能作为执行路径和观察证据，不能在 Bug 语境下独自定义 Oracle。

### 3. 同时审查 Fault Coverage 与 Fault Exposure

[TestCase-Eval (2025)](https://arxiv.org/abs/2506.12278) 将能力区分为覆盖不同故障类型，以及构造能真正暴露具体错误实现的输入。本 skill 的 Critic 同时检查“风险是否被想到”和“场景是否真的能杀死它”。

[IntTestGen (2026)](https://arxiv.org/abs/2605.26851) 聚焦使用真实项目内依赖的 low-level integration tests，强调 mock-based unit tests 会跳过对象构造、API 序列和组件交互 fault。故本 skill 将 verification fidelity、真实依赖语义和“不要 mock 掉 target property”加入充分性条件。

### 4. 强模型需要反馈和清晰标准，不需要僵硬 taxonomy

[TestForge (2025)](https://arxiv.org/abs/2503.14713) 通过执行与覆盖反馈迭代修复、扩展测试；[newer-model replication (2026)](https://arxiv.org/abs/2601.09695) 则显示现代模型上的简单方法可超过多种旧式复杂 pipeline。故本 skill 保留短而强的 Lens、Critic 与可观测反馈，不设置几十类机械规则。

[Evaluator-optimizer pattern](https://www.anthropic.com/engineering/building-effective-agents) 支持在评价标准清晰时用生成—评价循环。因此 Designer 与 Critic 使用不同任务表示；Critic 先独立重建风险面，避免只改写 Designer 的答案。

[TestAgent (2026)](https://arxiv.org/abs/2607.09101) 使用 requirement planner、test generator、test reviewer 与测试专用上下文协作，支持 planner/reviewer 角色和上下文分离。这里不强制多 agent，但要求 blind critique 在 candidate reveal 前固化 fault inventory。

### 5. Oracle、property 与组合交互

[CANDOR (2025)](https://arxiv.org/abs/2506.02943) 强调 test prefix 与 Oracle 都影响测试质量；本 skill 进一步要求 Oracle 来源可追溯，避免把模型共识误当业务事实。

[Metamorphic testing survey (2026)](https://arxiv.org/abs/2605.13898) 总结了用多次相关执行之间的必要关系缓解 Oracle 问题的研究，因此 property/metamorphic relation 是一级设计判断，但只有在关系本身有证据时才使用。

[Software evolution study (2026)](https://arxiv.org/abs/2603.23443) 研究 LLM 测试对 semantic-changing 与 semantic-preserving changes 的反应，提示对表面程序形态和 regression awareness 的风险。因此 Bug 修复使用 exact reproducer + fault mechanism + generalized neighbor，防止只 hardcode 事故实例。

[NIST combinatorial testing](https://csrc.nist.gov/pubs/journal/2024/02/combinatorial-testing-for-building-reliable-system/final) 说明少数参数交互可用 t-way 组合高效覆盖。这里把它作为已识别 interaction risk 的压缩工具，而不是默认展开所有参数组合。

### 6. Agentic TDD 的实际工作习惯

[First run the tests](https://simonwillison.net/guides/agentic-engineering-patterns/first-run-the-tests/) 强调既有测试帮助 agent 建立仓库语境并持续验证修改；[How OpenAI uses Codex](https://cdn.openai.com/pdf/6a2631dc-783e-479b-b1a4-af0cfbd38630/how-openai-uses-codex.pdf) 展示了用 property-based tests 改善测试覆盖的实践。故本 skill 先记录 baseline，并在适合时优先稳定 property，而不是只生成 examples。

### 7. AI Agent 需要独立 Eval Route

[Anthropic agent eval practice (2026)](https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents) 将 agent eval 区分为 task、trial、trajectory、outcome、grader 和 harness，强调非确定性行为需要 multiple trials、隔离环境，以及 code/model/human graders 的组合与校准。故本 skill 不用单次 testcase 评估 agent，并同时审查工具轨迹和真实环境结果。

[OpenAI Graders API](https://platform.openai.com/docs/api-reference/graders) 提供确定性 string/code grader、model grader 和 multi-grader 等组合机制。这里抽象为 grader stack，不绑定某一供应商；P0 forbidden effect 优先使用确定性 binary gate，开放质量才交给校准后的模型 grader。
