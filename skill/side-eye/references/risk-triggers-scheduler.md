# Scheduler Blind-Spot Trigger Pack

只有 Implementation Facts 涉及 cron、RRULE、scheduler、自然语言调度、next-run、timezone 或任务重排时才读取本文件。这里关注“结构化配置合法但执行语义错误”的跨层盲点，不重复普通 parser 校验或通用时间边界检查。

## 1. Human Time Intent != Executable Schedule

**Blind spot**：自然语言或 UI 表达的业务时间，在 timezone、DST、月底、工作日、节假日或 calendar policy 的 canonicalization 中改变含义。

**Retrieve / falsify**：追原始意图、澄清状态、canonical schedule、timezone 来源和至少数个真实 next occurrences；用 DST 切换、短月和 locale 边界做 round-trip，而不是只验证 cron/RRULE 可解析。

## 2. Partial Modification Erases Unmentioned Intent

**Blind spot**：用户只修改频率或时间，但多轮 state merge、DTO default 或 replace-style persistence 把未提及字段清空/重置。

**Retrieve / falsify**：区分 missing、zero、null 和 explicit clear；比较修改前快照、conversation state、tool args 与持久结果；验证未修改字段和原始 calendar semantics 保持不变。

## 3. Persisted Schedule != Runtime Execution Truth

**Blind spot**：DB 已更新且响应成功，但 scheduler memory、queue、timer wheel 或派生 execution entry 仍使用旧计划；或者运行时更新成功但持久状态未提交。

**Concrete path**：9 点任务在 DB 改为 10 点 → Agent 返回成功 → scheduler 未 reconcile → 次日仍于 9 点执行。

**Retrieve / falsify**：追 persistence commit、reload/reconcile、旧 entry 取消、新 entry 注册、next-run readback 和最终执行观察点。不要把 tool success 当成环境完成。

## 4. Reschedule Boundary Creates Duplicate or Missing Runs

**Blind spot**：修改、暂停、恢复或删除与即将触发的旧 run 并发，旧/新 schedule 对边界时间各自做出正确决定，组合后却重复或漏执行。

**Retrieve / falsify**：任务 identity、generation/version、claim 时点、old timer cancellation、in-flight run policy、misfire/catch-up semantics 和 dedup boundary；构造“旧 run 已 claim 但尚未执行时修改”的确定性场景。

## 5. Clock / Misfire Policy Changes Business Meaning

**Blind spot**：进程暂停、时钟跳变、长时间离线或 scheduler failover 后，基础设施默认的 skip/catch-up/coalesce 行为与业务 requirement 不同。

**Retrieve / falsify**：明确 missed occurrence 的业务规则；检查 wall clock/monotonic clock、misfire grace、catch-up 上限、coalescing、restart/failover 与重复保护；验证长停机后的首个实际 occurrence。

## 6. Task Identity Resolves Differently Across Conversation and Execution

**Blind spot**：对话中“那个日报任务”解析为一个稳定对象，但执行前名称、owner、tenant 或候选集合变化，最终修改了另一个 task。

**Retrieve / falsify**：从候选查询、澄清和 approval 一直追到 tool args；确认使用不可漂移的 canonical ID 与版本，并在 mutation 时重新校验 owner/tenant 和被批准内容。

## Scheduler Pack 输出约束

不要仅因出现 cron 就罗列 DST、时区和重复执行。只有 requirement 的输入、状态、持久化、runtime scheduler 或实际 next-run 链能触达时才创建 hypothesis；Finding 必须给出一个具体时间、状态和执行顺序案例。
