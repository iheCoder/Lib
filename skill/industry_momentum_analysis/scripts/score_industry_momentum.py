#!/usr/bin/env python3
"""
Score current industry momentum from structured evidence.

The "current" contract is strict:
- today is ideal
- up to the last 3 trading days is acceptable
- older than 3 days is background analysis, not current analysis
"""

from __future__ import annotations

import argparse
import json
from datetime import date
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


def read_number(section: dict[str, Any], *keys: str) -> float | None:
    for key in keys:
        value = section.get(key)
        if value is None:
            continue
        if isinstance(value, bool):
            return float(value)
        if isinstance(value, (int, float)):
            return float(value)
        raise ValueError(f"Field '{key}' must be numeric or boolean.")
    return None


def read_bool(section: dict[str, Any], *keys: str) -> bool | None:
    for key in keys:
        value = section.get(key)
        if value is None:
            continue
        if isinstance(value, bool):
            return value
        raise ValueError(f"Field '{key}' must be boolean.")
    return None


def fmt_pct(value: float | None, digits: int = 1) -> str:
    if value is None:
        return "缺失"
    return f"{value * 100:.{digits}f}%"


def label_from_score(score: float, risk_mode: bool = False) -> str:
    if risk_mode:
        if score >= 75:
            return "高风险"
        if score >= 58:
            return "中风险"
        return "可控"
    if score >= 75:
        return "强"
    if score >= 60:
        return "偏强"
    if score >= 45:
        return "一般"
    return "偏弱"


def bool_score(value: bool | None, positive: float = 88.0, negative: float = 25.0) -> float | None:
    if value is None:
        return None
    return positive if value else negative


def parse_staleness(meta: dict[str, Any]) -> int | None:
    as_of = meta.get("as_of")
    if not as_of:
        return None
    try:
        as_of_date = date.fromisoformat(as_of)
    except ValueError:
        return None
    return (date.today() - as_of_date).days


def currentness_label(staleness_days: int | None) -> str:
    if staleness_days is None:
        return "未知"
    if staleness_days <= 1:
        return "当前"
    if staleness_days <= 3:
        return "近3日"
    return "过期"


def confidence_from_coverage(coverage: float, staleness_days: int | None, compare_coverage: float) -> str:
    score = coverage
    if staleness_days is None:
        score -= 0.10
    elif staleness_days > 3:
        score -= 0.35
    elif staleness_days > 1:
        score -= 0.15
    if compare_coverage == 0:
        score -= 0.05
    if score >= 0.75:
        return "高"
    if score >= 0.45:
        return "中"
    return "低"


