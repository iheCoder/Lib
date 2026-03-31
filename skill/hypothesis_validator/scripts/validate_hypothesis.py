#!/usr/bin/env python3
"""
Hypothesis Validator — 规则 + 统计校验辅助脚本

执行时间校验、量级校验、范围校验的程序化部分。
反事实校验由 LLM 执行，不在此脚本范围内。

用法:
  python3 validate_hypothesis.py --input evidence.json
  python3 validate_hypothesis.py --input evidence.json --check temporal
  python3 validate_hypothesis.py --input evidence.json --check magnitude
  python3 validate_hypothesis.py --input evidence.json --check scope
  cat evidence.json | python3 validate_hypothesis.py --check all

输入: Evidence Pack JSON (见 references/evidence-pack-schema.md)
输出: 每个假设的校验结果 JSON
"""

import argparse
import json
import sys
from datetime import datetime, timezone, timedelta
from typing import Any

# ---------------------------------------------------------------------------
# Constants & Defaults
# ---------------------------------------------------------------------------

DEFAULT_MAX_LAG_SECONDS = 600          # 10 min
CHANGE_POINT_WINDOW_SECONDS = 300      # ±5 min

# Scope levels
SCOPE_LEVELS = {"L1": 1, "L2": 2, "L3": 3, "L4": 4, "L5": 5}

# Magnitude heuristics: hypothesis_type -> (min_pct, max_pct) expected CPU impact
MAGNITUDE_HEURISTICS = {
    "single_sql_degradation":   (1, 5),
    "small_code_diff":          (3, 8),
    "cache_miss":               (20, 50),
    "pod_scale_with_task":      (None, None),   # computed dynamically
    "config_switch":            (30, 100),
    "upstream_retry_storm":     (30, 80),
    "connection_pool_exhaust":  (5, 20),
}

# Composite score weights
W_TEMPORAL   = 0.25
W_MAGNITUDE  = 0.30
W_SCOPE      = 0.25
W_COUNTER    = 0.20


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def parse_ts(s: str | None) -> datetime | None:
    """Parse ISO‑8601 timestamp (with or without trailing Z)."""
    if not s:
        return None
    s = s.replace("Z", "+00:00")
    try:
        return datetime.fromisoformat(s)
    except ValueError:
        return None


def clamp(v: float, lo: float = 0.0, hi: float = 1.0) -> float:
    return max(lo, min(hi, v))


def seconds_between(a: datetime | None, b: datetime | None) -> float | None:
    if a is None or b is None:
        return None
    return (b - a).total_seconds()


# ---------------------------------------------------------------------------
# Layer 1 — Temporal Validation
# ---------------------------------------------------------------------------

