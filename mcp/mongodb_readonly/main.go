package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	configPath := flag.String("config", "", "MongoDB YAML config path (or MONGODB_CONFIG_PATH)")
	flag.Parse()

	registry, err := loadSources(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "MongoDB MCP startup failed: %v\n", err)
		os.Exit(1)
	}
	defer registry.close()

	tools := &readTools{registry: registry}
	mcpServer := server.NewMCPServer("mongodb-readonly-server", "1.0.0", server.WithToolCapabilities(true))
	sourceDescription := "数据源名称，可选值: " + strings.Join(registry.names(), ", ") + "。单数据源时可省略"
	databaseDescription := "数据库名；配置了 default_database 时可省略"

	// 元数据读取只调用官方列表 API；数据库权限决定实际可见范围。
	mcpServer.AddTool(mcp.NewTool("list_mongo_databases",
		mcp.WithDescription("列出当前数据源账号可见的 MongoDB 数据库"),
		mcp.WithString("source", mcp.Description(sourceDescription)),
	), tools.handle("list_databases", tools.listDatabases))
	mcpServer.AddTool(mcp.NewTool("list_mongo_collections",
		mcp.WithDescription("列出指定数据库中的集合"),
		mcp.WithString("source", mcp.Description(sourceDescription)),
		mcp.WithString("database", mcp.Description(databaseDescription)),
	), tools.handle("list_collections", tools.listCollections))
	mcpServer.AddTool(mcp.NewTool("list_mongo_indexes",
		mcp.WithDescription("查看集合索引定义"),
		mcp.WithString("source", mcp.Description(sourceDescription)),
		mcp.WithString("database", mcp.Description(databaseDescription)),
		mcp.WithString("collection", mcp.Description("集合名"), mcp.Required()),
	), tools.handle("list_indexes", tools.listIndexes))

	// 业务数据读取统一接受 Extended JSON，以保留 ObjectId、日期等 BSON 类型。
	mcpServer.AddTool(mcp.NewTool("find_mongo_documents",
		mcp.WithDescription("查询集合文档；最多返回 500 条、1 MiB，单次最多执行 10 秒"),
		mcp.WithString("source", mcp.Description(sourceDescription)),
		mcp.WithString("database", mcp.Description(databaseDescription)),
		mcp.WithString("collection", mcp.Description("集合名"), mcp.Required()),
		mcp.WithString("filter", mcp.Description("Extended JSON 查询条件，默认 {}")),
		mcp.WithString("projection", mcp.Description("Extended JSON 投影文档")),
		mcp.WithString("sort", mcp.Description("Extended JSON 排序文档，如 {\"created_at\":-1}")),
		mcp.WithNumber("skip", mcp.Description("跳过文档数，最多 100000")),
		mcp.WithNumber("limit", mcp.Description("返回文档数，默认 100、最多 500")),
	), tools.handle("find", tools.find))
	mcpServer.AddTool(mcp.NewTool("count_mongo_documents",
		mcp.WithDescription("按条件精确统计文档数；大集合可能需要扫描"),
		mcp.WithString("source", mcp.Description(sourceDescription)),
		mcp.WithString("database", mcp.Description(databaseDescription)),
		mcp.WithString("collection", mcp.Description("集合名"), mcp.Required()),
		mcp.WithString("filter", mcp.Description("Extended JSON 查询条件，默认 {}")),
	), tools.handle("count", tools.count))
	mcpServer.AddTool(mcp.NewTool("estimate_mongo_document_count",
		mcp.WithDescription("快速估算整个集合的文档数，不接受筛选条件"),
		mcp.WithString("source", mcp.Description(sourceDescription)),
		mcp.WithString("database", mcp.Description(databaseDescription)),
		mcp.WithString("collection", mcp.Description("集合名"), mcp.Required()),
	), tools.handle("estimated_count", tools.estimatedCount))
	mcpServer.AddTool(mcp.NewTool("distinct_mongo_values",
		mcp.WithDescription("按条件返回指定字段的不重复值；结果最多 1 MiB"),
		mcp.WithString("source", mcp.Description(sourceDescription)),
		mcp.WithString("database", mcp.Description(databaseDescription)),
		mcp.WithString("collection", mcp.Description("集合名"), mcp.Required()),
		mcp.WithString("field", mcp.Description("字段名"), mcp.Required()),
		mcp.WithString("filter", mcp.Description("Extended JSON 查询条件，默认 {}")),
	), tools.handle("distinct", tools.distinct))
	mcpServer.AddTool(mcp.NewTool("aggregate_mongo_documents",
		mcp.WithDescription("执行常用只读聚合，支持 $lookup、$facet、$unionWith 等；拒绝写入和脚本阶段"),
		mcp.WithString("source", mcp.Description(sourceDescription)),
		mcp.WithString("database", mcp.Description(databaseDescription)),
		mcp.WithString("collection", mcp.Description("集合名"), mcp.Required()),
		mcp.WithString("pipeline", mcp.Description("Extended JSON 聚合阶段数组"), mcp.Required()),
		mcp.WithNumber("limit", mcp.Description("返回文档数，默认 100、最多 500")),
	), tools.handle("aggregate", tools.aggregate))

	// stdio 的 stdout 只交给 MCP 协议；错误信息写入 stderr。
	if err := server.ServeStdio(mcpServer); err != nil {
		fmt.Fprintf(os.Stderr, "MongoDB MCP stopped: %v\n", err)
		os.Exit(1)
	}
}
