# 完整案例演练（Worked Examples）

本文档提供两个完整的 Hypothesis Validator 工作案例，展示四层校验如何运作。

---

## 案例 1：DB CPU 突增 — 代码小问题 vs 扩容并发任务

### 现象

- DB CPU 从 50% 跳到 100%
- 时间：2024-03-15 10:24 开始
- 影响范围：DB 实例全局
- LLM 初始在代码和 SQL 上来回纠缠

### 候选假设

| ID | 假设 | 来源 |
|----|------|------|
| H1 | 代码某处低效导致 DB CPU 升高 | code_review |
| H2 | 发版扩容后 8 个 Pod 并发全量任务导致 DB CPU 爆满 | metric_anomaly + deploy_record |

### 证据包（精简）

```json
{
  "evidence_items": [
    {
      "id": "ev-01", "source_type": "deploy_record",
      "ts_start": "2024-03-15T10:20:00Z",
      "fact_text": "10:20 开始发版 v2.3.1→v2.3.2",
      "confidence": 0.95, "layer": "deploy"
    },
    {
      "id": "ev-02", "source_type": "scaling_event",
      "ts_start": "2024-03-15T10:22:00Z",
      "fact_text": "Pod 从 1 扩到 8",
      "confidence": 0.95, "layer": "deploy"
    },
    {
      "id": "ev-03", "source_type": "job_schedule",
      "ts_start": "2024-03-15T10:23:00Z",
      "fact_text": "8 个 Pod 同时启动 full-sync-task",
      "confidence": 0.90, "layer": "app"
    },
    {
      "id": "ev-04", "source_type": "metric",
      "ts_start": "2024-03-15T10:24:00Z",
      "fact_text": "DB CPU 从 50% 阶跃到 100%",
      "confidence": 0.95, "layer": "database",
      "raw_data": {"baseline_mean": 0.50, "anomaly_peak": 1.00}
    },
    {
      "id": "ev-05", "source_type": "code_diff",
      "ts_start": "2024-03-15T10:20:00Z",
      "fact_text": "代码 diff 很小，仅涉及一个查询路径的微调",
      "confidence": 0.80, "layer": "app"
    },
    {
      "id": "ev-06", "source_type": "metric",
      "ts_start": "2024-03-15T10:24:00Z",
      "fact_text": "QPS 从 120 跳到 850，与 Pod 数增加近似同步",
      "confidence": 0.90, "layer": "app"
    }
  ]
}
```

---

### H1 校验过程：代码小问题导致 DB CPU 升高

#### 第 1 层：时间校验

| 检查项 | 结果 |
|--------|------|
| 因果时序 | ✅ 代码变更(10:20) < 异常开始(10:24) |
| 合理时延 | ⚠️ 4min，在合理范围内 |
| 变化点邻近 | ⚠️ 10:24 附近有 deploy(10:20)、scaling(10:22)、task(10:23)，代码只是其中一个因素 |
| 时间窗重叠 | ⚠️ 代码变更是一次性的，无持续活跃窗口 |

**temporal_fit = 0.35** — 时间上不矛盾，但无法排他地解释"为什么偏偏 10:24"。

#### 第 2 层：量级校验

| 检查项 | 结果 |
|--------|------|
| 预期影响上界 | 代码 diff 很小 → CPU +3%~5% |
| 观测影响 | CPU +50%（从 50%→100%） |
| 数量级比对 | 5% << 50%，差一个数量级 |
| 放大器 | 未识别到代码变更本身有乘法放大效应 |

**magnitude_fit = 0.10** — **量级严重不足，不足以解释主异常。**

#### 第 3 层：范围校验

| 检查项 | 结果 |
|--------|------|
| 现象范围 | L4 — 全量 Pod + DB 全局 |
| 假设范围 | L1 — 单个请求路径的代码微调 |
| 匹配度 | L1 vs L4，差 3 级 |
| 同步性 | 8 个 Pod 同步异常，与局部代码问题不符 |

**scope_fit = 0.20** — 范围严重不匹配。

