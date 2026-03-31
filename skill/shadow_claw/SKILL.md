---
name: shadow-claw
description: |
  Shadow Claw Investigation Engine (影爪调查引擎).
  面向复杂系统故障的自主调查循环。当用户报告故障、异常、性能问题，或需要进行根因分析时激活。
  不是一次性问答，而是驱动 观测->假设->验证->扩展世界 的闭环调查，直到找到真正的根因或明确的问题方向。
  协调 evidence-builder 和 hypothesis-validator 两个子 skill 完成调查。
---

# Shadow Claw — 影爪调查引擎

> **不是"帮你看看日志"，而是像高级工程师一样驱动一轮完整的事故调查。**

## 核心哲学

Shadow Claw 的价值不是猜得准，而是 **知道自己哪里不确定，并主动去消除不确定性**。

> 没有 Validator，系统会在一个错误世界里越分析越自信。
> 有 Validator，它会不断跳出错误世界。

## 定位

Shadow Claw 是整个调查系统的 **编排器和循环驱动器**。

它不亲自做事实提取（那是 Evidence Builder 的事），
也不亲自做假设审判（那是 Hypothesis Validator 的事），
它做的是 **驱动调查循环，直到收敛**。

### 三个角色，一个循环

```
  Evidence Builder  -->  Hypothesis Engine  -->  Hypothesis Validator
      (事实层)              (猜测层)                 (审判层)
        ^                                              |
        |          解释不成立 -> 扩展世界                |
        +----------------------------------------------+
```

| 角色 | 决定什么 |
|------|---------|
| Evidence Builder | 你 **看到了什么** |
| Hypothesis Engine | 你 **猜是什么** |
| Hypothesis Validator | 你 **该去看什么** |

> **Evidence Builder 不会主动扩展世界。是 Hypothesis Validator 在"逼它去扩展世界"。**

### 第四个角色：Capability Pool（能力池）

每个"世界"是抽象的（trace / deploy / scaling / network / config / code / ...），
但进入世界需要具体的工具。**Capability Pool 就是世界入口的注册表。**

```
Validator: "证据不足，需要进入 trace 世界"
     |
     v
Shadow Claw 查 Capability Pool: "谁能进入 trace 世界？"
     |
     v
Pool 返回: aliyun-sls-trace skill
     |
     v
Evidence Builder 用 aliyun-sls-trace 采集，转为结构化证据
```

能力分两类：

| 类型 | 用途 | 例子 |
|------|------|------|
| **observation** | 进入世界，采集证据 | aliyun-sls-trace, kubectl, prometheus, git log |
| **resolution** | 修复问题，执行处置 | kubectl rollback, config revert, service restart |

详见 [references/capability-pool.md](references/capability-pool.md)。

## 何时使用

- 用户报告系统故障、性能异常、请求超时等
- 需要从症状出发，找到根因或至少确定问题方向
- 问题复杂到不能一眼看出答案（多源、跨层、表象模糊）
- 需要排除多个可能性，而不是凭直觉给结论

---

## 调查循环：完整流程

### 总览

```
输入(症状) -> 循环 { 采集 -> 假设 -> 验证 -> [扩展世界] } -> 输出(报告)
```

最多执行 **5 轮**循环（可配置），每轮世界都会变大，认知都会升级。

---

### Phase 0: 接收症状 (Intake)

**输入**: 用户的工单、告警、口头描述、截图等任何形式的症状。

**执行**:

1. 从症状中提取关键信息:
   - **实体**: 服务名、接口路径、Pod名、DB名、request_id、trace_id
   - **时间窗**: 首次出现时间、持续时间、频率
   - **环境**: 集群、命名空间、区域
   - **症状描述**: 用户/系统看到的现象（不是根因）

2. 创建调查上下文 (Investigation Context):

```json
{
  "investigation_id": "inv-2024-0315-001",
  "symptom": "DB CPU 从 50% 跳到 100%",
  "entities": ["app-service", "db-instance-01"],
  "time_window": { "start": "2024-03-15T10:00:00Z", "end": "2024-03-15T11:30:00Z" },
  "round": 0,
  "max_rounds": 5,
  "status": "active",
  "current_world_coverage": [],
  "hypotheses": [],
  "evidence_pack": null,
  "investigation_log": []
}
```