def score_price(section: dict[str, Any]) -> dict[str, Any]:
    ret_1d = read_number(section, "ret_1d")
    ret_3d = read_number(section, "ret_3d")
    ret_5d = read_number(section, "ret_5d", "ret_5")
    ret_20d = read_number(section, "ret_20d")
    ret_60d = read_number(section, "ret_60d")
    rs_1d = read_number(section, "rs_1d", "relative_strength_1d")
    rs_3d = read_number(section, "rs_3d", "relative_strength_3d")
    close_strength_1d = read_number(section, "close_strength_1d")
    up_day_ratio_5d = read_number(section, "up_day_ratio_5d")
    max_drawdown_3d = read_number(section, "max_drawdown_3d")
    max_drawdown_20d = read_number(section, "max_drawdown_20d")
    trend_smoothness_5d = read_number(section, "trend_smoothness_5d")
    distance_to_breakout_level = read_number(section, "distance_to_breakout_level")
    distance_to_52w_high = read_number(section, "distance_to_52w_high")
    breakout = read_bool(section, "breakout")

    score, coverage = weighted_average(
        [
            (scale_linear(ret_1d, -0.04, 0.05) if ret_1d is not None else None, 0.14),
            (scale_linear(ret_3d, -0.08, 0.12) if ret_3d is not None else None, 0.18),
            (scale_linear(rs_1d, -0.03, 0.04) if rs_1d is not None else None, 0.12),
            (scale_linear(rs_3d, -0.05, 0.08) if rs_3d is not None else None, 0.14),
            (scale_linear(close_strength_1d, 0.20, 0.95) if close_strength_1d is not None else None, 0.08),
            (scale_linear(up_day_ratio_5d, 0.30, 0.85) if up_day_ratio_5d is not None else None, 0.07),
            (scale_reverse(max_drawdown_3d, 0.01, 0.10) if max_drawdown_3d is not None else None, 0.08),
            (scale_linear(trend_smoothness_5d, 0.15, 0.90) if trend_smoothness_5d is not None else None, 0.07),
            (scale_linear(ret_20d, -0.12, 0.25) if ret_20d is not None else None, 0.05),
            (scale_linear(ret_60d, -0.20, 0.45) if ret_60d is not None else None, 0.03),
            (
                scale_reverse(distance_to_breakout_level, 0.01, 0.12)
                if distance_to_breakout_level is not None
                else None,
                0.02,
            ),
            (
                scale_reverse(distance_to_52w_high, 0.02, 0.20)
                if distance_to_52w_high is not None
                else None,
                0.02,
            ),
            (bool_score(breakout) if breakout is not None else None, 0.08),
        ]
    )

    signals = []
    warnings = []
    if ret_1d is not None and ret_1d >= 0.02:
        signals.append(f"今日涨幅 {fmt_pct(ret_1d)}，当日脉冲明显。")
    if ret_3d is not None and ret_3d >= 0.05:
        signals.append(f"近 3 日累计涨幅 {fmt_pct(ret_3d)}，不是单日偶发。")
    if rs_3d is not None and rs_3d >= 0.03:
        signals.append(f"近 3 日相对基准超额 {fmt_pct(rs_3d)}，当前属于截面强势。")
    if trend_smoothness_5d is not None and trend_smoothness_5d >= 0.65:
        signals.append("近 5 日趋势平滑，回撤控制较好。")
    if breakout is True:
        signals.append("价格位置处于平台突破或临近突破。")
    if max_drawdown_3d is not None and max_drawdown_3d >= 0.06:
        warnings.append(f"近 3 日最大回撤 {fmt_pct(max_drawdown_3d)}，短线波动偏大。")
    if close_strength_1d is not None and close_strength_1d <= 0.35:
        warnings.append("收盘强度偏弱，尾盘承接不足。")
    if ret_20d is not None and ret_20d < 0 and ret_3d is not None and ret_3d > 0:
        warnings.append("当前更像短线反弹，20 日背景趋势还未完全修复。")

    return {
        "score": round(score, 1),
        "coverage": round(coverage, 2),
        "label": label_from_score(score),
        "signals": signals,
        "warnings": warnings,
    }


