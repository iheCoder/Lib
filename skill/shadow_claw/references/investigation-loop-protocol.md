# Investigation Loop Protocol

This document defines the detailed protocol for the Shadow Claw investigation loop,
including state transitions, world expansion rules, and convergence criteria.

---

## 1. State Machine

The investigation follows a strict state machine:

```
INTAKE -> OBSERVING -> HYPOTHESIZING -> VALIDATING -> DECIDING
                                                        |
                              +-----------+-------------+
                              |           |             |
                          EXPANDING   CONVERGED     NEW_ROUND
                              |           |             |
                              v           v             v
                          OBSERVING   RESOLVING    HYPOTHESIZING
                                        |
                                        v
                                      DONE
```

### State Definitions

| State | Description |
|-------|-------------|
| INTAKE | Extracting entities, time window, symptoms from user input |
| OBSERVING | Evidence Builder is collecting data (via Capability Pool) |
| HYPOTHESIZING | Generating candidate hypotheses from current evidence |
| VALIDATING | Hypothesis Validator is running 4-layer checks |
| DECIDING | Evaluating validation results to determine next action |
| EXPANDING | Evidence Builder is collecting data from new worlds (via Capability Pool) |
| CONVERGED | Investigation complete, generating report |
| RESOLVING | Generating resolution plan using Capability Pool resolution entries |
| NEW_ROUND | Starting a new round with updated evidence/hypotheses |
| DONE | Resolution plan delivered, investigation complete |

---

## 2. World Expansion Protocol

### 2.1 When to Expand

World expansion is triggered when:

1. Any hypothesis returns `decision: "insufficient_evidence"`
2. Multiple hypotheses are `retain_as_candidate` and cannot be distinguished
3. Evidence completeness metadata shows significant gaps

### 2.2 Expansion Priority — Discriminative Observation Design

**不要按固定优先级堆证据，而是设计最能区分竞争假设的观测。**

排查的目标不是"多看几个世界"，而是回答三个问题：

1. **H1 对不对？** — 这个世界的证据能直接证实或证伪 H1 吗？
2. **H2 和 H1 谁更合理？** — 这个世界的证据能让两个假设的分数产生差异吗？
3. **哪个观测最能一锤子把它们分开？** — 信息增益最大的世界是哪个？

#### 核心原则

> 每一次世界扩展都是一次实验。好的实验不是堆数据，而是设计一个**能区分 competing explanations 的观测**。

#### 信息增益排序算法

对每个候选世界，计算它对存活假设集的**区分力（Discriminative Power）**：

```
输入:
  surviving_hypotheses: 所有 retain/insufficient 的假设
  covered_worlds: 已经扩展过的世界集合
  all_candidate_worlds: 所有可到达的世界 - covered_worlds

对每个候选世界 W:
  distinguishing_power = 0
  diagnostic_value = 0

  对每对存活假设 (Hi, Hj):
    // 如果 Hi 预测 W 中会看到 X，但 Hj 预测不会 → 这个世界能区分它们
    if W 在 Hi.next_worlds_to_query 中 XOR W 在 Hj.next_worlds_to_query 中:
      distinguishing_power += 1
    // 即使两个假设都需要 W，它们对 W 中证据的预测可能不同
    elif Hi 对 W 的预期与 Hj 对 W 的预期矛盾:
      distinguishing_power += 0.5

  // 某些世界对特定校验层有直接价值
  for layer in [temporal, magnitude, scope]:
    if 某个假设在 layer 得到 insufficient_evidence 且 W 能补充该 layer 的数据:
      diagnostic_value += 1

  W.score = distinguishing_power * 2 + diagnostic_value

按 score 降序排列，取 top-3
```

#### 设计判别性观测（Discriminative Observation）

选定世界后，不是简单"去看看有什么"，而是**带着预测去看**：

```
对每个待扩展的世界 W:
  观测设计 = {
    "world": W,
    "what_to_look_for": [
      {
        "if_H1_true": "应该看到 X（例：deploy 记录在异常前 2min 内）",
        "if_H2_true": "应该看到 Y（例：无 deploy 记录，但有上游抖动）",
        "if_neither": "看到 Z（例：两者都没有 → 需要新假设）",
        "observation_type": "binary_discriminator | magnitude_check | absence_check"
      }
    ],
    "success_criteria": "如果看到 X 则 H1 +0.3，H2 -0.2；如果看到 Y 则反之"
  }
```

这样 Evidence Builder 采集时就有明确的**观测目标**，而不是漫无目的地扫描。

