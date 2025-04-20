package main

import (
	"fmt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"net/http"
)

func main() {
	// 数据库初始化
	var err error
	err = initGlobalDB()
	if err != nil {
		fmt.Printf("Failed to connect to MySQL: %v\n", err)
		return
	}

	// 创建 MCP 服务器
	mcpServer := server.NewMCPServer(
		"mysql-readonly-server",
		"1.0.0",
		server.WithResourceCapabilities(true, true),
		server.WithPromptCapabilities(true),
		server.WithToolCapabilities(true),
		server.WithToolHandlerMiddleware(loggingMiddleware),
	)

	// 添加查询mysql数据库数据的工具
	mcpServer.AddTool(mcp.NewTool(
		ToolExecuteMySQLQuery,
		mcp.WithDescription("查询mysql数据库数据"),
		mcp.WithString("query", mcp.Description("mysql查询语句"), mcp.Required()),
	), querySqlTool)

	// 创建 SSE 服务器
	sseServer := server.NewSSEServer(mcpServer,
		server.WithBaseURL("http://localhost:8080"),
		server.WithSSEEndpoint("/sse"),
		server.WithMessageEndpoint("/message"),
	)

	// 启动服务器
	go func() {
		if err := sseServer.Start(":8080"); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Failed to start server: %v\n", err)
		}
	}()

	fmt.Println("Server started on :8080")
	// 保持程序运行
	select {}
}