def score_flow(section: dict[str, Any]) -> dict[str, Any]:
    turnover_ratio_1d = read_number(section, "turnover_ratio_1d")
    turnover_ratio_3d = read_number(section, "turnover_ratio_3d")
    volume_ratio_1d = read_number(section, "volume_ratio_1d")
    volume_ratio_3d = read_number(section, "volume_ratio_3d")
    net_flow_1d_pct = read_number(section, "net_flow_1d_pct")
    net_flow_3d_pct = read_number(section, "net_flow_3d_pct")
    positive_flow_days_ratio_3d = read_number(section, "positive_flow_days_ratio_3d")
    net_flow_acceleration = read_number(section, "net_flow_acceleration")
    etf_flow_confirm_score = read_number(section, "etf_flow_confirm_score")
    northbound_confirm_score = read_number(section, "northbound_confirm_score")
    margin_confirm_score = read_number(section, "margin_confirm_score")
    leader_flow_share_pct = read_number(section, "leader_flow_share_pct", "concentration_top3_pct")
    follower_participation_ratio = read_number(section, "follower_participation_ratio")
    pullback_resilience_score = read_number(section, "pullback_resilience_score")

    score, coverage = weighted_average(
        [
            (scale_linear(turnover_ratio_1d, 0.80, 2.30) if turnover_ratio_1d is not None else None, 0.10),
            (scale_linear(turnover_ratio_3d, 0.80, 2.10) if turnover_ratio_3d is not None else None, 0.11),
            (scale_linear(volume_ratio_1d, 0.80, 2.30) if volume_ratio_1d is not None else None, 0.08),
            (scale_linear(volume_ratio_3d, 0.80, 2.10) if volume_ratio_3d is not None else None, 0.08),
            (scale_linear(net_flow_1d_pct, -0.02, 0.03) if net_flow_1d_pct is not None else None, 0.12),
            (scale_linear(net_flow_3d_pct, -0.04, 0.05) if net_flow_3d_pct is not None else None, 0.14),
            (
                scale_linear(positive_flow_days_ratio_3d, 0.0, 1.0)
                if positive_flow_days_ratio_3d is not None
                else None,
                0.10,
            ),
            (
                scale_linear(net_flow_acceleration, -0.60, 0.60)
                if net_flow_acceleration is not None
                else None,
                0.08,
            ),
            (
                scale_linear(etf_flow_confirm_score, 0.0, 1.0)
                if etf_flow_confirm_score is not None
                else None,
                0.05,
            ),
            (
                scale_linear(northbound_confirm_score, 0.0, 1.0)
                if northbound_confirm_score is not None
                else None,
                0.05,
            ),
            (
                scale_linear(margin_confirm_score, 0.0, 1.0)
                if margin_confirm_score is not None
                else None,
                0.03,
            ),
            (
                scale_reverse(leader_flow_share_pct, 0.35, 0.80)
                if leader_flow_share_pct is not None
                else None,
                0.03,
            ),
            (
                scale_linear(follower_participation_ratio, 0.20, 0.90)
                if follower_participation_ratio is not None
                else None,
                0.02,
            ),
            (
                scale_linear(pullback_resilience_score, 0.0, 1.0)
                if pullback_resilience_score is not None
                else None,
                0.01,
            ),
        ]
    )

    signals = []
    warnings = []
    if net_flow_1d_pct is not None and net_flow_1d_pct > 0:
        signals.append(f"今日净流入强度 {fmt_pct(net_flow_1d_pct)}，当日资金在主动推升。")
    if net_flow_3d_pct is not None and net_flow_3d_pct >= 0.015:
        signals.append(f"近 3 日净流入强度 {fmt_pct(net_flow_3d_pct)}，资金不是单日脉冲。")
    if positive_flow_days_ratio_3d is not None and positive_flow_days_ratio_3d >= 0.67:
        signals.append("近 3 日净流入持续性较好。")
    if etf_flow_confirm_score is not None and etf_flow_confirm_score >= 0.60:
        signals.append("ETF 或被动资金对主题有确认。")
    if follower_participation_ratio is not None and follower_participation_ratio >= 0.60:
        signals.append("跟风和中军也开始获得资金参与。")
    if leader_flow_share_pct is not None and leader_flow_share_pct >= 0.60:
        warnings.append(f"资金过度集中在龙头，龙头流量占比 {fmt_pct(leader_flow_share_pct, 0)}。")
    if net_flow_1d_pct is not None and net_flow_1d_pct < 0 and net_flow_3d_pct is not None and net_flow_3d_pct <= 0:
        warnings.append("今日和近 3 日资金都没有形成增量承接。")
    if pullback_resilience_score is not None and pullback_resilience_score < 0.35:
        warnings.append("回调日承接偏弱，资金持续性一般。")

    return {
        "score": round(score, 1),
        "coverage": round(coverage, 2),
        "label": label_from_score(score),
        "signals": signals,
        "warnings": warnings,
    }


