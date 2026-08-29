# Trigger Gain Evaluation Protocol

这个协议回答一个窄问题：某条 Trigger 是否真的补足强模型盲点，还是只重复模型本来就会做的事。

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
