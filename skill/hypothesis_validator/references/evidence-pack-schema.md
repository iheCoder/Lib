# 证据包 Schema（Evidence Pack Schema）

本文档定义 Hypothesis Validator 接收的输入数据结构。
证据包由 `evidence_builder` skill 生成，Validator 在此基础上做四层校验。

---

## 1. 顶层结构

```json
{
  "case_id": "case-2024-0315-db-cpu",
  "anomaly_summary": {
    "description": "DB CPU 从 50% 跳到 100%",
    "start_time": "2024-03-15T10:24:00Z",
    "end_time": "2024-03-15T11:05:00Z",
    "severity": "critical",
    "affected_entities": ["db-instance-01", "pod-app-01..08"]
  },
  "hypotheses": [ ... ],
  "evidence_items": [ ... ],
  "world_summaries": { ... },
  "metadata": {
    "created_at": "2024-03-15T12:00:00Z",
    "evidence_builder_version": "1.0",
    "evidence_completeness": "partial"
  }
}
```

---

## 2. 假设（Hypothesis）

```json
{
  "id": "h1",
  "text": "代码某处低效导致 DB CPU 升高",
  "source": "code_review",
  "proposed_cause_time": "2024-03-15T10:20:00Z",
  "proposed_cause_end_time": null,
  "proposed_scope": "single_request_path",
  "proposed_scope_level": "L1",
  "proposed_magnitude": "cpu_increase_5pct",
  "supporting_evidence_ids": ["ev-03", "ev-07"],
  "tags": ["code", "sql"]
}
```

### 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | ✅ | 假设唯一 ID |
| `text` | string | ✅ | 假设的自然语言描述 |
| `source` | string | ✅ | 假设来源（code_review / log_analysis / metric_anomaly / engineer_intuition） |
| `proposed_cause_time` | ISO8601 | ✅ | 假设认为的原因发生时间 |
| `proposed_cause_end_time` | ISO8601 | ❌ | 原因持续结束时间 |
| `proposed_scope` | string | ✅ | 假设影响范围的自然语言描述 |
| `proposed_scope_level` | string | ❌ | 范围级别 L1–L5 |
| `proposed_magnitude` | string | ✅ | 假设认为的影响量级描述 |
| `supporting_evidence_ids` | [string] | ❌ | 初步支持证据的 ID 列表 |
| `tags` | [string] | ❌ | 标签 |

---

## 3. 证据条目（Evidence Item）

```json
{
  "id": "ev-01",
  "source_type": "deploy_record",
  "source_path": "deploy-system/releases/2024-03-15.json",
  "ts_start": "2024-03-15T10:20:00Z",
  "ts_end": "2024-03-15T10:22:00Z",
  "entity_refs": ["app-service", "pod-app-01..08"],
  "fact_text": "10:20 开始发版，10:22 完成，Pod 从 1 扩到 8",
  "confidence": 0.95,
  "layer": "deploy",
  "tags": ["support", "scaling", "deploy"],
  "related_hypotheses": ["h2"],
  "raw_data": null
}
```

### 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | ✅ | 证据唯一 ID |
| `source_type` | enum | ✅ | 见下方枚举 |
| `source_path` | string | ❌ | 证据来源路径或 URL |
| `ts_start` | ISO8601 | ✅ | 证据覆盖的起始时间 |
| `ts_end` | ISO8601 | ❌ | 证据覆盖的结束时间 |
| `entity_refs` | [string] | ✅ | 涉及的实体（服务名、Pod名、DB名等） |
| `fact_text` | string | ✅ | 结构化的事实描述 |
| `confidence` | float | ✅ | 可信度 0.0–1.0 |
| `layer` | enum | ✅ | 见下方枚举 |
| `tags` | [string] | ✅ | 至少包含一个：support / contradict / unknown / anomaly |
| `related_hypotheses` | [string] | ❌ | 关联的假设 ID |
| `raw_data` | any | ❌ | 原始数据（日志行、指标值等） |

### source_type 枚举

| 值 | 说明 |
|----|------|
| `ticket` | 工单/告警 |
| `log` | 应用日志 |
| `trace` | 分布式追踪 |
| `metric` | 监控指标 |
| `deploy_record` | 发布记录 |
| `config_change` | 配置变更 |
| `scaling_event` | 扩缩容事件 |
| `job_schedule` | 定时任务/批处理 |
| `code_diff` | 代码变更 |
| `topology` | 拓扑/架构信息 |
| `network_stat` | 网络统计 |
| `environment` | 环境信息（OS、runtime 等） |

