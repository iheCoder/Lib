# Behavior Discovery Development Regression

## Verdict

```text
Pre-change verdict: DEVELOPMENT_GAPS_FOUND (real artifact omitted the candidate obligation)
Post-change verdict: SMOKE_PASS
Run date: 2026-08-29
Isolation: CONTEXT_CONTAMINATED
Old baseline: one real pre-change artifact + handwritten narrow executable checks
New output: same-context skill-guided plan, not an independent model run
Claim ceiling: new abstraction can express and execute the missing checks without observed negative-control expansion; independent generalization remains unproven
```

## Old vs New on the Real Incident

旧产物 `test-design-cron-builtin-natural-language-update.md` 从任务 Resolve、参数形成、确认和 Update 开始建立闭环。它覆盖了任务配置字段保留和特定试运行上下文中的正确工具过滤，但没有提出：一个仍未完成的用户目标跨轮继续时，完成它所依赖的非目标条件是否应保持、重新建立或明确退出。它也没有定位最终 `tool-not-found` 之前的最早不可达状态。

修改后计划 D1 在不知道具体 root cause 的公开输入上新增：

- 一个 `Candidate Obligation / UNKNOWN`，而不是擅自声明运行状态必须保持；
- “保持”与“允许变化”两个分支的产品后果；
- 从最终失败反向定位跨轮状态变化后的最早不可达点；
- 观察状态变化、恢复/退出行为和真实目标结果的 witness，而不是猜某个字段或工具名。

这说明新版更直接地修复了原产物的搜索空间缺口。不过执行者已看过事故分析，因此这里只是 development regression，不能当作 blind historical replay。

## Generalization and Negative Control

| Case | Same abstraction found | Oracle handling | Over-analysis check | Executable witness |
| --- | --- | --- | --- | --- |
| D1 cross-turn workflow | 非目标变化破坏完成条件 | Candidate / UNKNOWN | 未要求固定工具架构 | correct accepted; mutant killed |
| D2 context compression | 表示变化丢失仍有效限制 | Confirmed policy | 只记录关键压缩 transition | correct accepted; mutant killed |
| D3 replanning | 路径变化绕过审批 | Confirmed policy | 允许任意合法计划形状 | correct accepted; mutant killed |
| D4 stateless control | 无跨状态候选 | 不制造 UNKNOWN | 无 turn/state matrix | ordinary calculation checks pass |
| D5 deterministic PATCH | 局部更新覆盖非目标字段 | Confirmed API | 使用普通 Software Route | correct accepted; mutant killed |

这些 case 分别涉及会话、上下文表示、控制流和普通确定性 API；共同机制是“局部变化破坏仍有效条件”，不是工具生命周期。

## What Was Executed

```bash
GOCACHE=/tmp/tdd-ha-bridge-discovery-go-cache go test -race -v ./...
GOCACHE=/tmp/tdd-ha-bridge-discovery-go-cache go test -count=50 ./...
GOCACHE=/tmp/tdd-ha-bridge-discovery-go-cache go vet ./...
```

结果全部通过，包括 race detector、50 次重复和 vet。可执行 harness 对四个正向 case 同时运行正确实现与 plausible mutant。窄检查只观察局部成功，因此接受所有 mutant；Discovery-guided 检查同时观察被修改目标和仍有效条件，接受所有正确实现并拒绝全部四个 mutant。D4 使用普通输入分区，没有构造跨状态装置。

## Evidence Limits

- 没有运行隔离的同模型、同预算 old-skill/new-skill A/B；当前进程知道方案和 hidden fault。
- 窄 baseline 是为了证明 harness 区分度而手写的，不代表旧 skill 在 D2/D3/D5 必然失败。
- D1 的真实旧产物提供了直接前后差异证据，但新版计划仍是同上下文生成。
- executable mutant 证明新增 witness 有区分力，不证明未来每次调用 skill 都会生成它。
- 下一证据层应在未见上下文中只给 `cases.md`，分别加载 git 中旧版和新版 skill，由独立 Judge 按冻结的 `ground-truth.md` 评分。
