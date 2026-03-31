---
name: evidence-builder
description: 证据构建器。当需要从原始观测数据（日志、trace、metrics、工单、代码、配置、发布记录等）中抽取结构化事实时激活。把杂乱的跨源带噪声原始观测，转成可引用、可比较、可打分、可挂到假设上的证据对象。不做推理，只做事实提取。
---

# Evidence Builder

> **把"看到的东西"变成"事实"，但不做推理。**

## 定位

Evidence Builder 是一个 **事实层构建器**。
它不负责解释为什么出了问题，它负责把混乱的原始数据变成结构化的、可信度标注的、可被审判的证据。

核心原则：

> 证据构建必须先于开放式推理。不要让 LLM 直接面对原始世界。

## 何时使用

- 收到工单/告警/用户反馈/bug/错误，需要初步采集事实
- Hypothesis Validator 返回 `insufficient_evidence`，要求扩展世界
- 发现新的数据源需要纳入证据体系
- 需要刷新/补充现有证据包

## 不做什么

- ❌ 对证据做推理或解释
- ❌ 猜测根因
- ❌ 自行决定"该看哪里"（这是 Validator 的职责）
- ❌ 把原始文本直接当证据（必须结构化）
- ❌ 遗漏"没看到"这类关键缺失事实

## 六类证据（第一版聚焦）

### 1. 时间事实（Temporal Facts）

从日志、trace、metrics 中提取关键时间点。

```json
{
  "id": "ev-01",
  "source_type": "log",
  "fact_text": "client signed_at=14:00:00, gateway receive=14:00:30, delta=30s",
  "ts_start": "2024-03-15T14:00:00Z",
  "ts_end": "2024-03-15T14:00:30Z",
  "layer": "client",
  "confidence": 0.85,
  "tags": ["anomaly"]
}
```

关键：不仅记录单个时间点，还要计算和记录 **时间差**。

### 2. 缺失事实（Absence Facts）

**"没看到"在排障中常常比"看到了"更关键。**

```json
{
  "id": "ev-02",
  "source_type": "log",
  "fact_text": "upstream 在 14:00:00-14:01:00 窗口内无 request_id=xxx 的处理记录",
  "layer": "upstream",
  "confidence": 0.90,
  "tags": ["anomaly", "absence"]
}
```

必须主动检查：
- 上游有没有收到请求？
- 某时间窗内有没有类似异常？
- trace 中有没有缺失的 span？

### 3. 状态事实（State Facts）

系统返回的明确状态或错误。

```json
{
  "id": "ev-03",
  "source_type": "log",
  "fact_text": "gateway 拒绝原因: timestamp_expired, skew_delta=30s",
  "layer": "gateway",
  "confidence": 0.95,
  "tags": ["anomaly"]
}
```

### 4. 阶段事实（Phase Facts）

把一个请求/操作拆成多个阶段，标注每个阶段的耗时。

```json
{
  "id": "ev-04",
  "source_type": "log",
  "fact_text": "before_send(14:00:00) → socket_write_begin(14:00:28), 阶段间停顿 28s",
  "layer": "app",
  "confidence": 0.85,
  "tags": ["anomaly"]
}
```

**这是区分"晚发"和"晚到"的关键。**

### 5. 背景事实（Context Facts）

同时间窗内其他相关指标/事件。

```json
{
  "id": "ev-05",
  "source_type": "metric",
  "fact_text": "同时间窗内其他请求无类似 expired 异常，gateway reject rate 未整体上升",
  "layer": "gateway",
  "confidence": 0.75,
  "tags": ["context"]
}
```

### 6. 观测可信度事实（Trustworthiness Facts）

**证据本身可能是不可信的。**

```json
{
  "id": "ev-06",
  "source_type": "trace",
  "fact_text": "trace 中 client span 和 gateway span 时间存在 clock skew 风险（NTP offset 未知）",
  "layer": "observability",
  "confidence": 0.60,
  "tags": ["trust_warning"]
}
```

必须主动检查：
- 时钟是否同步？
- trace 中有没有 span 缺失？
- 日志时间戳是否可能有时区问题？

## 工作流

### Phase A：接收采集指令

输入来源有两种：

1. **初始采集**：收到工单/症状描述，从头开始
2. **扩展采集**：Validator 返回 `next_worlds_to_query`，按指令采集特定世界

```
初始采集: symptom → 全部数据源扫描
扩展采集: ["deploy", "scaling", "job"] → 只采集指定世界
```

### Phase B：实体与时间窗提取

从症状/工单中提取：

1. **实体**：服务名、Pod名、DB名、接口路径、request_id、trace_id
2. **时间窗**：异常首次出现时间、持续时间、相关时间点
3. **环境**：集群、命名空间、区域

### Phase C：多源证据采集

**通过 Capability Pool 进入具体世界。**

Evidence Builder 自身不知道"怎么查 trace"或"怎么看部署记录"。
它通过 Shadow Claw 提供的能力（来自 Capability Pool）访问每个世界。

```
Shadow Claw: "请采集 trace 世界，用 sls-trace-query 能力"
    |
    v
Evidence Builder:
    1. 调用 sls-trace-query (aliyun-sls-trace skill)
    2. 传入 entity_refs + time_window
    3. 收到原始 trace 数据
    4. 结构化为 evidence_items
    5. 追加到 Evidence Pack
```