def score_narrative(section: dict[str, Any]) -> dict[str, Any]:
    mention_burst_3d = read_number(section, "mention_burst_3d")
    catalyst_count_3d = read_number(section, "catalyst_count_3d")
    thesis_alignment = read_number(section, "thesis_alignment")
    source_diversity_score = read_number(section, "source_diversity_score")
    thesis_upgrade_score = read_number(section, "thesis_upgrade_score")
    earnings_revision_score = read_number(section, "earnings_revision_score")
    narrative_freshness = read_number(section, "narrative_freshness")

    score, coverage = weighted_average(
        [
            (scale_linear(mention_burst_3d, 0.0, 1.0) if mention_burst_3d is not None else None, 0.16),
            (scale_linear(catalyst_count_3d, 0.0, 4.0) if catalyst_count_3d is not None else None, 0.15),
            (scale_linear(thesis_alignment, 0.20, 0.95) if thesis_alignment is not None else None, 0.24),
            (scale_linear(source_diversity_score, 0.0, 1.0) if source_diversity_score is not None else None, 0.10),
            (scale_linear(thesis_upgrade_score, 0.0, 1.0) if thesis_upgrade_score is not None else None, 0.18),
            (
                scale_linear(earnings_revision_score, -0.50, 0.80)
                if earnings_revision_score is not None
                else None,
                0.10,
            ),
            (scale_linear(narrative_freshness, 0.0, 1.0) if narrative_freshness is not None else None, 0.07),
        ]
    )

    signals = []
    warnings = []
    if mention_burst_3d is not None and mention_burst_3d >= 0.65:
        signals.append("近 3 日话题热度快速抬升。")
    if thesis_alignment is not None and thesis_alignment >= 0.65:
        signals.append("市场在围绕相对统一的主叙事交易。")
    if thesis_upgrade_score is not None and thesis_upgrade_score >= 0.60:
        signals.append("叙事正在升级，不只是重复老故事。")
    if earnings_revision_score is not None and earnings_revision_score > 0.15:
        signals.append("盈利或景气预期在改善，叙事有基本面承接。")
    if thesis_alignment is not None and thesis_alignment < 0.45:
        warnings.append("叙事一致性不足，当前强势可能缺少统一解释。")
    if source_diversity_score is not None and source_diversity_score < 0.35:
        warnings.append("叙事信号过度依赖单一路径，容易被噪音污染。")
    if narrative_freshness is not None and narrative_freshness < 0.35:
        warnings.append("叙事新鲜度偏低，边际强化可能有限。")

    return {
        "score": round(score, 1),
        "coverage": round(coverage, 2),
        "label": label_from_score(score),
        "signals": signals,
        "warnings": warnings,
    }


def score_breadth(section: dict[str, Any]) -> dict[str, Any]:
    advancers_ratio_1d = read_number(section, "advancers_ratio_1d")
    advancers_ratio_3d = read_number(section, "advancers_ratio_3d")
    median_ret_1d = read_number(section, "median_ret_1d")
    median_ret_3d = read_number(section, "median_ret_3d")
    layer_diffusion_score = read_number(section, "layer_diffusion_score")
    subtheme_resonance_ratio = read_number(section, "subtheme_resonance_ratio")
    non_leader_participation_ratio = read_number(section, "non_leader_participation_ratio")
    leader_contribution_pct = read_number(section, "leader_contribution_pct")

    score, coverage = weighted_average(
        [
            (scale_linear(advancers_ratio_1d, 0.25, 0.90) if advancers_ratio_1d is not None else None, 0.15),
            (scale_linear(advancers_ratio_3d, 0.25, 0.90) if advancers_ratio_3d is not None else None, 0.20),
            (scale_linear(median_ret_1d, -0.02, 0.04) if median_ret_1d is not None else None, 0.12),
            (scale_linear(median_ret_3d, -0.04, 0.08) if median_ret_3d is not None else None, 0.18),
            (scale_linear(layer_diffusion_score, 0.0, 1.0) if layer_diffusion_score is not None else None, 0.14),
            (
                scale_linear(subtheme_resonance_ratio, 0.0, 1.0)
                if subtheme_resonance_ratio is not None
                else None,
                0.11,
            ),
            (
                scale_linear(non_leader_participation_ratio, 0.0, 1.0)
                if non_leader_participation_ratio is not None
                else None,
                0.05,
            ),
            (
                scale_reverse(leader_contribution_pct, 0.35, 0.80)
                if leader_contribution_pct is not None
                else None,
                0.05,
            ),
        ]
    )

    signals = []
    warnings = []
    if advancers_ratio_1d is not None and advancers_ratio_1d >= 0.65:
        signals.append(f"今日上涨成分占比 {fmt_pct(advancers_ratio_1d, 0)}，不是单票独舞。")
    if advancers_ratio_3d is not None and advancers_ratio_3d >= 0.65:
        signals.append(f"近 3 日上涨成分占比 {fmt_pct(advancers_ratio_3d, 0)}，扩散在持续。")
    if median_ret_3d is not None and median_ret_3d >= 0.03:
        signals.append(f"成分中位数近 3 日收益 {fmt_pct(median_ret_3d)}，整体参与度较好。")
    if layer_diffusion_score is not None and layer_diffusion_score >= 0.65:
        signals.append("龙头、中军、二线的分层扩散较健康。")
    if subtheme_resonance_ratio is not None and subtheme_resonance_ratio >= 0.60:
        signals.append("行业内多个子方向正在共振。")
    if leader_contribution_pct is not None and leader_contribution_pct >= 0.60:
        warnings.append(f"龙头对涨幅贡献过高，贡献度 {fmt_pct(leader_contribution_pct, 0)}。")
    if non_leader_participation_ratio is not None and non_leader_participation_ratio < 0.35:
        warnings.append("非龙头跟涨不足，更像局部行情。")

    return {
        "score": round(score, 1),
        "coverage": round(coverage, 2),
        "label": label_from_score(score),
        "signals": signals,
        "warnings": warnings,
    }


