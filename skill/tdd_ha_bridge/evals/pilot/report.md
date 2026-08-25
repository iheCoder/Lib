# Pilot Self-evaluation Report

## Verdict

```text
Verdict: SMOKE_PASS
Run date: 2026-08-25
Isolation: CONTEXT_CONTAMINATED
Baseline: handwritten/simple baseline, not an independent no-skill model run
Claim ceiling: 协议可执行，且本 development pilot 未发现明显结构性缺口；尚不能证明独立泛化
```

## What Was Actually Run

Public requirements、frozen ground truth、skill-guided plans 与 executable candidates 分开保存。Judge 对每组同时运行正确实现和 plausible faulty implementations。

```bash
env GOCACHE=/tmp/tdd-ha-bridge-go-cache go test -race -v ./...
env GOCACHE=/tmp/tdd-ha-bridge-go-cache go test -count=50 ./...
env GOCACHE=/tmp/tdd-ha-bridge-go-cache go vet ./...
```

Result：全部 `PASS`，包括 deterministic concurrency test 的 race detector 与 50 次重复执行。

## Executable Results

| Case | Capability | Correct accepted | Baseline kills | Skill-guided kills |
| --- | --- | ---: | ---: | ---: |
| P1 | missing vs explicit zero + neighbors | 1/1 | 1/4 | 4/4 |
| P2 | decision surface / independent influence | 1/1 | 0/4 | 4/4 |
| P3 | controlled concurrent interleaving | 1/1 | 0/1 | 1/1 |
| P4 | Agent task/trial/tool/state grader | 1/1 | 0/4 | 4/4 |
| **Total** |  | **4/4** | **1/13** | **13/13** |

这不是“skill 相对无 skill 提升 12 个”的因果结论：baseline 是为了校验 harness 区分度而写的最小常见检查，不是同模型、同预算、隔离上下文的 A/B 输出。

## Fault Kill Matrix

| Fault | Executable witness that rejected it |
| --- | --- |
| P1/F1 explicit zero treated as missing | T1 explicit `0` |
| P1/F2 missing treated as zero | T2 omitted field |
| P1/F3 every value overwritten by default | T3 normal non-zero neighbor |
| P1/F4 negative value accepted | T4 validation boundary |
| P2/F1 suspended state ignored | T3 suspended owner |
| P2/F2 `!suspended` widened to top-level OR | T5 owner-independent denial |
| P2/F3 feature flag incorrectly gates Admin | T1 Admin with flag disabled |
| P2/F4 suspension incorrectly denies Admin | T1 Admin with suspended state |
| P3/F1 lost update lets both reservations succeed | first-round read barrier + two workers |
| P4/F1 ticket prompt injection bypasses approval | A3 untrusted instruction + tool/state grader |
| P4/F2 agent reports success without ledger effect | A1 state/tool grader |
| P4/F3 timeout retry creates duplicate refund | A4 exact requestID + ledger cardinality |
| P4/F4 every fifth trial bypasses policy | A2 repeated trials + binary safety gate |

No seeded mutant survived. Every executable suite also accepted its correct candidate, so this pilot observed no false Oracle rejection.

## Manual Rubric Results

The non-executable cases test whether the skill refuses false certainty and selects the right evidence level:

| Case | Frozen gate | Result | Evidence in plan |
| --- | --- | --- | --- |
| P5 | expose commit outcome ambiguity; keep idempotency; reject ordinary mock | PASS | `BLOCKED_BY_ORACLE_AMBIGUITY`, explicit UNKNOWNs and pseudo-killer |
| P6 | preserve legacy observation without declaring business truth | PASS | `CHARACTERIZATION / CODE+TEST+CALLER`, allows refactor |
| P7 | do not infer billion-row rollout safety from small functional tests | PASS | compatibility, lock/lag/resources, resume, representative benchmark, canary/rollback |

Manual judgments are not blind in this run and therefore do not count as independent scoring.

## Fidelity and Determinism Audit

- P1 uses the real Go `encoding/json` presence behavior instead of a pre-normalized mock.
- P2 observes the public allow/deny decision and does not inspect candidate implementations.
- P3 injects a barrier exactly between read and conditional write; it uses no sleep or probabilistic stress loop.
- P4 checks tool calls, ledger state, idempotency key and multiple clean deterministic trials; final text is baseline-only evidence.
- P4 validates the eval design against simulated Agent faults. It does **not** estimate a real LLM's violation probability, confidence interval, prompt-distribution robustness, or grader/model coupling.

## What This Pilot Found About the Skill

The skill's strongest observable contribution is not case count. It changes the verification mechanism:

1. semantic presence is tested through the real decoder;
2. boolean combinations are reduced to independent decision witnesses;
3. concurrency uses a controlled interleaving;
4. Agent success is grounded in tools and durable state across trials;
5. ambiguous transaction semantics and production-scale rollout claims remain explicitly unproven.

No general failure cluster appeared in this contaminated development set, so adding more case-specific prompt rules would be overfitting. The self-evaluation protocol itself is the justified skill update from this exercise.

## Remaining Evidence Needed

To advance beyond `SMOKE_PASS`:

1. run the same public cases in fresh isolated contexts as same-model/no-skill vs same-model/skill A/B;
2. freeze a validation set whose hidden faults are not visible while the skill is edited;
3. replay historical pre-fix/post-fix bugs without revealing the fix diff to Designer;
4. run P4 against real stochastic agents, calibrated graders and repeated independent trials;
5. add real DB commit-ambiguity fault injection and representative migration rehearsal;
6. blind-review both outputs for key-risk recall, Oracle errors, review time and decision burden.

Only successful isolated validation supports `VALIDATION_PASS`; unseen holdout and historical/real-project replay are required for stronger claims.
