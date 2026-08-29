---
name: side-eye
description: 审查 diff、commit、分支或 PR 中由本次变更引入、激活或放大的高代价故障与发布风险。适用于代码审查、生产就绪性审查和高风险变更审计；通过场景前提、风险假设、故障链证据与主动证伪发现真实问题，而不是泛泛检查风格或罗列理论风险。
---

# Side Eye

这个 skill 的目标是找到**本次变更越过故障线的具体方式**：在什么现实前提下，由什么触发，沿哪条执行路径，最终破坏什么状态或行为。

优先阻止高代价错误进入代码库：不可逆的资金、数据、权限或状态破坏；核心需求失败；已有主流程回归；严重并发、可靠性、资源和性能问题；以及代码正确但现实数据、拓扑或发布顺序不支持的上线风险。最后才检查明显的维护性恶化和仓库规范。

不要把 review 退化成 checklist，也不要因为看到 Redis、Kafka、API 或 migration 就直接输出脑裂、乱序、兼容性或数据风险。技术信号只能激活待验证假设。

## 核心纪律

1. **风险必须先过适用性门槛。** 先确认问题成立所需的生命周期、拓扑、规模、数据和外部暴露前提。
2. **Finding 必须有故障链。** 至少能说明 `前提 → 触发 → 执行路径 → 破坏状态/行为 → 影响`。
3. **主动找反证。** 输出前检查幂等、约束、事务、调用方保护、部署事实、容量上界等是否已消除风险。
4. **只审本次 change 的因果责任。** 未修改代码可以作为上下文；只有 `Introduced / Exposed / Amplified` 的问题进入正式 Finding。纯历史问题默认不报。
5. **先广后深。** 完成 breadth scan 后才深入；每次 deep dive 后回到 coverage ledger，避免在一个聪明方向里耗尽 review。
6. **区分代码错误与现实前提。** 代码缺陷、发布/运行风险和待确认假设分别输出，不把未知包装成确定 Bug。
7. **没有发现不等于证明安全。** 结论限定在已确认场景、实际审查范围和完成的验证内。

## 工作流

### 1. 确定审查范围

优先根据用户请求和 Git 证据自动确定：working tree、staged diff、指定 commit/range、feature branch 对 base、PR 或指定文件。不要默认审整个仓库。

记录 base、head、包含内容和明确排除项。范围确实无法判定，且不同答案会改变审查对象时，才询问用户。

### 2. 建立 Change Contract

按可信度依次读取用户需求、issue/PR/commit 描述、设计文档、测试、diff 和调用方，压缩成：

- `Must`：核心行为；
- `Should`：次要行为；
- `Must Preserve`：不能意外破坏的既有行为；
- `Must Never`：资金、数据、安全或业务上绝不能发生的结果；
- `Unknown`：会改变结论但当前没有证据的需求。

不要从实现反推并虚构需求。缺少需求时，只能评价代码内部一致性和明确契约，不能高置信度声称“需求已正确实现”。

### 3. 提取 Change Facts 与场景上下文

先写客观事实，不混入风险判断：修改了哪些 symbol、状态读写、外部副作用、调用关系、派生关系、事务边界、错误处理和部署资产。

再确认只与当前风险有关的场景前提：

- `Lifecycle`：base 是否部署过，旧 contract 是否真实对外，是否 mixed-version rollout；
- `Topology`：单/多实例、worker/consumer、region 和共享 ownership；
- `Scale`：QPS、数据量、fan-out 上界、资源容量；
- `State`：历史数据、默认值、migration/backfill 和回滚状态；
- `Exposure`：API/SDK、消息、持久数据、其他服务或仓库消费者。

证据优先顺序通常是仓库代码与配置、设计/部署资料、用户已提供信息。只有答案会改变 Finding 是否成立、严重度、范围或调查方向时才提一个窄问题；其他未知保留为条件化分析，不阻塞整场 review。

### 4. 先做 Breadth Scan

深入前快速覆盖以下八个领域，并在 [review-state.md](references/review-state.md) 的 ledger 中标为 `High / Medium / Low / Unknown / Not Applicable`：

1. 不可逆状态、资金、安全、权限；
2. 当前核心需求；
3. 既有行为、跨模块回归、真实兼容性；
4. 并发、可靠性、资源、严重性能；
5. 发布、迁移、回滚、运行数据和环境前提；
6. 次要行为与有限边界；
7. 架构与长期可维护性；
8. 仓库规范、可读性与注释。

Breadth scan 只决定调查方向，不在此阶段无限展开。兼容性假设必须先确认“旧世界”真实存在；分布式假设必须先确认存在并发执行实体、共享状态或 ownership。

### 5. 生成并排序风险假设

读取 [risk-triggers.md](references/risk-triggers.md)，只启用与 Change Facts 匹配且适用性成立的触发器。优先调查：

