# Agent Blind-Spot Trigger Pack

只有 Implementation Facts 涉及 LLM、agent、tool call、session、memory、approval、delegation、planner 或 structured tool workflow 时才读取本文件。每条 Trigger 只创建待验证 hypothesis；先过适用性门槛，再沿 Agent state、tool trajectory 和真实环境状态取证。

在沿当前需求审查依赖模型的关键步骤时，先核对模型实际收到什么，再判断它的输出和程序处理：

- 从请求组装代码确认实际提示词、动态输入、历史消息、Schema 或工具说明，以及会影响该步骤的模型配置。有运行记录时对照实际请求；模板存在规则不等于该次调用收到规则，审查者从其他文件获知的信息也不等于模型已知。
- 对照需求核对完成该判断所需的信息和规则：是否缺失、含糊、互相冲突，或在筛选、摘要、截断、修订与纠正时发生变化。只追与关键判断有关的内容，不对所有提示词做风格改写。
- 有失败记录时区分输入缺失、指令冲突和已提供规则但输出未遵守。后者不能单凭一次失败归因于模型能力；“换模型更可靠”或“改提示会降低错误率”需要对应比较，静态发现的问题仍可由明确的缺失或冲突成立。
- 将输出继续追到实际转换、校验和调用方。检查纠正是否覆盖该错误、是否收到修正所需的信息；没有自动纠正是否有问题由需求决定。

按需用局部验证消除会改变结论的不确定性。无需为完成静态审查强制调用真实模型；没有调用或对照数据时，不声称测得了模型可靠性或提示词效果。已有代码或模拟验证可证明确定性处理，不能证明实际模型输出频率。

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