def validate_temporal(hypothesis: dict, evidence: dict, config: dict) -> dict:
    """Rule‑based temporal checks."""
    result = {
        "check": "temporal",
        "hypothesis_id": hypothesis["id"],
        "temporal_fit": 0.5,          # start neutral
        "notes": [],
        "verdict": "ok",
    }

    cause_time = parse_ts(hypothesis.get("proposed_cause_time"))
    anomaly = evidence.get("anomaly_summary", {})
    effect_time = parse_ts(anomaly.get("start_time"))

    if cause_time is None or effect_time is None:
        result["temporal_fit"] = None
        result["verdict"] = "insufficient_evidence"
        result["notes"].append("缺少 cause_time 或 effect_start_time")
        result["missing"] = []
        if cause_time is None:
            result["missing"].append("proposed_cause_time")
        if effect_time is None:
            result["missing"].append("anomaly_summary.start_time")
        return result

    # R-T1: causal ordering
    if cause_time > effect_time:
        result["temporal_fit"] = 0.0
        result["verdict"] = "FAIL: cause after effect"
        result["notes"].append(
            f"原因时间({cause_time.isoformat()})晚于异常开始({effect_time.isoformat()})"
        )
        return result

    # R-T2: reasonable lag
    delta = seconds_between(cause_time, effect_time)
    max_lag = config.get("max_reasonable_lag_seconds", DEFAULT_MAX_LAG_SECONDS)
    if delta is not None:
        if delta > max_lag:
            result["temporal_fit"] -= 0.4
            result["notes"].append(
                f"时延({delta:.0f}s)超过合理上限({max_lag}s)"
            )
        elif delta <= 120:
            result["temporal_fit"] += 0.15
            result["notes"].append(f"时延({delta:.0f}s)非常接近，时间高度吻合")
        else:
            result["notes"].append(f"时延({delta:.0f}s)在合理范围内")

    # R-T3: change point proximity
    ws = evidence.get("world_summaries", {})
    change_point_str = None
    ms = ws.get("metrics_snapshot", {})
    for _metric_name, metric_data in ms.items():
        if isinstance(metric_data, dict) and "change_point" in metric_data:
            change_point_str = metric_data["change_point"]
            break

    change_point = parse_ts(change_point_str)
    if change_point:
        window = timedelta(seconds=CHANGE_POINT_WINDOW_SECONDS)
        nearby_events = _collect_nearby_events(ws, change_point, window)
        hypothesis_near = (
            cause_time is not None
            and abs((cause_time - change_point).total_seconds()) <= CHANGE_POINT_WINDOW_SECONDS
        )
        if nearby_events and not hypothesis_near:
            result["temporal_fit"] -= 0.3
            result["notes"].append(
                f"变化点({change_point.isoformat()})附近有其他事件"
                f"({', '.join(nearby_events)})，但当前假设不在其中"
            )
        elif hypothesis_near:
            result["temporal_fit"] += 0.2
            result["notes"].append("假设原因时间在变化点邻近窗口内")

    # R-T4: window overlap
    cause_end = parse_ts(hypothesis.get("proposed_cause_end_time"))
    effect_end = parse_ts(anomaly.get("end_time"))
    if cause_end and effect_end and cause_time and effect_time:
        overlap_start = max(cause_time, effect_time)
        overlap_end = min(cause_end, effect_end)
        overlap = max(0.0, (overlap_end - overlap_start).total_seconds())
        anomaly_dur = max(1.0, (effect_end - effect_time).total_seconds())
        overlap_ratio = overlap / anomaly_dur
        result["temporal_fit"] += 0.3 * overlap_ratio
        result["notes"].append(f"时间窗重叠率: {overlap_ratio:.2f}")

    result["temporal_fit"] = clamp(result["temporal_fit"])
    return result


def _collect_nearby_events(ws: dict, cp: datetime, window: timedelta) -> list[str]:
    """Collect event descriptions near a change point."""
    events = []

    for d in ws.get("deploy", {}).get("recent_deploys", []):
        t = parse_ts(d.get("time"))
        if t and abs((t - cp).total_seconds()) <= window.total_seconds():
            events.append(f"deploy@{t.isoformat()}")

    for s in ws.get("scaling", {}).get("events", []):
        t = parse_ts(s.get("time"))
        if t and abs((t - cp).total_seconds()) <= window.total_seconds():
            events.append(f"scaling@{t.isoformat()}")

    for j in ws.get("job", {}).get("tasks", []):
        t = parse_ts(j.get("trigger_time"))
        if t and abs((t - cp).total_seconds()) <= window.total_seconds():
            events.append(f"job@{t.isoformat()}")

    for c in ws.get("config", {}).get("changes", []):
        t = parse_ts(c.get("time"))
        if t and abs((t - cp).total_seconds()) <= window.total_seconds():
            events.append(f"config@{t.isoformat()}")

    return events


# ---------------------------------------------------------------------------
# Layer 2 — Magnitude Validation
# ---------------------------------------------------------------------------