#### 第 4 层：反事实校验

| 反事实预测 | 证据 | 结果 |
|-----------|------|------|
| 应看到特定 SQL 耗时显著拉长 | 未找到 | ❌ |
| 影响应偏局部（涉及的请求路径） | 实际是全局 | ❌ |
| 发版前不应有同级异常 | 发版前确实没有 | ✅ |
| 回滚代码后异常应消失 | 未测试 | ❓ |

命中率: 1 / (1+2+0.5*1) = 0.29

**counterfactual_fit = 0.29**

#### H1 综合结果

```json
{
  "hypothesis_id": "h1",
  "hypothesis": "代码小问题导致 DB CPU 爆满",
  "temporal_fit": 0.35,
  "magnitude_fit": 0.10,
  "scope_fit": 0.20,
  "counterfactual_fit": 0.29,
  "composite_score": 0.225,
  "decision": "reject_as_primary_cause",
  "counterfactual_notes": [
    "无法解释为何异常在发版/扩容后立刻出现",
    "无法解释为何 8 个 pod 同步异常",
    "预期影响量级(+5%)明显小于观测量级(+50%)",
    "未找到特定 SQL 耗时显著增长的证据"
  ],
  "notes": "此假设最多为边际因子，不能作为主因"
}
```

> 计算: 0.25×0.35 + 0.30×0.10 + 0.25×0.20 + 0.20×0.29 = 0.0875 + 0.03 + 0.05 + 0.058 = 0.225

---

### H2 校验过程：8 Pod 并发全量任务导致 DB CPU 爆满

#### 第 1 层：时间校验

| 检查项 | 结果 |
|--------|------|
| 因果时序 | ✅ 扩容(10:22)+任务启动(10:23) < 异常开始(10:24) |
| 合理时延 | ✅ 1-2min，全量任务预热后触发 DB 负载非常合理 |
| 变化点邻近 | ✅ change_point(10:24) 与任务启动(10:23) 仅差 1min |
| 时间窗重叠 | ✅ 全量任务持续运行，覆盖整个异常窗口 |

**temporal_fit = 0.90** — 时间高度吻合。

#### 第 2 层：量级校验

| 检查项 | 结果 |
|--------|------|
| 预期影响 | Pod ×8，每个 Pod 启动全量任务 → 负载 ≈ ×8 |
| 观测影响 | QPS ×7（120→850），DB CPU ×2（50%→100%） |
| 数量级比对 | QPS ×7 与 Pod ×8 高度吻合；DB CPU 翻倍是负载翻数倍的自然结果 |
| 放大器 | Pod ×8 + 每 Pod 触发全量任务 = 明确的乘法放大器 |

**magnitude_fit = 0.85** — 量级充分匹配。

#### 第 3 层：范围校验

| 检查项 | 结果 |
|--------|------|
| 现象范围 | L4 — 全量 Pod + DB 全局 |
| 假设范围 | L4 — 发版扩容影响全部新 Pod |
| 匹配度 | L4 vs L4，完美匹配 |
| 同步性 | 8 Pod 同时启动 → 同步异常合理 |

**scope_fit = 0.90** — 范围高度匹配。

#### 第 4 层：反事实校验

| 反事实预测 | 证据 | 结果 |
|-----------|------|------|
| 发版/扩容时间与 CPU 抬升时间接近 | ev-01, ev-02, ev-04 | ✅ |
| 负载上升与 Pod 数变化近似同步 | ev-06 QPS×7 vs Pod×8 | ✅ |
| task count 应增加 | ev-03 8个Pod同时启动full-sync | ✅ |
| 影响面应是全量 Pod | DB 全局受影响 | ✅ |
| 缩容/停任务后异常应缓解 | 未测试 | ❓ |

命中率: 4 / (4+0+0.5*1) = 0.89

**counterfactual_fit = 0.89**

#### H2 综合结果

