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

由工程师盲审 baseline 与 skill 输出，比较关键风险发现率、错误 Oracle、审查耗时和决策负担。Synthetic kill rate 不能替代可读性与工程价值。

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
- **Human Review Cost**：关键信息发现时间与真正需要裁决的问题数量。

AI agent case 还报告 task/trial/grader coverage、safety violations、outcome/trajectory blind spot、trial isolation 和 grader calibration。

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
