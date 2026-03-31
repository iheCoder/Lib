---
name: hypothesis-validator
description: 因果合理性审查器。当排障流程已生成一个或多个候选假设，且需要判断这些假设是否站得住时激活。通过时间校验、量级校验、范围校验和反事实校验四层审判，淘汰"听起来像但解释不了现象"的假设，而非自由发挥找答案。依赖 evidence_builder 提供的结构化证据包。
---

# Hypothesis Validator

> **不是"再猜一次根因"，而是审判当前假设是否站得住。**

## 定位

Hypothesis Validator 是一个 **因果合理性审查器**。
它不负责发现答案，它负责淘汰那些"听起来像、但实际上解释不了现象"的答案。

核心审判准则：

> 凡是一个假设解释不了 **"为什么是现在、为什么这么大、为什么这么广"**，它就不配当主因。

## 何时使用

- 排障流程已产出一个或多个候选假设
- 需要在多个假设间做优先级排序
- 需要判断某个假设是否足以当主因
- 需要决定是否继续扩展世界（拉取更多证据源）

## 不做什么

- ❌ 重新读一遍所有材料自由发挥
- ❌ 直接代替 LLM 生成漂亮报告
- ❌ 对每个假设做玄学打分
- ❌ 一上来就试图证明谁是根因
- ❌ 在证据不足时给出唯一根因

## 输入契约

调用 Validator 前，必须准备好以下信息：

### 1. 假设（Hypothesis）

```json
{
  "id": "h1",
  "text": "代码某处低效导致 DB CPU 升高",
  "source": "code_review",
  "proposed_cause_time": "2024-03-15T10:20:00Z",
  "proposed_scope": "single_request_path",
  "proposed_magnitude": "cpu_increase_5pct"
}
```

### 2. 证据包（Evidence Pack）

来自 `evidence_builder` 的结构化证据，每条带有 `source_type`、`ts_start`/`ts_end`、`entity_refs`、`confidence`、`layer`、`tags`。详见 [references/evidence-pack-schema.md](references/evidence-pack-schema.md)。

### 3. 世界摘要（World Summaries）

与假设相关的上下文信息：

- **deploy**: 发布历史（时间、变更内容、影响范围）
- **scaling**: 扩缩容记录（pod 数变化、时间点）
- **job**: 定时任务/全量任务调度记录
- **config**: 配置变更记录
- **metrics**: 关键指标时序快照（CPU、QPS、RT、错误率）

## 工作流：四层校验

**严格按顺序执行**，每一层的结果决定是否继续深入。

### 第 1 层：时间校验（Temporal Validation）

**最先做，因为最硬。**

规则检查（不交给 LLM）：

1. **因果时序**：`cause_time ≤ effect_time`，否则直接否决
2. **合理时延**：`effect_time - cause_time < max_reasonable_lag`（根据场景设定，通常 ≤ 5min）
3. **变化点邻近**：异常开始时间附近是否有 deploy/config/scale/job 事件
4. **时间窗重叠**：候选原因的活跃窗口是否覆盖异常持续时间

可用脚本辅助：

```bash
python3 scripts/validate_hypothesis.py --check temporal --input evidence.json
```

输出：`temporal_fit` 分数 (0.0–1.0) + 具体判定理由。

### 第 2 层：量级校验（Magnitude Validation）

**不是精确建模，而是量纲 sanity check。**

规则检查：

1. **预期影响上界**：该假设理论上最多带来多大影响？
2. **观测影响量级**：实际指标变化有多大？
3. **数量级比对**：两者是否在同一数量级？

经验上界参考：

| 假设类型 | 典型影响上界 |
|----------|-------------|
| 单次 SQL 变差 | 单请求成本上升，不会凭空让 fleet-wide CPU 翻倍 |
| 代码小 diff | CPU +3%~5% |
| Pod 数 N 倍 + 全量任务 | 负载近似 N 倍放大 |
| 配置切换（限流/缓存关闭） | 可导致全局突变 |

如果预期影响 << 观测影响（差一个数量级以上），标记：

> **量级不足，不足以解释主异常**

可用脚本辅助：

```bash
python3 scripts/validate_hypothesis.py --check magnitude --input evidence.json
```

### 第 3 层：范围校验（Scope Validation）

**判断假设解释的范围与现象的范围是否匹配。**

规则检查：