```json
{
  "hypothesis_id": "h2",
  "hypothesis": "发版扩容后 8 个 Pod 并发全量任务导致 DB CPU 爆满",
  "temporal_fit": 0.90,
  "magnitude_fit": 0.85,
  "scope_fit": 0.90,
  "counterfactual_fit": 0.89,
  "composite_score": 0.883,
  "decision": "promote_to_primary",
  "counterfactual_notes": [
    "发版/扩容时间与异常时间高度吻合(差1-2min)",
    "QPS增幅(×7)与Pod增幅(×8)近似线性",
    "8个Pod同时启动全量任务有明确证据",
    "全量影响与系统性原因匹配"
  ],
  "notes": "四层校验均给出强支持，推荐为主因"
}
```

> 计算: 0.25×0.90 + 0.30×0.85 + 0.25×0.90 + 0.20×0.89 = 0.225 + 0.255 + 0.225 + 0.178 = 0.883

---

### 比较结论

```json
{
  "case_id": "case-2024-0315-db-cpu",
  "validation_results": [
    {"hypothesis_id": "h1", "composite_score": 0.225, "decision": "reject_as_primary_cause"},
    {"hypothesis_id": "h2", "composite_score": 0.883, "decision": "promote_to_primary"}
  ],
  "ranking": ["h2", "h1"],
  "distinguishable": true,
  "summary": "不是'代码导致CPU爆掉'，而是'发版导致Pod扩增，8个实例同时启动全量任务，负载成倍放大，把DB CPU顶满；代码层问题最多只是边际因子。'"
}
```

---

## 案例 2：跨层时间漂移 — 时钟偏差 vs 应用层阻塞

### 现象

- 客户端在 T1 记录"准备发送请求"
- 网关在 T1+30s 才收到请求
- 网关因 timestamp expired 拒绝
- 上游无处理记录

### 候选假设

| ID | 假设 |
|----|------|
| H3 | 客户端时钟漂移导致签名过期 |
| H4 | 应用层发送前阻塞（goroutine/连接池）导致请求延迟发出 |

### 证据包（精简）

```json
{
  "evidence_items": [
    {
      "id": "ev-10", "source_type": "log",
      "ts_start": "2024-03-15T14:00:00Z",
      "fact_text": "client log: signed_at=14:00:00, prepare_send=14:00:00",
      "layer": "client", "confidence": 0.85
    },
    {
      "id": "ev-11", "source_type": "log",
      "ts_start": "2024-03-15T14:00:30Z",
      "fact_text": "gateway access log: receive_time=14:00:30, reject reason=timestamp_expired",
      "layer": "gateway", "confidence": 0.95
    },
    {
      "id": "ev-12", "source_type": "metric",
      "ts_start": "2024-03-15T14:00:00Z",
      "fact_text": "客户端节点 NTP offset: +2s（轻微）",
      "layer": "client", "confidence": 0.70
    },
    {
      "id": "ev-13", "source_type": "metric",
      "ts_start": "2024-03-15T13:59:50Z",
      "fact_text": "客户端 goroutine 数量在 13:59:50 突增到 500+",
      "layer": "app", "confidence": 0.80
    },
    {
      "id": "ev-14", "source_type": "log",
      "ts_start": "2024-03-15T14:00:28Z",
      "fact_text": "客户端 socket_write_begin=14:00:28（比 signed_at 晚 28 秒）",
      "layer": "app", "confidence": 0.85
    },
    {
      "id": "ev-15", "source_type": "network_stat",
      "ts_start": "2024-03-15T14:00:00Z",
      "fact_text": "网络层无明显丢包或重传",
      "layer": "network", "confidence": 0.75
    }
  ]
}
```

---

### H3 校验：时钟漂移

#### 时间校验

- NTP offset 仅 +2s，而请求延迟 30s → 时钟漂移不足以解释 30s 的延迟
- **temporal_fit = 0.30**

#### 量级校验

- 时钟偏差 2s，而过期窗口 30s → 即使时钟偏差存在，也只占 2/30 ≈ 7%
- **magnitude_fit = 0.15**

#### 范围校验