### layer 枚举

| 值 | 说明 |
|----|------|
| `client` | 客户端层 |
| `app` | 应用层 |
| `network` | 网络层 |
| `gateway` | 网关层 |
| `upstream` | 上游服务层 |
| `database` | 数据库层 |
| `infra` | 基础设施层 |
| `deploy` | 发布/部署层 |
| `config` | 配置层 |
| `observability` | 观测系统自身 |

---

## 4. 世界摘要（World Summaries）

```json
{
  "deploy": {
    "recent_deploys": [
      {
        "time": "2024-03-15T10:20:00Z",
        "service": "app-service",
        "change_summary": "版本 v2.3.1 → v2.3.2, 小优化",
        "diff_size": "small",
        "rollback_available": true
      }
    ]
  },
  "scaling": {
    "events": [
      {
        "time": "2024-03-15T10:22:00Z",
        "service": "app-service",
        "from_replicas": 1,
        "to_replicas": 8,
        "trigger": "deploy"
      }
    ]
  },
  "job": {
    "tasks": [
      {
        "name": "full-sync-task",
        "trigger_time": "2024-03-15T10:23:00Z",
        "trigger": "pod_startup",
        "concurrency": 8,
        "estimated_load": "high"
      }
    ]
  },
  "config": {
    "changes": []
  },
  "metrics_snapshot": {
    "db_cpu": {
      "baseline_window": "10:00-10:20",
      "baseline_mean": 0.50,
      "anomaly_window": "10:24-11:00",
      "anomaly_mean": 0.95,
      "anomaly_peak": 1.00,
      "change_point": "2024-03-15T10:24:00Z"
    },
    "app_qps": {
      "baseline_mean": 120,
      "anomaly_mean": 850
    },
    "pod_count": {
      "baseline": 1,
      "anomaly": 8
    }
  }
}
```

### Validator 如何使用世界摘要

| 校验层 | 使用的摘要字段 |
|--------|--------------|
| 时间校验 | `deploy.recent_deploys[].time`, `scaling.events[].time`, `job.tasks[].trigger_time`, `metrics_snapshot.*.change_point` |
| 量级校验 | `metrics_snapshot.*` (baseline vs anomaly), `scaling.events[].from/to_replicas`, `job.tasks[].concurrency` |
| 范围校验 | `scaling.events[].service`, `anomaly_summary.affected_entities`, 各 evidence_item 的 `entity_refs` |
| 反事实校验 | 所有字段均可能被用到 |

---

## 5. Validator 输出结构

每个假设的校验结果：

```json
{
  "hypothesis_id": "h1",
  "hypothesis": "代码小问题导致 DB CPU 爆满",
  "temporal_fit": 0.35,
  "temporal_notes": "发版时间与异常时间邻近，但假设未解释时间突变原因",
  "magnitude_fit": 0.10,
  "magnitude_notes": "代码 diff 极小，预期 CPU +3~5%，实际 +50%，量级差一个数量级",
  "scope_fit": 0.20,
  "scope_notes": "假设为局部代码路径(L1)，实际影响全量 Pod(L4)，范围严重不匹配",
  "counterfactual_fit": 0.15,
  "counterfactual_notes": [
    "无法解释为何异常在发版后立刻出现",
    "无法解释为何 8 个 pod 同步异常",
    "预期影响量级明显小于观测量级",
    "未找到特定 SQL 耗时显著增长的证据"
  ],
  "composite_score": 0.21,
  "decision": "reject_as_primary_cause",
  "confidence": 0.85,
  "next_worlds_to_query": [],
  "notes": "此假设最多为边际因子，不能作为主因"
}
```

### 多假设比较输出

```json
{
  "case_id": "case-2024-0315-db-cpu",
  "validation_results": [
    { "hypothesis_id": "h1", "composite_score": 0.21, "decision": "reject_as_primary_cause" },
    { "hypothesis_id": "h2", "composite_score": 0.82, "decision": "promote_to_primary" }
  ],
  "ranking": ["h2", "h1"],
  "distinguishable": true,
  "summary": "H2（8 pod 并发全量任务）在时间、量级、范围、反事实四层校验中均显著优于 H1（代码小问题）"
}
```

