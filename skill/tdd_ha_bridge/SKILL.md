---
name: tdd-ha-bridge
description: 为普通开发者生成易读、可快速审批的测试或 AI agent eval 设计；默认用业务语言展示待决策事项、六类 Lens 覆盖和未证明风险，并在内部用风险建模、预期来源、验证真实度与错误实现挑战保证严谨性。适用于“先审场景和验证方法、再写测试或实现”；不用于机械追求用例数或覆盖率。
---

# TDD Human-Approval Bridge

这个 skill 交付的不是“尽可能多的测试用例”，而是一份普通后端开发者能在 1–3 分钟内找到重点的测试审批材料：

- 系统必须守住哪些行为义务；
- 为什么选这些高价值场景；
- 每个场景准备捕获什么现实错误；
- 需要什么真实度、控制点和观察点才能实际暴露错误；
- 哪些重要行为仍不确定、只写了场景但还测不出来，或完全没覆盖；
- 工程师此刻需要决定什么，批准后下一步做什么。

模型内部仍要完成 `Behavior → Risk → Fault → Verification → Coverage` 的严谨推演，但**这条推演链不是默认用户界面**。不要把内部 ID 图、专家术语和重复映射直接倾倒给读者。

默认只完成测试/eval 设计与充分性审查。除非用户明确要求继续，否则不要编写测试代码、eval harness 或生产实现；把 Human Approval 保留为真实边界。

## 六个不可妥协的原则

1. **Behavior first**：先建行为、不变量、状态转换和禁止行为，再列场景。
2. **Adaptive risk decomposition**：按当前变更的风险结构选择 Lens，不为凑分类而制造测试。
3. **Oracle provenance**：关键 Expected 必须有来源；无法确认就写 `UNKNOWN`，不能替业务做决定。
4. **Fault challenge**：寻找能通过现有场景、但违反契约的 plausible wrong implementation。
5. **Executable witness**：P0/P1 不只要有场景，还必须有足够真实、可控、可观察的验证策略。不要 mock 掉正在验证的语义。
6. **Internal rigor, external simplicity**：内部保留完整证据链；外部先用业务语言呈现“要守住什么、哪里没测到、需要决定什么”。人的注意力应集中到高风险、预期不明确、测不出来和完全未覆盖的部分。

## 默认读者与两层产物

默认读者是熟悉业务和代码、但不要求掌握测试理论术语的普通后端开发者。除非用户明确要求“专家版”“完整充分性论证”或需要留存审计证据，否则只交付第一层。

### 第一层：开发者审批主文档

必须让读者不追踪任何 `B/R/T/V/M` ID，就能回答：

1. 这次最需要防住哪几个真实错误？
2. 六类 Lens 中哪些已覆盖、部分覆盖、未覆盖或与本次无关？
3. 目前哪些测试已经跑通，哪些只是纸面场景，哪些完全没有验证手段？
4. 有哪 0–3 个问题需要人做业务选择？推荐选项及影响是什么？
5. 批准后最小下一步是什么？

严格按 [references/output-contract.md](references/output-contract.md) 生成。主文档不用 `Behavior Map`、`Risk Map`、`Plausible Fault Challenge`、`Oracle`、`linearization`、`harness`、`seam`、`survivor`、`pseudo-killer` 等词充当标题或关键结论。确需出现代码/架构术语时，先说人话，再在括号里给术语。

### 第二层：技术审计证据

内部分析始终执行，但只在以下情况输出：用户要求完整证据；存在高风险未决项需要精确追踪；产物需要进入正式审计。优先写入独立的 `*.evidence.md`，不要把主文档重新拉长。它可以保留 Source Ledger、`B/R/T/V/M` 映射、验证真实度声明和错误实现挑战。

第二层不能改变第一层结论。若技术证据显示某项只是纸面覆盖，主文档必须明确写“部分覆盖”或“未覆盖”，不能只在附录降级。

## 语言与可理解性规则

