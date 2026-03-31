# 校验协议详解（Validation Protocol）

本文档是 `hypothesis-validator` skill 四层校验的详细操作规范。
Agent 执行校验时，必须严格按本协议操作，不得跳过任何一层。

---

## 1. 时间校验（Temporal Validation）

### 1.1 目的

判断假设中的候选原因，是否在时间上能够解释观测到的异常。

### 1.2 规则（硬约束，不交给 LLM）

#### R-T1：因果时序

```
IF cause_time > effect_start_time THEN temporal_fit = 0, verdict = "FAIL: cause after effect"
```

候选原因必须发生在异常开始之前。这是不可违反的物理约束。

#### R-T2：合理时延

```
delta = effect_start_time - cause_time
IF delta > max_reasonable_lag THEN temporal_fit -= 0.4
IF delta < 0 THEN temporal_fit = 0  (same as R-T1)
```

`max_reasonable_lag` 按场景设定：

| 场景类型 | 建议 max_lag |
|----------|-------------|
| 发版后异常 | 10 min |
| 配置切换 | 3 min |
| 扩缩容 | 5 min |
| 请求链路问题 | 30 sec |
| 定时任务触发 | 与 cron 周期对齐 |

#### R-T3：变化点邻近

从 metrics 时序中识别异常开始的变化点（change point），检查其附近（±5min 窗口）是否存在已知事件：

- deploy
- config change
- scaling event
- job/task start
- upstream incident

```
nearby_events = events WHERE abs(event_time - change_point) < window
IF len(nearby_events) > 0 AND hypothesis NOT IN nearby_events THEN temporal_fit -= 0.3
IF hypothesis IN nearby_events THEN temporal_fit += 0.2
```

#### R-T4：时间窗重叠

候选原因的活跃时间窗口应覆盖异常的持续时间。

```
overlap = intersect(cause_active_window, anomaly_window)
overlap_ratio = overlap / anomaly_duration
temporal_fit += 0.3 * overlap_ratio
```

### 1.3 评分

`temporal_fit` 取值 `[0.0, 1.0]`：

| 分数 | 含义 |
|------|------|
| 0.0–0.2 | 时间上几乎不可能 |
| 0.2–0.5 | 时间上勉强，需要额外证据 |
| 0.5–0.8 | 时间上合理 |
| 0.8–1.0 | 时间上高度吻合 |

### 1.4 不足时

如果缺少关键时间戳（如 deploy time、change point 无法识别），输出：

```json
{
  "temporal_fit": null,
  "temporal_verdict": "insufficient_evidence",
  "missing": ["deploy_time", "change_point"]
}
```

---

## 2. 量级校验（Magnitude Validation）

### 2.1 目的

判断假设所能解释的影响量级，是否与观测到的指标变化量级匹配。

### 2.2 规则（硬约束）

#### R-M1：预期影响上界

根据假设类型，查询经验上界表：

| 假设类型 | 预期影响上界 |
|----------|-------------|
| 单条 SQL 退化 | 单请求 RT +50%~200%，全局 CPU +1%~5% |
| 代码逻辑小 diff | CPU +3%~8% |
| 缓存失效/穿透 | RT ×2~×10，CPU +20%~50% |
| Pod 数 ×N + 并发任务 | 负载 ≈ ×N（可能超线性） |
| 限流关闭/配置切换 | 可导致阶跃突变 |
| 上游抖动 + 重试风暴 | RT ×5~×20，CPU +30%~80% |
| 连接池耗尽 | 请求阻塞，RT 飙升，CPU 可能不高 |

#### R-M2：观测影响计算

```
observed_magnitude = metric_after - metric_before  (或 ratio = metric_after / metric_before)
```

使用异常前稳态窗口（建议取 change_point 前 15min 均值）作为 baseline。

#### R-M3：数量级比对

```
IF expected_max < observed_magnitude * 0.1 THEN magnitude_fit = 0.1, verdict = "量级严重不足"
IF expected_max < observed_magnitude * 0.3 THEN magnitude_fit = 0.3, verdict = "量级不足，最多为次要因子"
IF expected_max >= observed_magnitude * 0.5 THEN magnitude_fit = 0.6+, verdict = "量级可解释"
```

#### R-M4：乘法放大器识别

检查是否存在放大因子：

- Pod 数变化
- 并发任务数变化
- 重试次数变化
- 扇出请求数变化

```
amplification_factor = new_pods / old_pods * concurrent_tasks
adjusted_expected = base_expected * amplification_factor
```

### 2.3 评分

`magnitude_fit` 取值 `[0.0, 1.0]`：

| 分数 | 含义 |
|------|------|
| 0.0–0.2 | 量级完全不匹配，差一个数量级以上 |
| 0.2–0.4 | 量级不足，只能当次要因子 |
| 0.4–0.7 | 量级部分匹配，需要其他因素叠加 |
| 0.7–1.0 | 量级充分匹配 |

### 2.4 不足时

如果缺少 metrics baseline 或无法估算预期影响：

```json
{
  "magnitude_fit": null,
  "magnitude_verdict": "insufficient_evidence",
  "missing": ["metrics_baseline", "expected_impact_estimate"]
}
```

---

## 3. 范围校验（Scope Validation）

### 3.1 目的

判断假设解释的影响范围，是否与异常的空间分布匹配。

### 3.2 范围分类