def score_crowding(section: dict[str, Any]) -> dict[str, Any]:
    valuation_percentile_5y = read_number(section, "valuation_percentile_5y")
    short_term_spike_3d = read_number(section, "short_term_spike_3d")
    short_term_spike_10d = read_number(section, "short_term_spike_10d")
    rsi_14 = read_number(section, "rsi_14")
    turnover_share_percentile_1y = read_number(section, "turnover_share_percentile_1y")
    narrative_crowding_score = read_number(section, "narrative_crowding_score")
    second_line_mania_score = read_number(section, "second_line_mania_score")
    event_risk_score = read_number(section, "event_risk_score")

    score, coverage = weighted_average(
        [
            (
                scale_linear(valuation_percentile_5y, 0.20, 0.95)
                if valuation_percentile_5y is not None
                else None,
                0.18,
            ),
            (
                scale_linear(short_term_spike_3d, 0.01, 0.12)
                if short_term_spike_3d is not None
                else None,
                0.18,
            ),
            (
                scale_linear(short_term_spike_10d, 0.02, 0.20)
                if short_term_spike_10d is not None
                else None,
                0.14,
            ),
            (scale_linear(rsi_14, 45.0, 85.0) if rsi_14 is not None else None, 0.14),
            (
                scale_linear(turnover_share_percentile_1y, 0.20, 0.95)
                if turnover_share_percentile_1y is not None
                else None,
                0.10,
            ),
            (
                scale_linear(narrative_crowding_score, 0.0, 1.0)
                if narrative_crowding_score is not None
                else None,
                0.10,
            ),
            (
                scale_linear(second_line_mania_score, 0.0, 1.0)
                if second_line_mania_score is not None
                else None,
                0.08,
            ),
            (
                scale_linear(event_risk_score, 0.0, 1.0)
                if event_risk_score is not None
                else None,
                0.08,
            ),
        ]
    )

    signals = []
    warnings = []
    if score <= 45:
        signals.append("当前拥挤度和交易风险仍相对可控。")
    if valuation_percentile_5y is not None and valuation_percentile_5y >= 0.75:
        warnings.append(f"估值已到近 5 年 {fmt_pct(valuation_percentile_5y, 0)} 分位。")
    if short_term_spike_3d is not None and short_term_spike_3d >= 0.06:
        warnings.append(f"近 3 日脉冲 {fmt_pct(short_term_spike_3d)}，当前追高难度上升。")
    if rsi_14 is not None and rsi_14 >= 72:
        warnings.append(f"RSI14 为 {rsi_14:.1f}，短线偏热。")
    if second_line_mania_score is not None and second_line_mania_score >= 0.65:
        warnings.append("二线杂毛补涨明显，可能接近情绪高潮。")
    if turnover_share_percentile_1y is not None and turnover_share_percentile_1y >= 0.75:
        warnings.append("行业成交占比已处高位，注意拥挤交易反噬。")

    return {
        "score": round(score, 1),
        "coverage": round(coverage, 2),
        "label": label_from_score(score, risk_mode=True),
        "signals": signals,
        "warnings": warnings,
    }


