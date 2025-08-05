package main

import (
	"Lib/mcp/middleware"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// getConfigPath 获取数据库配置文件路径
// 优先级：命令行参数 > 环境变量 > 默认路径
func getConfigPath(configFlag string) string {
	// 1. 首先检查命令行参数
	if configFlag != "" {
		return configFlag
	}

	// 2. 检查环境变量
	if envPath := os.Getenv("MYSQL_CONFIG_PATH"); envPath != "" {
		return envPath
	}

	// 3. 使用默认路径（空字符串将在initGlobalDB中处理为默认路径）
	return ""
}

func main() {
	// 解析命令行参数
	var mode = flag.String("mode", "stdio", "Server mode: stdio or sse")
	var port = flag.String("port", "8080", "Port for SSE server")
	var configPath = flag.String("config", "", "Path to database config file (can also be set via MYSQL_CONFIG_PATH env var)")
	flag.Parse()

	// 获取配置文件路径
	dbConfigPath := getConfigPath(*configPath)

	// 数据库初始化
	err := initGlobalDB(dbConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to MySQL: %v\n", err)
		os.Exit(1)
	}

	// 创建 MCP 服务器
	mcpServer := server.NewMCPServer(
		"mysql-readonly-server",
		"1.0.0",
		server.WithResourceCapabilities(true, true),
		server.WithPromptCapabilities(true),
		server.WithToolCapabilities(true),
		server.WithToolHandlerMiddleware(middleware.LoggingMiddleware),
	)

	// 添加查询mysql数据库数据的工具
	mcpServer.AddTool(mcp.NewTool(
		ToolExecuteMySQLQuery,
		mcp.WithDescription("查询mysql数据库数据"),
		mcp.WithString("query", mcp.Description("mysql查询语句"), mcp.Required()),
		mcp.WithNumber("max_tokens", mcp.Description("限制查询返回数据的最大token数量，请输入本模型的最大支持tokens数，默认为60000")),
		mcp.WithString("source", mcp.Description(fmt.Sprintf("指定使用的数据源名称，可选值：%v", getAvailableSources()))),
	), querySqlTool)

	// 根据模式启动相应的服务器
	switch *mode {
	case "sse":
		startSSEServer(mcpServer, *port)
	case "stdio":
		startStdioServer(mcpServer)
	default:
		startStdioServer(mcpServer)
	}
}

// startStdioServer 启动 stdio 模式服务器
func startStdioServer(mcpServer *server.MCPServer) {
	if err := server.ServeStdio(mcpServer); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start stdio server: %v\n", err)
		os.Exit(1)
	}
}

// startSSEServer 启动 SSE 模式服务器
func startSSEServer(mcpServer *server.MCPServer, port string) {
	// 创建 SSE 服务器
	sseServer := server.NewSSEServer(mcpServer,
		server.WithBaseURL(fmt.Sprintf("http://localhost:%s", port)),
		server.WithSSEEndpoint("/sse"),
		server.WithMessageEndpoint("/message"),
	)

	fmt.Printf("Starting SSE server on port %s\n", port)
	fmt.Printf("SSE endpoint: http://localhost:%s/sse\n", port)
	fmt.Printf("Message endpoint: http://localhost:%s/message\n", port)

	// 启动服务器
	if err := sseServer.Start(fmt.Sprintf(":%s", port)); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "Failed to start SSE server: %v\n", err)
		os.Exit(1)
	}
}