- 先描述现实事件，再给术语。例如：“同一轮任务被重试，或两台服务同时执行”优先于“跨 trigger/跨实例幂等”。
- 问人决策时使用完整问题：“这种情况下同一用户能不能收到两份结果？”不要让读者解释名词后才能作答。
- 每个问题给出推荐选项、推荐理由、选择其他方案的具体后果；没有证据时可以不推荐，但要说明缺什么。
- 状态统一写成 `已覆盖 / 部分覆盖 / 未覆盖 / 不适用`。不要只给 `✓ △ ≈ ? ✗` 让读者查图例。
- `部分覆盖` 必须说明缺的是业务规则、触发手段、真实依赖、观察点还是尚未执行。
- ID 只用于追踪，不能成为句子主语，不能要求读者在多个表之间来回跳转。
- 同一事实默认只解释一次。主文档不得分别在规则、风险、场景、验证、错误挑战中重复五遍。

## 工作边界

### 适用

- 新功能或行为变更，希望先审批测试设计；
- Bug 修复，需要防止把当前错误行为固化成回归测试；
- 重构或迁移，需要明确 preserved behaviors；
- 幂等、事务、消息、缓存、并发、超时、重试等后端高风险链路；
- 已有测试计划，需要独立审查“够不够”和“Expected 是否可信”。
- 使用 LLM、多轮交互、工具调用和环境副作用的 AI agent，需要设计多 trial eval、trajectory/outcome grader 与安全边界。

### 不做

- 用 testcase 数量、行覆盖率或分支覆盖率冒充充分性；
- 从当前实现直接抄出 Expected，并把它包装成需求；
- 穷举与本次变更无关的极端输入；
- 在需求不明时擅自决定 rollback、retry、兼容性或错误码语义；
- 用 repository mock 验证真实事务提交歧义，或用随机 sleep/重复运行冒充并发控制；
- 用一次成功 trial 宣称非确定性 agent 可靠，或只检查最终文本而忽略工具调用和环境状态；
- 声称“覆盖全部场景”。除非状态空间有限且已形式化枚举，否则只能给出有边界的充分性论证。

## 先选择验证路由

- **Deterministic Software Route**：传统函数、服务、数据库、消息、迁移、并发协议等；按本文件主流程和 [references/design-protocol.md](references/design-protocol.md) 执行。
- **AI Agent Eval Route**：结果会随模型采样变化，且系统会多轮推理、调用工具或修改环境；读取并执行 [references/agent-eval-protocol.md](references/agent-eval-protocol.md)。不要把它降格成单次 deterministic testcase。
- **Hybrid Route**：agent 外层行为使用 Agent Eval；工具权限、金额阈值、幂等、状态写入等确定性 guardrail 同时使用 Software Route。P0 安全边界尽量由确定性代码强制，并单独测试。

## 先校准任务语境

开始前明确本次属于哪一种：

| 语境 | 当前实现可否作为 Oracle | 主要验证目标 |
| --- | --- | --- |
| 新功能 | 否 | 新契约与风险 |
| 已知 Bug 修复 | 否，尤其是缺陷路径 | 测试能在修复前暴露 Bug、修复后通过 |
| 可信行为的回归增强 | 可作为较弱证据 | 防止未来变更破坏现有行为 |
| 纯重构 | 可与测试、调用方共同作为证据 | preserved behaviors 与副作用不变 |
| 需求不明确的遗留行为 | 仅作观察，不作事实 | 暴露冲突并请求裁决 |

如果代码可用，先读取仓库规则、相关需求、接口、调用方、既有测试和变更范围。安全可行时先运行相关既有测试，记录 baseline；测试失败不能被自动解释为“旧测试错了”。

## 证据与 Oracle 纪律

为关键行为建立 Source Ledger，区分：

- `REQ`：用户或正式需求明确规定；
- `DOMAIN`：已确认的业务不变量；
- `API`：公开接口、协议或 schema 契约；
- `CALLER`：真实调用方依赖；
- `TEST`：已有测试表达的历史行为；
- `CODE`：当前实现表现；
- `CHARACTERIZATION`：明确记录当前观察行为，只承诺本次不意外改变，不声称它是正确业务契约；
- `ASSUME`：为推进设计做出的推断；
- `UNKNOWN`：当前证据无法确定。

