# Agent Blind-Spot Trigger Pack

只有 Implementation Facts 涉及 LLM、agent、tool call、session、memory、approval、delegation、planner 或 structured tool workflow 时才读取本文件。每条 Trigger 只创建待验证 hypothesis；先过适用性门槛，再沿 Agent state、tool trajectory 和真实环境状态取证。

## A. Multi-Turn State / Capability Drift

**Blind spot**：跨轮重建、state reducer 或模型切换后，required facts、tool availability、authorization、approval 或 task state 悄悄丢失。

**Retrieve / falsify**：逐轮比较 canonical task state、tool registry、用户已补字段、权限和 reducer merge 规则；确认旧轮事实不会被空值、摘要或新 planner 覆盖。

## B. Completion Claim ≠ Environment Truth

**Blind spot**：LLM 最终回复“已完成”，但 tool、persistence、scheduler 或外部环境没有达到 requirement。

**Retrieve / falsify**：从 final response 反向追 tool result、持久状态、运行时 reconcile 和可观察 outcome。文本正确、tool 返回 success 或结构化输出合法都不能单独证明完成。

## C. Approval Drift / Approval TOCTOU

**Blind spot**：用户批准的 action 与真正执行的 action 在 approval 后因重新解析、状态变化或 replanning 而不同。

**Concrete path**：用户批准 `Task A → 10:00` → 参数被重新解析或 task state 更新 → planner 选择 `Task B → 10:00` → 系统沿用旧 approval 执行。

**Retrieve / falsify**：approval 是否绑定 canonical action、resource identity、关键参数和版本；执行前是否重新校验；任何变化是否强制重新批准。

## D. Checkpoint / Environment Skew

**Blind spot**：Agent checkpoint 与外部环境恢复到不同时间点，导致重复 tool call 或永久漏执行。

**Retrieve / falsify**：checkpoint 的提交时点、tool outcome token、environment transaction、resume 决策和 reconciliation。分别模拟“环境已完成/checkpoint 未完成”和“checkpoint 已完成/环境回滚”。

## E. Untrusted Observation → Instruction / Authority

**Blind spot**：web page、email、document、search result 或 tool response 等不可信数据，被提升为 control instruction 并跨越 privileged tool boundary。

**Retrieve / falsify**：追 `untrusted data → agent reasoning/state → privileged tool args`；确认数据/指令分离、authority 来源、allowlist、approval 和输出编码，而不是只贴“prompt injection”标签。

## F. Delegation Loses Constraints

**Blind spot**：parent 的 tenant、scope、policy、Must Never 或 approval requirement 没有完整传给 sub-agent，而 sub-agent 拥有更强工具。

**Retrieve / falsify**：比较 parent contract、delegation payload、sub-agent tool authority 和执行前 gate；验证约束是否显式、不可被摘要丢失并由执行端强制。

## G. Partial / Stale Observation Treated as Complete Truth

**Blind spot**：pagination、partial query、tool truncation、eventual consistency、stale cache 或 search lag 被错误解释成全集或“不存在”。

**Retrieve / falsify**：检查 completion/page token、result truncation metadata、freshness、index lag、fallback query 和 negative conclusion 的证据门槛。

## H. Long-Horizon Goal / Policy Drift

**Blind spot**：长对话、恢复、memory 或 subtask 后，原始 requirement、Must Never、approval requirement 和 scope 逐渐消失。

**Retrieve / falsify**：在每个 checkpoint、summary、handoff 和 replanning 边界比较 invariant；确认高优先级用户限制来自 canonical state，而不是依赖模型记忆。

## I. Canonicalization Changes Meaning

**Blind spot**：自然语言转结构化参数后格式完全合法，但时间、timezone、currency、identity、resource name、SKU、address 或 schedule semantics 已改变。

**Concrete path**：用户说“每月最后一个工作日” → parser 生成 `day=31` → schema validation 通过 → scheduler 在短月不执行或执行错误日期。

**Retrieve / falsify**：保存原始意图与 canonical representation 的可比映射；检查 locale/timezone、歧义澄清、round-trip/display confirmation 和实际执行样例。

## J. Tool Composition Creates New Capability

**Blind spot**：单个 tool 各自安全，但组合后形成新的高风险能力，例如 `read_secret + http_request` 导致 exfiltration，或 `read untrusted email + send email` 形成间接注入链。

**Retrieve / falsify**：按完整 tool chain 分析可达数据和 authority；确认 taint、egress policy、目的地限制、approval 与 least privilege 是否在组合边界生效，而不是只审单个 tool schema。

## Agent Pack 输出约束

不要因为存在 LLM 就泛泛报告 hallucination、prompt injection 或“不确定性”。Finding 必须绑定 requirement 的真实 trajectory、环境状态和可复现故障链。多个 Agent Trigger 指向同一 invariant 时遵循 Core Library 的 Hypothesis Merge。