def score_compare(section: dict[str, Any]) -> dict[str, Any]:
    peer_overall_percentile = read_number(section, "peer_overall_percentile")
    peer_price_percentile = read_number(section, "peer_price_percentile")
    peer_flow_percentile = read_number(section, "peer_flow_percentile")
    peer_narrative_percentile = read_number(section, "peer_narrative_percentile")
    peer_breadth_percentile = read_number(section, "peer_breadth_percentile")

    score, coverage = weighted_average(
        [
            (
                scale_linear(peer_overall_percentile, 0.0, 1.0)
                if peer_overall_percentile is not None
                else None,
                0.30,
            ),
            (
                scale_linear(peer_price_percentile, 0.0, 1.0)
                if peer_price_percentile is not None
                else None,
                0.20,
            ),
            (
                scale_linear(peer_flow_percentile, 0.0, 1.0)
                if peer_flow_percentile is not None
                else None,
                0.20,
            ),
            (
                scale_linear(peer_narrative_percentile, 0.0, 1.0)
                if peer_narrative_percentile is not None
                else None,
                0.15,
            ),
            (
                scale_linear(peer_breadth_percentile, 0.0, 1.0)
                if peer_breadth_percentile is not None
                else None,
                0.15,
            ),
        ]
    )

    signals = []
    warnings = []
    if peer_overall_percentile is not None and peer_overall_percentile >= 0.75:
        signals.append(f"横向比较位于候选行业前 {fmt_pct(1 - peer_overall_percentile, 0)}。")
    if coverage == 0:
        warnings.append("缺少跨行业比较数据，只能做绝对判断。")

    return {
        "score": round(score, 1),
        "coverage": round(coverage, 2),
        "label": label_from_score(score),
        "signals": signals,
        "warnings": warnings,
    }


def classify_stage(scores: dict[str, dict[str, Any]], payload: dict[str, Any]) -> dict[str, Any]:
    price = scores["price"]["score"]
    flow = scores["flow"]["score"]
    narrative = scores["narrative"]["score"]
    breadth = scores["breadth"]["score"]
    crowding = scores["crowding"]["score"]
    ret_1d = read_number(payload.get("price", {}), "ret_1d") or 0.0
    ret_3d = read_number(payload.get("price", {}), "ret_3d") or 0.0

    if price >= 74 and flow >= 66 and breadth >= 62 and crowding >= 68:
        return {"name": "拥挤", "explanation": "当前价格、资金、扩散都很强，但拥挤与风险已经明显抬升。"}
    if price >= 68 and flow >= 60 and narrative >= 58 and breadth >= 58 and crowding < 68:
        return {"name": "加速", "explanation": "当前价格、资金、叙事和扩散正在共振，主升仍在延续。"}
    if (ret_1d > 0 or ret_3d > 0) and price >= 55 and narrative >= 52 and flow >= 48:
        return {"name": "启动", "explanation": "当前开始转强，但还需要更多资金和扩散确认。"}
    return {"name": "退潮", "explanation": "当前强度、承接或扩散不足，更像高位分歧、退潮或弱修复。"}


def overall_view(scores: dict[str, dict[str, Any]], stage: str, staleness_days: int | None) -> dict[str, Any]:
    base_items = [
        scores["price"]["score"],
        scores["flow"]["score"],
        scores["narrative"]["score"],
        scores["breadth"]["score"],
        100.0 - scores["crowding"]["score"],
    ]
    if scores["compare"]["coverage"] > 0:
        base_items.append(scores["compare"]["score"])
    composite = round(sum(base_items) / len(base_items), 1)

    if staleness_days is not None and staleness_days > 3:
        conclusion = "这份结果不能再叫“当前动量分析”，最多只能作为背景参考。"
    elif stage == "加速" and scores["crowding"]["score"] < 65:
        conclusion = "当前动量仍有参与价值，但应优先选择结构更完整、资金确认更强的细分方向。"
    elif stage == "拥挤":
        conclusion = "当前依然强，但风险收益比开始恶化，更适合等分歧或择强持有，不适合无差别追高。"
    elif stage == "启动":
        conclusion = "当前刚进入可跟踪区间，适合继续观察近 1 到 3 日资金和扩散能否跟上。"
    else:
        conclusion = "当前不适合把强弱解读成可持续主升，除非后续重新出现资金和结构共振。"

    if scores["breadth"]["score"] < 50 and scores["price"]["score"] >= 60:
        conclusion += " 当前更像龙头行情，不是完整板块行情。"
    if scores["compare"]["coverage"] == 0:
        conclusion += " 缺少横向排序数据，因此“值不值得投”的判断要更保守。"

    return {"composite_score": composite, "conclusion": conclusion}


