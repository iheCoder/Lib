---
name: tdd-ha-bridge
description: 为确定性软件变更或非确定性 AI agent 行为设计可供工程师审批的测试/eval 契约，并用风险建模、Oracle 溯源、verification fidelity 和 plausible-fault challenge 审计充分性。适用于要求“先审场景和验证策略、再写测试、eval harness 或实现”的开发；不用于只追求覆盖率或机械补齐测试数量。
---

# TDD Human-Approval Bridge

这个 skill 交付的不是“尽可能多的测试用例”，而是一个工程师能快速审查的 **Test/Eval Design + Adequacy Argument**：

- 系统必须守住哪些行为义务；
- 为什么选这些高价值场景；
- 每个场景准备捕获什么现实错误；
- 需要什么真实度、控制点和观察点才能实际暴露错误；
- 哪些重要行为仍不确定或未覆盖；
- 是否仍有高风险错误实现可以骗过当前设计。

默认只完成测试/eval 设计与充分性审查。除非用户明确要求继续，否则不要编写测试代码、eval harness 或生产实现；把 Human Approval 保留为真实边界。

## 六个不可妥协的原则

1. **Behavior first**：先建行为、不变量、状态转换和禁止行为，再列场景。
2. **Adaptive risk decomposition**：按当前变更的风险结构选择 Lens，不为凑分类而制造测试。
3. **Oracle provenance**：关键 Expected 必须有来源；无法确认就写 `UNKNOWN`，不能替业务做决定。
4. **Fault challenge**：寻找能通过现有场景、但违反契约的 plausible wrong implementation。
5. **Executable witness**：P0/P1 不只要有场景，还必须有足够真实、可控、可观察的验证策略。不要 mock 掉正在验证的语义。
6. **Human attention compression**：把人的注意力集中到 P0/P1、低可信 Oracle、Testability Gap、未覆盖义务和高风险 survivor。

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

### 8. 交付 Human Review Pack

严格使用 [references/output-contract.md](references/output-contract.md) 的紧凑结构。主视图只放：

1. 一分钟摘要；
2. Behavior Map；
3. P0/P1 Risk Map；
4. 最小高价值场景；
5. P0/P1 Verification Strategies；
6. Fault Challenge survivors 与 pseudo-killers；
7. Coverage Gaps、Testability Gaps 与 Oracle Uncertainties；
8. 需要人决定的少量问题。

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

若 P0/P1 Oracle 不明确且不同答案会改变实现，使用 `BLOCKED_BY_ORACLE_AMBIGUITY`；若关键语义无法被现有 harness 真实、确定地控制或观察，使用 `BLOCKED_BY_TESTABILITY_GAP`；其他可修复缺口使用 `REVISION_REQUIRED`。这些状态都要给出最小下一步。`READY_FOR_HUMAN_REVIEW` 不是“已经正确”的证明，只表示当前论证足以进入工程师审批。

## 研究依据

这个流程为何强调 specification-first、fault exposure、变异挑战、反馈循环和适度结构，见 [references/research-basis.md](references/research-basis.md)。研究只用于校准设计原则，不替代当前仓库和业务证据。
