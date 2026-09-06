# Trigger Gain Evaluation Protocol

这个协议比较审查指令的实际收益：新增内容是否补足模型盲点，还是只重复模型本来就会做的事。单条 Trigger 和整版技能分别按下面的方法比较。

## A/B design

同一 testcase 使用相同模型、reasoning effort、token/time budget、repository state、requirement、seed files 和 runtime context：

- `Run A — no-trigger`：使用 Side Eye 的 requirement scope、Implementation Slice、Breadth Scan、证据与输出规则，但移除被测 Trigger 的内容和名称；
- `Run B — trigger`：唯一变化是启用被测 Trigger；
- `Judge`：在不知道 A/B 身份的隔离上下文中，对照 hidden ground truth 评估，不把预期 Finding 文案当成唯一答案。

每个 case 至少记录：

- defect recall，尤其 P0/P1 recall；
- false positives 和错误严重度；
- 是否追到了必要的跨文件/跨状态证据；
- Finding 是否通过 causality 与 applicability gate；
- attention、token、tool call 和 wall-time cost；
- 输出是否让普通开发者理解具体场景和 Current Relevance。

不要用同一作者已看到 hidden defect 的结果证明因果增益。Development、validation 和 holdout 必须分开；修改 Trigger 后不得回头用 holdout 调 prompt。Oracle 无法确定时标为 `UNKNOWN`，不强行计入 hit/miss。

## 比较整版审查指令

修改公共审查步骤或 Agent 检查要求时，比较完整旧版与候选版；不要用单个 Trigger 的开关代替整版比较。每轮冻结技能文件与案例，审查者只接收本轮指定版本、相同的用户要求和必要材料，不接收事故答案、修复代码或其他轮结论。

- 开发案例可用于修改指令；最终验证案例须由未接收改动方案的独立人员或 Agent 准备，修改者定版前不读取其内容。合成案例与真实项目分别报告；模拟输出不能称为真实模型采样。已用来改指令的案例不能继续作为独立验证。
- 在同一审查模型、推理配置与可控制的预算下比较两版，进行多次独立运行并打散顺序。模型更换后重新比较并分别报告。若无法强制相同 token 或时间预算，记录实际限制，不声称严格同预算。
- 同时覆盖 Agent 和其他工程领域的真实缺陷、保护充分的实现及需求允许的失败停止；审查者不知道各类比例。分别报告漏报、误报和普通工程退步，不能只看总命中数。
- 判分者不知道匿名报告对应哪个版本，核查具体机制与影响，接受合理的不同表述；真值有遗漏或歧义时先审查题目，不把额外 Finding 自动判错。查看检索与验证记录解释失败，但读取指定文件或遵守步骤本身不计入发现成绩。
- 比较实际问题发现与证据范围，同时记录耗时、输出长度和可获得的调用用量。基线已经全部命中时，两版通过不能证明改善；单次命中差异只作为样本证据。为修复本轮退步而修改指令后，另用未读过的案例验证。

## Retention rule

- Baseline recall 已很高、Trigger 几乎不改变检索路线，却增加噪声或成本：删除或降级；
- Trigger 显著提高隐藏故障 recall，且 false positives 与成本可接受：保留；
- 结果只在同上下文 smoke 成立：`INCONCLUSIVE`，不得声称增益；
- 先按 failure mechanism 聚合，不允许靠重复表述同一 Finding 虚增 recall。

不要把 `95% → 96%` 或单个偶然命中包装成有意义增益；是否保留应结合样本量、P0/P1 权重、误报和检索成本。

## Blind-spot coverage matrix

每个条目至少需要一个 positive case 和一个能拒绝相邻理论风险的 negative case：

1. Hidden retry after successful side effect；
2. Environment / checkpoint mismatch；
3. Hidden multi-writer；
4. Cross-layer amplification；
5. Production data premise mismatch；
6. Live backfill overwriting fresh state；
7. v1/v2 mixed-version emergent bug；
8. Multiple truth divergence；
9. Lease expiry + missing fencing；
10. Ordering unit mismatch；
11. Reconcile destroys fresh state；
12. Feature-flag combination；
13. Agent capability drift；
14. Agent completion claim != environment truth；
15. Approval TOCTOU；
16. Agent recovery duplicate tool call；
17. Untrusted tool result → privileged instruction；
18. Sub-agent loses parent constraints；
19. Pagination/stale result treated as complete truth；
20. Canonicalization changes user intent；
21. Tool composition creates unexpected authority。

Required negative controls include：

- Redis 是单实例、可容忍陈旧的展示 cache：不得报告 fencing；
- API 从未发布、无 consumer、无历史数据：不得制造 backward compatibility；
- loop 固定 3 次：不得制造 scalability Finding；
- Agent 在成功回复前 read-back 验证真实环境：不得泛泛报告 completion mismatch。

## Current evidence status

`dry-run-report.md` 只完成 5 类 requirement-scoped、同上下文 development smoke：普通业务、分布式/数据一致性、两个 Agent 场景、release/migration。21 项矩阵与 no-trigger A/B 尚未独立执行，因此当前 Trigger gain verdict 是 `INCONCLUSIVE`。

2026-09-06 的[整版对照实验](agent-review-20260906/REPORT.md)另行比较原版与一份针对失败处理、结论证据范围和实际模型请求的候选版。已知事故没有写入候选指令或计分题。案例、报告、独立判分、原始版本与复跑程序均随报告保存；这不等同于上述 21 项逐条 Trigger 实验。

该轮共 17 个独立合成项目，包含 11 个 Agent 项目和 6 个普通工程项目；两版在 50 次逐项目判断中均命中全部已确认缺陷并正确放行保护案例，未证明候选增益，实验结束时初步决定保留原版运行指令。仅使用一种审查模型，且时间预算未被硬性约束；没有观察到本轮普通工程退步，不代表已证明跨领域无副作用。

2026-09-07 根据用户的采用决定，将同一候选版加入正式技能：已有真实漏审提供了补充检查的理由，改动范围明确，且本轮对照未观察到漏报、误报或普通工程退步，因此可以采用这项小改动。采用不以本轮必须拉开命中率差距为前提；提高处理概率是预期收益，尚未由本轮实验测得。原始成绩、案例和两个版本快照保持不变。
