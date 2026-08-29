# Risk Trigger Library

这个库把 change signal 转成**待验证假设**。使用前先确认现实前提；命中关键词、技术组件或代码形状，不能直接生成 Finding。

每次只读取与当前 Change Facts 有关的条目。优先组合信号：单个 `retry`、`Redis`、`loop` 或 `API` 通常不足以激活高风险调查。

## 1. Retry / timeout + 外部副作用

**Signals**：retry、timeout retry、付款、发送消息、创建外部资源、发货、扣库存、调用第三方 mutation。

**Applicable when**：调用可能在副作用已成功但响应丢失后重试，或调用方无法区分“未执行”和“执行结果未知”。

**Hypotheses**：重复扣款/发货/创建；本地与外部状态分歧；重试风暴。

**Retrieve / falsify**：retry boundary、idempotency key 的生成与稳定性、唯一约束、provider dedup contract、状态机、超时后的 reconciliation、调用方是否真的重试。

## 2. 多个状态写入或跨系统副作用

**Signals**：DB write + publish、两张表更新、local state + remote call、先删后建、transaction callback。

**Applicable when**：多个动作需要共同满足一个业务不变量，但没有天然原子性。

**Hypotheses**：partial success；commit 成功但 publish 失败；补偿再次破坏数据；rollback 只覆盖部分状态；重试重复前一步。

**Retrieve / falsify**：transaction boundary、outbox/inbox、执行顺序、failure handling、compensation、reconciliation、可重复恢复语义，以及每个步骤之间 crash 的结果。

## 3. Read-modify-write / check-then-act

**Signals**：读取后计算再写回；先检查不存在再创建；余额、库存、计数器、版本或状态转换。

**Applicable when**：同一实体可能有并发写者，或读取结果到写入之间允许状态变化。

**Hypotheses**：lost update、重复创建、越权状态转换、超卖、陈旧写覆盖新值。

**Retrieve / falsify**：事务隔离、CAS/version、条件更新、唯一约束、锁粒度、所有写入入口、冲突重试语义。只有单写者且可由拓扑/ownership 证明时才拒绝并发假设。

## 4. 一对多派生 / 全局模板或配置

**Signals**：一个模板派生多个对象；全局配置变化传播到用户数据；同步 fan-out；批量刷新。

**Applicable when**：真实 fan-out 可能大于常数级，或部分传播会留下长期不一致。

**Hypotheses**：写放大、超大事务、partial propagation、update storm、重试重复更新、旧派生数据与新模板不兼容。

**Retrieve / falsify**：最大 cardinality、同步/异步、分页与 batch、事务范围、索引、恢复点、重复执行、速率限制、派生对象是快照还是动态引用。

## 5. Loop / recursion + DB、RPC 或大对象处理

**Signals**：循环内 query/RPC、递归加载、逐项序列化/复制、无界集合、并发 fan-out。

**Applicable when**：集合大小可随用户/数据增长，且单项代价并非纯内存常数操作。

**Hypotheses**：N+1、连接池压力、延迟/内存放大、下游限流、goroutine 爆炸。

**Retrieve / falsify**：真实上界、batch API、query placement、分页、concurrency limit、timeout、benchmark/query plan。小而固定的集合可以拒绝该风险。

## 6. Schema / migration / 新必填字段

**Signals**：新 non-null/required 字段、默认值语义变化、读取假定历史数据已存在、rename/drop、backfill、索引变化。

**Applicable when**：旧数据、旧代码、旧消息或 mixed-version 实例真实存在。

**Hypotheses**：历史数据违反新假定；部署顺序导致新旧版本互相不可读；大表锁；backfill 中断或重复；rollback 后旧代码无法读取；默认值悄悄改变业务含义。

**Retrieve / falsify**：migration/backfill 文件、数据分布证据、expand-contract 顺序、null/default handling、旧版本读写、回滚路径、操作耗时与锁行为。没有旧世界时，把它视为 design evolution，不报兼容性回归。

## 7. API / event / persisted contract change

**Signals**：字段删除/改名/改类型、enum 扩展、错误码变化、JSON missing 与 zero/null 语义、消息 schema、持久化格式。

**Applicable when**：已有消费者、已发布 SDK、历史消息/数据或旧版本实例可能存在。

**Hypotheses**：旧客户端解析失败；旧 consumer 不认识新 enum；缺失值被误解释；rolling rollout 互操作失败；历史数据反序列化失败。

**Retrieve / falsify**：真实调用方与 consumer、版本策略、schema registry、宽容读取、producer/consumer rollout 顺序、部署历史、branch 状态。开发分支从未部署且无外部消费者时不要制造兼容性 Finding。

## 8. Async / goroutine / worker lifecycle

**Signals**：goroutine/thread、background task、channel/queue、callback、parallel map、worker pool、shutdown hook。

**Applicable when**：并发路径可同时运行，或任务生命周期可能超过请求/进程阶段。

**Hypotheses**：race、deadlock、leak、取消/超时丢失、unbounded concurrency、panic 隔离失败、shutdown 丢任务、closure 捕获错误。

