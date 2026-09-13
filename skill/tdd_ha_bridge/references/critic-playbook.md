# Adequacy Critic Playbook

Critic 的目标不是“再多想几个测试”，而是找出当前计划中 **现实发生概率不低且后果显著** 的缺口。新增候选数量由发现的真实风险决定，优先挑战 P0/P1，不用固定数量限制分析。

## Critic 输入

- 原始需求、变更范围和仓库证据；
- Source Ledger 与冲突；
- Behavior Map；
- Risk Map；
- 候选场景；
- P0/P1 Verification Strategies；
- 当前 baseline 测试结果（若有）。

不要只看 Designer 的摘要，否则 Critic 会继承同一盲区。若任务走 AI Agent Eval Route，同时读取 policy、tool schema、eval environment 和 grader contract。

## Pass 1 — 独立重建风险面

Critic 的 blind 阶段不能接收候选场景、Designer 的 Behavior/Risk Map 或其结论。只给原始需求、diff/change scope、API/policy、关键依赖和调用边界，独立写出：

- 必须成立的 behavior/invariant；
- 高价值结果依赖但原始需求未必明说的候选条件，以及它们的证据或 `UNKNOWN` 状态；
- 本次变更最可能出现的 P0/P1 错误实现；
- 最危险的状态、外部交互、时序和兼容边界；
- Oracle 可能被当前代码误导的位置。

先固化这份 `Blind Fault Inventory`，再 reveal Designer 产物并比较。不得在 reveal 后静默改写 inventory 使其与计划一致；新增发现要单独标记。缺失的义务优先于缺失的 testcase。

若只能在同一上下文执行，仍要先输出并冻结 blind artifact，再读取候选计划；在结论中声明 Critic 与 Designer 的认知错误可能相关。运行时允许且用户已授权委派时，高风险任务优先使用隔离 reviewer context。

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

还应把下面这些作为**构造错误实现的方法**，只在当前系统确实存在对应风险时实例化；它们不是每份设计都要机械执行的 testcase checklist：

- **局部变化破坏 preserved state**：目标内容修改正确，却顺手删除、覆盖、绕过或使一个仍应成立的条件失效；
- **同一事实的表示发生分歧**：两个组件、阶段或存储都表示同一语义，其中一个变旧，系统仍按错误认知继续；
- **系统认知与现实脱节**：系统认为动作已完成、检查已执行、状态仍有效或操作可执行，但真实环境不是如此；
- **中间变化绕过约束**：replan、retry、resume、handoff、fallback 或恢复路径绕过权限、审批、状态检查、必要步骤或仍有效的用户限制；
- **局部成功被当成最终成功**：中间步骤返回成功，但 durable outcome、后续步骤或必要副作用没有完成。

若这些构造暴露出 Designer 根本没有识别的候选义务，先回到 Source Ledger 判断它是 Confirmed、Candidate 还是 `UNKNOWN`；Critic 不能为了杀 mutant 擅自补写业务契约。

建立映射：

| Fault | Violates | Witness | Verify | Result / action |
| --- | --- | --- | --- | --- |
| M1 | B3 | T4 | V2 | killed |
| M2 | B4 | NONE | NONE | survivor: add / accept / clarify |

这里同时检查两件事：

- **Fault Coverage**：风险家族是否被代表；
- **Fault Exposure**：场景的输入、状态或时序是否真的触发该 fault，而非只在标题里提到。

## Pass 3 — Verification Fidelity Challenge

对每个 P0/P1 `Fault → Witness → Verification Strategy` 追问：

1. Verification level 是否保留 target property？
2. mock/fake 是否把真实事务、锁、offset、lease、权限或网络语义删除了？
3. failure/interleaving 是否能确定性控制，还是靠 sleep、压力和运气？
4. Observable 是否能区分正确与错误实现，并检查 durable state/forbidden residue？
5. 环境与生产差异是否足以改变结论？
6. 失败后是否有足够 Diagnostic Value 定位到 obligation/failure boundary？

场景和 Oracle 正确，但验证装置无法实际产生或观察目标 fault 时，标为 `PSEUDO_KILLER`，等同未覆盖。需要新增 test seam 或更高 fidelity 环境时标 `TESTABILITY_GAP`。

详细判据见 [verification-strategy.md](verification-strategy.md)。AI agent 还要检查 trial isolation、trajectory capture、environment outcome 和 grader calibration。

## Pass 4 — Oracle Challenge

逐个审查 P0/P1 Expected：

