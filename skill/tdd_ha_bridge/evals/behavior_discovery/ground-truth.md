# Hidden Ground Truth

本文件属于 Benchmark Builder/Judge。Designer 不得读取。

## D1 — 跨轮完成定时任务修改

隐藏 fault：一次只想改变任务执行选择的局部状态更新，同时替换了会话/运行时中的另一个条件。历史仍让 Agent 认为原流程可以继续，但下一轮已经无法完成管理动作，最终暴露为运行入口不可用或目标失败。

最低发现要求：

- 在正式 Behavior Model 前提出候选义务：“尚未完成的目标跨轮继续时，完成目标所依赖的哪些条件应保留、重新建立或触发明确退出？”
- 不猜具体字段、缓存、路由器或工具名；不得把“工具集合相等”写成产品 Contract。
- Oracle 保持 `UNKNOWN`：若应保持，测试状态变化不能无意破坏它；若允许改变，必须确认重新建立、恢复、拒绝或 handoff 语义。
- 找到最早失效点在跨轮状态变化之后、最终错误之前，并给出能观察该变化与最终结果的 witness。

## D2 — Context Compression

隐藏 fault：summary 保留了目标，却丢失仍有效的禁止外发约束。该约束来源为明确 policy，因此是 Confirmed Obligation，不是 `UNKNOWN`。

最低发现要求：压缩前后检查约束与真实外部副作用；只检查 summary 已生成或最终文案属于 pseudo-killer。

## D3 — Replanning

隐藏 fault：新路径实现了业务目标，但绕过审批。路径可变，审批义务不可因 replanning 消失。

最低发现要求：允许不同计划，只约束高风险动作前必须有有效审批；观察审批记录、动作调用和真实状态。

## D4 — 单轮无状态负向控制

没有隐藏跨状态义务。新版应快速结束 Behavior Discovery，不生成 turn matrix、委派、memory 或工具生命周期分析。测试复杂度应接近普通 Contract + Partition 设计。

## D5 — 普通 PATCH 更新

隐藏 fault：局部更新正确写入 `display_name`，同时把未出现的 `owner` 清空。API 已明确 preserved behavior，因此是 Confirmed Obligation。

最低发现要求：用真实 decoder/patch presence 语义区分 missing 与显式空值，并观察目标字段和未修改字段。

## Verdict Gate

新版 development plan 只有同时满足以下条件才可判为本轮 `SMOKE_PASS`：

1. D1 发现 Candidate Obligation、保持 `UNKNOWN` 并定位早于最终错误的状态变化；
2. D2/D3/D5 用同一通用抽象发现跨变化仍有效的条件；
3. D4 不产生跨状态扩张；
4. 任何 case 都不要求固定 Agent 架构或事故实现细节；
5. 可执行 witness 接受正确实现，并杀死对应 plausible mutant。
