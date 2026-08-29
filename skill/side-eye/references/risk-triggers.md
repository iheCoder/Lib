# Core Model Blind-Spot Trigger Library

这个库只补 frontier model 在普通 review 中仍容易忽略的盲点：局部代码往往正确，但跨文件、跨状态、跨时间或跨运行环境组合后会越过故障线。

Trigger 只能改变检索方向并创建 hypothesis，不能直接生成 Finding。普通 read-modify-write、N+1、API compatibility、IDOR、delete/default/error handling、Kafka duplicate/ordering、goroutine race/deadlock 仍由 Breadth Scan 负责，不占 Core instruction budget。

## Trigger 准入标准

Core Trigger 至少满足以下 3 项，否则删除或降级：

1. 需要跨文件、跨模块或跨系统推理；
2. 需要跨时间、失败或恢复过程推理；
3. 局部代码通常看起来正确；
4. 存在隐藏运行环境前提；
5. 普通 frontier-model review 容易低显著性漏掉；
6. 会明显改变后续检索方向；
7. 一旦成立影响较高；
8. lint、compiler 或 static analyzer 不容易直接发现。

使用时先过 applicability gate，再记录 `Why Activated / Applicable When / Need Evidence / Retrieve or Falsify`。多个 Trigger 指向同一 broken invariant 时合并 hypothesis。

## 1. Ambiguous Completion / Hidden Retry

**Blind spot**：副作用结果是 unknown，系统却把 unknown 当成 failure 再次执行。

**Gate**：调用者无法区分“没有执行”和“已经执行但响应丢失”，且某处可能重放。不要只找显式 `retry()`；继续追 SDK、proxy、queue redelivery、workflow engine、Agent recovery、RPC middleware 和 client library。

**Retrieve / falsify**：所有 retry boundary；idempotency key 的稳定性；唯一约束和 provider dedup；unknown outcome 的 reconciliation；状态机是否把 ambiguous 与 failed 分开。

## 2. Recovery Asymmetry

**Blind spot**：crash、retry 或 resume 后，不同状态系统恢复到不同时间点。

**Concrete path**：DB commit 成功 → Agent checkpoint 尚未写入 → crash → resume 认为 tool 未完成 → 副作用再次执行。反向也可能是 orchestration 标记 completed，但环境事务回滚。

**Retrieve / falsify**：checkpoint、transaction、environment state、commit/ack 顺序、resume 规则、reconciliation，以及每两个持久步骤之间 crash 后的状态。

## 3. Hidden Multi-Writer / Invariant Split

**Blind spot**：HTTP API、consumer、cron、admin、Agent tool 或 backfill 共同修改同一业务状态，但每个入口都认为自己拥有状态转换权。

**Gate**：至少两个真实 writer 能触达同一实体或 invariant；不要只因存在多个函数而激活。

**Retrieve / falsify**：枚举所有 writers；确认 source of authority；比较 validation、versioning、状态转换和副作用规则；检查入口之间是否覆盖、绕过或形成不同 invariant。

## 4. Cross-Layer Multiplicative Amplification

**Blind spot**：每层局部成本都合理，但 `fan-out × nested operation × retry × downstream consumers` 形成乘法放大；这不是普通 N+1。

**Gate**：至少两层可增长基数或重放因子真实相乘。

**Retrieve / falsify**：建立端到端 work equation，找现实 cardinality、重试次数、事件数量、consumer fan-out、并发上界和容量保护。固定且很小的上界应拒绝该 hypothesis。

## 5. Runtime Reality Mismatch

**Blind spot**：实现依赖一个 repo 内无法证明的线上前提，例如历史数据已有新字段、base 已部署、flag 必开、scheduler 单实例、consumer 全升级或配置必存在。

**Gate**：该前提一旦为假会改变 requirement 的正确性或发布安全。

**Retrieve / falsify**：部署资料、配置默认值、migration/backfill、数据分布、拓扑和用户确认。`repo 未找到证据` 只能形成 Open Assumption，不能直接证明现实不存在。

## 6. Live Backfill / Migration Race

**Blind spot**：不仅历史数据需要迁移，backfill 还与实时业务写入并发，可能用旧快照覆盖新状态，或旧版本继续生产旧格式数据。

