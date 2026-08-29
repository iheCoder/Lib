---
name: side-eye
description: 围绕一个明确需求及其 seed files、symbols 或模块，沿最小实现链审查由该需求引入、激活或放大的隐蔽故障与发布风险。适用于 requirement-scoped code review；不用于通用 diff/PR review、全仓审计或普通代码规范检查。
---

# Side Eye

Side Eye 是一个 **Requirement-Scoped Failure-Oriented Code Review** skill。它只做一件事：

> 从用户给出的需求和 seed code 出发，沿需求从输入到可观察结果的最小行为链，寻找当前实现引入、激活或放大的高代价故障、发布风险和隐含前提。

不要把它扩展成通用 diff reviewer、working tree reviewer、repository audit、静态规范检查器或 Bug 百科。Git diff、history 和 blame 只能帮助理解实现及因果，不能定义审查疆界。

## 开始条件与边界

开始一次 review 需要：

- 一个明确的 requirement；
- 用户给出的 seed files、symbols 或足够窄的模块范围。

Seed code 是起点，不是硬边界。为了验证 requirement，可以继续追 direct callers/callees、state readers/writers、持久层、异步组件、配置、migration、相关测试和外部副作用；每次扩展都必须说明它为何是验证当前 requirement 所必需的。不要因为文件出现在 diff 或看起来相关就顺带审查。

如果 requirement 或起点缺失，提出一个窄问题补齐它；不要退回到自动扫描整个 diff。用户已给出足够窄的模块且能可靠定位入口时，可以自行找到具体 symbols。

## 核心纪律

1. **Requirement 是注意力中心。** 所有检索、风险和 Finding 都必须能回到当前需求。
2. **建立 Implementation Slice。** 审查最小行为链，而不是整个 call graph。
3. **风险先过适用性门槛。** 先确认生命周期、拓扑、规模、数据和外部暴露前提。
4. **Finding 必须有故障链。** 至少说明 `前提 → 触发 → 执行路径 → 破坏状态/行为 → 影响`。
5. **主动找反证。** 检查幂等、约束、事务、调用方保护、部署事实和容量上界是否已经消除风险。
6. **只报 requirement 的因果责任。** 正式 Finding 只允许 `Introduced / Exposed / Amplified`；纯历史问题默认不报。
7. **先广后深，并允许回流。** Deep dive 发现新拓扑、状态所有权或运行事实时，更新 facts，重新运行相关 gate，而不是坚持最初假设。
8. **内部严格，外部易读。** 内部保留完整推理结构；默认报告用具体场景和普通开发者能快速理解的语言。
9. **Instruction budget 只补模型盲点。** 普通强模型本来就大概率能发现的 N+1、命名、显式错误处理等，不写成 Core Trigger。

## Review 工作流

### 1. 建立 Requirement Contract

按可信度读取用户描述、设计文档、验收标准、相关测试和代码，压缩成：

- `Must`：必须实现的核心行为；
- `Should`：次要目标；
- `Must Preserve`：不能意外破坏的既有行为；
- `Must Never`：资金、数据、安全或业务上绝不能发生的结果；
- `Unknown`：会改变结论但当前没有证据的需求。

不要从实现反推并虚构 requirement。缺少需求事实时，只能给条件化结论。

### 2. 建立 Implementation Slice

Implementation Slice 是从需求输入到最终可观察结果之间，为验证正确性所需的最小行为链。例如：

```text
用户自然语言输入
→ 多轮状态与参数解析
→ tool selection
→ 修改内置任务
→ authorization / approval
→ database update
→ scheduler reconcile
→ next execution state
→ Agent final response
```

记录 slice 的入口、关键状态转换、持久状态、外部副作用、异步边界和最终观察点。与该链无实际关系的模块默认不展开。

### 3. 提取 Implementation Facts 与现实上下文

先写客观事实，不提前产生 Finding：哪些入口、状态、数据转换、工具、副作用、持久状态、异步组件和运行环境前提参与需求。

只确认会改变结论的现实上下文：

- `Lifecycle`：旧版本、旧 contract、rolling rollout、恢复阶段；
- `Topology`：实例、worker、region、writer 和 ownership；
- `Scale`：QPS、数据量、fan-out 与资源上界；
- `State/Data`：历史数据、migration/backfill、缓存和运行时表示；
- `Exposure`：API、SDK、消息、外部服务和不可信数据边界。

只有答案会改变 Finding、严重度、Current Relevance 或调查路线时才问用户一个窄问题；其余未知写成条件化分析。

### 4. Requirement Breadth Scan

围绕当前 requirement 快速覆盖八个领域，并在 [review-state.md](references/review-state.md) 的 ledger 中记录：

1. 不可逆状态、资金、安全、权限；
2. requirement 核心行为；
3. 既有行为和真实 regression；
4. 并发、可靠性、资源和严重性能；
5. 发布、migration、回滚、运行环境和数据前提；
6. 次要行为与边界；
7. 架构与长期维护性；
8. repository conventions。

每个领域先问“当前 requirement 是否可能触达？”；不要给整个 repository 做八类体检。

### 5. 激活 Blind-Spot Triggers

在 facts 建立后读取 [Core Model Blind-Spot Triggers](references/risk-triggers.md)。Trigger 只能创建待验证 hypothesis，不能直接创建 Finding。

按需加载 domain pack：

- facts 涉及 LLM、agent、tool call、session、memory、approval、delegation 或 planner 时，读取 [Agent Blind-Spot Pack](references/risk-triggers-agent.md)；
- facts 涉及 cron、RRULE、scheduler、自然语言调度、next-run 或 timezone 时，读取 [Scheduler Blind-Spot Pack](references/risk-triggers-scheduler.md)。