def build_answers(payload: dict[str, Any], scores: dict[str, dict[str, Any]], stage: dict[str, Any], staleness_days: int | None) -> dict[str, str]:
    price = payload.get("price", {})
    flow = payload.get("flow", {})
    narrative = payload.get("narrative", {})
    breadth = payload.get("breadth", {})
    crowding = payload.get("crowding", {})
    compare = payload.get("compare", {})

    price_answer = (
        f"{'是' if scores['price']['score'] >= 60 else '还不够稳'}。"
        f" 今日涨幅 {fmt_pct(read_number(price, 'ret_1d'))}，近 3 日涨幅 {fmt_pct(read_number(price, 'ret_3d'))}，"
        f"近 3 日相对超额 {fmt_pct(read_number(price, 'rs_3d'))}。"
        f" 20 日收益 {fmt_pct(read_number(price, 'ret_20d'))} 只作为背景，当前价格分 {scores['price']['score']:.1f}/100。"
    )
    flow_answer = (
        f"{'有' if scores['flow']['score'] >= 60 else '还不够充分'}。"
        f" 今日净流入 {fmt_pct(read_number(flow, 'net_flow_1d_pct'))}，近 3 日净流入 {fmt_pct(read_number(flow, 'net_flow_3d_pct'))}，"
        f"近 3 日正流入占比 {fmt_pct(read_number(flow, 'positive_flow_days_ratio_3d'), 0)}，资金分 {scores['flow']['score']:.1f}/100。"
    )
    narrative_answer = (
        f"{'有相对统一的理由' if scores['narrative']['score'] >= 60 else '统一叙事仍不足'}。"
        f" 近 3 日催化数量 {int(read_number(narrative, 'catalyst_count_3d') or 0)}，"
        f"一致性 {fmt_pct(read_number(narrative, 'thesis_alignment'), 0)}，升级强度 {fmt_pct(read_number(narrative, 'thesis_upgrade_score'), 0)}，"
        f"叙事分 {scores['narrative']['score']:.1f}/100。"
    )
    structure_answer = (
        f"{'更偏板块扩散' if scores['breadth']['score'] >= 60 else '更像龙头独舞'}。"
        f" 今日上涨占比 {fmt_pct(read_number(breadth, 'advancers_ratio_1d'), 0)}，"
        f"近 3 日上涨占比 {fmt_pct(read_number(breadth, 'advancers_ratio_3d'), 0)}，"
        f"龙头贡献度 {fmt_pct(read_number(breadth, 'leader_contribution_pct'), 0)}，结构分 {scores['breadth']['score']:.1f}/100。"
    )
    stage_answer = f"当前更接近“{stage['name']}”阶段。{stage['explanation']}"
    risk_answer = (
        f"{'是，已经有明显过热迹象' if scores['crowding']['score'] >= 70 else '还没完全交易过头，但风险在抬升' if scores['crowding']['score'] >= 55 else '暂未看到明显交易过头'}。"
        f" 近 3 日脉冲 {fmt_pct(read_number(crowding, 'short_term_spike_3d'))}，"
        f"估值分位 {fmt_pct(read_number(crowding, 'valuation_percentile_5y'), 0)}，"
        f"风险分 {scores['crowding']['score']:.1f}/100。"
    )

    compare_answer = "缺失"
    if scores["compare"]["coverage"] > 0:
        compare_answer = (
            f"横向比较位于候选行业的 {fmt_pct(read_number(compare, 'peer_overall_percentile'), 0)} 分位，"
            f"价格/资金/叙事/扩散分位分别为 "
            f"{fmt_pct(read_number(compare, 'peer_price_percentile'), 0)} / "
            f"{fmt_pct(read_number(compare, 'peer_flow_percentile'), 0)} / "
            f"{fmt_pct(read_number(compare, 'peer_narrative_percentile'), 0)} / "
            f"{fmt_pct(read_number(compare, 'peer_breadth_percentile'), 0)}。"
        )

    answers = {
        "价格上，它是不是在持续走强？": price_answer,
        "资金上，有没有增量和持续性？": flow_answer,
        "叙事上，市场有没有统一理由去交易它？": narrative_answer,
        "结构上，是龙头独舞还是板块扩散？": structure_answer,
        "阶段上，现在是启动、加速、拥挤还是退潮？": stage_answer,
        "风险上，这个强势是不是已经被交易过头？": risk_answer,
    }
    if scores["compare"]["coverage"] > 0:
        answers["横向比较上，它在当前候选行业里处于什么位置？"] = compare_answer
    if staleness_days is not None and staleness_days > 3:
        answers["当前性提示"] = "数据已经超过 3 天，结论不能直接当作当前判断。"
    return answers


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
        f"- 时点：{meta.get('as_of_time', '未提供')}",
        f"- 市场：{meta.get('market', '未提供')}",
        f"- 基准：{meta.get('benchmark', '未提供')}",
        f"- 当前性：{result['currentness']}",
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
        ("breadth", "扩散"),
        ("crowding", "拥挤风险"),
        ("compare", "横向比较"),
    ]
    for key, title in module_order:
        item = scores[key]
        lines.append(f"| {title} | {item['score']} | {item['label']} | {item['coverage']} |")

    if result["positives"]:
        lines.extend(["", "## 主要正向信号"])
        for item in result["positives"]:
            lines.append(f"- {item}")
    if result["risks"]:
        lines.extend(["", "## 主要风险"])
        for item in result["risks"]:
            lines.append(f"- {item}")

    return "\n".join(lines)


