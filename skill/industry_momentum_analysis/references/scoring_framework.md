# Scoring Framework

这个版本把“当前动量”定义为今天到近 `3` 个交易日内的强化或衰减状态。`20d/60d` 不是当前本身，而是背景上下文。

## 时间框架

- 核心层：`1d`、`3d`
- 辅助层：`5d`、`10d`
- 背景层：`20d`、`60d`、`120d`

脚本的主要模块会优先给 `1d/3d` 更高权重。没有今天数据时，最多允许退到近 `3` 个交易日；如果 `as_of` 距今天超过 `3` 天，脚本会把“当前性”降级。

## 输入结构

```json
{
  "meta": {},
  "price": {},
  "flow": {},
  "narrative": {},
  "breadth": {},
  "crowding": {},
  "compare": {}
}
```

## meta

- `sector`: 行业或主题名称
- `market`: `CN`、`HK`、`US`、`GLOBAL`
- `benchmark`: 对比基准
- `as_of`: 数据日期，`YYYY-MM-DD`
- `as_of_time`: 可选，盘中时点
- `basket_note`: 行业口径说明

## price

### 当前强弱

- `ret_1d`
- `ret_3d`
- `rs_1d`: 相对基准 1 日超额
- `rs_3d`
- `close_strength_1d`: 收盘强度或盘中收在高位的强度，`0` 到 `1`
- `breakout`: 是否刚突破关键平台

### 平滑度和延续性

- `up_day_ratio_5d`
- `max_drawdown_3d`
- `max_drawdown_20d`
- `trend_smoothness_5d`: 收益/波动类指标，`0` 到 `1`

### 背景位置

- `ret_5d`
- `ret_20d`
- `ret_60d`
- `distance_to_breakout_level`
- `distance_to_52w_high`

## flow

### 成交额与换手率扩张

- `turnover_ratio_1d`
- `turnover_ratio_3d`
- `volume_ratio_1d`
- `volume_ratio_3d`

### 净流入持续性

- `net_flow_1d_pct`
- `net_flow_3d_pct`
- `positive_flow_days_ratio_3d`
- `net_flow_acceleration`: 最近 `3d` 相比此前 `3d` 的边际改善，`-1` 到 `1`

### 不同资金类型确认

- `etf_flow_confirm_score`: `0` 到 `1`
- `northbound_confirm_score`: `0` 到 `1`
- `margin_confirm_score`: `0` 到 `1`

### 龙头与跟风结构

- `leader_flow_share_pct`
- `follower_participation_ratio`
- `pullback_resilience_score`: 回调日资金承接强度，`0` 到 `1`

## narrative

### 频率层

- `mention_burst_3d`: 最近 `3d` 话题热度相对过去区间的抬升，`0` 到 `1`
- `catalyst_count_3d`

### 一致性层

- `thesis_alignment`
- `source_diversity_score`

### 升级层

- `thesis_upgrade_score`: 叙事是否从概念升级到订单、盈利、景气或政策兑现，`0` 到 `1`
- `earnings_revision_score`: `-1` 到 `1`
- `narrative_freshness`

## breadth

- `advancers_ratio_1d`
- `advancers_ratio_3d`
- `median_ret_1d`
- `median_ret_3d`
- `layer_diffusion_score`: 龙头、中军、二线、小票的扩散情况，`0` 到 `1`
- `subtheme_resonance_ratio`: 子方向共振比例，`0` 到 `1`
- `non_leader_participation_ratio`: 非龙头参与率，`0` 到 `1`
- `leader_contribution_pct`

## crowding

- `valuation_percentile_5y`
- `short_term_spike_3d`
- `short_term_spike_10d`
- `rsi_14`
- `turnover_share_percentile_1y`: 行业成交额占全市场比例在过去 1 年中的分位
- `narrative_crowding_score`
- `second_line_mania_score`: 二线杂毛补涨和题材泛化强度，`0` 到 `1`
- `event_risk_score`

## compare

这是可选模块，用于跨行业排序。

- `peer_overall_percentile`
- `peer_price_percentile`
- `peer_flow_percentile`
- `peer_narrative_percentile`
- `peer_breadth_percentile`

## 模块评分

脚本会输出：

- `price`
- `flow`
- `narrative`
- `breadth`
- `crowding`
- `compare`

其中：

- `price/flow/narrative/breadth/compare` 越高越强
- `crowding` 越高越危险

## 阶段识别

阶段不是总分映射，而是规则判断：

- `启动`：当前 `1d/3d` 转强，但资金和扩散还没完全确认
- `加速`：当前价格、资金、叙事、扩散同时走强，拥挤尚未失控
- `拥挤`：仍强，但二线泛化、短期脉冲、估值和交易热度都偏高
- `退潮`：当前脉冲减弱，扩散收缩，资金承接转弱

## 当前性约束

脚本会检查 `as_of` 到系统当天的差值：

- `0-1` 天：当前性正常
- `2-3` 天：当前性下降，但仍可勉强视作当前
- 超过 `3` 天：自动提示“不能称为当前动量分析”

## 置信度

置信度由三部分决定：

- 字段覆盖度
- 数据日期新鲜度
- 横向比较是否具备

即使字段很全，只要日期过旧，最终置信度也必须下调。