---

### Phase 1: 初始观测 (Observe)

**调用 Evidence Builder** 进行初始采集。

第一轮时，采集所有 **直接可见** 的数据源:

```json
{
  "mode": "initial",
  "entities": ["..."],
  "time_window": {"start": "...", "end": "..."},
  "sources": ["log", "trace", "metric", "ticket"]
}
```

Evidence Builder 返回初始 Evidence Pack。

**关键检查**:

- 是否提取了时间事实?
- 是否检测了缺失事实?
- 是否标注了观测可信度?
- 是否有 world_summaries?（初始轮可能没有 deploy/scaling/job）

记录:

```
[Round 1] 初始采集完成: 6 条证据, 覆盖世界: [log, trace, metric]
[Round 1] 未覆盖: [deploy, scaling, job, config]
```

---

### Phase 2: 假设生成 (Hypothesize)

**这一步由 Shadow Claw 自己完成（LLM 推理）。**

基于当前证据包，生成候选假设。

#### 规则:

1. **至少生成 3 个假设**，涵盖不同层级
2. 每个假设必须有结构化格式
3. **不允许只有一个假设** — 这意味着你还没真正思考
4. 假设应覆盖不同层级的可能性（不要全猜在同一层）
5. 每个假设必须能被证伪（否则不是假设，是废话）

#### 假设格式:

```json
{
  "id": "h1",
  "text": "具体假设描述",
  "source": "log_analysis|metric_anomaly|code_review|engineer_intuition|pattern_match",
  "proposed_cause_time": "ISO8601",
  "proposed_cause_end_time": "ISO8601 or null",
  "proposed_scope": "影响范围描述",
  "proposed_scope_level": "L1-L5",
  "proposed_magnitude": "预期影响量级",
  "tags": ["关键词"],
  "reasoning": "为什么提出这个假设（一句话）"
}
```

#### 层级覆盖清单:

| 层级 | 典型假设方向 |
|------|------------|
| 代码层 | 代码变更/SQL退化/逻辑错误 |
| 配置层 | 配置切换/限流变更/缓存开关 |
| 部署层 | 发版/扩缩容/灰度 |
| 依赖层 | 上游抖动/DB过载/缓存击穿 |
| 基础设施层 | 网络/DNS/资源竞争 |
| 观测层 | 时钟偏移/trace失真/日志丢失 |

---

### Phase 3: 假设验证 (Validate)

**调用 Hypothesis Validator** 对每个假设进行四层校验。

可以用脚本做规则+统计校验:

```bash
python3 hypothesis_validator/scripts/validate_hypothesis.py --input evidence.json --pretty
```

然后由 LLM 补充反事实校验（第 4 层）。

#### 验证后，对每个假设得到:

```json
{
  "hypothesis_id": "h1",
  "temporal_fit": 0.35,
  "magnitude_fit": 0.10,
  "scope_fit": 0.20,
  "counterfactual_fit": 0.29,
  "composite_score": 0.225,
  "decision": "reject_as_primary_cause",
  "next_worlds_to_query": ["deploy", "scaling"]
}
```

---

### Phase 4: 决策 (Decide)

根据验证结果，做出下一步决策。

#### 决策树:

```
有 promote_to_primary?
  YES -> Phase 5 (收敛), score < 0.8 时标注 "moderate confidence"
  NO  -> 继续判断
    有 insufficient_evidence?
      YES -> 收集 next_worlds_to_query -> Phase 4a (扩展世界)
      NO  -> 继续判断
        有 retain_as_candidate?
          YES -> 需要更多证据区分 -> Phase 4a (扩展世界)
          NO  -> 所有假设都被 reject
            已达 max_rounds?
              YES -> Phase 5 (带不确定性收敛)
              NO  -> 生成新假设 -> 回到 Phase 2
```

#### Phase 4a: 扩展世界

**这是整个系统最关键的一步。**

收集所有假设的 `next_worlds_to_query`，去重后 **查询 Capability Pool** 找到对应的能力:

```
1. next_worlds_to_query = ["deploy", "scaling", "trace"]
2. 查 Capability Pool:
     deploy  → git-recent-commits, kubectl-events
     scaling → kubectl-pod-history, kubectl-hpa
     trace   → aliyun-sls-trace (sls-trace-query)
3. 调用 Evidence Builder，传入世界 + 对应能力
```

当某个世界 **没有注册能力** 时，生成一条能力缺口证据:

```json
{
  "id": "ev-gap-01",
  "source_type": "capability_gap",
  "fact_text": "无法进入 network 世界: 当前环境没有注册 network 类型的能力",
  "tags": ["absence", "trust_warning"]
}
```

Evidence Builder 返回新增证据，**增量追加** 到 Evidence Pack。

然后:

1. 基于新证据，可能生成新假设（回到 Phase 2）
2. 对所有存活假设重新验证（回到 Phase 3）
3. round++

---

### Phase 5: 收敛与报告 (Converge)

调查结束条件:

1. **有假设被 promote_to_primary** — 正常收敛
2. **达到 max_rounds** — 超时收敛
3. **所有假设被 reject 且无法生成新假设** — 死胡同收敛

#### 输出: 调查报告

报告必须包含以下全部部分:

**1. 一句话结论** — 用一句话概括根因。

**2. 关键证据链** — 按时间排列的因果链，每环节引用具体证据。

**3. 被推翻的假设** — 每个被推翻的假设 + 推翻理由（这证明调查是严谨的）。

**4. 剩余不确定性** — 明确说还有什么不确定的。

**5. 解决方案 / 排查方向** — 至少给出可操作的下一步。

**6. 事件时间线** — 关键事件按时间排列。

**7. 调查日志** — 完整的调查轮次记录。

---

### Phase 6: 处置建议 (Resolve)

**找到问题不等于解决问题。这一步把诊断结论转化为可执行的处置方案。**

当 Phase 5 收敛后（convergence_type = `promote_to_primary` 或 `max_rounds_reached`），
进入处置阶段。

#### 6.1 处置方案生成

基于确认的根因（或最优假设），生成分层处置方案:

```json
{
  "resolution_plan": {
    "immediate_actions": [
      {
        "action": "限制 full-sync-task 并发数为 1",
        "type": "mitigation",
        "priority": "P0",
        "reasoning": "立即降低 DB 负载，阻止 CPU 继续 100%",
        "capability": "kubectl-scale",
        "risk": "medium",
        "requires_confirmation": true
      }
    ],
    "short_term_fixes": [
      {
        "action": "为 full-sync-task 增加分布式锁，确保全局只有 1 个实例运行",
        "type": "fix",
        "priority": "P1",
        "reasoning": "防止扩容时多 Pod 同时触发全量任务"
      }
    ],
    "long_term_improvements": [
      {
        "action": "review 发版扩容流程，新 Pod 启动时不自动触发全量任务",
        "type": "prevention",
        "priority": "P2",
        "reasoning": "从流程上杜绝同类问题"
      }
    ],
    "monitoring_actions": [
      {
        "action": "为 full-sync-task 并发数添加告警（阈值 > 2）",
        "type": "monitor",
        "priority": "P1"
      }
    ]
  }
}
```

#### 6.2 处置方案的层级

| 层级 | 含义 | 时间框架 |
|------|------|---------|
| **immediate** | 止血 — 现在就做，阻止影响扩大 | 分钟级 |
| **short_term** | 修复 — 解决根因 | 小时到天 |
| **long_term** | 预防 — 从流程/架构上杜绝 | 周到月 |
| **monitoring** | 观测 — 确保修复有效 + 预警复发 | 持续 |

#### 6.3 处置能力查找

对于可自动化的处置动作，查询 Capability Pool 中的 resolution 类能力:

```
处置方案: "回滚到上一版本"
  → 查 Capability Pool: resolution 类, action=rollback
  → 找到: kubectl-rollback
  → 生成执行命令: kubectl rollout undo deployment/app-service -n production
  → 标记: requires_confirmation=true, risk_level=high
```

#### 6.4 安全约束

1. **永远不自动执行处置** — 只生成方案，由人决定是否执行
2. **标注风险等级** — 每个动作都标 low/medium/high
3. **提供回滚方案** — 每个处置动作都要有对应的回滚方式
4. **如果不确定就说不确定** — 宁可只给方向，不给错误的具体命令