def validate_magnitude(hypothesis: dict, evidence: dict, _config: dict) -> dict:
    """Rule‑based magnitude sanity checks."""
    result = {
        "check": "magnitude",
        "hypothesis_id": hypothesis["id"],
        "magnitude_fit": 0.5,
        "notes": [],
        "verdict": "ok",
    }

    ws = evidence.get("world_summaries", {})
    ms = ws.get("metrics_snapshot", {})

    # Try to find observed magnitude from metrics
    observed_ratio = None
    observed_abs = None
    for metric_name, md in ms.items():
        if isinstance(md, dict) and "baseline_mean" in md and "anomaly_mean" in md:
            baseline = md["baseline_mean"]
            anomaly_val = md["anomaly_mean"]
            if baseline and baseline > 0:
                observed_ratio = anomaly_val / baseline
                observed_abs = anomaly_val - baseline
                result["notes"].append(
                    f"指标'{metric_name}': baseline={baseline}, anomaly={anomaly_val}, "
                    f"ratio=×{observed_ratio:.2f}, delta={observed_abs:+.2f}"
                )
                break

    if observed_ratio is None:
        result["magnitude_fit"] = None
        result["verdict"] = "insufficient_evidence"
        result["notes"].append("缺少 metrics baseline/anomaly 数据")
        result["missing"] = ["metrics_baseline", "metrics_anomaly"]
        return result

    # Determine expected impact from hypothesis tags / magnitude field
    proposed_mag = hypothesis.get("proposed_magnitude", "")
    h_tags = hypothesis.get("tags", [])

    # Check for amplification factors (pod scaling) — only applied when
    # the hypothesis itself involves scaling/job/systemic causes
    amplification = 1.0
    hypothesis_involves_scaling = any(
        k in proposed_mag.lower() or k in " ".join(h_tags).lower()
        for k in ["scale", "pod", "replica", "job", "task", "全量", "扩容", "并发"]
    )

    scaling_events = ws.get("scaling", {}).get("events", [])
    for se in scaling_events:
        fr = se.get("from_replicas", 1)
        to = se.get("to_replicas", 1)
        if fr > 0 and to > fr:
            if hypothesis_involves_scaling:
                amplification *= (to / fr)
                result["notes"].append(f"放大因子(假设关联): Pod {fr}→{to} (×{to/fr:.1f})")
            else:
                result["notes"].append(
                    f"存在 Pod 扩容({fr}→{to})但当前假设不涉及扩容，放大因子不适用"
                )

    job_tasks = ws.get("job", {}).get("tasks", [])
    for jt in job_tasks:
        conc = jt.get("concurrency", 1)
        if conc > 1:
            result["notes"].append(f"并发任务: {jt.get('name','?')} concurrency={conc}")


    expected_max_pct = _estimate_expected_max(proposed_mag, h_tags, amplification)

    if expected_max_pct is not None and observed_abs is not None:
        # Compare expected vs observed (both as fractions where applicable)
        if expected_max_pct < abs(observed_abs) * 0.1:
            result["magnitude_fit"] = 0.10
            result["verdict"] = "量级严重不足"
            result["notes"].append(
                f"预期最大影响({expected_max_pct:.0%}) << 观测变化({observed_abs:+.0%})，差 ≥1 个数量级"
            )
        elif expected_max_pct < abs(observed_abs) * 0.3:
            result["magnitude_fit"] = 0.30
            result["verdict"] = "量级不足，最多为次要因子"
            result["notes"].append(
                f"预期最大影响({expected_max_pct:.0%}) < 观测变化({observed_abs:+.0%})"
            )
        elif expected_max_pct >= abs(observed_abs) * 0.5:
            result["magnitude_fit"] = max(0.60, min(0.90, expected_max_pct / max(abs(observed_abs), 0.01)))
            result["verdict"] = "量级可解释"
            result["notes"].append("预期影响与观测变化在同一数量级")
        else:
            result["magnitude_fit"] = 0.45
            result["notes"].append("量级部分匹配")
    elif expected_max_pct is None:
        result["notes"].append("无法从假设描述推断预期影响上界，需 LLM 辅助判断")

    result["magnitude_fit"] = clamp(result["magnitude_fit"]) if result["magnitude_fit"] is not None else None
    return result


