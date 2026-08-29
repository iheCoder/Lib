# Side Eye V2 Requirement-Scoped Dry-Run Report

## Evidence label

`CONTEXT_CONTAMINATED DEVELOPMENT SMOKE`

本报告由实现 V2 的同一上下文执行，作者知道方案目标，也能看到 case 集。它只能证明当前指令能够被一致地应用到这些 synthetic cases，不能证明 Trigger 对独立 frontier model 的因果增益，也不能替代 same-model/same-budget no-trigger A/B、隔离 grader 或 holdout。

## Procedure

对 `requirement-scoped-cases.md` 的 5 个 case 分别执行一次 V2 路由：建立 Requirement Contract 和 Implementation Slice；只从 seeds 向必要依赖扩展；完成 Breadth Scan；按需加载 Core/Agent/Scheduler pack；合并 hypothesis；形成具体 failure trace；寻找 case 内 counter-evidence；检查 causality、scenario group 和 Current Relevance。最后检查四个 negative controls 是否被误报。

## Results

| Case | Expected blind spot | Observed route | User-facing classification | Result |
|---|---|---|---|---|
| 1. Ordinary patch | Replace-style update clears unmentioned preferences | Breadth Scan 的 core behavior/regression 直接发现；没有强行套 Core Trigger | `正常业务路径`，`[P1 · 当前适用]` | PASS |
| 2. Repair | Old snapshot overwrites a newer quota update | Core 8 `Multiple Truths` 与 Core 11 `Repair` 合并为“fresh quota must not be overwritten” | `多实例 / 分布式前提`，`[P1 · 当前适用]` | PASS |
| 3. Agent + scheduler | DB success is not runtime scheduling success | Agent B 与 Scheduler 3 合并；slice 追到 scheduler next-run，而非停在 tool result | `Agent 多轮 / retry / recovery`，`[P1 · 当前适用]` | PASS |
| 4. Approval | Approval binds session, not canonical action | Agent C 激活；以 task identity/order drift 构造 approval TOCTOU trace | `Agent 多轮 / retry / recovery`，`[P0 · 条件适用]` | PASS |
| 5. Backfill | v1 writer and backfill produce semantically stale `status_v2` | Core 6 与 Core 7 合并；因 rollout 前提未证明，输出 operational risk/open assumption | `历史数据 / 发布环境`，`[P1 · 待确认]` | PASS |

### Representative developer-facing Finding

```text
## Agent 多轮 / retry / recovery

### [P1 · 当前适用] 修改成功后 scheduler 仍执行旧时间 — scheduler.go

问题：任务表已经更新为 10 点，但常驻 scheduler 仍持有启动时加载的 9 点 entry。

具体场景：用户把每天 9 点改为 10 点。Tool 更新 DB 后立即返回“修改成功”，但成功路径没有 reload/reconcile；第二天 scheduler 仍会在 9 点触发旧 entry。

证据：`Tool.Execute` 只调用 `UpdateSchedule`；`Scheduler.Start` 只在进程启动时构建 entries，当前 slice 中没有更新或替换 runtime entry 的路径。

怎么验证：启动 9 点任务 → 不重启 scheduler → 修改为 10 点 → 读取 runtime next-run，并确认 9 点不再触发。
```

## Scope and noise observations

- Case 1 由基础 Breadth Scan 处理，没有为普通 replace/update bug 发明新 Trigger。
- 5 个 case 都能从 requirement + seeds 开始；没有把 working tree 或整个 repository 当 scope。
- Case 2、3、4、5 的多个 Trigger 都按 broken invariant 合并，没有机械输出重复 Finding。
- 条件性问题分别使用 `当前适用 / 条件适用 / 待确认`，没有只按 P0/P1 堆叠。
- 四个 negative controls 均应被 applicability gate 拒绝；本轮没有生成 fencing、compatibility、scale 或 Agent 泛化告警。

## Gaps and next evidence

- 本轮不是独立模型运行，不能测量 Trigger 的真实 recall 增益或 false-positive delta。
- 没有真实 repository、编译、并发 barrier、migration engine 或 scheduler runtime，因此只验证 review route 与报告可读性，不验证 synthetic code 的可执行性。
- 下一层证据应按方案执行 per-trigger same-model/same-budget A/B，并把 hidden defect 与 reviewer context 隔离；低增益 Trigger 应删除。

## Verdict

`SMOKE_PASS / CONTEXT_CONTAMINATED`
