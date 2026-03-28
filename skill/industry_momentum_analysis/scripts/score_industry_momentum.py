#!/usr/bin/env python3
"""
Score sector momentum from structured evidence.

Input: JSON file with meta/price/flow/narrative/breadth/risk sections.
Output: Markdown by default, JSON when --format json is passed.
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any


def clamp(value: float, low: float = 0.0, high: float = 1.0) -> float:
    return max(low, min(high, value))


def scale_linear(value: float, low: float, high: float) -> float:
    if high == low:
        return 50.0
    return clamp((value - low) / (high - low)) * 100.0


def scale_reverse(value: float, low: float, high: float) -> float:
    return 100.0 - scale_linear(value, low, high)


def weighted_average(items: list[tuple[float | None, float]]) -> tuple[float, float]:
    total_weight = 0.0
    weighted_sum = 0.0
    available = 0
    for score, weight in items:
        if score is None:
            continue
        weighted_sum += score * weight
        total_weight += weight
        available += 1
    if total_weight == 0:
        return 50.0, 0.0
    coverage = available / max(len(items), 1)
    return weighted_sum / total_weight, coverage


def read_number(section: dict[str, Any], key: str) -> float | None:
    value = section.get(key)
    if value is None:
        return None
    if isinstance(value, bool):
        return float(value)
    if isinstance(value, (int, float)):
        return float(value)
    raise ValueError(f"Field '{key}' must be numeric or boolean.")


def fmt_pct(value: float | None, digits: int = 1) -> str:
    if value is None:
        return "缺失"
    return f"{value * 100:.{digits}f}%"


def label_from_score(score: float, risk_mode: bool = False) -> str:
    if risk_mode:
        if score >= 75:
            return "高风险"
        if score >= 55:
            return "中风险"
        return "可控"
    if score >= 75:
        return "强"
    if score >= 60:
        return "偏强"
    if score >= 45:
        return "一般"
    return "偏弱"


def confidence_from_coverage(coverage: float) -> str:
    if coverage >= 0.75:
        return "高"
    if coverage >= 0.45:
        return "中"
    return "低"


def bool_bonus(value: Any, positive: float = 8.0, negative: float = -5.0) -> float | None:
    if value is None:
        return None
    return positive if bool(value) else negative


def score_price(section: dict[str, Any]) -> dict[str, Any]:
    ret_20d = read_number(section, "ret_20d")
    ret_60d = read_number(section, "ret_60d")
    relative_strength_20d = read_number(section, "relative_strength_20d")
    up_day_ratio_20d = read_number(section, "up_day_ratio_20d")
    max_drawdown_20d = read_number(section, "max_drawdown_20d")
    distance_to_52w_high = read_number(section, "distance_to_52w_high")
    breakout = bool_bonus(section.get("breakout"))

    score, coverage = weighted_average(
        [
            (scale_linear(ret_20d, -0.10, 0.25) if ret_20d is not None else None, 0.22),
            (scale_linear(ret_60d, -0.20, 0.45) if ret_60d is not None else None, 0.24),
            (
                scale_linear(relative_strength_20d, -0.10, 0.15)
                if relative_strength_20d is not None
                else None,
                0.20,
            ),
            (
                scale_linear(up_day_ratio_20d, 0.35, 0.70)
                if up_day_ratio_20d is not None
                else None,
                0.14,
            ),
            (
                scale_reverse(max_drawdown_20d, 0.03, 0.18)
                if max_drawdown_20d is not None
                else None,
                0.12,
            ),
            (
                scale_reverse(distance_to_52w_high, 0.02, 0.22)
                if distance_to_52w_high is not None
                else None,
                0.08,
            ),
            (50.0 + breakout if breakout is not None else None, 0.05),
        ]
    )

    signals = []
    warnings = []
    if ret_20d is not None and ret_20d >= 0.12:
        signals.append(f"20 日涨幅 {fmt_pct(ret_20d)}，短中期趋势明确。")
    if relative_strength_20d is not None and relative_strength_20d >= 0.05:
        signals.append(f"相对基准 20 日超额 {fmt_pct(relative_strength_20d)}，不是纯 beta 反弹。")
    if up_day_ratio_20d is not None and up_day_ratio_20d >= 0.60:
        signals.append(f"20 日上涨天数占比 {fmt_pct(up_day_ratio_20d, 0)}，走势有连续性。")
    if section.get("breakout") is True:
        signals.append("价格结构出现平台突破。")
    if max_drawdown_20d is not None and max_drawdown_20d >= 0.12:
        warnings.append(f"20 日最大回撤 {fmt_pct(max_drawdown_20d)}，趋势并不平滑。")
    if distance_to_52w_high is not None and distance_to_52w_high > 0.18:
        warnings.append("距离 52 周高点仍远，更像修复而非强趋势。")

    return {
        "score": round(score, 1),
        "coverage": round(coverage, 2),
        "label": label_from_score(score),
        "signals": signals,
        "warnings": warnings,
    }


def score_flow(section: dict[str, Any]) -> dict[str, Any]:
    volume_ratio_20d = read_number(section, "volume_ratio_20d")
    turnover_ratio_20d = read_number(section, "turnover_ratio_20d")
    net_flow_5d_pct = read_number(section, "net_flow_5d_pct")
    net_flow_20d_pct = read_number(section, "net_flow_20d_pct")
    positive_flow_days_ratio_20d = read_number(section, "positive_flow_days_ratio_20d")
    concentration_top3_pct = read_number(section, "concentration_top3_pct")

    score, coverage = weighted_average(
        [
            (
                scale_linear(volume_ratio_20d, 0.80, 2.20)
                if volume_ratio_20d is not None
                else None,
                0.18,
            ),
            (
                scale_linear(turnover_ratio_20d, 0.80, 2.20)
                if turnover_ratio_20d is not None
                else None,
                0.16,
            ),
            (
                scale_linear(net_flow_5d_pct, -0.03, 0.05)
                if net_flow_5d_pct is not None
                else None,
                0.18,
            ),
            (
                scale_linear(net_flow_20d_pct, -0.05, 0.10)
                if net_flow_20d_pct is not None
                else None,
                0.22,
            ),
            (
                scale_linear(positive_flow_days_ratio_20d, 0.35, 0.75)
                if positive_flow_days_ratio_20d is not None
                else None,
                0.16,
            ),
            (
                scale_reverse(concentration_top3_pct, 0.35, 0.80)
                if concentration_top3_pct is not None
                else None,
                0.10,
            ),
        ]
    )

    signals = []
    warnings = []
    if net_flow_20d_pct is not None and net_flow_20d_pct >= 0.03:
        signals.append(f"20 日净流入强度 {fmt_pct(net_flow_20d_pct)}，存在持续增量资金。")
    if positive_flow_days_ratio_20d is not None and positive_flow_days_ratio_20d >= 0.55:
        signals.append(f"20 日正净流入天数占比 {fmt_pct(positive_flow_days_ratio_20d, 0)}，持续性较好。")
    if volume_ratio_20d is not None and volume_ratio_20d >= 1.4:
        signals.append(f"量能放大至过去区间的 {volume_ratio_20d:.2f} 倍。")
    if net_flow_5d_pct is not None and net_flow_5d_pct < 0:
        warnings.append("近 5 日净流入转负，短线资金有松动迹象。")
    if concentration_top3_pct is not None and concentration_top3_pct >= 0.60:
        warnings.append(f"前三大标的吸走 {fmt_pct(concentration_top3_pct, 0)} 资金，扩散度不足。")

    return {
        "score": round(score, 1),
        "coverage": round(coverage, 2),
        "label": label_from_score(score),
        "signals": signals,
        "warnings": warnings,
    }


def score_narrative(section: dict[str, Any]) -> dict[str, Any]:
    catalyst_count_14d = read_number(section, "catalyst_count_14d")
    thesis_alignment = read_number(section, "thesis_alignment")
    earnings_revision_score = read_number(section, "earnings_revision_score")
    policy_support_score = read_number(section, "policy_support_score")
    narrative_freshness = read_number(section, "narrative_freshness")

    score, coverage = weighted_average(
        [
            (
                scale_linear(catalyst_count_14d, 0.0, 6.0)
                if catalyst_count_14d is not None
                else None,
                0.18,
            ),
            (
                scale_linear(thesis_alignment, 0.20, 0.95)
                if thesis_alignment is not None
                else None,
                0.30,
            ),
            (
                scale_linear(earnings_revision_score, -0.50, 0.80)
                if earnings_revision_score is not None
                else None,
                0.20,
            ),
            (
                scale_linear(policy_support_score, 0.0, 1.0)
                if policy_support_score is not None
                else None,
                0.16,
            ),
            (
                scale_linear(narrative_freshness, 0.0, 1.0)
                if narrative_freshness is not None
                else None,
                0.16,
            ),
        ]
    )

    signals = []
    warnings = []
    if thesis_alignment is not None and thesis_alignment >= 0.65:
        signals.append("市场交易理由相对统一，叙事不是零散拼接。")
    if catalyst_count_14d is not None and catalyst_count_14d >= 3:
        signals.append(f"近 14 日有 {int(catalyst_count_14d)} 个以上有效催化。")
    if earnings_revision_score is not None and earnings_revision_score > 0.15:
        signals.append("盈利预期在改善，叙事有基本面承接。")
    if thesis_alignment is not None and thesis_alignment < 0.45:
        warnings.append("市场叙事分散，尚未形成统一交易理由。")
    if narrative_freshness is not None and narrative_freshness < 0.35:
        warnings.append("题材新鲜度偏低，可能已被反复交易。")

    return {
        "score": round(score, 1),
        "coverage": round(coverage, 2),
        "label": label_from_score(score),
        "signals": signals,
        "warnings": warnings,
    }


def score_breadth(section: dict[str, Any]) -> dict[str, Any]:
    pct_above_ma20 = read_number(section, "pct_above_ma20")
    pct_above_ma60 = read_number(section, "pct_above_ma60")
    advance_decline_ratio = read_number(section, "advance_decline_ratio")
    median_ret_20d = read_number(section, "median_ret_20d")
    leader_contribution_pct = read_number(section, "leader_contribution_pct")

    score, coverage = weighted_average(
        [
            (
                scale_linear(pct_above_ma20, 0.25, 0.85)
                if pct_above_ma20 is not None
                else None,
                0.22,
            ),
            (
                scale_linear(pct_above_ma60, 0.20, 0.80)
                if pct_above_ma60 is not None
                else None,
                0.22,
            ),
            (
                scale_linear(advance_decline_ratio, 0.60, 2.50)
                if advance_decline_ratio is not None
                else None,
                0.16,
            ),
            (
                scale_linear(median_ret_20d, -0.05, 0.18)
                if median_ret_20d is not None
                else None,
                0.20,
            ),
            (
                scale_reverse(leader_contribution_pct, 0.35, 0.80)
                if leader_contribution_pct is not None
                else None,
                0.20,
            ),
        ]
    )

    signals = []
    warnings = []
    if pct_above_ma20 is not None and pct_above_ma20 >= 0.65:
        signals.append(f"站上 20 日线的成分占比 {fmt_pct(pct_above_ma20, 0)}，有板块共振。")
    if advance_decline_ratio is not None and advance_decline_ratio >= 1.50:
        signals.append(f"涨跌家数比 {advance_decline_ratio:.2f}，扩散度健康。")
    if median_ret_20d is not None and median_ret_20d >= 0.06:
        signals.append(f"成分中位数 20 日收益 {fmt_pct(median_ret_20d)}，不是只靠个别权重股。")
    if leader_contribution_pct is not None and leader_contribution_pct >= 0.60:
        warnings.append(f"龙头贡献度达 {fmt_pct(leader_contribution_pct, 0)}，更像单核驱动。")
    if pct_above_ma60 is not None and pct_above_ma60 < 0.40:
        warnings.append("站上 60 日线的比例不高，中期扩散仍弱。")

    return {
        "score": round(score, 1),
        "coverage": round(coverage, 2),
        "label": label_from_score(score),
        "signals": signals,
        "warnings": warnings,
    }


def score_risk(section: dict[str, Any]) -> dict[str, Any]:
    valuation_percentile_5y = read_number(section, "valuation_percentile_5y")
    short_term_spike_10d = read_number(section, "short_term_spike_10d")
    rsi_14 = read_number(section, "rsi_14")
    event_risk_score = read_number(section, "event_risk_score")
    narrative_crowding_score = read_number(section, "narrative_crowding_score")

    score, coverage = weighted_average(
        [
            (
                scale_linear(valuation_percentile_5y, 0.20, 0.95)
                if valuation_percentile_5y is not None
                else None,
                0.22,
            ),
            (
                scale_linear(short_term_spike_10d, 0.02, 0.20)
                if short_term_spike_10d is not None
                else None,
                0.24,
            ),
            (scale_linear(rsi_14, 45.0, 85.0) if rsi_14 is not None else None, 0.20),
            (
                scale_linear(event_risk_score, 0.0, 1.0)
                if event_risk_score is not None
                else None,
                0.16,
            ),
            (
                scale_linear(narrative_crowding_score, 0.0, 1.0)
                if narrative_crowding_score is not None
                else None,
                0.18,
            ),
        ]
    )

    signals = []
    warnings = []
    if score <= 45:
        signals.append("风险和拥挤度暂时可控。")
    if valuation_percentile_5y is not None and valuation_percentile_5y >= 0.75:
        warnings.append(f"估值处于近 5 年 {fmt_pct(valuation_percentile_5y, 0)} 分位，安全边际偏薄。")
    if short_term_spike_10d is not None and short_term_spike_10d >= 0.10:
        warnings.append(f"10 日涨幅已达 {fmt_pct(short_term_spike_10d)}，短线容易拥挤。")
    if rsi_14 is not None and rsi_14 >= 72:
        warnings.append(f"RSI14 为 {rsi_14:.1f}，动量偏热。")
    if event_risk_score is not None and event_risk_score >= 0.60:
        warnings.append("未来 2 到 4 周事件窗口密集。")
    if narrative_crowding_score is not None and narrative_crowding_score >= 0.65:
        warnings.append("叙事一致性过高，容易出现预期透支。")

    return {
        "score": round(score, 1),
        "coverage": round(coverage, 2),
        "label": label_from_score(score, risk_mode=True),
        "signals": signals,
        "warnings": warnings,
    }


def classify_stage(scores: dict[str, dict[str, Any]], payload: dict[str, Any]) -> dict[str, Any]:
    price = scores["price"]["score"]
    flow = scores["flow"]["score"]
    narrative = scores["narrative"]["score"]
    breadth = scores["breadth"]["score"]
    risk = scores["risk"]["score"]
    short_term_spike = read_number(payload.get("risk", {}), "short_term_spike_10d") or 0.0

    if price >= 72 and flow >= 65 and breadth >= 55 and (risk >= 68 or short_term_spike >= 0.12):
        stage = "拥挤"
        explanation = "价格、资金和扩散都强，但估值/短期涨幅/情绪已明显抬升。"
    elif price >= 65 and flow >= 58 and breadth >= 52:
        stage = "加速"
        explanation = "价格、资金、结构基本共振，趋势仍在强化。"
    elif price >= 50 and narrative >= 55 and flow >= 45:
        stage = "启动"
        explanation = "趋势和叙事开始转强，但扩散和拥挤度还没走到极致。"
    else:
        stage = "退潮"
        explanation = "价格或资金、结构不足以支撑持续强化，更像退潮或弱修复。"

    if stage == "加速" and risk >= 72:
        stage = "拥挤"
        explanation = "趋势仍强，但风险已经主导交易难度。"

    return {"name": stage, "explanation": explanation}


def overall_view(scores: dict[str, dict[str, Any]], stage: str) -> dict[str, Any]:
    price = scores["price"]["score"]
    flow = scores["flow"]["score"]
    narrative = scores["narrative"]["score"]
    breadth = scores["breadth"]["score"]
    risk = scores["risk"]["score"]
    composite = round((price + flow + narrative + breadth + (100.0 - risk)) / 5.0, 1)

    if stage == "加速" and risk < 65:
        conclusion = "动量仍有参与价值，但应沿着趋势和结构强弱做选择。"
    elif stage == "拥挤":
        conclusion = "趋势未必立刻结束，但更适合持有优化或等回撤，不适合无差别追高。"
    elif stage == "启动":
        conclusion = "进入可跟踪区间，适合继续确认资金与扩散是否跟上。"
    else:
        conclusion = "暂不适合把当前强弱解读成可持续主升。"

    if breadth < 50 and price >= 65:
        conclusion += " 当前更像强龙头行情，板块化参与要保守。"
    if narrative < 50:
        conclusion += " 叙事一致性不足，会削弱持续性。"

    return {"composite_score": composite, "conclusion": conclusion}


def build_answers(payload: dict[str, Any], scores: dict[str, dict[str, Any]], stage: dict[str, Any]) -> dict[str, str]:
    price = payload.get("price", {})
    flow = payload.get("flow", {})
    narrative = payload.get("narrative", {})
    breadth = payload.get("breadth", {})
    risk = payload.get("risk", {})

    price_score = scores["price"]["score"]
    flow_score = scores["flow"]["score"]
    narrative_score = scores["narrative"]["score"]
    breadth_score = scores["breadth"]["score"]
    risk_score = scores["risk"]["score"]

    price_answer = (
        f"{'是' if price_score >= 60 else '不是很稳'}。"
        f" 20 日收益 {fmt_pct(read_number(price, 'ret_20d'))}，60 日收益 {fmt_pct(read_number(price, 'ret_60d'))}，"
        f"相对基准 20 日超额 {fmt_pct(read_number(price, 'relative_strength_20d'))}，价格动量分 {price_score:.1f}/100。"
    )
    flow_answer = (
        f"{'有' if flow_score >= 60 else '不够充分'}。"
        f" 20 日净流入强度 {fmt_pct(read_number(flow, 'net_flow_20d_pct'))}，"
        f"正净流入天数占比 {fmt_pct(read_number(flow, 'positive_flow_days_ratio_20d'), 0)}，资金分 {flow_score:.1f}/100。"
    )
    narrative_answer = (
        f"{'有相对统一的理由' if narrative_score >= 60 else '统一叙事还不够强'}。"
        f" 近 14 日催化数量 {int(read_number(narrative, 'catalyst_count_14d') or 0)}，"
        f"叙事一致性 {fmt_pct(read_number(narrative, 'thesis_alignment'), 0)}，叙事分 {narrative_score:.1f}/100。"
    )
    structure_answer = (
        f"{'更偏板块扩散' if breadth_score >= 60 else '更像龙头主导'}。"
        f" 站上 MA20 的比例 {fmt_pct(read_number(breadth, 'pct_above_ma20'), 0)}，"
        f"龙头贡献度 {fmt_pct(read_number(breadth, 'leader_contribution_pct'), 0)}，结构分 {breadth_score:.1f}/100。"
    )
    stage_answer = f"当前更接近“{stage['name']}”阶段。{stage['explanation']}"
    risk_answer = (
        f"{'是，已经有明显过热迹象' if risk_score >= 70 else '还没有完全交易过头，但风险在抬升' if risk_score >= 55 else '暂未看到明显交易过头'}。"
        f" 估值分位 {fmt_pct(read_number(risk, 'valuation_percentile_5y'), 0)}，"
        f"10 日涨幅 {fmt_pct(read_number(risk, 'short_term_spike_10d'))}，风险分 {risk_score:.1f}/100。"
    )

    return {
        "价格上，它是不是在持续走强？": price_answer,
        "资金上，有没有增量和持续性？": flow_answer,
        "叙事上，市场有没有统一理由去交易它？": narrative_answer,
        "结构上，是龙头独舞还是板块扩散？": structure_answer,
        "阶段上，现在是启动、加速、拥挤还是退潮？": stage_answer,
        "风险上，这个强势是不是已经被交易过头？": risk_answer,
    }


def render_markdown(result: dict[str, Any]) -> str:
    meta = result["meta"]
    scores = result["scores"]
    answers = result["answers"]
    stage = result["stage"]
    overall = result["overall"]

    lines = [
        f"# 行业动量分析：{meta.get('sector', '未命名行业')}",
        "",
        f"- 截止日期：{meta.get('as_of', '未提供')}",
        f"- 市场：{meta.get('market', '未提供')}",
        f"- 基准：{meta.get('benchmark', '未提供')}",
        f"- 阶段：{stage['name']}",
        f"- 综合分：{overall['composite_score']}/100",
        f"- 置信度：{result['confidence']}",
        "",
        "## 一句话结论",
        overall["conclusion"],
        "",
        "## 六个关键问题",
    ]

    for index, (question, answer) in enumerate(answers.items(), start=1):
        lines.append(f"{index}. {question}")
        lines.append(f"   {answer}")

    lines.extend(
        [
            "",
            "## 模块分数",
            "| 模块 | 分数 | 标签 | 覆盖度 |",
            "| --- | ---: | --- | ---: |",
        ]
    )
    module_order = [
        ("price", "价格"),
        ("flow", "资金"),
        ("narrative", "叙事"),
        ("breadth", "结构"),
        ("risk", "风险"),
    ]
    for key, title in module_order:
        item = scores[key]
        lines.append(f"| {title} | {item['score']} | {item['label']} | {item['coverage']} |")

    positives = result["positives"]
    risks = result["risks"]
    if positives:
        lines.extend(["", "## 主要正向信号"])
        for item in positives:
            lines.append(f"- {item}")
    if risks:
        lines.extend(["", "## 主要风险"])
        for item in risks:
            lines.append(f"- {item}")

    return "\n".join(lines)


def main() -> None:
    parser = argparse.ArgumentParser(description="Score industry momentum from structured JSON input.")
    parser.add_argument("input_path", help="Path to the JSON input file.")
    parser.add_argument("--format", choices=["markdown", "json"], default="markdown")
    args = parser.parse_args()

    payload = json.loads(Path(args.input_path).read_text())

    scores = {
        "price": score_price(payload.get("price", {})),
        "flow": score_flow(payload.get("flow", {})),
        "narrative": score_narrative(payload.get("narrative", {})),
        "breadth": score_breadth(payload.get("breadth", {})),
        "risk": score_risk(payload.get("risk", {})),
    }
    stage = classify_stage(scores, payload)
    overall = overall_view(scores, stage["name"])
    answers = build_answers(payload, scores, stage)

    coverage = sum(item["coverage"] for item in scores.values()) / len(scores)
    positives = []
    risks = []
    for key in ("price", "flow", "narrative", "breadth"):
        positives.extend(scores[key]["signals"])
        risks.extend(scores[key]["warnings"])
    positives.extend(scores["risk"]["signals"])
    risks.extend(scores["risk"]["warnings"])

    result = {
        "meta": payload.get("meta", {}),
        "scores": scores,
        "stage": stage,
        "overall": overall,
        "answers": answers,
        "positives": positives[:6],
        "risks": risks[:6],
        "confidence": confidence_from_coverage(coverage),
        "coverage": round(coverage, 2),
    }

    if args.format == "json":
        print(json.dumps(result, ensure_ascii=False, indent=2))
    else:
        print(render_markdown(result))


if __name__ == "__main__":
    main()