| 级别 | 范围 | 典型原因 |
|------|------|---------|
| L1 | 单请求 / 单用户 | 特定参数、数据问题 |
| L2 | 单 Pod / 单机 | 资源竞争、本地配置 |
| L3 | 部分 Pod / 部分机器 | 灰度发布、部分批次 |
| L4 | 全量 Pod / 全 fleet | 全量发版、全局配置、共享依赖 |
| L5 | 跨服务 / 跨集群 | 基础设施、DNS、网络 |

### 3.3 规则

#### R-S1：范围识别

从证据包中识别：

```
phenomenon_scope = 受影响实体的范围级别 (L1-L5)
hypothesis_scope = 假设能影响的范围级别 (L1-L5)
```

#### R-S2：匹配判定

```
IF phenomenon_scope > hypothesis_scope + 1 THEN scope_fit -= 0.4, note = "现象比假设广太多"
IF phenomenon_scope < hypothesis_scope - 1 THEN scope_fit -= 0.2, note = "假设范围比现象广，需解释为何只局部"
IF abs(phenomenon_scope - hypothesis_scope) <= 1 THEN scope_fit += 0.3
```

#### R-S3：同步性检查

如果多实体同步出现异常：

```
IF multiple_entities_simultaneous AND hypothesis_is_local THEN scope_fit -= 0.3
IF multiple_entities_simultaneous AND hypothesis_is_systemic THEN scope_fit += 0.2
```

### 3.4 评分

`scope_fit` 取值 `[0.0, 1.0]`：

| 分数 | 含义 |
|------|------|
| 0.0–0.3 | 范围严重不匹配 |
| 0.3–0.6 | 范围部分匹配 |
| 0.6–1.0 | 范围高度匹配 |

---

## 4. 反事实校验（Counterfactual Validation）

### 4.1 目的

检验：如果这个假设为真，世界还应该呈现什么样子？

### 4.2 流程（由 LLM 执行）

#### Step 1：生成反事实预测

对每个假设，生成 3–5 个可验证的预测：

> 如果 H 为真，那么：
> - P1: 应该看到 ...
> - P2: 不应该看到 ...
> - P3: ... 应该与 ... 相关
> - P4: 在 ... 之前应该有 ...

#### Step 2：证据查证

逐个预测，在证据包中搜索支持或反对证据：

```
对每个预测 Pi:
  - 找到支持证据 → hit
  - 找到反对证据 → miss
  - 无法确认 → unknown
```

#### Step 3：统计命中

```
counterfactual_fit = hits / (hits + misses + 0.5 * unknowns)
```

### 4.3 反事实模板

以下是常见假设类型的反事实预测模板：

#### 代码变更导致性能问题

- 应该看到特定路径/SQL 的 RT 显著增长
- 变更前不应有同级异常
- 影响范围应限于变更涉及的请求路径
- 回滚后异常应消失

#### 扩缩容导致负载突变

- 负载变化应与 Pod 数变化时间对齐
- 负载增幅应与 Pod 数增幅近似同步
- 如果有并发任务，task count 应同步增加
- 影响面应是全量 Pod

#### 上游/依赖异常

- 上游的异常应先于本服务异常
- 受影响的请求应集中在依赖该上游的路径
- 其他不依赖该上游的路径不受影响
- 上游恢复后本服务应恢复

#### 配置切换

- 配置变更时间应略早于异常开始
- 影响应覆盖配置生效范围
- 未受配置影响的实例不应有异常
- 回滚配置后异常应消失

### 4.4 评分

`counterfactual_fit` 取值 `[0.0, 1.0]`，基于命中率。

---

## 5. 综合决策矩阵

### 5.1 综合分数

```
score = 0.25 * temporal_fit + 0.30 * magnitude_fit + 0.25 * scope_fit + 0.20 * counterfactual_fit
```

量级权重最高（0.30），因为 **"解释不了量级"是最常见的假阳性来源**。

### 5.2 决策规则

```
IF any layer == null (insufficient_evidence):
  decision = "insufficient_evidence"

IF temporal_fit == 0:
  decision = "reject_as_primary_cause"  # 时间约束是硬约束

IF magnitude_fit < 0.2 AND scope_fit < 0.3:
  decision = "reject_as_primary_cause"  # 量级+范围双不足

IF score >= 0.7:
  decision = "promote_to_primary"

IF score >= 0.4:
  decision = "retain_as_candidate"

ELSE:
  decision = "reject_as_primary_cause"
```

### 5.3 阈值可调

以上阈值是初始建议值。在实际使用中应根据 case library 的表现不断校准。

---

## 6. 执行注意事项

### 6.1 顺序不可跳过

必须按 时间 → 量级 → 范围 → 反事实 的顺序执行。如果时间校验直接否决（fit = 0），后续校验仍需执行但权重降为参考。

### 6.2 多假设并行

多个假设可并行校验，但最终比较时必须使用相同的基线和证据包。

### 6.3 世界扩展触发

如果任一层返回 `insufficient_evidence`，Validator 必须输出 `next_worlds_to_query`，典型值包括：

- `deploy` — 发布历史
- `scaling` — 扩缩容记录
- `job` — 任务调度记录
- `config` — 配置变更
- `upstream` — 上游服务状态
- `network` — 网络指标
- `metrics_baseline` — 异常前基线指标

### 6.4 避免确认偏误

Validator 应对所有假设一视同仁。不能因为某个假设来自"高级工程师"或"代码 review"就降低审查标准。

