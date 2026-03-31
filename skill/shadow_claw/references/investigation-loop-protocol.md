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

### 2.2 Expansion Priority

When multiple worlds need expansion, prioritize by information gain:

| Priority | World | Why |
|----------|-------|-----|
| P0 | deploy | Most common cause of sudden changes |
| P0 | scaling | Directly affects magnitude calculations |
| P1 | job | Can explain concurrent load amplification |
| P1 | config | Can explain sudden behavior changes |
| P2 | upstream | Needed when dependency issues suspected |
| P2 | network | Needed when transport issues suspected |
| P3 | metrics_baseline | Needed for magnitude validation |
| P3 | code | Needed for code-level hypotheses |
| P4 | environment | Rarely the primary cause, check last |

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

## 7. Integration with Real Data Sources via Capability Pool

When Shadow Claw operates on real systems, world expansion goes through
the Capability Pool (see `capability-pool.md`):

```
Validator says: "need deploy world"
  → Shadow Claw queries Capability Pool
  → Pool returns: git-recent-commits (for any env), kubectl-events (for k8s)
  → Shadow Claw picks best match for current environment
  → Evidence Builder invokes the capability
  → Raw data → structured evidence
```

The Capability Pool decouples "what world to enter" from "how to enter it":

| World | Example Capabilities | Notes |
|-------|---------------------|-------|
| log | sls-trace-query (Aliyun), local-log-grep (any) | Environment-dependent |
| trace | sls-trace-query (Aliyun) | `aliyun-sls-trace` skill |
| metric | prometheus-query | PromQL range queries |
| deploy | git-recent-commits, kubectl-events | Git for code changes, K8s for deploy events |
| scaling | kubectl-pod-history, kubectl-hpa | Kubernetes specific |
| config | sls-config-resolve, config-diff | Environment-dependent |
| code | git-diff, git-recent-commits | Available everywhere with Git |
| network | network-stats | Linux specific (ss, netstat) |
| database | mysql-readonly-query | Database specific |

When a world has **no registered capability** in the current environment,
this is itself an evidence item (`capability_gap`) — the system explicitly
records "I cannot see into this world" rather than silently ignoring it.

See `capability-pool.md` for the full registry schema and how to register
new capabilities.

---

## 8. Resolution Protocol

After convergence (Phase 5), the system enters Phase 6 (Resolution).

### 8.1 Resolution Plan Generation

Based on the promoted hypothesis, generate a 4-tier plan:

| Tier | Purpose | Timeframe | Example |
|------|---------|-----------|---------|
| immediate | Stop the bleeding | Minutes | Scale down concurrent tasks |
| short_term | Fix root cause | Hours-Days | Add distributed lock |
| long_term | Prevent recurrence | Weeks-Months | Redesign startup sequence |
| monitoring | Verify fix + alert on recurrence | Ongoing | Add concurrency alert |

### 8.2 Resolution Capability Lookup

For automatable actions, query the Capability Pool for `resolution` type capabilities:

```
Plan: "rollback deployment"
  → Pool lookup: resolution, action=rollback
  → Found: kubectl-rollback
  → Generate command: kubectl rollout undo deployment/app-service -n production
  → Mark: requires_confirmation=true, risk=high
```

### 8.3 Safety Rules

1. **Never auto-execute** — Only generate the plan, human decides
2. **Risk labels required** — Every action gets low/medium/high
3. **Rollback plan required** — Every action needs a corresponding undo
4. **Dry-run first** — If capability supports it, suggest dry-run before real execution
5. **Scope awareness** — Resolution scope must match the diagnosed scope