#### 回退规则：当无法计算信息增益时

如果当前只有 1 个存活假设（没有竞争），或假设的 `next_worlds_to_query` 为空，
**不要回退到固定的世界列表**，而是：

1. **回到操作边界**：第一处未确认边界是什么？优先扩展那里
2. **回到故障族**：当前故障族的关键视角还有哪些世界没覆盖？
3. **回到盲区**：有没有进不去但应该看的世界？向用户求助

| Fallback Priority | 策略 | 说明 |
|-------------------|------|------|
| F0 | 假设明确要求的世界 | 假设的 `next_worlds_to_query` 直接指定 |
| F1 | 操作边界的下一跳 | 推进边界确认 |
| F2 | 当前故障族的关键视角 | 网络族→物理链路，变更族→时间线，资源族→消费者 |
| F3 | 盲区世界（向用户求助） | 不要在可见世界里硬凑答案 |

#### 扩展请求格式（增强版）

```json
{
  "mode": "expand",
  "worlds_to_query": ["deploy", "scaling"],
  "entities": ["app-service", "db-instance-01"],
  "time_window": {
    "start": "2024-03-15T10:00:00Z",
    "end": "2024-03-15T11:30:00Z"
  },
  "context": "H2 needs deploy/scaling data to validate magnitude",
  "discriminative_design": {
    "deploy": {
      "if_H1_true": "应在 10:20-10:25 之间有发版记录",
      "if_H2_true": "该窗口内无发版，仅有上游错误率上升",
      "observation_goal": "区分 H1(发版触发) 和 H2(上游传导)"
    },
    "scaling": {
      "if_H1_true": "Pod 数从 N 变为 M，且 M*单任务负载 ≈ 观测负载",
      "if_H2_true": "Pod 数无变化",
      "observation_goal": "验证 H1 的量级是否成立"
    }
  }
}
```

### 2.3 Expansion Rules

1. **Never expand all worlds at once** - prioritize by relevance to surviving hypotheses
2. **Maximum 3 worlds per expansion round** - avoid information overload
3. **Track what has been expanded** - never re-expand the same world
4. **New evidence is incremental** - append to existing pack, don't replace

### 2.4 Expansion Request Format

```json
{
  "mode": "expand",
  "worlds_to_query": ["deploy", "scaling"],
  "entities": ["app-service", "db-instance-01"],
  "time_window": {
    "start": "2024-03-15T10:00:00Z",
    "end": "2024-03-15T11:30:00Z"
  },
  "context": "H2 needs deploy/scaling data to validate magnitude"
}
```

---

## 3. Hypothesis Lifecycle

Each hypothesis goes through a lifecycle:

```
PROPOSED -> VALIDATING -> REJECTED | RETAINED | PROMOTED
                              |
                          INSUFFICIENT -> [wait for world expansion] -> REVALIDATING
```

### 3.1 Hypothesis ID Convention

- Format: `h{round}-{sequence}` (e.g., `h1-1`, `h1-2`, `h2-3`)
- IDs are immutable once assigned
- New hypotheses in later rounds get new IDs

### 3.2 Hypothesis Survival Rules

1. **REJECTED** hypotheses are never re-validated (but their rejection is recorded)
2. **RETAINED** hypotheses are re-validated in the next round with new evidence
3. **INSUFFICIENT** hypotheses wait for world expansion, then re-validate
4. **PROMOTED** hypotheses trigger convergence

### 3.3 New Hypothesis Generation

When new evidence arrives, the engine should:

1. Check if new evidence suggests hypotheses not yet considered
2. Check if new evidence merges/refines existing retained hypotheses
3. Generate at most 2 new hypotheses per round (avoid hypothesis explosion)

---

## 4. Convergence Criteria

### 4.1 Normal Convergence (promote_to_primary)

Requirements:
- composite_score >= 0.7
- No other hypothesis with score within 0.15 of the top
- At least 2 layers (temporal + one other) with fit >= 0.6

### 4.2 Moderate Confidence Convergence

When the top hypothesis has:
- composite_score >= 0.6 but < 0.7
- OR another hypothesis within 0.15 score distance

Action: Converge but explicitly label "moderate confidence" and include
the competing hypothesis in the report.

### 4.3 Timeout Convergence (max_rounds_reached)

When round >= max_rounds:
- Report the best available hypothesis with its current score
- List all evidence gaps that prevented higher confidence
- Recommend specific manual investigation steps

