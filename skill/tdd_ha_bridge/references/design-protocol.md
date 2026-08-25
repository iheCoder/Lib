# Test Design Protocol

本协议负责从需求与变更中产生候选测试设计。它不是固定 taxonomy；先识别风险结构，再选择需要展开的 Lens。

## 1. Context Boundary 与 Source Ledger

先回答四个问题：

1. 本次新增、修改或必须保持的行为是什么？
2. 哪些组件、调用方、状态和外部交互在变更边界内？
3. 哪些证据能说明 intended behavior，哪些只反映 current behavior？
4. 若当前实现可能有 Bug，哪些材料会误导 Oracle？

建议记录：

| Evidence | Source | Role | Authority in this task | Conflict / gap |
| --- | --- | --- | --- | --- |
| 幂等键重复不能重复扣款 | API 文档 | intended contract | High | 无 |
| publish 失败后返回 500 | 当前代码 | observed behavior | Low | 需求未说明是否应重试 |

### 来源冲突处理

- 显式新需求优先描述“要变成什么”，但不能静默覆盖未讨论的兼容约束。
- API/schema 与真实调用方共同定义外部可观察契约。
- 既有测试是回归证据，不自动等于业务真相。
- 当前代码可说明执行路径、failure injection point 和历史行为；在新功能/已知 Bug 路径中不能单独定义 Expected。
- 遗留系统若 intended behavior 不可恢复，但当前行为稳定且被调用方依赖，可建立 `CHARACTERIZATION / CODE+TEST+CALLER` 场景：它只防止本次变更意外漂移，不把观察行为升级为永久 contract。
- 无法裁决时保留 `UNKNOWN`，同时说明它阻塞哪些场景。

只有当新实现必须主动选择 A/B，且选择会造成实质业务分叉时，Oracle ambiguity 才阻塞。能够原样保持的未知遗留行为应使用 characterization mode 推进。

### Bug 修复的三层 witness

已知事故不能只冻结 exact input。先提炼故障机制，再覆盖邻近语义：

```text
Incident reproducer → Fault mechanism → Generalized neighbors
```

至少包含：

1. 能在修复前失败的 exact reproducer；
2. 对根因的抽象，例如“缺失值与显式零值被混淆”；
3. 至少一个能区分“真正修复机制”和“只 hardcode 事故输入”的邻近 witness。

邻近 witness 必须来自同一语义分区，不是随意多加一个数值。

## 2. Behavior Model

不要从 testcase 开始。先产出 3–7 条核心义务为宜；复杂变更可更多，但需要合并重复语义。

每条 obligation 包含：

```text
B{id} 义务或不变量
Precondition: 何时适用
Observable: 返回值 / 状态 / 副作用 / 非副作用 / 事件 / 可观测信号
Source: REQ | DOMAIN | API | CALLER | TEST | CODE | ASSUME | UNKNOWN
```

重点寻找：

- 成功必须发生什么；
- 失败或非法状态绝不能发生什么；
- 一次业务请求只能产生几次业务效果；
- 多资源更新之间必须保持什么一致性；
- 哪些历史行为或兼容面必须保持；
- 是否存在跨多组输入都成立的 property。

对异步或最终一致系统，Behavior Map 必须同时问：

- **Safety**：绝不能发生什么，例如重复扣款、越权调用、状态回退；
- **Liveness**：最终必须发生什么，以及可接受的时间/重试边界，例如 committed event 最终送达。

不能只证明 Safety。一个永远不处理任务的实现可能“从不重复”，但同时违反 Liveness。

### Property 与 Metamorphic Relation

当精确输出难枚举、输入空间大或输出具有结构性时，寻找：

- invariant：长度、守恒、单调性、唯一性、排序性、权限不提升；
- idempotence：`f(f(x)) = f(x)` 或重复命令不增加业务效果；
- round trip：encode/decode、serialize/deserialize；
- metamorphic relation：改变无关输入后结果应保持，或输入变换与输出变换满足稳定关系。

Property 仍需写明适用域与例外，不能用过强性质替业务做决定。