def main() -> None:
    parser = argparse.ArgumentParser(description="Score current industry momentum from structured JSON input.")
    parser.add_argument("input_path", help="Path to the JSON input file.")
    parser.add_argument("--format", choices=["markdown", "json"], default="markdown")
    args = parser.parse_args()

    payload = json.loads(Path(args.input_path).read_text())
    staleness_days = parse_staleness(payload.get("meta", {}))

    scores = {
        "price": score_price(payload.get("price", {})),
        "flow": score_flow(payload.get("flow", {})),
        "narrative": score_narrative(payload.get("narrative", {})),
        "breadth": score_breadth(payload.get("breadth", {})),
        "crowding": score_crowding(payload.get("crowding", {})),
        "compare": score_compare(payload.get("compare", {})),
    }
    stage = classify_stage(scores, payload)
    overall = overall_view(scores, stage["name"], staleness_days)
    answers = build_answers(payload, scores, stage, staleness_days)

    module_coverages = [scores[key]["coverage"] for key in ("price", "flow", "narrative", "breadth", "crowding")]
    base_coverage = sum(module_coverages) / len(module_coverages)
    confidence = confidence_from_coverage(base_coverage, staleness_days, scores["compare"]["coverage"])

    positives = []
    risks = []
    for key in ("price", "flow", "narrative", "breadth"):
        positives.extend(scores[key]["signals"])
        risks.extend(scores[key]["warnings"])
    positives.extend(scores["crowding"]["signals"])
    risks.extend(scores["crowding"]["warnings"])
    positives.extend(scores["compare"]["signals"])
    risks.extend(scores["compare"]["warnings"])
    if staleness_days is not None and staleness_days > 3:
        risks.insert(0, "数据日期已超过 3 天，这份分析不应再当作“当前”依据。")

    result = {
        "meta": payload.get("meta", {}),
        "scores": scores,
        "stage": stage,
        "overall": overall,
        "answers": answers,
        "positives": positives[:8],
        "risks": risks[:8],
        "confidence": confidence,
        "coverage": round(base_coverage, 2),
        "staleness_days": staleness_days,
        "currentness": currentness_label(staleness_days),
    }

    if args.format == "json":
        print(json.dumps(result, ensure_ascii=False, indent=2))
    else:
        print(render_markdown(result))


if __name__ == "__main__":
    main()