1. **现象范围**：单机 / 单 pod / 单请求 / 整个 fleet / 整批 pod？
2. **假设解释范围**：局部（某代码路径）还是全局（发版/扩容/配置切换）？
3. **匹配度**：
   - 现象广 + 解释窄 → 优先级下降
   - 现象窄 + 解释广 → 需要额外证据解释为何只影响局部
   - 范围匹配 → 优先级上升

可用脚本辅助：

```bash
python3 scripts/validate_hypothesis.py --check scope --input evidence.json
```

### 第 4 层：反事实校验（Counterfactual Validation）

**这是最强的一层，交给 LLM 做。**

核心问题：

> **如果这个假设是真的，我们还应该看到什么？**

步骤：

1. 为当前假设生成 3–5 个反事实预测
2. 逐个去证据包中查找是否有支持/反对证据
3. 统计命中率

示例：

| 假设 | 反事实预测 | 证据是否支持 |
|------|-----------|-------------|
| 代码小问题导致 DB CPU 爆满 | 应看到某些特定 SQL 耗时显著拉长 | ❌ 未观测到 |
| 代码小问题导致 DB CPU 爆满 | 影响应偏局部 | ❌ 实际是全局 |
| 8 pod 并发全量任务 | 发版/扩容时间与 CPU 抬升接近 | ✅ 时间吻合 |
| 8 pod 并发全量任务 | 负载上升与 pod 数变化近似同步 | ✅ 成比例 |

## 输出契约

每个假设的验证结果，必须输出如下结构化 JSON：

```json
{
  "hypothesis": "代码小问题导致 DB CPU 爆满",
  "temporal_fit": 0.35,
  "magnitude_fit": 0.10,
  "scope_fit": 0.20,
  "counterfactual_notes": [
    "无法解释为何异常在发版后立刻出现",
    "无法解释为何 8 个 pod 同步异常",
    "预期影响量级明显小于观测量级"
  ],
  "decision": "reject_as_primary_cause",
  "confidence": 0.85,
  "next_worlds_to_query": ["deploy", "scaling", "job"]
}
```

### Decision 枚举

| 值 | 含义 |
|----|------|
| `reject_as_primary_cause` | 此假设不足以当主因，最多是边际因子 |
| `retain_as_candidate` | 此假设通过初步校验，值得继续深入 |
| `promote_to_primary` | 此假设在四层校验中均获得强支持 |
| `insufficient_evidence` | 当前证据不足以判定，需要扩展世界 |

### 何时输出 `insufficient_evidence`

- 时间线不完整（关键事件时间缺失）
- 量级估计缺失（无 metrics 对比基线）
- 世界还没扩展到 deploy/config/job
- 证据可信度普遍偏低

此时必须同时输出 `next_worlds_to_query`，告诉上游应该去拉取什么新证据。

## 多假设比较

当有多个假设时：

1. 对每个假设独立跑四层校验
2. 按综合分数排序
3. 输出比较矩阵
4. 如果最优假设和次优假设分差很小，明确标注"尚无法区分"

综合分数建议权重（可调）：

```
score = 0.25 * temporal_fit + 0.30 * magnitude_fit + 0.25 * scope_fit + 0.20 * counterfactual_fit
```

量级权重最高，因为"解释不了量级"是最常见的假阳性来源。

## 安全约束

1. **不够证据时允许"不判"** — 永远不要因为被催促而降低标准
2. **不允许只因为看到代码 diff 就下主因结论** — 必须通过量级和范围校验
3. **不允许忽略时间线** — 时间先后是最硬的约束
4. **不允许把局部解释直接套到全局异常上** — 范围不匹配必须被标记

## 混合校验器架构

| 层 | 方法 | 负责什么 |
|----|------|---------|
| 规则校验 | 程序化 / 脚本 | 时间先后、同步性、范围匹配、缺失证据、change point 邻近 |
| 统计校验 | 程序化 / 脚本 | 均值对比、倍率计算、change point detection |
| LLM 校验 | 模型推理 | 反事实生成、多源证据整合、可读结论、标注 unknowns |

> **Validator 不是一个模型，而是一套审判流程。**

## 脚本

- `scripts/validate_hypothesis.py` — 规则+统计校验辅助工具，详见 [references/validation-protocol.md](references/validation-protocol.md)

## 参考文档

- 校验协议详解：[references/validation-protocol.md](references/validation-protocol.md)
- 证据包 Schema：[references/evidence-pack-schema.md](references/evidence-pack-schema.md)
- 完整案例演练：[references/examples.md](references/examples.md)
- 设计理念原文：[references/design-rationale.md](references/design-rationale.md)

