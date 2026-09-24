# MongoDB readonly MCP

这个服务只提供明确的读取工具：列出数据库、集合和索引，查询文档，计数、去重及聚合。默认使用 stdio，工具输出为 JSON；ObjectId 和日期使用 MongoDB Extended JSON 表示。

## 配置与启动

将 `config/database.example.yaml` 复制为 `config/database.yaml`，填写 MongoDB URI。真实配置已由此目录的 `.gitignore` 排除；不要把密码写进 `mcp.json`。每个 `source` 可配置 `default_database`，调用时传入 `database` 可以覆盖默认值。单数据源时 `source` 可以省略。

在本目录执行：

```powershell
go run . -config config/database.yaml
```

也可以通过 `MONGODB_CONFIG_PATH` 指定配置路径。`mcp.json` 给出了 Codex 的 stdio 启动配置。

## 查询边界

- `find_mongo_documents` 支持 Extended JSON 的 `filter`、`projection`、`sort`、`skip`、`limit`。计数同时提供带筛选的精确值和整个集合的快速估算值。
- `aggregate_mongo_documents` 支持常见读阶段及 `$lookup`、`$facet`、`$unionWith` 的嵌套管道；写入阶段、未知阶段和服务端 JavaScript 表达式会被拒绝。具体阶段仍需目标 MongoDB 版本支持。
- 查询默认最多返回 100 条，可提高到 500 条；输出最多 1 MiB，单次调用超时为 10 秒。`count`、`distinct` 和聚合仍可能扫描大量数据，因此需要合理筛选条件与索引。
- 数据库账号应仅授予目标数据库的读取权限。工具自身的操作白名单不能代替数据库授权。

本地实例报告 wire version 6，因此依赖支持 MongoDB 3.6 的官方 Go 驱动 v1。该实例的副本集发现地址不可从当前机器访问，本地 URI 使用 `directConnection=true` 固定入口地址。

## 验证

```powershell
go test .
$env:MONGODB_READONLY_TEST_CONFIG = 'config/database.yaml'
go test . -run TestLiveReadTools -v
```
