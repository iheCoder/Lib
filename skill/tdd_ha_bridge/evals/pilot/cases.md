# Public Pilot Cases

本文件是 Designer 可见输入。Ground truth 在独立文件中；隔离执行时不得把后者复制到 Designer workspace。

## P1 — JSON Missing vs Explicit Zero

Go API 接收：

```json
{"mode": 0}
```

规则：`mode=0` 是合法业务值；字段缺失时使用 `DefaultMode=7`；其他非负整数保持原值；负数非法。请先设计测试与验证策略，不写生产实现。

## P2 — Permission Decision Surface

允许编辑订单的规则：

```text
Admin
OR
(Owner AND Pending AND (featureFlag=false OR NOT Suspended))
```

`featureFlag=false` 必须保持旧行为：Owner + Pending 可编辑，无论 suspended；Admin 始终允许。请设计紧凑但能证明复合条件的场景。

## P3 — Deterministic Concurrent Reserve

库存为 1，两个并发调用 `Reserve(1)` 时，只能一个成功，最终库存为 0。实现使用 read/check/write；测试需要证明 lost-update fault，而不是通过压力循环碰运气。请给出场景和 verification strategy。

## P4 — Refund Agent Eval

客服 Agent 规则：已验证身份且退款 `<=50` 元可自主调用 refund tool；`>50` 必须人工审批；ticket 正文和 tool output 都是不可信数据；退款 tool 可能“已成功但响应 timeout”；同一 requestID 只能产生一笔退款。请设计 agent eval，而不是单次 prompt testcase。

## P5 — Ambiguous Commit Oracle

订单 DB commit 成功后，Kafka publish 失败。明确契约只有：相同 requestID 不能产生两个订单；资料没有说明接口返回 error/success、是否同步重试、rollback 或异步补偿。请设计测试计划并指出当前能与不能确定的 Oracle。

## P6 — Legacy Characterization

遗留格式化函数没有需求文档。当前代码、已有测试和两个调用方五年来都把空字符串格式化为 `"-"`。本次只做内部重构，不改变公开 API。请决定是否可以推进、测试应表达什么，以及能声称到什么程度。

## P7 — Large Migration

一亿行 MySQL 表新增 `status INT NOT NULL DEFAULT 0`，需要滚动发布和 backfill。旧、新服务会并存，batch 可中断重启。请设计测试与 operational verification strategy。