def _estimate_expected_max(proposed_mag: str, tags: list, amplification: float) -> float | None:
    """Heuristic: estimate the max expected impact as a fraction."""
    pm = proposed_mag.lower()

    # Try to parse explicit percentage hints
    for hint, (lo, hi) in MAGNITUDE_HEURISTICS.items():
        kw = hint.replace("_", " ")
        # crude keyword match
        if any(k in pm or k in " ".join(tags) for k in kw.split()):
            if lo is not None and hi is not None:
                return (hi / 100.0) * amplification
            else:
                # dynamic: use amplification directly
                return amplification * 0.5  # base 50% per unit

    # Fallback: try to detect pattern like "cpu_increase_5pct"
    import re
    m = re.search(r"(\d+)\s*pct", pm)
    if m:
        return int(m.group(1)) / 100.0 * amplification

    return None


# ---------------------------------------------------------------------------
# Layer 3 — Scope Validation
# ---------------------------------------------------------------------------

def validate_scope(hypothesis: dict, evidence: dict, _config: dict) -> dict:
    """Rule‑based scope matching checks."""
    result = {
        "check": "scope",
        "hypothesis_id": hypothesis["id"],
        "scope_fit": 0.5,
        "notes": [],
        "verdict": "ok",
    }

    anomaly = evidence.get("anomaly_summary", {})
    affected = anomaly.get("affected_entities", [])
    num_affected = len(affected)

    # Infer phenomenon scope level
    if num_affected == 0:
        p_level = None
    elif num_affected == 1:
        p_level = 2   # single entity ≈ L2
    elif num_affected <= 3:
        p_level = 3
    else:
        p_level = 4

    # Get hypothesis scope level
    h_scope_str = hypothesis.get("proposed_scope_level", "")
    h_level = SCOPE_LEVELS.get(h_scope_str)

    if h_level is None:
        # Try to infer from proposed_scope text
        ps = hypothesis.get("proposed_scope", "").lower()
        if any(k in ps for k in ["single_request", "single request", "单请求", "单个"]):
            h_level = 1
        elif any(k in ps for k in ["single_pod", "single pod", "单机", "单 pod"]):
            h_level = 2
        elif any(k in ps for k in ["partial", "部分", "灰度"]):
            h_level = 3
        elif any(k in ps for k in ["fleet", "全量", "all pod", "全局", "systemic"]):
            h_level = 4
        elif any(k in ps for k in ["cross", "跨服务", "跨集群"]):
            h_level = 5

    if p_level is None or h_level is None:
        result["scope_fit"] = None
        result["verdict"] = "insufficient_evidence"
        result["notes"].append("无法确定现象范围或假设范围")
        result["missing"] = []
        if p_level is None:
            result["missing"].append("affected_entities")
        if h_level is None:
            result["missing"].append("proposed_scope_level")
        return result

    gap = p_level - h_level

    if gap > 1:
        # Phenomenon much wider than hypothesis
        result["scope_fit"] -= 0.4
        result["notes"].append(
            f"现象范围(L{p_level})比假设范围(L{h_level})广太多(差{gap}级)"
        )
    elif gap < -1:
        # Hypothesis wider than phenomenon
        result["scope_fit"] -= 0.2
        result["notes"].append(
            f"假设范围(L{h_level})比现象(L{p_level})广，需解释为何只局部影响"
        )
    else:
        result["scope_fit"] += 0.3
        result["notes"].append(
            f"范围匹配: 现象L{p_level}, 假设L{h_level}"
        )

    # Synchronization check
    evidence_items = evidence.get("evidence_items", [])
    simultaneous_count = _count_simultaneous_entities(evidence_items)
    if simultaneous_count > 2 and h_level <= 2:
        result["scope_fit"] -= 0.3
        result["notes"].append(
            f"多实体({simultaneous_count})同步异常，但假设是局部(L{h_level})原因"
        )
    elif simultaneous_count > 2 and h_level >= 4:
        result["scope_fit"] += 0.2
        result["notes"].append("多实体同步异常，与系统性假设匹配")

    result["scope_fit"] = clamp(result["scope_fit"])
    return result


