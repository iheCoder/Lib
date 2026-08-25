# Verification Strategy Protocol

本协议回答的不是“应该测什么”，而是：

> 需要什么环境、控制手段和观察点，才能让这个场景实际杀死目标 fault？

每个 P0/P1 fault 必须沿着下面的链路闭合：

```text
Fault
  ↓
Witness Scenario
  ↓
Control / Injection Point
  ↓
Verification Level + Fidelity
  ↓
Observable Oracle
  ↓
Repeatability + Diagnostic Value
```

## 1. Verification Strategy 契约

为每个策略分配 `V{id}`，至少记录：

| Field | Question |
| --- | --- |
| Target fault | 要暴露的具体错误机制是什么？ |
| Verification level | unit / component / integration / contract / e2e / property / chaos/eval？ |
| Realism requirement | 哪段真实语义必须保留？ |
| Control / injection | 如何制造输入、故障、时序或依赖状态？ |
| Observable | 从哪里区分正确与错误实现？ |
| Repeatability | 是否能确定性复现？依赖哪些前提？ |
| Diagnostic value | 失败后能定位到哪条 obligation/failure boundary？ |
| Gap | 当前 harness 缺什么 test seam、环境或可观测性？ |

场景标题里出现 `timeout/concurrent/failure` 不等于已经具有对应控制能力。

## 2. 选择最低但足够的 Verification Level

| Level | 适合验证 | 不足以验证 |
| --- | --- | --- |
| Unit | 纯决策、分区、property、错误映射 | 真实事务、锁、协议、依赖时序 |
| Component | 单进程内多个真实组件与受控替身 | 跨进程协议与真实存储语义 |
| Integration | DB/MQ/cache/filesystem 等真实依赖语义 | 完整部署、网络拓扑、版本共存 |
| Contract | API/schema/consumer-provider 兼容与错误语义 | 真实业务状态迁移和容量 |
| E2E | 部署链路、跨服务副作用、用户可见结果 | 精确定位所有内部 fault |
| Property | 大输入域上的 invariant/metamorphic relation | 依赖真实性和特定故障顺序 |
| Chaos / fault injection | 网络分区、进程终止、节点/连接故障后的韧性 | 精确业务 Oracle 缺失 |
| Eval | 非确定性模型/agent 多 trial 行为 | 确定性 guardrail 的细粒度证明 |

选择能保留 target property 的最低层级，避免无意义地全部上 E2E；但不能为了速度把必须真实的语义降成 mock。

## 3. Fidelity Rule

> **Do not mock away the property being tested.**

若目标是以下语义，默认需要真实依赖或保留等价协议语义的专用 harness：

- DB unique constraint、isolation、lock、commit outcome ambiguity；
- Kafka offset/transaction/rebalance/order；
- Redis lease、TTL、owner token 与原子命令；
- filesystem atomic rename、fsync、permission；
- 网络半开、ACK 丢失、连接中断；
- schema/serialization 的真实兼容性。

普通 mock 可以验证“代码收到 error 后调用了哪个分支”，但不能证明真实系统会以相同状态和时序产生该 error。

对每个替身写一句 Fidelity Claim：

```text
这个 fake/mock 保留了目标语义 X；它没有模拟 Y，因此不能支持结论 Z。
```

若无法提出可信 claim，提升验证层级或标记 `TESTABILITY_GAP`。

## 4. Deterministic Control

涉及 concurrency、timeout、retry、cancellation、expiry 或 partial failure ordering 的 P0/P1 场景，必须说明怎样控制关键 interleaving。

优先使用：

- barrier / channel / latch；
- fake clock / virtual time；
- failpoint 或显式 test hook；
- controllable executor / scheduler；
- transaction isolation harness；
- 可暂停的 fake transport；
- 可定位到 commit/publish 前后的连接 fault injection。

反模式：

- `sleep` 猜测另一个 goroutine 已运行到某一步；
- 开 100 个 goroutine 重复跑，希望 scheduler 恰好触发；
- 用 timeout error mock 冒充“服务端已成功、ACK 未返回”；
- 测试失败后无法知道实际 interleaving。

若只有概率触发：

```text
Gap: TESTABILITY_GAP
Missing seam: 需要在 read 与 conditional update 之间加入仅测试可用的 barrier
Residual claim: 当前 stress test 只能提高发现概率，不能证明目标 interleaving 已被执行
```

## 5. Observable Oracle

为 target fault 选择能区分正确/错误实现的最小观察集合：

- return/error 与错误类型；
- durable state，而非只看 mock call；
- side-effect count、ordering、dedup key；
- committed offset、outbox row、lease owner、version column；
- forbidden residue；
- safety 与 liveness 的时间界限；
- resource/SLO 指标及稳定 baseline。

“没有报错”通常不是充分 Oracle。“最终状态正确”也可能漏掉非法中间副作用；反之，只检查 trajectory 顺序可能拒绝合法实现。观察点必须服务行为契约。

## 6. Diagnostic Value

测试不仅要 detect，也要帮助 localize。为每个 P0/P1 策略写明失败时最接近哪个 boundary：

```text
DB commit outcome / publish / retry / dedup
```

若一个 E2E 场景同时跨越多个 failure boundary，至少补充：

- 阶段性 state/trace；或
- 更窄的 integration/contract test；或
- 可明确区分失败位置的诊断信号。

不要为了 minimality 合并到失去诊断能力。

## 7. Operational Fidelity

对 migration、batch、stream、fan-out、rolling deploy 和资源敏感变更，Verification Strategy 还需说明：

- 代表性数据规模、分布和 skew；
- 与生产规模的外推假设；
- 并发流量与新旧版本组合；
- lock/CPU/IO/memory/connection/queue 观察点；
- crash、resume、rollback 和 backpressure 注入；
- SLO/预算阈值及来源。

小数据集功能正确不能证明一亿行 backfill 可上线。若无法在测试环境复现规模，采用分层证据：算法复杂度/查询计划 + 代表性 benchmark + canary/rollout guardrail，并显式保留外推不确定性。

## 8. Testability Gap 与停止条件

以下任一条件构成 P0/P1 `TESTABILITY_GAP`：

- 无法制造目标状态或故障顺序；
- 依赖替身删除了 target property；
- 无法观察 durable outcome 或 forbidden residue；
- 只能靠概率调度触发关键 interleaving；
- 测试环境与生产语义差异足以改变结论；
- agent eval 无法记录 trajectory/tool calls/environment outcome。

输出最小补强动作：增加 test seam、采用真实依赖、增加状态检查、引入 fake clock、构建 sandbox 或补 telemetry。若这些动作超出用户授权，只报告 gap，不自行扩展实现范围。
