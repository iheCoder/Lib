# Capability Pool — 能力池

> **每个能力，本质上是进入某个世界的方式。**

## 核心概念

Capability Pool 是一个 **世界入口注册表**。

它回答一个问题：

> 当系统需要进入某个世界（deploy / log / trace / scaling / network / ...），
> 具体该调用什么工具、什么脚本、什么 skill？

```
Hypothesis Validator: "证据不足，需要进入 trace 世界"
         |
         v
Shadow Claw: "Capability Pool 里谁能进入 trace 世界？"
         |
         v
Capability Pool: "aliyun-sls-trace skill 可以"
         |
         v
Evidence Builder: 用 aliyun-sls-trace 采集 trace 数据，转为结构化证据
```

---

## 为什么需要 Capability Pool

没有 Capability Pool 时，Evidence Builder 的"世界扩展协议"是一张静态表：

```
deploy → 查询 deploy 系统
scaling → 查询 K8s
trace → 查询 Jaeger
```

问题在于：

1. **不同环境的工具不同** — 阿里云用 SLS，AWS 用 CloudWatch，自建用 Loki
2. **工具会演化** — 今天用脚本查，明天可能有 MCP server
3. **有些世界需要多步** — 进入"网络世界"可能需要先 SSH 到节点再跑 tcpdump
4. **处置也需要工具** — 回滚需要 CI/CD 工具，扩缩容需要 K8s 工具

Capability Pool 把"世界"和"工具"解耦：

```
世界是抽象的：trace, deploy, scaling, network, config, code, ...
能力是具体的：aliyun-sls-trace, kubectl, git log, prometheus query, ...
```

---

## 注册表结构

### 能力注册项 (Capability Entry)

```yaml
- capability_id: sls-trace-query
  name: "阿里云 SLS 链路追踪查询"
  worlds: [trace, log]            # 能进入哪些世界
  type: observation                # observation | mutation | resolution
  skill_ref: aliyun-sls-trace     # 对应的 skill name
  invocation:                      # 怎么调用
    method: script
    command: "python3 scripts/query_trace_response.py"
    params:
      - name: trace_id
        required: false
      - name: query
        required: false
      - name: profile
        default: prod
      - name: from
        required: false
      - name: to
        required: false
  output_format: json              # 输出格式
  evidence_types: [temporal, phase, state, absence]  # 能产出的证据类型
  prerequisites:                   # 前置条件
    - "SLS 配置已解析 (sls_config_resolver.py)"
  environments: [aliyun]           # 适用环境
  reliability: 0.90                # 可靠性评分
```

### 完整注册表

