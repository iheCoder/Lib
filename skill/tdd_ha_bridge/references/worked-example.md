# Worked Example: CreateOrder

本例仅展示结构和审查密度，不定义任何真实项目的订单语义。

## Verdict

```text
Status: BLOCKED_BY_ORACLE_AMBIGUITY
Scope: CreateOrder 的库存扣减、订单落库与消息发布
Baseline: 相关单元测试通过；未发现 publish failure 的契约测试
```

## One-minute Summary

正常创建、库存不足和并发幂等已有清晰 Oracle；最高风险在“订单已提交、消息发布失败”边界。需求没有说明应 rollback、保留并重试还是返回成功，因此不能提前把任一策略写成测试事实。

## Behavior Map

| ID | Obligation | Observable | Source | Confidence |
| --- | --- | --- | --- | --- |
| B1 | 合法请求只创建一个订单并正确扣减库存 | 订单数、库存、返回 ID | REQ | High |
| B2 | 库存不足不创建订单且库存不变 | 错误类型、订单数、库存 | DOMAIN | High |
| B3 | 相同幂等键不产生重复业务效果 | 订单数、库存扣减次数 | API | High |
| B4 | publish 失败后系统仍保持定义的一致性语义 | 订单、库存、事件、返回值 | UNKNOWN | Low |
| B5 | 缺省参数保持旧调用方行为 | 返回与副作用 | CALLER + TEST | Medium |

## Risk Map

| Pri | Risk | Why realistic now | Violates |
| --- | --- | --- | --- |
| P0 | commit 成功、publish 失败后客户端重试造成重复订单 | 调用方会重试 5xx，变更触及 publish | B3, B4 |
| P0 | 两个同幂等键请求同时穿过 pre-check | 当前唯一约束只在应用层 | B3 |
| P1 | `quantity == stock` 被误判为不足 | 本次修改了比较条件 | B1 |
| P1 | context 取消后事务仍提交 | 取消点位于 commit 前后不清晰 | B4 |

## Minimal High-value Scenarios

| ID | Pri | Intent | Given / Trigger | Expected + forbidden effects | Protects | Exposes | Oracle |
| --- | --- | --- | --- | --- | --- | --- | --- |
| T1 | P0 | 正常创建只产生一次效果 | stock > qty | 一个订单；库存扣减一次；一个 ID | B1 | 基线错误 | REQ / High |
| T2 | P0 | 库存不足无残留 | stock < qty | 明确错误；无订单；库存不变；不 publish | B2 | 先写后校验 | DOMAIN / High |
| T3 | P0 | 并发相同幂等键 | 两请求同闸门释放 | 只产生一次订单和一次扣减 | B3 | 应用层 check-then-act race | API / High |
| T4 | P1 | 恰好耗尽库存 | stock == qty | 成功；库存为 0 | B1 | `<`/`<=` 错误 | REQ / High |
| T5 | P0 | commit 后 publish 失败 | publish 注入失败 | 待裁决 | B4 | ambiguous retry | UNKNOWN / Low |

## Fault Challenge

| Fault | Wrong implementation | Violates | Killed by | Result |
| --- | --- | --- | --- | --- |
| M1 | 先建订单，再校验库存 | B2 | T2 | killed |
| M2 | 幂等只做非原子 pre-check | B3 | T3 | killed |
| M3 | `stock <= qty` 一律拒绝 | B1 | T4 | killed |
| M4 | publish 失败返回 5xx，但订单已提交且无稳定幂等 | B3, B4 | NONE | survivor: Oracle unknown |

## Coverage Audit

```text
Behavior coverage: B1 ✓  B2 ✓  B3 ✓  B4 ?  B5 △
Risk coverage:     R1 ?  R2 ✓  R3 ✓  R4 ?
Relevant lenses:   Contract ✓ / State ✓ / Partition ✓ / Interaction ? / Time-Concurrency △ / Regression △
```

## Human Review Required

1. publish 失败后的业务语义是什么？
   - 若订单保留：需要稳定幂等、outbox/retry 可观察性与重试测试。
   - 若整体失败：需要证明订单、库存和事件均无残留。
   - 若接口返回成功：需要明确事件最终送达保证和告警语义。
2. context cancellation 发生在 commit 之后时，API 应返回已成功结果还是取消错误？

本例没有为了“全面”加入 max-int、宇宙射线或所有依赖组合；它把人的注意力放在会实质改变实现的两个 Oracle 上。