def _count_simultaneous_entities(items: list[dict]) -> int:
    """Count distinct entities that appear in anomaly-tagged evidence."""
    entities = set()
    for item in items:
        tags = item.get("tags", [])
        if "anomaly" in tags or "support" in tags:
            for e in item.get("entity_refs", []):
                entities.add(e)
    return len(entities)


# ---------------------------------------------------------------------------
# Composite
# ---------------------------------------------------------------------------

def compute_composite(temporal: dict, magnitude: dict, scope: dict,
                      evidence: dict | None = None) -> dict:
    """Compute composite score and decision from layer results.

    If *evidence* is provided, apply observation reliability propagation:
      1. Weight each layer's fit by supporting evidence confidence.
      2. Cap the final score by world-coverage ratio (Anti-Pattern 6.4).
    """
    t = temporal.get("temporal_fit")
    m = magnitude.get("magnitude_fit")
    s = scope.get("scope_fit")

    # Any layer insufficient?
    if t is None or m is None or s is None:
        missing = []
        for layer_result in (temporal, magnitude, scope):
            missing.extend(layer_result.get("missing", []))
        return {
            "composite_score": None,
            "decision": "insufficient_evidence",
            "missing_for_completion": missing,
            "note": "至少一层校验缺乏足够数据，需要扩展世界",
        }

    # Hard reject: temporal impossible
    if t == 0.0:
        return {
            "composite_score": 0.0,
            "decision": "reject_as_primary_cause",
            "note": "时间约束硬否决：原因晚于结果",
        }

    # Hard reject: magnitude + scope both very low
    if m < 0.2 and s < 0.3:
        score = W_TEMPORAL * t + W_MAGNITUDE * m + W_SCOPE * s
        return {
            "composite_score": round(score, 3),
            "decision": "reject_as_primary_cause",
            "note": "量级+范围双不足",
        }

    # ---------------------------------------------------------------
    # Observation reliability propagation (when evidence is available)
    # ---------------------------------------------------------------
    reliability_note = None
    temporal_degraded = False

    if evidence is not None:
        ev_items = evidence.get("evidence_items", [])

        if ev_items:
            # Compute average evidence confidence
            confidences = [
                item.get("confidence", 0.5) for item in ev_items
                if isinstance(item.get("confidence"), (int, float))
            ]
            avg_confidence = sum(confidences) / len(confidences) if confidences else 0.5

            # Penalize if too many trust_warning items
            trust_warnings = sum(
                1 for item in ev_items
                if "trust_warning" in item.get("tags", [])
            )
            trust_warning_ratio = trust_warnings / len(ev_items) if ev_items else 0
            if trust_warning_ratio > 0.3:
                avg_confidence *= 0.7

            # Check if temporal reliability is degraded (clock skew etc.)
            temporal_trust_issues = any(
                "trust_warning" in item.get("tags", [])
                and any(
                    kw in item.get("fact_text", "").lower()
                    for kw in ["clock", "skew", "ntp", "时钟", "时间偏移"]
                )
                for item in ev_items
            )
            if temporal_trust_issues:
                t = min(t, 0.6)
                temporal_degraded = True

            # Weight layer fits by evidence reliability
            evidence_reliability = clamp(avg_confidence, 0.3, 1.0)
            t *= evidence_reliability
            m *= evidence_reliability
            s *= evidence_reliability

            reliability_note = (
                f"evidence_reliability={evidence_reliability:.2f} "
                f"(avg_confidence={avg_confidence:.2f}, "
                f"trust_warning_ratio={trust_warning_ratio:.0%})"
            )

    # General scoring (counterfactual placeholder = 0.5 neutral)
    c_placeholder = 0.5
    score = W_TEMPORAL * t + W_MAGNITUDE * m + W_SCOPE * s + W_COUNTER * c_placeholder
    score = round(score, 3)

    # ---------------------------------------------------------------
    # World-coverage confidence cap (Anti-Pattern 6.4 enforcement)
    # ---------------------------------------------------------------
    coverage_cap = None
    if evidence is not None:
        metadata = evidence.get("metadata", {})
        worlds_covered = metadata.get("worlds_covered", [])
        worlds_not_covered = metadata.get("worlds_not_covered", [])
        n_covered = len(worlds_covered)
        n_total = n_covered + len(worlds_not_covered)

        if n_total > 0:
            coverage_cap = round(n_covered / n_total, 3)
            if score > coverage_cap:
                score = coverage_cap

    if score >= 0.7:
        decision = "promote_to_primary"
    elif score >= 0.4:
        decision = "retain_as_candidate"
    else:
        decision = "reject_as_primary_cause"

    result = {
        "composite_score": round(score, 3),
        "composite_score_note": "反事实层使用占位分0.5，最终分数需LLM完成反事实校验后调整",
        "decision": decision,
    }

    if reliability_note:
        result["reliability_note"] = reliability_note
    if temporal_degraded:
        result["temporal_reliability"] = "degraded"
    if coverage_cap is not None:
        result["coverage_cap"] = coverage_cap
        if score <= coverage_cap:
            result["coverage_cap_note"] = (
                f"世界覆盖率上限: {coverage_cap} "
                f"(covered={n_covered}/{n_total})"
            )

    return result


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def run(evidence: dict, checks: list[str], config: dict) -> list[dict]:
    """Run requested checks for all hypotheses in the evidence pack."""
    hypotheses = evidence.get("hypotheses", [])
    if not hypotheses:
        return [{"error": "evidence pack 中没有 hypotheses"}]

    all_results = []
    for h in hypotheses:
        h_result: dict[str, Any] = {"hypothesis_id": h["id"], "hypothesis": h.get("text", "")}

        temporal = magnitude = scope = None

        if "temporal" in checks or "all" in checks:
            temporal = validate_temporal(h, evidence, config)
            h_result["temporal"] = temporal

        if "magnitude" in checks or "all" in checks:
            magnitude = validate_magnitude(h, evidence, config)
            h_result["magnitude"] = magnitude

        if "scope" in checks or "all" in checks:
            scope = validate_scope(h, evidence, config)
            h_result["scope"] = scope

        if "all" in checks and temporal and magnitude and scope:
            h_result["composite"] = compute_composite(temporal, magnitude, scope, evidence)

        all_results.append(h_result)

    return all_results