```yaml
capability_pool:
  version: "1.0"
  last_updated: "2026-03-31"

  # ============================================================
  # 观测类能力 (Observation) — 进入世界，采集证据
  # ============================================================

  observation:

    # --- Trace & Log 世界 ---
    - capability_id: sls-trace-query
      name: "阿里云 SLS 链路追踪查询"
      worlds: [trace, log]
      skill_ref: aliyun-sls-trace
      invocation:
        method: script
        command: "python3 {skill_path}/scripts/query_trace_response.py"
        params: [trace_id, query, profile, from, to, lines, offset]
      output_format: json
      evidence_types: [temporal, phase, state, absence]
      environments: [aliyun]

    - capability_id: sls-config-resolve
      name: "阿里云 SLS 配置解析"
      worlds: [config]
      skill_ref: aliyun-sls-trace
      invocation:
        method: script
        command: "python3 {skill_path}/scripts/sls_config_resolver.py"
        params: [profile, project, logstore]
      output_format: json
      evidence_types: [context]
      environments: [aliyun]

    - capability_id: local-log-grep
      name: "本地日志 grep 搜索"
      worlds: [log]
      skill_ref: null  # 内置能力，不依赖外部 skill
      invocation:
        method: shell
        command: "grep -n {pattern} {log_path}"
        params: [pattern, log_path, time_range]
      output_format: text
      evidence_types: [temporal, state, phase]
      environments: [any]

    # --- Metrics 世界 ---
    - capability_id: prometheus-query
      name: "Prometheus 指标查询"
      worlds: [metric, metrics_baseline]
      skill_ref: null
      invocation:
        method: http
        endpoint: "http://{prometheus_host}:9090/api/v1/query_range"
        params: [query, start, end, step]
      output_format: json
      evidence_types: [temporal, context]
      environments: [any]

    # --- Deploy 世界 ---
    - capability_id: git-recent-commits
      name: "Git 最近提交查询"
      worlds: [deploy, code]
      skill_ref: null
      invocation:
        method: shell
        command: "git log --oneline --since={since} --until={until} -- {path}"
        params: [since, until, path]
      output_format: text
      evidence_types: [temporal, context]
      environments: [any]

    - capability_id: git-diff
      name: "Git 代码变更 Diff"
      worlds: [code]
      skill_ref: null
      invocation:
        method: shell
        command: "git diff {from_ref}..{to_ref} -- {path}"
        params: [from_ref, to_ref, path]
      output_format: text
      evidence_types: [context]
      environments: [any]

    # --- Scaling 世界 ---
    - capability_id: kubectl-events
      name: "Kubernetes 事件查询"
      worlds: [scaling, deploy, environment]
      skill_ref: null
      invocation:
        method: shell
        command: "kubectl get events -n {namespace} --sort-by='.lastTimestamp' --field-selector involvedObject.name={name}"
        params: [namespace, name, since]
      output_format: text
      evidence_types: [temporal, state, context]
      environments: [k8s]

    - capability_id: kubectl-pod-history
      name: "Kubernetes Pod 历史"
      worlds: [scaling]
      skill_ref: null
      invocation:
        method: shell
        command: "kubectl get pods -n {namespace} -l {selector} -o wide --sort-by='.status.startTime'"
        params: [namespace, selector]
      output_format: text
      evidence_types: [temporal, context]
      environments: [k8s]

    - capability_id: kubectl-hpa
      name: "Kubernetes HPA 状态查询"
      worlds: [scaling]
      skill_ref: null
      invocation:
        method: shell
        command: "kubectl describe hpa -n {namespace} {hpa_name}"
        params: [namespace, hpa_name]
      output_format: text
      evidence_types: [temporal, state, context]
      environments: [k8s]

    # --- Network 世界 ---
    - capability_id: network-stats
      name: "网络统计查询"
      worlds: [network]
      skill_ref: null
      invocation:
        method: shell
        command: "ss -tni state established | head -50"
        params: []
      output_format: text
      evidence_types: [state, context]
      environments: [linux]

    # --- Web / 外部信息世界 ---
    - capability_id: web-search
      name: "Web 搜索"
      worlds: [external_knowledge]
      skill_ref: web-access
      invocation:
        method: web_tool
        command: "web.search_query"
        params: [query]
      output_format: text
      evidence_types: [context]
      environments: [any]

    - capability_id: web-browse
      name: "Web 页面浏览"
      worlds: [external_knowledge]
      skill_ref: web-access
      invocation:
        method: web_tool
        command: "web.open"
        params: [url]
      output_format: text
      evidence_types: [context]
      environments: [any]

    # --- Config 世界 ---
    - capability_id: config-diff
      name: "配置变更对比"
      worlds: [config]
      skill_ref: null
      invocation:
        method: shell
        command: "diff {old_config} {new_config}"
        params: [old_config, new_config]
      output_format: text
      evidence_types: [context, state]
      environments: [any]

    # --- Database 世界 ---
    - capability_id: mysql-readonly-query
      name: "MySQL 只读查询"
      worlds: [database]
      skill_ref: null
      invocation:
        method: mcp_tool
        tool_ref: "mcp/mysql_readonly"
        params: [query]
      output_format: json
      evidence_types: [state, context, temporal]
      environments: [mysql]

  # ============================================================
  # 处置类能力 (Resolution) — 修复问题
  # ============================================================

  resolution:

    - capability_id: kubectl-scale
      name: "Kubernetes 扩缩容"
      action: scale
      skill_ref: null
      invocation:
        method: shell
        command: "kubectl scale deployment/{name} -n {namespace} --replicas={replicas}"
        params: [name, namespace, replicas]
      risk_level: medium
      requires_confirmation: true
      environments: [k8s]

    - capability_id: kubectl-rollback
      name: "Kubernetes 回滚"
      action: rollback
      skill_ref: null
      invocation:
        method: shell
        command: "kubectl rollout undo deployment/{name} -n {namespace}"
        params: [name, namespace]
      risk_level: high
      requires_confirmation: true
      environments: [k8s]

    - capability_id: config-revert
      name: "配置回滚"
      action: config_revert
      skill_ref: null
      invocation:
        method: shell
        command: "cp {backup_config} {active_config}"
        params: [backup_config, active_config]
      risk_level: high
      requires_confirmation: true
      environments: [any]

    - capability_id: restart-service
      name: "服务重启"
      action: restart
      skill_ref: null
      invocation:
        method: shell
        command: "kubectl rollout restart deployment/{name} -n {namespace}"
        params: [name, namespace]
      risk_level: medium
      requires_confirmation: true
      environments: [k8s]
```

