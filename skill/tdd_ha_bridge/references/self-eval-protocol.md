# Self-validation Protocol

本模式验证 `tdd-ha-bridge` 自身是否改变了模型的测试设计行为，并且是否产生了能实际杀死错误实现的 tests/evals。

## 1. 证据层级

### Layer A — Structural Validation

检查 frontmatter、引用、资源、未完成占位符和格式。它只能证明 skill 可加载，不能证明有效。

### Layer B — Synthetic Executable Forward Eval

使用几十行到数百行的 micro-project，精确隔离一种能力：decision surface、missing-vs-zero、concurrency、partial failure、Oracle ambiguity、agent grader 等。每题包含正确实现和 hidden plausible mutants。

### Layer C — Historical Bug Replay

使用真实 issue + pre-fix repo，不向 Designer 暴露 fix diff。检查设计能否导出在 pre-fix 失败、post-fix 通过的测试，并评估 context discovery、noise resistance 和依赖 fidelity。

### Layer D — Human Workflow Evidence

由工程师盲审 baseline 与 skill 输出，比较关键风险发现率、错误 Oracle、理解完整性、决策准确性和无关信息负担。Synthetic kill rate 不能替代可读性与工程价值。

这一层必须包含普通后端开发者，而不能只找测试专家。至少给审阅者以下理解任务，不提前解释 skill 术语：

1. 用自己的话说明这次验证的对象、范围、输入、环境以及是否已经实际运行；
2. 逐个说明六类 Lens 在当前需求中检查了什么、当前覆盖状态为什么成立、由哪些场景支撑；
3. 复述至少一个重要通过和一个重要失败或缺口的上下文、触发顺序、预期、实际表现、判断理由和影响；
4. 用自己的话复述每个待决策问题，并说明不同选择会怎样改变用户结果、测试和实现；
5. 区分“已有测试跑通”“场景已设计但未运行”“测试环境当前无法触发”和“正确业务结果还没确定”；
6. 指出结论仍不能证明什么，以及批准后的下一步。

记录答错、漏读、误解因果、需要打开附件、回看术语表和跨表跳转的位置；同时记录主文档中与审批无关、让读者分散注意力的内容。只问“你觉得清楚吗”不能作为可用性证据。

## 2. 隔离角色与信息边界

```text
Benchmark Builder
  └─ requirement + correct implementation + hidden fault inventory

Test Designer
  └─ 只看 requirement + public repo + skill

Adversarial Implementer
  └─ 根据 contract 和测试寻找现实 survivor；不得测试探测或 hardcode case

Judge
  └─ 运行 correct + mutants，检查 Oracle、fidelity、kill matrix 和报告
```

四阶段使用不同上下文；支持时可使用不同模型。至少把 public case 与 hidden ground truth 放在不同目录/输入包，启动 Designer 时只复制 public artifacts 到隔离临时工作区。

同一上下文无法真正隐藏答案。本模式允许这种执行作为 smoke test，但报告必须标记 `CONTEXT_CONTAMINATED`，不能计作 independent holdout evidence。

## 3. Benchmark Case Contract

每题冻结：

```text
case_id
capability_under_test
public_requirement
public_repo_or_harness
correct_behavior
hidden_plausible_faults
fault_priority
forbidden_oracle_claims
minimum_verification_fidelity
judge_method
```

Fault 必须现实可信，不允许通过检测测试环境、读取测试源码、hardcode case 或随机崩溃作弊。

## 4. 先冻结 Ground Truth，再运行 Designer

顺序不可反转：

1. 写 public requirement；
2. 写 correct implementation；
3. 写 hidden mutants 与判定理由；
4. 冻结 rubric；
5. 才运行 baseline 和 skill；
6. 把场景转成 executable tests/evals；
7. 运行 correct 和所有 mutants；
8. 分析 survivor cluster，再决定是否修改 skill。

若看过输出后才新增“它恰好命中的 fault”，该 fault 只能进入下一轮 development set，不能回填本轮得分。

## 5. 对照组

至少比较：

- `baseline`：同一模型、同一 public input，不加载 skill；
- `skill`：相同模型/config/input，加载当前 skill；
- 可选 `previous-skill`：上一版本；
- 可选 `ablation`：移除 verification fidelity、critic 或 agent route 中某一机制。

保持模型、reasoning、repo snapshot 和执行预算一致。若无法真正运行独立 baseline，明确报告为“handwritten/simple baseline”，不要冒充模型 A/B 实验。

## 6. 核心观测

不要压成一个神秘总分。至少分别报告：