**Concrete path**：backfill 读取旧值 → 用户提交新值 → backfill 按旧快照写回 → 新值被静默覆盖。

**Retrieve / falsify**：snapshot 时点、write ownership、CAS/version、处理顺序、dual-write period、分区游标、resume/retry，以及旧 producer 停止的证据。

## 7. Mixed-Version Emergent Behavior

**Blind spot**：v1 和 v2 独立运行都正确，但 rolling deployment 的组合世界错误。

**Gate**：新旧 producer/consumer、writer/scheduler 或 reader/schema 会真实并存。

**Retrieve / falsify**：枚举 `v1→v2` 和 `v2→v1` 的数据与控制流组合；检查 rollout/rollback 顺序、默认值、能力协商和共享状态语义。没有旧世界时记录 design evolution，不制造兼容性 Finding。

## 8. Multiple Truths / Derived-State Divergence

**Blind spot**：同一业务事实存在 MySQL、Redis、scheduler memory、search index、derived object 或 Agent session 等多个持久/运行时表示。

**Gate**：至少两个表示会影响可观察行为或不可逆副作用，而不是纯展示缓存。

**Retrieve / falsify**：确定 source of truth；找每个 representation 的 writer/reader；确认更新和失效链；检查旧 representation 是否仍能驱动副作用，以及 divergence 如何被发现和修复。

## 9. Ownership / Lease / Fencing

**Blind spot**：旧 owner 失去 lease 后仍能继续在最终资源产生副作用；“有 Redis lock”不等于安全。

**Applicability gate**：

```text
多个执行实体真实存在
→ 竞争 shared ownership
→ lease 失效或网络分区后旧 owner 仍可继续
→ 最终资源可能接受旧 owner 的写入
```

**Retrieve / falsify**：lease expiry、renewal failure、GC pause、partition、operation duration、ownership transfer，以及 fencing token 是否由最终资源强制校验。任一关键前提不成立就停止。

## 10. Temporal / Ordering Invariant Mismatch

**Blind spot**：基础设施保证的顺序单位，与业务真正要求的顺序单位不同。Kafka 按 `order_id` 有序，不等于业务需要的 `user_id` 全局顺序成立。

**Gate**：存在业务顺序 invariant，且多个事件可落入不同基础设施顺序域。

**Retrieve / falsify**：明确业务 ordering key、broker/partition/transaction 的保证单位、跨 key 合并、重试/replay 和陈旧事件处理。不要泛泛报告“消息可能乱序”。

## 11. Repair / Reconciliation Can Destroy Fresh State

**Blind spot**：reconcile、repair、periodic sync、cache rebuild、cleanup 或 recovery 根据旧快照，把暂时不一致误判为错误并覆盖/删除最新正确状态。

**Concrete path**：repair 读取旧 snapshot → 正常流更新 → repair 发现“差异” → 用旧值覆盖新值；或 eventual consistency 尚未收敛时把正确资源当 orphan 删除。

**Retrieve / falsify**：读快照时点、freshness/version guard、write authority、delete safety window、eventual-consistency delay、dry-run 和可逆性。

## 12. Configuration Combination / Untested World

**Blind spot**：多个 feature flag、tenant/region config 和 legacy mode 的组合形成单项测试从未覆盖的新控制流。

**Gate**：至少两个独立配置共同改变同一行为路径，且该组合在线上可达。

**Retrieve / falsify**：构造配置矩阵和控制流；确认默认值、override precedence、动态刷新、rollout/rollback 组合以及测试覆盖。单 flag 的普通分支错误不属于这个 Trigger。

## Hypothesis Merge

Trigger 是 supporting signal，不是 investigation 数量。先写 broken invariant，例如“未经用户批准不得产生 destructive side effect”；authorization、approval、retry 和 Agent workflow 可以共同支持它。只有触发条件、执行路径或被破坏状态真正不同才拆成多个 hypotheses。

## Discovery Re-entry

Deep dive 若发现新的 topology、state ownership、side-effect graph、cardinality、lifecycle、external exposure 或 runtime state，必须更新 Implementation Facts，重新执行相关 applicability gate 和 Coverage，再决定是否激活其他 Trigger。Review 不是单向流水线。