## 3. 六个自适应 Lens

先按系统形态确定重心，不要让所有 Lens 获得相同篇幅：

| Change shape | Primary lenses | Usually omit unless evidence triggers it |
| --- | --- | --- |
| 纯函数/转换器 | Contract, Partition, Property | Interaction, Concurrency |
| CRUD service | Contract, State, Interaction, Regression | Reorder, Expiration |
| 分布式锁/租约 | State, Time-Concurrency, Interaction | 大量普通输入枚举 |
| 消息 consumer | State, Interaction, Time-Concurrency, Regression | 无关数值极值 |
| 数据迁移 | State, Partition, Regression, Time-Concurrency | 与 schema 无关的 API 组合 |

表格只是路由示例。真实代码边界和 failure surface 可以覆盖这些默认值。

### Lens A — Contract

始终应用。提炼：输入、前置状态、结果、副作用、禁止行为和错误语义。

典型产物：Behavioral Obligations、Forbidden Effects、Oracle 来源。

#### Decision Surface Trigger

当结果由多个布尔条件、权限、feature flag、枚举、状态与身份的复合谓词共同决定时：

1. 将自然语言规则规范化为 cause-effect expression；
2. 构造 decision table，合并真正等价的行并标出不可能组合；
3. 对权限、计费、风控等高风险决策，用 MC/DC 思维确认每个原子条件都能在其他相关条件固定时独立改变结果；
4. 再用 pairwise/t-way 补充非核心交互，而不是让 pairwise 代替决策逻辑覆盖。

重点挑战运算符优先级、缺少括号、feature flag 只包住一半分支、deny 条件错误提升为 allow 等 plausible faults。

### Lens B — State

触发条件：行为依赖持久状态、生命周期、缓存、锁、事务或先前调用。

建模：

```text
Previous State + Trigger → New State + Side Effects
```

至少识别：

- 相关前置状态；
- 合法转换；
- 禁止转换；
- 重复转换；
- 失败后的残留或补偿状态。

只覆盖本次变更可能触达的状态，不穷举完整产品状态机。

### Lens C — Partition

触发条件：行为随数值区间、枚举、权限、数据形态或配置切换。

先找“业务行为相同”的分区，再选代表值和决策边界：

```text
非法区 / 正常区 / 临界区 / 拒绝区
```

每个被选值必须能说明它代表哪个分区或边界。避免 `0, 1, -1, maxint` 的无理由机械列表。

Partition 只覆盖单因素行为分区。若多个条件共同决定结果，转到 Decision Surface；不能因为每个因素都单独测过就宣称组合决策充分。

### Lens D — Interaction / Partial Failure

触发条件：存在数据库、消息、RPC、文件、缓存、锁、事务、第三方调用或多步副作用。

从真实执行链提取 Failure Surface：

| Step / boundary | Failure injection | Expected state | Forbidden residue | Oracle |
| --- | --- | --- | --- | --- |
| DB commit | timeout / ambiguous result | ? | 重复业务效果 | UNKNOWN |

每个外部交互都是候选注入点，但只有会改变契约或一致性的点才进入主场景。重点关注“第 N 步失败”、成功但响应丢失、补偿失败和 ambiguous result。

### Lens E — Time / Concurrency

触发条件：共享状态、幂等键、并发更新、事件处理、锁/租约、异步工作、timeout、retry 或 cancellation。

按风险选择：

- Repeat：顺序重复；
- Concurrent：同时进入；
- Reorder：事件乱序；
- Retry：成功但响应丢失后重试；
- Delay：慢依赖和资源占用；
- Cancellation：取消发生在可撤销/不可撤销边界前后；
- Expiration：TTL、lease、token 或 lock 临界过期。

不要用顺序重复测试代替并发重复测试；它们暴露的错误不同。

### Lens F — Regression

触发条件：修改既有路径、默认值、接口、schema、数据、配置或调用语义。

从既有测试、调用方、API、schema 和历史行为提炼 `Preserved Behaviors`。特别检查：