1. 来源是否真实存在，是否与任务语境匹配？
2. 是否从可疑实现或旧 Bug 反推行为？
3. 是否只断言“失败了”，却没断言状态和 forbidden side effects？
4. 是否把内部调用顺序当 contract，导致合法重构失败？
5. 来源冲突是否被静默裁决？
6. property 是否过强、适用域是否缺失？

对 `ASSUME/UNKNOWN` 不要伪造答案。把它升级为 Human Review Required，并说明不同裁决会怎样改变测试和实现。

## Pass 5 — Coverage Audit

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

### Verification Coverage

每个 P0/P1 fault 是否有：

- 合适的 verification level；
- 保留 target property 的依赖真实性；
- 确定性 control/injection point；
- 可观察 durable outcome 与 forbidden effect；
- 可重复 witness 与合理 diagnostic value。

覆盖符号：

- `✓`：有可执行 witness 且 Oracle 可信；
- `△`：有场景但触发或断言不足；
- `≈`：有纸面场景，但 verification fidelity/control 不足，是 pseudo-killer；
- `?`：Oracle 不确定；
- `✗`：未覆盖；
- `N/A`：已说明为何与本变更无关。

`N/A` 必须有基于代码/架构边界的理由，不能用于逃避分析。

## Pass 6 — 相关性、可理解性与信息组织

删除或降级：

- 不保护任何 obligation、也不暴露任何现实 fault 的场景；
- 与其他场景等价，只换了无关样例值的场景；
- 与当前变更关系很弱、不会改变判断的 P3 极端风险；
- 用 implementation detail 断言制造的脆弱场景。

只有当另一场景以相同或更高 fidelity 覆盖相同 obligations + faults，并保持相近 Diagnostic Value 时才能删除。如果一个场景同时杀死多个高风险 fault，保留并明确映射；如果它跨越过多 failure boundary 导致失败无法定位，则拆分或补诊断观察点。主文档应完整解释所有影响批准结论、实现选择或主要风险判断的场景。附件只保存复现材料，不能接走正文所需的上下文、预期、实际表现和判断理由。

Critic 还必须站在普通后端开发者视角检查默认主文档，而不是只审理论完整性：

1. 不看技术附件，能否说明这次验证了什么、使用什么输入或环境，以及当前是纸面设计还是实际运行？
2. 六类 Lens 是否全部用 `已覆盖 / 部分覆盖 / 未覆盖 / 不适用` 展示，并说明当前需求中实际检查的内容、支撑场景和判断依据？
3. 每个会改变总体判断的场景，是否完整说明背景、触发顺序、预期、验证办法、实际表现、判断理由和影响？尚未运行时是否明确说明没有实际结果？
4. 任何 Oracle 不明确、PSEUDO_KILLER 或 TESTABILITY_GAP，是否已经翻译成“正确结果缺依据”或“当前测试环境无法稳定制造/观察”，并同步降低对应 Lens 状态？
5. 是否要求读者打开附件或跨 `B/R/T/V/M` 五张表拼出同一个结论？若是，把理解所需的解释写回主文档，只把复现映射留在附件。
6. 同一风险是否在规则、风险、场景、验证和 fault challenge 中机械重复？若是，保留一份完整解释并在其他位置引用场景名称，不能通过删除不同阶段和因果关系来追求短小。
7. 决策标题是否描述现实事件和用户后果；选项是否说明会怎样改变预期、测试和实现？
8. 是否用“已检查轨迹”“评分部分通过”“详见附件”等句子代替了具体发生的事情？若是，补齐证据故事。

理论上充分但普通开发者无法只凭主文档解释结论的产物，仍应判为 `REVISION_REQUIRED`。

## Critic 结论

只能使用以下状态：

- `READY_FOR_HUMAN_REVIEW`：无未解释 P0/P1 survivor 或 pseudo-killer，所有 P0/P1 Oracle 可信且 verification strategy 足够真实、可控、可观察；低优先级假设已显式列出。
- `REVISION_REQUIRED`：存在可修复的行为、场景、触发或断言缺口。
- `BLOCKED_BY_ORACLE_AMBIGUITY`：关键业务语义无法从证据确定，不同答案会实质改变实现。
- `BLOCKED_BY_TESTABILITY_GAP`：P0/P1 语义无法被现有 harness 足够真实、确定地控制或观察。

内部技术证据的结论必须附：

```text
Strongest adequacy argument:
Top remaining uncertainty:
Unexplained P0/P1 survivors:
Pseudo-killers / testability gaps:
Smallest next action:
```

不要输出脱离证据的综合分数。充分性是可审查论证，不是神秘的 87/100。

上述英文键值不是默认主文档结尾。交付时按 [output-contract.md](output-contract.md) 翻译为中文结论、Lens 缺口和最小下一步；只有技术证据文件保留原始键值。
