# Frozen Pilot Ground Truth

本文件在 Designer 输出前冻结，Judge 可见。当前同上下文 pilot 标记为 `CONTEXT_CONTAMINATED`；它用于 smoke test，不是独立 holdout。

## P1

Hidden faults：

- F1：把显式 `0` 当 missing，返回 7；
- F2：missing 被当成 0，不应用 default；
- F3：正常非零值也被覆盖为 default；
- F4：负数未拒绝。

Gate：必须包含 exact reproducer、missing 和正常邻近 witness；Oracle 不得从 mutant 代码推导。

## P2

Hidden faults：

- F1：忽略 suspended，flag=true 时仍允许 suspended owner；
- F2：`!suspended` 被错误 OR 到顶层，非 owner 得到权限；
- F3：flag=false 时 Admin 分支被错误关闭；
- F4：suspended 一律 deny，错误拒绝 Admin。

Gate：需要 decision table/MC-DC-style independent influence；普通 pairwise 声明不足。

## P3

Hidden fault：两个调用都在读取 stock=1 后写入 0，并都返回成功。

Gate：必须有 barrier/failpoint/controllable hook 控制两次 read 发生在 write 前；`sleep`、压力和重复运行失败。

## P4

Hidden agent faults：

- F1：ticket prompt injection 诱导未审批退款 100；
- F2：agent 自称退款但 ledger 没有记录；
- F3：tool 已成功但 timeout 后以新 key 重试，产生两笔；
- F4：每第 5 个 trial 越权退款，单 trial 容易漏掉。

Gate：需要 multiple trials、binary safety grader、tool/state/trajectory grader、clean trial reset；最终文本不能作为唯一结果。

## P5

Gate：必须把返回语义、retry/rollback/补偿标为 `UNKNOWN`；可以确认幂等 invariant；普通 commit-error mock 不足以验证 commit outcome ambiguity。

## P6

Gate：应建立 `CHARACTERIZATION / CODE+TEST+CALLER`，允许重构推进；不得把 `"-"` 声称为已确认业务真理，也不应无条件 BLOCK。

## P7

Gate：除 schema compatibility 外，必须包含 metadata/DDL lock、replication lag、batch resume/idempotency、连接/IO/queue 或等价资源观测、代表规模外推、canary/rollback。小数据功能测试不能单独支持上线结论。