不要仅凭枚举顺序机械决定可信度。若来源冲突，列出冲突和影响；`CODE`/`TEST` 在 Bug 语境下可能只是错误行为的证据。P0/P1 场景若只有 `ASSUME` 或 `UNKNOWN` Oracle，必须进入 Human Review Required。

## 主流程

```text
Requirement / Change
        ↓
Evidence & Context Boundary
        ↓
Behavior Model
        ↓
Adaptive Risk Model
        ↓
Minimal High-value Scenarios
        ↓
Verification Strategy
        ↓
Plausible Fault Challenge
        ↓
Coverage & Oracle Audit
        ↓
Human Approval
```

### 1. 建立 Context Boundary

输出本次改变什么、不改变什么、相关调用方/状态/外部交互，以及证据缺口。不要在完整理解前生成场景。

### 2. 建立 Behavior Model

将需求压缩成少量可审查的 Behavioral Obligations：正常义务、不变量、禁止行为、状态转换和 preserved behaviors。每条必须可被一个或多个观察点验证。

### 3. 自适应选择 Test Lens

从 `Contract / State / Partition / Interaction / Time-Concurrency / Regression` 中只展开相关 Lens。详细触发器和建模方法见 [references/design-protocol.md](references/design-protocol.md)。

同时判断是否存在比示例断言更稳定的 property 或 metamorphic relation；适合时优先表达不变量，而不是堆具体 input/output。

### 4. 建立 Risk Map

风险参考 `Impact / Likelihood / Change Proximity / Detectability` 做定性排序，不计算伪精确分数，也不把它们机械相乘。认证授权、租户隔离、资金、不可逆数据、secret 和 destructive operation 等 guardrail 只要变更可触达，就不能被低 likelihood 抵消：

- `P0`：严重业务/安全后果且变更可触达，或实现错误概率高且后果显著；
- `P1`：现实概率不低，会造成明显错误或回归；
- `P2`：防御性保障；
- `P3`：极端或低价值，默认不进入主审查包。

风险必须具体到失效机制，例如“DB commit 成功、publish 失败后重试造成重复业务效果”，不要写“异常情况”。

### 5. 选择最小高价值场景

场景不是分类清单。优先选择同时验证行为义务、暴露高风险故障、覆盖关键边界的代表性 witness。只在交互风险真实存在时做 pairwise/t-way 组合，不展开笛卡尔积。

每个 P0/P1 场景至少写清：`Given/Trigger`、`Expected + forbidden effects`、`Protects`、`Exposes`、`Oracle Source`、`Confidence` 和验证策略引用。

### 6. 设计 Verification Strategy

为每个 P0/P1 fault 建立：

```text
Fault → Witness Scenario → Control / Injection Point
      → Verification Level / Fidelity → Observable Oracle
```

根据被验证语义选择 `unit / component / integration / contract / e2e / property / chaos/eval`，而不是默认 unit test。涉及 concurrency/time/failure ordering 时必须给出确定性 interleaving、fake clock、failpoint、barrier 或等价控制；只能概率触发时标记 `TESTABILITY_GAP`。详细协议见 [references/verification-strategy.md](references/verification-strategy.md)。

### 7. 执行 Adequacy Critic

Critic 不重新生成另一套测试。它先从原始需求和代码边界独立重建行为/风险面，再挑战候选计划：

> 假设一个实现已通过当前所有场景。构造最可能由工程师或 coding agent 写出的错误实现，使其仍通过测试，但违反行为义务。

把 plausible faults 映射到能杀死它的场景及验证策略。对 `P0/P1 → NONE` 的 survivor，或纸面存在但 fidelity/control 不足的 pseudo-killer，必须补场景、补 test seam、降低充分性结论，或说明为何接受。完整审查协议见 [references/critic-playbook.md](references/critic-playbook.md)。

