# Requirement Review State and Checkpoint

在 review 期间维护这份内部状态，防止 requirement 漂移、scope 扩散、重复询问和单方向深挖。它也可以作为跨会话 checkpoint。默认不要把整份状态倾倒给用户。

## Checkpoint Template

```text
Review Identity
- Requirement:
- Seed files / symbols / modules:
- Explicit exclusions:
- Repository revision:
- Relevant file fingerprints:
- Pass number:
- Pass mode: Continue | Fresh-Eyes
- Checkpoint created at:

Requirement Contract
- Must:
- Should:
- Must Preserve:
- Must Never:
- Unknown:

Implementation Slice
- Input / entry:
- State and transformations:
- Persistence / side effects:
- Async or recovery boundaries:
- Runtime reconciliation:
- Observable outcome:

Implementation Facts
- [只记录代码、配置、数据流、状态所有权和运行前提中的客观事实]

Confirmed Context
- User-confirmed:
- Repository-confirmed:
- Inferred with evidence:
- Lifecycle:
- Topology:
- Scale:
- State / data:
- External exposure:

Risk Domains / Coverage Ledger
- State, money, security, permission:
  Status: pending | checked | deep | not-applicable
  Risk: High | Medium | Low | Unknown | Not Applicable
  Evidence / limit:
- Core requirement: ...
- Regression / real compatibility: ...
- Reliability / concurrency / resource / performance: ...
- Release / migration / runtime preconditions: ...
- Secondary behavior / edge cases: ...
- Architecture / maintainability: ...
- Repository conventions: ...

Active Hypotheses
- H1:
  Broken invariant:
  Supporting triggers:
  Why activated:
  Applicable when:
  Need evidence:
  Next retrieval:

Confirmed Findings
- Type:
  Severity:
  Current relevance: 当前适用 | 条件适用 | 待确认
  Causality: Introduced | Exposed | Amplified
  Preconditions:
  Failure trace:
  Evidence:
  Counter-evidence checked:
  Confidence:
  Scenario group:

Rejected Hypotheses
- Hypothesis:
  Counter-evidence:
  Evidence location / fingerprint:
  Revalidate when:

Evidence Dependencies
- Finding or rejection:
  Depends on file / config / runtime fact:
  Current evidence:
  Staleness signal:

Open Assumptions
- Exact question:
  Why decision-changing:
  If yes:
  If no:

Blind-Spot Pass
- Irreversible or silent state corruption:
- Core path impossible:
- Existing main flow regression:
- Missing release / data premise:
- Implementation Slice gap:
- High / Unknown domain not deep-dived:

Next Investigation Queue
- Priority:
  Domain / hypothesis:
  Retrieval target:
  Stop condition:

Checkpoint Status
- Staleness: fresh | partially-stale | stale | not-checked
- Completed this pass:
- Still pending:
- Verdict: BLOCK | NEEDS CONFIRMATION | PASS WITH RISKS | PASS | CHECKPOINTED
```

## 状态推进规则

1. 用户补充的事实进入 `User-confirmed`，并更新相关 hypothesis、Current Relevance 和 severity；不要重复询问。
2. 仓库直接证明的事实进入 `Repository-confirmed`；可推断但未直接证明的事实进入 `Inferred with evidence`。
3. 只有答案会改变 Finding、Current Relevance、severity、scope 或 investigation route 的未知才进入 `Open Assumptions`。
4. Trigger 命中只进入 `Active Hypotheses`。多个信号指向同一 broken invariant 时先合并。
5. 形成 failure trace、取得证据、找过反证并通过 causality gate 后，才能进入 `Confirmed Findings`。
6. 一次 deep dive 结束后立刻更新 ledger 和 queue；存在未处理的 High 领域时不能输出最终 PASS。
7. `Rejected Hypotheses` 保留反证依赖，避免重复调查，也方便证据变化后重新激活。
8. Deep dive 发现新 topology、writer、side effect、cardinality 或 lifecycle 时，更新 facts，重新执行相关 gate 和 coverage。

## Requirement Scope Gate

每次准备读取 seed 之外的文件前，写清它与 Implementation Slice 的关系。允许扩展到 direct callers/callees、相关 state readers/writers、persistence、scheduler、config/migration、tests 和 external effects；禁止因为 Git diff 或 working tree 中存在其他变化就顺带审查。

## Applicability Gates

### Compatibility / Mixed Version

先确认旧世界是否真实存在：旧版本是否部署、是否有外部 consumer/SDK、历史消息/持久数据、rolling deployment 或其他仓库依赖。全部否定时记录为 `design evolution`。

### Distributed Ownership

```text
并发执行实体真实存在
→ 访问共享业务状态或竞争 ownership
→ 协调失效后旧 owner 仍可能产生副作用
```

任一关键前提不成立就停止；不要把“用了 Redis/Kafka”当场景证明。

### Scale / Amplification

必须有至少两个可增长因子真实相乘，并取得现实上界。固定小集合不生成高并发或大规模 Finding。

## Deep-Dive Stop Rule

满足任一条件就停止当前方向并返回 ledger：

- hypothesis 被可靠反证；
- 影响被边界或容量上限限制到不值得报告；
- 已有足够证据形成 Finding；
- 继续检索的边际信息价值很低，缺失事实应转为 Open Assumption。

## Resume and Staleness Protocol

恢复 checkpoint 前必须比较：

1. repository revision；
2. seed 与 Implementation Slice 相关文件的 fingerprint；
3. Confirmed Finding 的证据依赖；
4. Rejected Hypothesis 的反证依赖；
5. 关键配置、migration 和用户确认的 runtime facts。

证据变化时只使依赖它的状态失效，不必全部推倒重来。例如旧 review 依赖 unique constraint 拒绝重复副作用假设，而 migration 已删除该约束，则把该 Rejected Hypothesis 重新放回 queue。

### Continue Pass

加载 Requirement Contract、confirmed context、Implementation Facts、findings、rejected hypotheses、coverage、evidence dependencies 和 next queue，继续未完成领域。适合节省 token。

### Fresh-Eyes Pass

只加载 Requirement Contract、confirmed runtime context、seed scope 和已完成的 coverage 大类；暂不加载旧 hypotheses 与 findings。独立完成 blind-spot search 后再 reconcile，降低 anchoring。

## Interaction Rule

好问题必须窄且改变决策，例如“这个 scheduler 在线上是否可能同时运行多个实例？”或“backfill 运行时旧版本是否仍会写同一字段？”。不要问“请介绍系统架构/业务背景/线上规模”。用户尚未回答时继续检查其他领域，并写条件化结论。

## Verdict Rules

- `BLOCK`：存在已证实的 P0/P1 code defect，或确定无法安全发布；
- `NEEDS CONFIRMATION`：关键 P0/P1 结论依赖少量外部事实；
- `PASS WITH RISKS`：最终 review 没有 blocking defect，但有有限风险或非阻塞 Finding；
- `PASS`：在明确 requirement、场景和覆盖内未发现 Finding，且关键领域获得与风险相称的验证；
- `CHECKPOINTED`：本轮只完成部分调查并保存状态，仍有明确待审领域；它不是最终 verdict。

`PASS` 也不能表述成“证明代码没有问题”。
