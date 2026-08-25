# Adequacy Critic Playbook

Critic 的目标不是“再多想几个测试”，而是找出当前计划中 **现实发生概率不低且后果显著** 的缺口。默认最多提出 3–5 个新增候选，优先挑战 P0/P1。

## Critic 输入

- 原始需求、变更范围和仓库证据；
- Source Ledger 与冲突；
- Behavior Map；
- Risk Map；
- 候选场景；
- 当前 baseline 测试结果（若有）。

不要只看 Designer 的摘要，否则 Critic 会继承同一盲区。

## Pass 1 — 独立重建风险面

暂时忽略候选场景，从原始材料独立写出：

- 必须成立的 3–7 个 behavior/invariant；
- 本次变更最可能出现的 P0/P1 错误实现；
- 最危险的状态、外部交互、时序和兼容边界；
- Oracle 可能被当前代码误导的位置。

将重建结果与 Designer 的 Behavior/Risk Map 比对。缺失的义务优先于缺失的 testcase。

## Pass 2 — Plausible Fault Challenge

对每个 P0/P1 risk 构造最小 plausible mutation。它应满足：

- 是工程师或 coding agent 在当前代码中可能写出的错误；
- 改动小、语义具体；
- 违反某条行为义务；
- 理论上能被一个可执行场景暴露；
- 不是宇宙射线、极端硬件故障等低价值幻想。

挑战问题：

> 哪个错误实现可以通过当前全部场景，却仍违反 Bx？

优先变异真实决策点：

- `>` / `>=`、错误 default、条件反转；
- 副作用顺序交换或漏掉一步；
- commit 成功但 response/publish 失败；
- 幂等记录过早/过晚；
- 顺序正确但并发失效；
- cancel/timeout 后继续提交；
- 错误被吞、替换或错误重试；
- 旧调用方或缺省配置被新逻辑污染。

建立映射：

| Fault | Violates | Killed by | Survivor reason | Action |
| --- | --- | --- | --- | --- |
| M1 | B3 | T4 | — | keep |
| M2 | B4 | NONE | 缺并发重复 | add / accept / clarify |

这里同时检查两件事：

- **Fault Coverage**：风险家族是否被代表；
- **Fault Exposure**：场景的输入、状态或时序是否真的触发该 fault，而非只在标题里提到。

## Pass 3 — Oracle Challenge

逐个审查 P0/P1 Expected：

1. 来源是否真实存在，是否与任务语境匹配？
2. 是否从可疑实现或旧 Bug 反推行为？
3. 是否只断言“失败了”，却没断言状态和 forbidden side effects？
4. 是否把内部调用顺序当 contract，导致合法重构失败？
5. 来源冲突是否被静默裁决？
6. property 是否过强、适用域是否缺失？

对 `ASSUME/UNKNOWN` 不要伪造答案。把它升级为 Human Review Required，并说明不同裁决会怎样改变测试和实现。

## Pass 4 — Coverage Audit

至少建立两个映射：

### Behavior Coverage

每个核心 behavior/invariant 是否有：

- 正向或反向 witness；
- 明确观察点；
- 可信 Oracle；
- 与场景的可追踪关系。

### Fault Coverage

每个 P0/P1 fault 是否有：

- 能触发 fault 的输入/状态/时序；
- 能区分正确与错误实现的断言；
- 对 partial failure 的残留状态检查；
- 对并发风险的真实并发控制，而非顺序替代。

覆盖符号：

- `✓`：有可执行 witness 且 Oracle 可信；
- `△`：有场景但触发或断言不足；
- `?`：Oracle 不确定；
- `✗`：未覆盖；
- `N/A`：已说明为何与本变更无关。

`N/A` 必须有基于代码/架构边界的理由，不能用于逃避分析。

## Pass 5 — Minimality 与 Reviewability

删除或降级：

- 不保护任何 obligation、也不暴露任何现实 fault 的场景；
- 与其他场景等价，只换了无关样例值的场景；
- P3 极端风险却占据主审查视图的场景；
- 用 implementation detail 断言制造的脆弱场景。

如果一个新增场景同时杀死多个高风险 fault，保留并明确映射。主审查包应该小而强，完整细节可放附录。

## Critic 结论

只能使用以下状态：

- `READY_FOR_HUMAN_REVIEW`：无未解释 P0/P1 survivor，且所有 P0/P1 Oracle 可信；低优先级假设已显式列出。
- `REVISION_REQUIRED`：存在可修复的行为、场景、触发或断言缺口。
- `BLOCKED_BY_ORACLE_AMBIGUITY`：关键业务语义无法从证据确定，不同答案会实质改变实现。

结论必须附：

```text
Strongest adequacy argument:
Top remaining uncertainty:
Unexplained P0/P1 survivors:
Smallest next action:
```

不要输出脱离证据的综合分数。充分性是可审查论证，不是神秘的 87/100。