按数据源逐个采集：

| 数据源 | 对应世界 | Capability Pool 中的能力 | 产出类型 |
|--------|---------|------------------------|---------|
| 工单/告警 | ticket | (直接读取) | 状态事实 + 时间事实 |
| 应用日志 | log | sls-trace-query, local-log-grep | 时间事实 + 阶段事实 + 状态事实 |
| 分布式追踪 | trace | sls-trace-query | 时间事实 + 阶段事实 + 可信度事实 |
| 监控指标 | metric | prometheus-query | 时间事实 + 背景事实 |
| 发布记录 | deploy | git-recent-commits, kubectl-events | 时间事实 + 背景事实 |
| 配置变更 | config | sls-config-resolve, config-diff | 时间事实 + 背景事实 |
| 扩缩容记录 | scaling | kubectl-pod-history, kubectl-hpa | 时间事实 + 背景事实 |
| 代码变更 | code | git-diff | 背景事实 |
| 网络指标 | network | network-stats | 背景事实 + 可信度事实 |
| 数据库 | database | mysql-readonly-query | 状态事实 + 背景事实 |

**当某个世界没有对应能力时**，生成一条 `capability_gap` 证据而不是静默跳过。

### Phase D：结构化与标注

对每条原始数据，执行：

1. **结构化**：转为标准 evidence item JSON
2. **时间标注**：提取 ts_start/ts_end
3. **实体关联**：标注 entity_refs
4. **层级标注**：标注 layer (client/app/network/gateway/upstream/database/infra/deploy/config/observability)
5. **可信度评估**：标注 confidence (0.0-1.0)
6. **标签标注**：标注 tags (support/contradict/unknown/anomaly/absence/context/trust_warning)

### Phase E：缺失检测

**主动检查"应该有但没有"的证据：**

```
对于每个预期数据源:
  IF 该源无数据 THEN 生成一条 absence fact
  IF 该源数据不完整 THEN 生成一条 trust_warning fact
```

### Phase F：打包输出

输出标准 Evidence Pack JSON，包含：

- `case_id`
- `anomaly_summary`
- `evidence_items[]`
- `world_summaries`
- `metadata`（含 completeness 标注）

## 输出契约

### Evidence Item

```json
{
  "id": "ev-XX",
  "source_type": "log|trace|metric|deploy_record|config_change|scaling_event|job_schedule|code_diff|ticket|topology|network_stat|environment",
  "source_path": "来源路径或 URL",
  "ts_start": "ISO8601",
  "ts_end": "ISO8601 (可选)",
  "entity_refs": ["服务名", "Pod名", ...],
  "fact_text": "结构化的事实描述（一句话）",
  "confidence": 0.0-1.0,
  "layer": "client|app|network|gateway|upstream|database|infra|deploy|config|observability",
  "tags": ["support|contradict|unknown|anomaly|absence|context|trust_warning"],
  "related_hypotheses": ["h1", ...],
  "raw_data": null
}
```

### World Summaries

```json
{
  "deploy": { "recent_deploys": [...] },
  "scaling": { "events": [...] },
  "job": { "tasks": [...] },
  "config": { "changes": [...] },
  "metrics_snapshot": { ... }
}
```

### Completeness 标注

```json
{
  "metadata": {
    "evidence_completeness": "partial|complete",
    "worlds_covered": ["log", "trace", "metric"],
    "worlds_not_covered": ["deploy", "scaling", "job", "config"],
    "known_gaps": ["缺少发布记录", "未查询扩缩容历史"]
  }
}
```

## 世界扩展协议

当 Hypothesis Validator 返回 `next_worlds_to_query` 时，Evidence Builder 按以下映射采集：

| 世界 | 采集目标 |
|------|---------|
| `deploy` | 发布记录、变更内容、影响范围 |
| `scaling` | Pod 数变化、扩缩容触发原因、时间点 |
| `job` | 定时任务调度、全量任务执行、并发数 |
| `config` | 配置变更记录、变更前后对比 |
| `upstream` | 上游服务状态、依赖关系、上游日志 |
| `network` | 网络指标、丢包、重传、延迟 |
| `metrics_baseline` | 异常前基线指标、历史同期对比 |
| `code` | 相关代码变更 diff |
| `environment` | 运行环境信息（OS、runtime、资源限制） |

扩展采集后，将新证据 **增量追加** 到现有 Evidence Pack 中，更新 `metadata.worlds_covered`。

## 质量约束

1. **每条证据必须有明确的 fact_text** — 不是原始日志行，而是一句话事实
2. **confidence 不是随便给的** — 基于数据完整性和来源可靠性
3. **缺失必须显式标注** — "没看到"也是一条证据
4. **不做推理** — "CPU 升高"是事实，"因为代码问题导致 CPU 升高"是推理，不该出现在证据里
5. **时间标注必须精确** — 使用 ISO8601，标注时区

## 参考文档

- 证据包 Schema 详解：见 `hypothesis_validator/references/evidence-pack-schema.md`
- 能力池（世界入口注册表）：见 `shadow_claw/references/capability-pool.md`