多个 trigger 指向同一 broken invariant 时先合并 hypothesis，把 trigger 作为 supporting signals；只有 failure mechanism 不同才拆分。

### 6. 风险定向检索、工具验证与 Discovery Re-entry

只检索验证 active hypothesis 所需的上下文：

```text
seed code
→ Implementation Slice 上的 callers/callees
→ related state and all relevant readers/writers
→ external effects/contracts
→ related tests
→ config/migration/deployment assets
→ history 或更广模块（仅在仍有高信息价值时）
```

根据风险使用 tests、compiler、static analyzer、race detector、benchmark、migration/contract check 等提供证据。命令成功不等于需求正确；不要运行与 hypothesis 无关的全仓重型检查来制造完成感。

若新证据改变 topology、state ownership、side-effect graph、cardinality、lifecycle、external exposure 或 runtime state：更新 Implementation Facts，重新执行相关 applicability gate 和 coverage，再决定是否激活新 trigger。

### 7. 跑通、证伪并通过 Causality Gate

候选 Finding 必须形成：

```text
Scenario Preconditions
→ Trigger
→ Execution Path
→ Broken State / Behavior
→ User/System Impact
```

随后主动寻找 counter-evidence。保护真实存在则把 hypothesis 记为 Rejected，不输出。

- `Introduced`：当前 requirement 的实现直接制造问题；
- `Exposed`：旧风险原本不进入该行为路径，新 requirement 首次使其可达；
- `Amplified`：旧风险存在，但新 requirement 显著增加发生概率、影响或副作用；
- `Unchanged`：纯历史问题，默认不报。

### 8. Blind-Spot Pass、完成或 Checkpoint

完成 high-risk deep dives 后，暂时放下已有 hypothesis，独立检查是否遗漏：不可逆副作用、核心链无法完成、既有主流程回归、现实数据/发布前提、Implementation Slice 的关键断点，以及未被调查的 High/Unknown 领域。

只有 requirement 和 seeds 明确、slice 已建立、breadth scan 完成、所有 High 领域已调查或明确不可验证、Finding 已找反证并通过因果门槛，才给最终 verdict。

复杂 review 可以按 [review-state.md](references/review-state.md) 保存可持久化 checkpoint：

- `Continue Pass` 载入 facts、findings、rejected hypotheses、coverage 和 next queue，继续未完成领域；
- `Fresh-Eyes Pass` 暂不载入旧 hypotheses/findings，独立寻找 blind spot，最后再 reconcile。

恢复前必须检查 repository revision、相关文件、Finding 证据和旧反证是否已过期。尚有明确待审领域时使用 `CHECKPOINTED`，不要伪装成 `PASS WITH RISKS`。

## 内部分类

Finding 类型：

- `Code Defect`：实现行为本身错误；
- `Release / Operational Risk`：代码可能正确，但数据、拓扑、发布或运行前提不满足；
- `Open Assumption`：少量外部事实会改变结论。

Severity 只表达“发生后的影响”：

- `P0`：不可逆资金、关键数据、安全、权限或巨大业务损失；
- `P1`：核心流程失败、严重回归、可靠性/性能事故或无法安全发布；
- `P2`：次要需求失败、有限边界缺陷或明显维护性恶化；
- `P3`：repository conventions、普通可读性和轻微设计问题。

另用 `Current Relevance` 表达当前优先性：`当前适用 / 条件适用 / 待确认`。不要因为 P0 只按严重度排序，就让条件极窄的问题淹没当前必现的 P1。Confidence 仍可在内部记录，但默认不作为报告字段。

维护性 Finding 只报告需求实现造成的明显恶化，如同一业务规则散落、多套事实源、新隐式耦合、重复状态机或难以验证的多职责控制流。行数和 if 数只能触发检查，不能单独定罪。规范、命名和注释最后检查，不得挤占高代价故障的调查预算。

## 面向开发者的输出

先给结论，再按**现实发生场景**动态分组；没有 Finding 的分类不要显示。可用分类包括：

- 正常业务路径；
- Agent 多轮 / retry / recovery；
- 多实例 / 分布式前提；
- 高并发 / 大数据量前提；
- 历史数据 / 发布环境；
- 特殊配置组合；
- 维护性 / 可读性。

每条默认使用：

```text
### [P1 · 当前适用] 标题 — path/to/file.go:123

问题：一句话说明被破坏的行为。

具体场景：用真实输入、状态和执行顺序讲清问题怎么发生。

证据：列出当前代码、调用关系、数据模型、测试或配置中的直接依据。

怎么验证：给出能够证实或复现的最小动作。
```

涉及 lease/fencing、partial failure、mixed version、agent recovery、migration race 或 ordering 时，必须给一个具体发生案例，不能只堆抽象术语。只有在帮助理解时才展示 Type、Causality、Confidence 等内部字段。

随后简述：

- `Open Assumptions`：仍依赖的外部事实，以及不同答案如何改变结论；
- `Verdict`：`BLOCK / NEEDS CONFIRMATION / PASS WITH RISKS / PASS / CHECKPOINTED`；
- `Coverage`：已检查、不适用和未充分验证的范围；
- `Verification`：实际执行的验证及证据边界。

不要输出完整 Review State、Coverage Ledger、大量 rejected hypotheses、理论风险清单或十几个没有当前相关性区分的 P0/P1。没有 Finding 时写“在当前已确认场景和已检查范围内，没有发现 blocking finding”，不能写成“代码完全没问题”。

## 长期准入规则

向 Side Eye 增加任何规则前，先问：

> 如果完全不写这条 instruction，当前 frontier model 是否本来就大概率会做到？

答案为“是”时默认不加。Side Eye 的 instruction budget 只用于模型天然容易忽略、但真实工程代价很高且会改变检索方向的问题。