- 如果是时钟漂移，应该影响该节点所有请求 → 需要检查其他请求是否也过期
- 如果只有这一个请求过期 → 不太像纯时钟问题
- **scope_fit = 0.40**（取决于其他请求的表现，暂按"部分匹配"）

#### 反事实校验

| 预测 | 结果 |
|------|------|
| 同节点其他请求也应过期 | ❓ 未确认 |
| socket_write 时间应与 signed_at 接近（时钟偏差不影响发送速度） | ❌ ev-14 显示差 28s |
| 网络传输应正常 | ✅ ev-15 |

**counterfactual_fit = 0.33**

#### H3 结果

```json
{
  "hypothesis_id": "h3",
  "composite_score": 0.288,
  "decision": "reject_as_primary_cause",
  "notes": "时钟偏差(2s)远不足以解释30s延迟，且socket_write证据直接反驳纯时钟假设"
}
```

---

### H4 校验：应用层发送前阻塞

#### 时间校验

- goroutine 突增(13:59:50) < signed_at(14:00:00) < socket_write(14:00:28) < gateway_receive(14:00:30)
- 因果链时间完全顺序正确，且各环节时延合理
- **temporal_fit = 0.85**

#### 量级校验

- goroutine 阻塞可以导致任意长度的发送延迟（取决于阻塞时长）
- 观测到 28s 延迟（signed_at → socket_write）
- goroutine 阻塞解释 28s 延迟是量级匹配的
- gateway 在 socket_write 后 2s 收到（14:00:28→14:00:30），网络传输正常
- **magnitude_fit = 0.80**

#### 范围校验

- goroutine 阻塞通常影响同时段该节点的多个请求 → L2 级别
- 现象集中在该客户端节点 → L2 级别
- 范围匹配
- **scope_fit = 0.80**

#### 反事实校验

| 预测 | 结果 |
|------|------|
| signed_at 与 socket_write 之间应有明显时间差 | ✅ ev-14 差 28s |
| 同节点应有 goroutine/线程/连接池异常 | ✅ ev-13 goroutine 突增 |
| 网络层不应有异常 | ✅ ev-15 网络正常 |
| 阻塞解除后后续请求应恢复正常 | ❓ 未确认 |

**counterfactual_fit = 0.86**

#### H4 结果

```json
{
  "hypothesis_id": "h4",
  "composite_score": 0.826,
  "decision": "promote_to_primary",
  "notes": "应用层阻塞完美解释了'签名早但发送晚'的现象，goroutine突增证据直接支持"
}
```

---

### 比较结论

```json
{
  "ranking": ["h4", "h3"],
  "distinguishable": true,
  "summary": "不是纯时钟漂移(偏差仅2s)，而是应用层goroutine阻塞导致请求在签名28s后才真正发出，网关收到时签名已过期。时钟偏差最多是叠加的微弱因素。",
  "key_evidence": "ev-14(socket_write比signed_at晚28s)是区分两个假设的关键证据"
}
```

---

## 关键教训

### 1. 量级是最有效的否决器

案例 1 中，代码小 diff 的 +5% 预期 vs +50% 观测，一个数量级的差距直接否决。
案例 2 中，2s 时钟偏差 vs 30s 延迟，同样是量级不匹配。

### 2. 反事实预测能找到"决定性证据"

案例 2 中，"socket_write 应与 signed_at 接近"这个反事实预测，直接命中了 ev-14（差 28s），成为区分 H3 和 H4 的关键。

### 3. 世界扩展是必要的

如果证据包中没有 socket_write 时间、goroutine 指标、NTP offset，两个假设无法区分。
Validator 的 `insufficient_evidence` + `next_worlds_to_query` 机制正是为了触发这种扩展。

### 4. 不要被第一个"像答案的东西"骗住

案例 1 中"代码导致 CPU 升高"听起来很合理，但过不了量级校验。
案例 2 中"时钟漂移导致过期"是最直觉的解释，但 2s 偏差解释不了 30s 延迟。
**四层校验的价值就在于逼系统在直觉之外做工程化审查。**