### 4.4 Dead-End Convergence (all_rejected_no_new)

When all hypotheses are rejected and no new ones can be generated:
- This usually means the investigation needs a fundamentally different approach
- Recommend expanding to worlds not yet considered
- Suggest bringing in domain expertise for hypothesis generation

---

## 5. Investigation Report Schema

```json
{
  "investigation_id": "inv-2024-0315-001",
  "symptom": "...",
  "rounds_executed": 2,
  "convergence_type": "promote_to_primary",

  "conclusion": {
    "primary_cause": {
      "hypothesis_id": "h1-2",
      "hypothesis": "...",
      "confidence": 0.88,
      "composite_score": 0.883,
      "key_evidence": ["ev-01", "ev-02", "ev-03"],
      "explanation": "..."
    },
    "contributing_factors": [
      { "hypothesis": "...", "contribution": "marginal" }
    ],
    "rejected_causes": [
      { "hypothesis": "...", "reason": "...", "score": 0.225 }
    ],
    "remaining_uncertainty": ["..."]
  },

  "recommended_actions": [
    {
      "action": "...",
      "priority": "P0|P1|P2",
      "reasoning": "...",
      "type": "fix|investigate|monitor"
    }
  ],

  "timeline": [
    { "time": "ISO8601", "event": "...", "evidence_id": "ev-XX" }
  ],

  "evidence_summary": {
    "total_evidence": 10,
    "worlds_covered": ["log", "trace", "metric", "deploy", "scaling", "job"],
    "worlds_not_covered": ["config", "network"],
    "critical_gaps": ["..."]
  },

  "investigation_log": [
    {
      "round": 1,
      "phase": "OBSERVE",
      "action": "...",
      "result_summary": "...",
      "decision": "..."
    }
  ]
}
```

---

## 6. Anti-Patterns

### 6.1 Premature Convergence

**Symptom**: Promoting a hypothesis in round 1 without expanding any worlds.

**Why it is wrong**: Initial evidence is almost always incomplete. The first
plausible explanation is often wrong.

**Rule**: Never promote in round 1 unless composite_score >= 0.85 AND
all 4 validation layers have score >= 0.7.

### 6.2 Hypothesis Fixation

**Symptom**: Only generating hypotheses in the same layer (e.g., all code-level).

**Why it is wrong**: Complex incidents often have cross-layer causes.

**Rule**: Hypotheses must span at least 3 different layers in the first round.

### 6.3 Evidence Hoarding

**Symptom**: Expanding all worlds at once instead of targeting specific gaps.

**Why it is wrong**: Too much evidence at once makes it hard to determine
which new evidence changed the picture.

**Rule**: Maximum 3 worlds per expansion round.

### 6.4 Overconfidence

**Symptom**: Reporting 0.95 confidence when key worlds haven't been checked.

**Why it is wrong**: You can't be 95% confident if you haven't checked deploy history.

**Rule**: Confidence cannot exceed (worlds_covered / total_relevant_worlds).

### 6.5 Ignoring Absence

**Symptom**: Not generating absence facts (e.g., "upstream has no record").

**Why it is wrong**: Absence of evidence is often the most diagnostic evidence.

**Rule**: Evidence Builder must check for absence in every data source.

---

## 7. Integration with Real Data Sources

When Shadow Claw operates on real systems, world expansion goes through
the Capability Pool (see `capability-pool.md`):

When a world **cannot be entered** (no tool, no permission), record this as
an absence fact. Don't silently ignore it.

---

## 8. Resolution Protocol

After convergence (Phase 5), the system enters Phase 6 (Resolution).

### 8.1 Resolution Plan

Generate a 4-tier plan in **markdown format** (not JSON — it's for humans):

| Tier | Purpose | Timeframe |
|------|---------|-----------|
| Immediate | Stop the bleeding | Minutes |
| Short-term | Fix root cause | Hours to days |
| Long-term | Prevent recurrence | Weeks to months |
| Monitoring | Verify fix + alert on recurrence | Ongoing |

### 8.2 Execution by Side Effect Level

Not "never execute" — decide based on side effects:

| Side Effect | Action |
|-------------|--------|
| None (read-only, analysis, create branch) | Execute directly |
| Low (local config, new branch + commit) | Execute directly |
| Medium (scale, rate-limit) | Ask user, then execute |
| High (rollback, restart) | Ask user, then execute |
| Critical (production DB/config) | Only suggest, user executes |

### 8.3 After Resolution

Write valuable findings to project-cognition for future reference.

