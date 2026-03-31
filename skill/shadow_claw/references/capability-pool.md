# Capability Pool — 能力池

> 每个能力，本质上是进入某个世界的方式。

## 核心概念

调查过程中，系统需要进入不同的"世界"（trace / log / deploy / scaling / database / ...）去采集证据。

大多数世界的入口是 **天然存在的** — 你本来就会 `grep` 日志、`git log` 看提交、`kubectl` 查 Pod。这些不需要"注册"，它们就是基本操作能力。

但有些世界的入口 **需要专门的 skill/工具才能进入**，这些才是 Capability Pool 真正要管理的：

## 已注册的外部能力

### 1. aliyun-sls-trace — 链路追踪与日志查询

**能进入的世界**: trace, log

用于查询阿里云 SLS 的链路追踪日志。当需要按 trace_id 追踪请求链路、按时间窗口查询特定服务的日志时使用。

使用方式详见 [aliyun-sls-trace SKILL.md](../../aliyun-sls-trace/SKILL.md)

典型调用:
```bash
# 按 trace_id 查询
python3 scripts/query_trace_response.py --trace-id <trace_id> --profile prod

# 按查询语句查询
python3 scripts/query_trace_response.py --query 'level: "error" and service: "app-service"' --profile prod --from "2024-03-15 10:00:00" --to "2024-03-15 11:00:00"
```

### 2. mysql-readonly-query — 数据库只读查询

**能进入的世界**: database

用于只读查询 MySQL 数据库，排查数据层面的问题（慢查询、数据不一致、连接数等）。

使用方式详见 [mysql_readonly](../../mcp/mysql_readonly/)

### 3. web-access — 外部信息获取

**能进入的世界**: external_knowledge

当需要查询外部文档、搜索已知问题、查看第三方服务状态页时使用。

使用方式详见 [web-access SKILL.md](../../web-access/SKILL.md)

## 天然能力（不需要注册）

以下操作是 Agent 的基本能力，直接使用即可：

| 世界 | 怎么进入 |
|------|---------|
| log (本地) | `grep`, `tail`, `cat`, 直接读文件 |
| code | `git log`, `git diff`, `git blame`, 读源码 |
| deploy | `git log`, `kubectl get events`, 读 CI/CD 配置 |
| scaling | `kubectl get pods`, `kubectl describe hpa`, `kubectl get events` |
| config | `diff`, 读配置文件, `kubectl get configmap` |
| network | `ss`, `netstat`, `curl`, `ping`, `traceroute` |
| metric (Prometheus) | `curl` Prometheus API |
| environment | `kubectl describe node`, `top`, `free`, `df` |

这些不需要注册到能力池。你知道怎么用就行。

## 世界探索触发流程

```
Hypothesis Validator: "证据不足，需要进入 trace 世界"
     |
     v
Shadow Claw: "trace 世界 → 需要 aliyun-sls-trace skill"
     |
     v
用 aliyun-sls-trace 采集 → 交给 Evidence Builder 结构化
```

```
Hypothesis Validator: "证据不足，需要进入 deploy 世界"
     |
     v
Shadow Claw: "deploy 世界 → 直接 git log + kubectl get events"
     |
     v
读取结果 → 交给 Evidence Builder 结构化
```

## 当某个世界进不去时

如果需要进入某个世界但缺少对应能力（比如没有配置 SLS，或者没有 K8s 权限），不要静默跳过。记录这个事实：

> "无法查询 trace 数据：当前环境未配置 aliyun-sls-trace"

这本身就是一条重要证据 — 你的观测能力有盲区。这会导致 Validator 给出 `insufficient_evidence` 而不是虚假的高 confidence。

## 如何扩展

团队有了新的数据源/工具时：

1. 它是基本操作（shell 命令、读文件）？→ 不需要注册，直接用
2. 它需要专门的 skill（有配置、有脚本、有特殊协议）？→ 写一个 skill，加到上面的列表
3. 它是 MCP server？→ 在 mcp/ 目录下配置，加到上面的列表