Critic 必须先在看不到 candidate scenarios 的阶段，从原始证据生成并固化 fault inventory，再 reveal 计划比较。高风险任务优先使用隔离 reviewer context；只有在运行时政策允许且用户已授权委派时才使用 subagent，否则执行同上下文的 blind-first artifact，并降低对“无遗漏”的置信度。

### 8. 交付开发者审批主文档

严格使用 [references/output-contract.md](references/output-contract.md) 的信息层级。默认主视图只放：

1. 先看结论：能否进入审批、最危险的事情、当前还差什么；
2. 需要人决定的 0–3 个具体业务问题；
3. 六类 Lens 覆盖速查表，所有缺口在一张表里可见；
4. 用业务故事组织的最小高价值场景及当前验证状态；
5. 尚未证明的风险和最小补强动作；
6. 可勾选的审批项与批准后的下一步。

Source Ledger、Behavior/Risk/Fault 全量表和验证策略明细属于技术证据层，按需输出到独立文件。不要为了证明分析严谨而牺牲主文档可读性。

需要理解呈现方式时，读取 [references/worked-example.md](references/worked-example.md)。不要照抄案例业务规则。

## 完成条件

只有同时满足以下条件，才能给出 `READY_FOR_HUMAN_REVIEW`：

- 每个核心 behavior/invariant 都有验证方式；
- 所有高风险状态转换均已覆盖或显式接受风险；
- 相关 partition/boundary 有代表值；
- 关键 side effect 的 partial failure 已考虑；
- 相关 concurrency/time/retry 风险已考虑；
- 每个 P0/P1 Oracle 都有可信依据；低优先级不确定性已显式列出；
- 每个 P0/P1 fault 都有足够真实、可控、可观察且可重复的 verification strategy；
- Fault Challenge 后不存在未解释的 P0/P1 survivor；
- 重复、低价值场景已从主审查包移除。
- 六个 Lens 都在主文档中明确标为 `已覆盖 / 部分覆盖 / 未覆盖 / 不适用`，且 `不适用` 有具体理由；
- 普通开发者只读主文档即可指出待决策项、未覆盖 Lens 和下一步，不需要理解内部缩写或跨表追踪 ID；
- 主文档中的每个 `部分覆盖/未覆盖` 都说明“缺什么”和“不补会怎样”，没有把关键降级藏在技术附录。

若 P0/P1 预期结果不明确且不同答案会改变实现，内部状态使用 `BLOCKED_BY_ORACLE_AMBIGUITY`；若关键语义无法被现有测试环境真实、确定地制造或观察，使用 `BLOCKED_BY_TESTABILITY_GAP`；其他可修复缺口使用 `REVISION_REQUIRED`。主文档先写中文结论，再把机器状态放在括号或元数据中。这些状态都要给出最小下一步。`READY_FOR_HUMAN_REVIEW` 不是“已经正确”的证明，只表示当前论证足以进入工程师审批。

## 研究依据

这个流程为何强调 specification-first、fault exposure、变异挑战、反馈循环和适度结构，见 [references/research-basis.md](references/research-basis.md)。研究只用于校准设计原则，不替代当前仓库和业务证据。

## Self-validation Mode

当用户要求验证或迭代这个 skill 本身时，读取 [references/self-eval-protocol.md](references/self-eval-protocol.md)。除可执行正确性外，必须加入“普通开发者能否快速找到未覆盖 Lens 与待决策项”的可用性验证。使用隔离的 benchmark builder、designer、adversarial implementer 和 judge 阶段；Designer 不得看到 hidden faults。优先以正确实现通过、seeded plausible mutants 被 executable tests/evals 杀死作为证据，而不是让模型给自己的文字打分。

同模型、同上下文的自测只能作为 smoke/development evidence，必须明确污染风险；不能据此宣称 skill 已被独立验证。最终结论还需要未参与调优的 holdout、真实历史 Bug 或独立 reviewer/model。