---

## 收敛类型与对应行为

| 收敛类型 | 含义 | 输出要求 |
|---------|------|---------|
| `promote_to_primary` | 找到高置信度主因 | 完整报告 + 解决方案 |
| `max_rounds_reached` | 循环次数用尽 | 当前最优解释 + 明确标注不确定性 + 建议人工排查方向 |
| `all_rejected_no_new` | 所有假设被推翻且无法生成新的 | 坦率说明当前证据不足以定位 + 建议扩大排查范围 |
| `evidence_unreliable` | 关键证据可信度太低 | 标注哪些证据不可信 + 建议先修复观测能力 |

---

## 调查日志 (Investigation Log)

每一步操作都必须记录，包含: 轮次(round)、阶段(phase)、具体操作、输出摘要、决策理由。

示例:

```
[Round 0] INTAKE: 收到症状 "DB CPU 100%", 提取实体 [app-service, db-instance-01]
[Round 1] OBSERVE: 初始采集 6 条证据, 覆盖 [log, trace, metric]
[Round 1] HYPOTHESIZE: 生成 4 个假设 [H1-H4]
[Round 1] VALIDATE: H1=reject(0.23), H2=retain(0.65), H3=retain(0.40), H4=insufficient
[Round 1] DECIDE: 需要扩展世界 [deploy, scaling, job, config]
[Round 2] OBSERVE: 扩展采集 4 条新证据, 覆盖新增 [deploy, scaling, job]
[Round 2] HYPOTHESIZE: 基于新证据调整假设, 保留 H2/H3, 新增 H5
[Round 2] VALIDATE: H2=promote(0.88), H3=reject(0.35), H5=retain(0.50)
[Round 2] DECIDE: H2 晋升为主因, 调查收敛
[Round 2] CONVERGE: 输出报告
```

---

## 安全约束

### 绝对不做

1. **不在证据不足时给唯一结论** — 宁可说"我还不确定"
2. **不跳过假设生成直接给答案** — 必须有竞争假设
3. **不忽略被推翻的假设** — 推翻理由是报告的重要组成部分
4. **不无限循环** — 最多 5 轮，超时就带不确定性输出
5. **不忽略 Validator 的 insufficient_evidence** — 必须去扩展世界

### 始终做到

1. **每轮都记录调查日志**
2. **每个假设都经过四层校验**
3. **缺失事实必须显式标注**
4. **报告中包含推翻过程** — 这比最终结论还重要
5. **报告中包含剩余不确定性**
6. **报告中包含可操作的解决方案或排查方向**

---

## 与子 Skill 的调用关系

```
shadow-claw (编排器)
  |
  +-- capability-pool (能力池)
  |     +-- observation 类: aliyun-sls-trace, kubectl, git, prometheus, ...
  |     +-- resolution 类: kubectl-rollback, kubectl-scale, config-revert, ...
  |     +-- 世界 -> 能力映射表
  |
  +-- evidence-builder (事实层)
  |     +-- 初始采集: Phase 1 (用 capability-pool 中的 observation 能力)
  |     +-- 扩展采集: Phase 4a (由 Validator 触发, 通过 capability-pool 查找入口)
  |
  +-- [hypothesis generation] (猜测层, shadow-claw 自身 LLM 推理)
  |     +-- Phase 2
  |
  +-- hypothesis-validator (审判层)
  |     +-- 四层校验: Phase 3
  |     +-- 输出 next_worlds_to_query -> 驱动世界扩展
  |
  +-- [resolution advisor] (处置层, shadow-claw 自身 LLM 推理)
        +-- Phase 6: 基于确认根因生成处置方案
        +-- 查 capability-pool 中的 resolution 能力
        +-- 永远不自动执行, 只建议
```

---

## 参考文档

- Evidence Builder: [../evidence_builder/SKILL.md](../evidence_builder/SKILL.md)
- Hypothesis Validator: [../hypothesis_validator/SKILL.md](../hypothesis_validator/SKILL.md)
- 能力池规范: [references/capability-pool.md](references/capability-pool.md)
- 调查循环协议: [references/investigation-loop-protocol.md](references/investigation-loop-protocol.md)
- 系统设计理念: [ref_im.md](ref_im.md)
