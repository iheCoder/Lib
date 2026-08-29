# Review State

在 review 期间维护这份内部状态，防止上下文漂移、重复询问和单方向深挖。默认不把整份状态倾倒给用户；最终只输出 findings、未决前提和覆盖边界。

## State Template

```text
Review Scope
- Base:
- Head:
- Included:
- Explicit exclusions:

Change Contract
- Must:
- Should:
- Must Preserve:
- Must Never:
- Unknown:

Change Facts
- [只记录代码、配置、数据流和调用关系中的客观事实]

Scenario Context
- Confirmed:
- Inferred:
- Unknown but decision-changing:
- Lifecycle:
- Topology:
- Scale:
- State/data:
- External exposure:

Risk Domains / Coverage Ledger
- State, money, security, permission: pending | checked | deep | not-applicable
  Risk: High | Medium | Low | Unknown | Not Applicable
  Evidence/limit:
- Core requirement: ...
- Regression / real compatibility: ...
- Reliability / concurrency / resource / performance: ...
- Release / migration / runtime preconditions: ...
- Secondary behavior / edge cases: ...
- Architecture / maintainability: ...
- Repository conventions: ...

Active Hypotheses
- H1:
  Why activated:
  Applicable when:
  Need evidence:
  Next retrieval:

Rejected Hypotheses
- Hypothesis:
  Counter-evidence:

Candidate Findings
- Type:
  Severity:
  Change causality:
  Preconditions:
  Failure trace:
  Evidence:
  Counter-evidence checked:
  Confidence:

Open Questions
- Exact question:
  Why decision-changing:
  If yes:
  If no:

Blind Spot Pass
- Irreversible or silent state corruption:
- Core path impossible:
- Existing main flow regression:
- Missing release/data premise:
- Dangerous diff not deep-dived:
```

## 状态推进规则

1. 用户补充的事实进入 `Confirmed`，并更新相关 hypothesis 和 severity；不要在后续轮次重复询问。
2. 代码可推断但未直接证明的事实进入 `Inferred`，保留证据位置和置信度。
3. 只有答案会改变 finding、severity、scope 或 investigation route 的未知才进入 `Open Questions`。
4. 触发器命中只创建 `Active Hypothesis`；形成 failure trace、找到证据、找过反证并通过 causality gate 后才能进入 `Candidate Findings`。
5. 一次 deep dive 结束后立刻更新 ledger，再选择下一个 High/Unknown 领域。存在未检查的 High 领域时不能收尾。
6. `Rejected Hypotheses` 默认不输出，但保留到 review 结束，避免同一怀疑反复消耗上下文。

## Compatibility Gate

检测到 contract 改变时先回答“旧世界是否真实存在”：

- base/旧版本是否部署过；
- 是否已有外部消费者或已发布 SDK；
- 是否已有持久数据或历史消息；
- rolling deployment 时是否会有新旧版本并存；
- 是否有其他仓库/分支已基于旧 contract 开发。

全部否定时，记录为 `design evolution`，不要生成 backward-compatibility finding。

## Distributed Applicability Gate

只有以下链条成立才深入分布式 ownership 风险：

```text
并发执行实体真实存在
→ 它们访问共享业务状态或竞争 ownership
→ 失去协调时仍可能继续产生相关副作用
```

如果 topology 未知且答案会改变 P0/P1 结论，提一个窄问题或输出 Open Assumption。不要把“用了 Redis/Kafka”替代场景证明。

## Deep-dive Stop Rule

满足任一条件就停止当前方向并返回 ledger：

- 假设被可靠反证；
- 影响已被边界或容量上限限制到不值得报告；
- 已取得足够证据形成 finding；
- 继续检索的边际信息价值很低，且缺失信息应转为 open assumption。

## Interaction Rule

好问题必须窄且改变决策，例如：

- “这个 scheduler 在线上是否可能同时运行多个实例？”
- “这个 base 分支是否已经部署到测试或生产环境？”
- “线上历史记录是否已完成 `source_task_id` backfill？”
- “单个模板现实中最多派生几十、几万还是百万个对象？”

不要问“请介绍系统架构/业务背景/线上规模”。用户尚未回答时继续检查其他领域，并写条件化结论：

```text
若 scheduler 始终单实例，该 ownership 风险不适用；
若多个实例可同时执行同一任务，则继续验证 lease expiry 与 fencing。
```

## Verdict Rules

- `BLOCK`：存在已证实的 P0/P1 code defect，或确定无法安全发布；
- `NEEDS CONFIRMATION`：关键 P0/P1 结论依赖少量外部事实，答案会改变是否阻塞；
- `PASS WITH RISKS`：没有 blocking defect，但有有限风险、未验证项或非阻塞 finding；
- `PASS`：在明确范围和已确认场景中未发现 finding，且关键领域已获得与风险相称的验证。

`PASS` 仍然不能表述为“证明代码没有问题”。