---

## 世界 → 能力映射（快速查找表）

Evidence Builder 在需要进入某个世界时，通过此表查找可用能力：

| 世界 (World) | 可用能力 (Capabilities) | 优先级 |
|-------------|----------------------|--------|
| trace | sls-trace-query | P0 (阿里云环境) |
| log | sls-trace-query, local-log-grep | P0: SLS, P1: local grep |
| metric | prometheus-query | P0 |
| metrics_baseline | prometheus-query | P0 |
| deploy | git-recent-commits, kubectl-events | P0: git, P1: k8s events |
| scaling | kubectl-pod-history, kubectl-hpa, kubectl-events | P0 |
| code | git-diff, git-recent-commits | P0 |
| config | sls-config-resolve, config-diff | 按环境选择 |
| network | network-stats | P0 |
| database | mysql-readonly-query | P0 (MySQL 环境) |
| upstream | sls-trace-query, local-log-grep | 复用 log/trace 能力 |
| environment | kubectl-events, kubectl-pod-history | P0 (K8s 环境) |
| external_knowledge | web-search, web-browse | P0 |
| job | kubectl-events, local-log-grep | 按环境选择 |

---

## 触发流程

### 1. Validator 触发世界探索

```
Validator 输出:
  decision: insufficient_evidence
  next_worlds_to_query: ["deploy", "scaling"]
```

### 2. Shadow Claw 查找能力

```python
for world in next_worlds_to_query:
    capabilities = capability_pool.lookup(world, current_environment)
    if not capabilities:
        log(f"WARNING: 无法进入 {world} 世界，没有可用能力")
        continue
    best_capability = capabilities[0]  # 按优先级排序
    evidence_builder.collect(world, best_capability, entities, time_window)
```

### 3. Evidence Builder 使用能力采集

```
Evidence Builder:
  1. 接收: world="deploy", capability=git-recent-commits
  2. 调用: git log --oneline --since=2024-03-15T10:00 --until=2024-03-15T12:00
  3. 结构化: 提取时间、变更内容、影响范围
  4. 输出: evidence_items[] 追加到 Evidence Pack
```

---

## 能力发现与缺失处理

### 能力存在

正常流程：查找 → 调用 → 采集 → 结构化 → 追加证据

### 能力不存在

当某个世界没有注册能力时：

```yaml
evidence_item:
  id: ev-gap-01
  source_type: capability_gap
  fact_text: "无法进入 network 世界：当前环境没有注册 network 类型的能力"
  layer: observability
  confidence: 1.0
  tags: [absence, trust_warning]
```

这本身就是一条重要证据 — **你的观测能力有盲区**。

### 能力执行失败

```yaml
evidence_item:
  id: ev-fail-01
  source_type: capability_failure
  fact_text: "sls-trace-query 执行失败: connection timeout to SLS endpoint"
  layer: observability
  confidence: 1.0
  tags: [trust_warning]
```

---

## 如何注册新能力

当团队有新工具/数据源时，按以下步骤注册：

1. **确定世界**: 这个工具能进入哪个世界？
2. **确定类型**: observation（采集）还是 resolution（处置）？
3. **写调用规范**: command / params / output_format
4. **标注环境**: 这个能力在什么环境下可用？
5. **追加到注册表**

示例 — 注册一个新的日志查询工具：

```yaml
- capability_id: loki-query
  name: "Grafana Loki 日志查询"
  worlds: [log]
  skill_ref: null
  invocation:
    method: http
    endpoint: "http://{loki_host}:3100/loki/api/v1/query_range"
    params: [query, start, end, limit]
  output_format: json
  evidence_types: [temporal, state, phase, absence]
  environments: [grafana-stack]
```

---

## 处置类能力的安全约束

Resolution 类能力有额外约束：

| 约束 | 说明 |
|------|------|
| `requires_confirmation: true` | 必须经过用户确认才能执行 |
| `risk_level` | low / medium / high — 高风险操作需要额外审批 |
| `dry_run` | 所有处置能力应支持 dry-run 模式 |
| `rollback_capability` | 每个处置操作应关联其回滚能力 |

**Shadow Claw 永远不会自动执行 resolution 类能力。它只生成处置方案，由人决定是否执行。**