- 影响高且变更可触达；
- 难以及时观测、恢复或回滚；
- 本次修改接近故障边界；
- 能用代码、测试或工具取得明确证据。

每个 active hypothesis 记录：`Why Activated / Applicable When / Need Evidence`。触发器命中绝不是 Finding。

### 6. 风险定向检索与工具验证

只扩展验证当前 hypothesis 所需的上下文，通常按以下顺序：

```text
changed code
→ direct callers/callees
→ related state/model and readers/writers
→ external side effects/contracts
→ related tests
→ config/migration/deployment assets
→ history/blame or broader modules（仅在仍有高信息价值时）
```

根据风险使用确定性工具提供证据：相关 tests、compiler、static analyzer、race detector、benchmark、migration/contract check 等。不要把命令成功本身当作行为正确；也不要运行与假设无关的全仓库重型检查来制造完成感。

当假设被证伪、影响被可靠限制、已取得足够证据，或继续检索的边际价值很低时停止 deep dive，并回到 coverage ledger。

### 7. 跑通故障链并主动证伪

候选 Finding 必须明确：

```text
Scenario Preconditions
→ Trigger
→ Execution Path
→ Broken State / Behavior
→ User/System Impact
```

随后主动搜索能推翻它的证据。例：怀疑超时重试造成重复副作用，就继续检查 idempotency key、唯一约束、状态机、provider dedup 和 retry boundary。可靠保护存在则记录为 `Rejected Hypothesis`，不输出。

再通过 Change Causality Gate：

- `Introduced`：此次修改直接产生；
- `Exposed`：旧风险原本不可达，此次修改首次使其可达；
- `Amplified`：旧风险存在，但此次修改显著增加概率或影响；
- `Unchanged`：与此次修改无关，默认不进入正式结果。只有极端严重时可作为不阻塞的 out-of-scope observation。

### 8. Blind Spot Pass 与完成条件

完成 high-risk deep dives 后，暂时放下已有 hypothesis，独立检查：

- 是否遗漏不可逆副作用或静默写错状态；
- 当前核心需求是否存在根本无法完成的路径；
- 既有主流程是否被 deletion、default、error handling 或 contract change 破坏；
- 线上数据、发布顺序、回滚或 mixed-version 前提是否不存在；
- diff 中是否有未进入任何 deep dive 的危险改动。

只有 scope 已明确、change intent 已尽可能理解、breadth scan 完成、所有 High 领域已调查或明确不可验证、正式 Finding 已找过反证并通过因果门槛、blind spot pass 已完成，才结束 review。

## Finding 分类与严重度

类型：

- `Code Defect`：代码行为本身错误；
- `Release / Operational Risk`：代码可能正确，但数据、拓扑、发布或运行前提不满足；
- `Open Assumption`：仓库证据无法确认，用户答案或生产事实会改变结论。

严重度只表达影响，不把稀有场景自动降级：

- `P0`：不可逆资金、关键数据、安全、权限或巨大业务损失；
- `P1`：核心流程失败、严重回归、严重可靠性/性能问题或无法安全发布；
- `P2`：次要需求失败、有限边界缺陷或明显维护性恶化；
- `P3`：规范、命名、普通可读性和轻微设计问题。

`Confidence` 单独使用 `High / Medium / Low`。场景概率不明确时描述现实前提，不使用伪精确概率。

维护性 Finding 只报告明显恶化：大量补丁式分支、同一业务规则跨模块散落、多套事实源、新的隐式耦合、重复状态机、难以验证的控制流或多个独立职责纠缠。行数和 if 数只能触发检查，不能单独定罪。仓库对流程注释、测试说明和命名的明确要求在最后检查，不得挤占 P0/P1 调查预算。

## 输出契约

先给 Findings，按严重度排序；不要用大段过程描述淹没结论。每条正式 Finding 使用紧凑结构：

```text
[P1] 标题 — path/to/file.go:123
Type: Code Defect
Change Causality: Introduced
Applicable when: 问题成立的现实前提；若始终不成立，明确说明不适用。
Failure trace: 触发 → 关键执行路径 → 被破坏状态/行为。
Impact: 业务或系统后果。
Evidence: 当前代码、调用关系、数据模型、测试或配置中的直接证据。
Confidence: High
Suggested verification: 能实际证实或复现该故障的最小动作。
```

仍依赖外部事实时放入 `Open Assumptions`，写清不同答案如何改变结论。代码正确但不能证明可安全上线的问题放入 `Release / Operational Risks`。

最后简述：

- `Verdict`：`BLOCK / NEEDS CONFIRMATION / PASS WITH RISKS / PASS`；
- `Coverage`：已检查、不适用、未充分验证；
- `Verification`：实际运行了什么，以及证据边界。

若没有 Finding，明确写“在当前已确认场景和已检查范围内，没有发现 blocking finding”，而不是“代码完全没问题”。除非用户要求审计明细，不要输出完整 Review State、所有 rejected hypotheses 或冗长 checklist。
