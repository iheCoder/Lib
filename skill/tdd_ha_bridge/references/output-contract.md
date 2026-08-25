# Human Review Pack

默认使用中文，代码符号、错误名和业务术语保留仓库原样。主视图面向 1–3 分钟审查；只有被追问时再展开 P2/P3 或完整测试代码草案。

## 0. Verdict

```text
Status: READY_FOR_HUMAN_REVIEW | REVISION_REQUIRED | BLOCKED_BY_ORACLE_AMBIGUITY
Scope: 本次测试设计覆盖的变更边界
Baseline: 已运行的既有测试与结果；未运行则说明原因
```

## 1. One-minute Summary

- 一句话说明要守住的业务结果；
- P0/P1 风险数量和最危险的 failure boundary；
- 是否存在 UNKNOWN Oracle；
- 工程师此刻需要决定什么。

## 2. Behavior Map

| ID | Behavioral obligation / invariant | Observable | Source | Confidence |
| --- | --- | --- | --- | --- |
| B1 | ... | 返回值 + 状态 + 非副作用 | REQ | High |

只写工程师真正需要审批的规则，避免把每个测试步骤变成 behavior。

## 3. Risk Map

| Priority | Risk / plausible failure mechanism | Why realistic now | Violates |
| --- | --- | --- | --- |
| P0 | ... | 变更触及事务与 publish 边界 | B3 |

P2/P3 默认折叠成一句摘要。

## 4. Minimal High-value Scenarios

| ID | Pri | Scenario intent | Given / Trigger | Expected + forbidden effects | Protects | Exposes | Oracle |
| --- | --- | --- | --- | --- | --- | --- | --- |
| T1 | P0 | ... | ... | ... | B1, B3 | M1 | REQ / High |

约束：

- `Scenario intent` 用业务语言，不用 `TestFooBar`；
- `Expected` 同时说明关键结果和绝不能发生的副作用；
- `Protects` 映射 Behavior；
- `Exposes` 映射 plausible fault；
- `Oracle` 写来源和置信度，`UNKNOWN` 不得藏在脚注。

## 5. Plausible Fault Challenge

| Fault | Plausible wrong implementation | Violates | Killed by | Result |
| --- | --- | --- | --- | --- |
| M1 | ... | B3 | T4 | killed |
| M2 | ... | B4 | NONE | survivor |

只展示最有价值的 3–5 个 challenge。若有 P0/P1 survivor，必须说明补场景、接受风险还是等待业务裁决。

## 6. Coverage Audit

```text
Behavior coverage: B1 ✓  B2 ✓  B3 △  B4 ?
Risk coverage:     R1 ✓  R2 ✗  R3 ✓
Relevant lenses:   Contract ✓ / State ✓ / Interaction △ / Time-Concurrency N/A / Regression ✓
```

随后只解释 `△ ? ✗` 和有争议的 `N/A`。不重复已通过项。

## 7. Human Review Required

按优先级只列会改变测试或实现的决定：

```text
1. [P0 Oracle] publish 失败后订单应保留并重试，还是整体失败？
   Evidence: 当前代码保留；需求未定义；调用方会对 5xx 重试。
   If A: 增加 outbox/retry contract 测试。
   If B: 增加 rollback/no-residue 测试。
```

不要询问可从仓库直接查明的问题。

## 8. Approval Handoff

结尾明确：

- 建议批准的 behavior 与场景；
- 待裁决项；
- 获批后下一步是“写测试并确认 meaningful red”，还是仅归档设计。

Meaningful red 是因目标行为尚未实现而失败；编译错误、fixture 损坏、环境依赖失败不能冒充 TDD Red。