- **Correct Acceptance**：正确实现是否被错误拒绝；
- **P0/P1 Fault Kill Matrix**：每个 fault 被哪个 executable witness 杀死；
- **Survivors**：仍能通过所有测试的错误实现及其 fault cluster；
- **Oracle Errors**：Expected 与冻结 contract 冲突；
- **UNKNOWN Calibration**：该未知时是否暴露、不该未知时是否逃避；
- **Verification Fidelity**：是否 mock 掉 target property，是否出现 pseudo-killer；
- **Determinism**：并发/时间 witness 是否可重复；
- **Scenario Efficiency**：场景数、重复度和每个场景的 fault/obligation mapping；
- **Diagnostic Value**：失败能否定位到行为或 failure boundary；
- **Human Review Burden**：理解结论时遇到的无关内容、重复内容、术语障碍和附件跳转；篇幅与耗时只记录，不作为自动优劣指标。
- **Lens Coverage Comprehension**：普通开发者能否说明每个 Lens 在当前需求中检查了什么、状态为什么成立、由哪些场景支撑；
- **Decision Comprehension**：能否不用测试专家解释，复述现实问题、推荐方案和分支后果；
- **Evidence-layer Independence**：不读技术证据文件时，主文档结论是否仍完整且不误导；
- **Terminology Burden**：完成审批必须先查多少个内部术语或追踪多少次 `B/R/T/V/M` 映射。
- **Finding Comprehension**：不打开原始轨迹或评分 JSON，普通开发者能否复述一个重要通过/失败发生在什么上下文、预期是什么、Skill 与 baseline 分别怎么做、为何得到这个判断。

AI agent case 还报告 task/trial/grader coverage、safety violations、outcome/trajectory blind spot、trial isolation 和 grader calibration。

这些观测不能只输出计数。Self-eval 报告必须把每个会改变 Skill 判断的失败、部分通过、条件差异和评分分歧写成完整证据故事。至少包含：自然任务背景、逐轮触发、预期行为、Skill 实际行为、baseline 实际行为、判断理由、用户或工程影响、关键原文。没有观察到差异的维度也要用一个真实场景说明双方实际上如何通过，不能只写“2/2 PASS”。

如果多个 trial 表现一致，可以选择一个代表样本完整展开，再报告其余重复是否相同。若重复中出现不同表现，必须分别说明差异，不能用平均值覆盖。原始文件链接只用于核查，正文必须独立表达事实。

## 7. Dataset Split 与抗过拟合

推荐维护：

- Development：允许查看并用于改 skill；
- Validation：选择设计时使用，但不逐题写规则；
- Holdout：调优过程不可读取，只在里程碑运行；
- External replay：开源历史 Bug 和团队真实事故。

同一 fault family 的语法换皮不算独立 holdout。定期加入新的领域、语言和 failure mechanism。修改 skill 后同时跑旧 regression 和未见 holdout，防止只是背答案。

## 8. Failure-driven Skill Update

只在出现可复现 failure cluster 时修改 skill：

```text
Observed failures
  ↓
Shared missing decision mechanism
  ↓
Smallest general correction
  ↓
Development regression + untouched validation
```

不要因为单个 case 增加专用规则。若 correction 只是把 benchmark 答案写进 prompt，拒绝该修改。

## 9. Self-eval Verdict

使用：

- `SMOKE_PASS`：污染上下文中的 synthetic pilot 未发现结构性失败；
- `DEVELOPMENT_GAPS_FOUND`：发现可复现 survivor/Oracle/fidelity cluster；
- `VALIDATION_PASS`：隔离 validation 相对 baseline 有可执行改进且无明显回归；
- `HOLDOUT_PASS`：未见 holdout 达到预先冻结 gate；
- `REAL_WORLD_EVIDENCE`：历史 Bug 或团队项目回放有效；
- `INCONCLUSIVE`：harness、grader、样本或隔离不足。

`SMOKE_PASS` 不能升级表述为“skill 已证明有效”。报告必须写清模型、skill commit、case split、上下文隔离、执行命令、mutant 结果和剩余盲区。

## 10. 单份真实产物的回归审核

当用户提供一份“不好读”的真实产物并要求升级 skill 时，在完整独立 A/B 之外，先做可复现的 development regression：

1. 冻结原产物的事实结论，不靠删风险换取短小；
2. 用新版输出契约重写主文档；
3. 对照核查所有原 P0/P1 风险、未知业务规则和测不出来的场景是否仍可找到；
4. 确认六类 Lens 状态与技术证据一致，尤其不能把 paper scenario 当作“已覆盖”；
5. 核对主文档没有把理解结论所需的信息移到附件，也没有用内部术语、重复映射或无关方法论增加负担；行数、字数和阅读耗时只能作为描述性数据，不能用于自动判定好坏；
6. 执行上面的完整理解任务；没有独立工程师时标记 `CONTEXT_CONTAMINATED`，只能给 development-level 结论；
7. 让未读原始轨迹的读者只看报告，复述至少一个重要失败和一个重要通过的“验证对象—Lens 依据—上下文—预期—实际—判断—影响”，并说明待决策事项和未证明风险；若只能重复“发现一次缺口”等摘要，回归仍然失败。

回归审核必须报告事实保留情况和仍然失败的理解任务。不能因为新版更短就自动判定通过。