**Retrieve / falsify**：ownership、join/wait、context propagation、channel close 责任、共享状态同步、并发上界、panic/error 汇聚、graceful shutdown，并用 deterministic barrier/fake clock/race detector 验证。

## 9. Queue / event delivery

**Signals**：Kafka、stream、queue、consumer retry、ack/commit、DLQ、event handler。

**Applicable when**：broker contract 允许重复、乱序、延迟、批次部分成功，或 consumer 会重平衡。

**Hypotheses**：重复业务效果；ack 与业务提交错序；poison message 阻塞；旧事件覆盖新状态；重平衡期间丢失/重复；schema rollout 不兼容。

**Retrieve / falsify**：delivery guarantee、partition key/order boundary、ack timing、consumer idempotency、offset transaction、DLQ/replay、version handling。不要仅因出现 Kafka 就假设全局乱序。

## 10. Cache / derived index

**Signals**：cache-aside、write-through、失效、TTL、negative cache、搜索索引或读模型。

**Applicable when**：缓存/索引结果会影响正确性或在高并发下放大依赖压力。

**Hypotheses**：stale read 违反业务不变量；更新 DB 后失效失败；stampede；negative cache 隐藏新数据；多 key 更新部分成功。

**Retrieve / falsify**：source of truth、允许的陈旧窗口、写入/失效顺序、singleflight/lock、TTL jitter、fallback、rebuild/reconciliation。只承载可容忍陈旧数据的单实例 Redis cache 不自动产生分布式 ownership 风险。

## 11. Lock / lease / leader / ownership

**Signals**：distributed lock、lease、leader election、fencing token、renewal、ownership transfer。

**Applicability gate**：

```text
多个并发执行实体？
→ 共享状态或竞争同一 ownership？
→ lease/lock 过期或网络分区时旧 owner 还能继续产生副作用？
```

任一关键前提不成立时停止。

**Hypotheses**：双 owner；lease 执行中到期；旧 owner 缺少 fencing 仍写入；ownership 转移期间重复/遗漏执行。

**Retrieve / falsify**：实例拓扑、lease duration vs operation duration、renewal failure、fencing enforcement 在最终资源端是否生效、clock assumptions、partition 行为、幂等/去重。单实例或无共享副作用时不报脑裂。

## 12. Authorization / tenant / secret / privacy boundary

**Signals**：鉴权顺序变化、资源 ID、tenant/user scope、管理员路径、日志/错误输出、token/secret、批量导出。

**Applicable when**：不可信调用者可控制标识或返回内容跨越权限/租户边界。

**Hypotheses**：IDOR、先读后鉴权泄露、跨租户查询/缓存污染、日志泄密、默认放行、批处理部分越权。

**Retrieve / falsify**：入口 authn/authz、资源归属条件是否进入 DB query、所有分支与 fallback、缓存 key、错误响应、日志字段、服务间身份。不要仅凭“有用户 ID”报安全问题。

## 13. Delete / overwrite / default / error handling change

**Signals**：删除 guard、扩大 delete/update 条件、zero/default 行为变化、忽略 error、fallback 从 fail-closed 变 fail-open、defer/cleanup 改动。

**Applicable when**：修改会触达持久状态、权限、外部副作用或核心流程控制。

**Hypotheses**：全表/跨租户修改；静默数据覆盖；错误后继续成功响应；cleanup 删除正确资源；默认值绕过约束；异常被吞掉后触发重试或重复操作。

**Retrieve / falsify**：where/scope 条件、affected rows、dry-run/backup、error propagation、caller behavior、默认值来源、defer 捕获值与执行顺序、测试是否观测 forbidden effect。

## 14. Scheduler / time / natural-language automation

**Signals**：cron/RRULE、时区、自然语言转结构化任务、多轮确认、next-run 计算、重排/去重、任务持久化。

**Applicable when**：用户输入需要跨层转换并最终产生可执行、持久化的调度副作用。

**Hypotheses**：对话成功但任务未创建；创建了错误时区/频率；DST/月底语义漂移；确认轮次丢字段；回复与真实任务状态不一致；重试创建重复任务；scheduler 未加载新任务。

**Retrieve / falsify**：从输入解析、澄清/确认、tool call、持久化、scheduler reload 到用户回复的端到端链路；时区来源；幂等键；失败状态；实际 next occurrence，而不只检查文本格式。

## 15. AI agent / tool workflow

**Signals**：多轮 LLM、tool calling、审批、环境副作用、模型重试、structured output、恢复会话。

**Applicable when**：成功不仅取决于最终文本，还取决于工具轨迹或环境状态。

**Hypotheses**：声称成功但工具未执行；重复 tool call；审批前产生副作用；恢复后重复提交；格式正确但环境错误；失败后状态被错误标成完成。

**Retrieve / falsify**：trajectory、tool result、state transition、approval boundary、idempotency、retry/recovery、最终环境 state。验证应同时检查 outcome 与副作用，不把单次 happy-path 文本当作完成证据。