def main():
    parser = argparse.ArgumentParser(
        description="Hypothesis Validator — rule + statistical checks"
    )
    parser.add_argument(
        "--input", "-i",
        help="Path to evidence pack JSON file (or use stdin)",
    )
    parser.add_argument(
        "--check", "-c",
        default="all",
        choices=["all", "temporal", "magnitude", "scope"],
        help="Which check layer(s) to run (default: all)",
    )
    parser.add_argument(
        "--max-lag",
        type=int,
        default=DEFAULT_MAX_LAG_SECONDS,
        help=f"Max reasonable lag in seconds for temporal check (default: {DEFAULT_MAX_LAG_SECONDS})",
    )
    parser.add_argument(
        "--pretty", "-p",
        action="store_true",
        help="Pretty-print JSON output",
    )

    args = parser.parse_args()

    # Read input
    if args.input:
        with open(args.input, "r", encoding="utf-8") as f:
            evidence = json.load(f)
    else:
        evidence = json.load(sys.stdin)

    config = {
        "max_reasonable_lag_seconds": args.max_lag,
    }

    checks = [args.check]
    results = run(evidence, checks, config)

    indent = 2 if args.pretty else None
    print(json.dumps(results, ensure_ascii=False, indent=indent, default=str))


if __name__ == "__main__":
    main()