- 新开关关闭或缺省时是否保持旧行为；
- 新旧客户端/服务版本并存；
- schema 前后兼容；
- 非目标路径与错误类型是否改变；
- 测试是否过度绑定内部实现，妨碍合法重构。

## 4. Cross-cutting Triggers

这些不是每次强制展开的新 Lens，只在 change shape 触发时加入 Behavior/Risk Map。

### Resource / Operational Correctness

当变更可能改变复杂度、内存、goroutine/task 数、DB lock/scan、IO、queue depth、connection usage、latency、throughput 或 backpressure 时激活。常见触发器包括 migration、batch、cache rebuild、fan-out、stream 和 rolling deploy。

至少说明：

- 资源预算或 SLO 来源；
- 代表性规模与生产量级差距；
- 锁、连接、队列、复制延迟和恢复行为的观察点；
- crash/restart 后是否可重入、可续跑；
- 新旧版本并存与 rollback 的操作边界。

逻辑结果正确但会耗尽资源、阻塞线上或无法恢复，仍属于 correctness failure。

### Guardrail Override

若变更能触达以下 invariant，不得仅因估计 likelihood 低而降为 P2/P3：

- authentication / authorization；
- tenant isolation；
- 金钱重复或丢失；
- 不可逆数据损坏；
- secret leakage；
- destructive operation。

这些风险需要明确的 deny/forbidden-effect 场景和足够 fidelity 的验证策略。Impact 不能被低概率抵消。

## 5. Risk Model

从变更位置和业务后果生成具体 fault hypotheses，而不是场景类别。

常见但必须结合上下文的错误形态：

- 条件或边界写错；
- 校验与副作用顺序颠倒；
- 只校验返回值，遗漏持久状态或 forbidden effect；
- 部分成功后错误 rollback/retry；
- 幂等记录时机错误；
- 顺序重复安全但并发重复失效；
- timeout/cancel 后后台继续提交；
- 缺省值或旧调用方行为改变；
- stale read、lost update、out-of-order；
- 错误被吞掉、换型或错误地重试。

定性排序：

- Impact：错了造成的数据、资金、权限、可用性或兼容后果；
- Likelihood：工程师或 agent 是否容易这样实现错；
- Change Proximity：本次变更是否直接碰到该决策点；
- Low Detectability：没有专门测试时是否很难被其他机制发现。

这个表达用于比较普通风险，不是纯乘法公式。Guardrail Override 和法规/权限/资金等硬约束优先。

## 6. Minimal High-value Scenarios

一个主场景应尽量承担清晰且不冲突的验证职责。合并条件：相同 setup 下可自然同时证明多个义务；拆分条件：失败原因、Oracle 或业务意图会被混淆。

只有当另一个场景以相同或更高 fidelity 覆盖相同 obligations + faults，并保持相近 Diagnostic Value 时，才能以 minimality 为由删除。避免一个超级 testcase 同时验证 response、DB、Kafka、cache、metric、retry 和 idempotency，失败后却无法定位是哪条契约破坏。

选择顺序：

1. 每个核心 obligation 的最小正向 witness；
2. P0/P1 fault 的最小暴露输入或时序；
3. 关键决策边界的两侧与等值点；
4. 状态/partial failure/concurrency 中无法被前述覆盖的风险；
5. 代表性 regression；
6. P2/P3 仅在成本低或用户要求时进入附录。

若多个参数存在真实交互风险，先列 interaction hypothesis，再用 pairwise 或更高 t-way 压缩组合。不要因为“可能有组合”就覆盖所有参数对。

## 7. 场景 Oracle

场景必须同时观察适用的多面结果：

- return/error；
- persistent state；
- emitted event / external call；
- forbidden side effects；
- count / ordering / idempotent business effect；
- 必要的 log/metric/trace，仅当它们属于契约或诊断要求。

弱断言示例：`err != nil`。

更强但不过度绑定的断言：错误类型符合 API；订单数不变；库存不变；publish 未被调用。

如果 Expected 只能从当前可疑代码得出，标成 `CODE / Low` 或 `UNKNOWN`，不得让测试把它升级成 contract。
