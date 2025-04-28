package main

import (
	"Lib/mcp/middleware"
	"fmt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"net/http"
)

func main() {
	// 创建 MCP 服务器
	mcpServer := server.NewMCPServer(
		"file-server",
		"1.0.0",
		server.WithResourceCapabilities(true, true),
		server.WithPromptCapabilities(true),
		server.WithToolCapabilities(true),
		server.WithToolHandlerMiddleware(middleware.LoggingMiddleware),
	)

	// 添加查看文件列表的工具
	mcpServer.AddTool(mcp.NewTool(
		ToolListFiles,
		mcp.WithDescription("List files in ~/Documents/mcp"),
	), handleListFiles)

	// 添加创建文件的工具
	mcpServer.AddTool(mcp.NewTool(
		ToolCreateFile,
		mcp.WithDescription("Create a file in ~/Documents/mcp"),
		mcp.WithString("filename", mcp.Description("The name of the file to create")),
	), handleCreateFile)

	// 添加删除文件的工具
	mcpServer.AddTool(mcp.NewTool(
		ToolDeleteFile,
		mcp.WithDescription("Delete a file in ~/Documents/mcp"),
		mcp.WithString("filename", mcp.Description("The name of the file to delete")),
	), handleDeleteFile)

	// 添加写入文件的工具
	mcpServer.AddTool(mcp.NewTool(
		ToolWriteToFile,
		mcp.WithDescription("Write content to a file in ~/Documents/mcp"),
		mcp.WithString("filename", mcp.Description("The name of the file to write to"), mcp.Required()),
		mcp.WithString("content", mcp.Description("The content to write into the file"), mcp.Required()),
		mcp.WithString("mode", mcp.Description("Write mode: append (default) or overwrite")),
	), handleWriteToFile)

	// 添加读取文件的工具
	mcpServer.AddTool(mcp.NewTool(
		ToolReadFile,
		mcp.WithDescription("Read content from a file from local filesystem"),
		mcp.WithString("path", mcp.Description("The path of the file to read"), mcp.Required()),
		mcp.WithNumber("offset", mcp.Description("The offset to start reading from, default is 0")),
		mcp.WithNumber("size", mcp.Description("The number of bytes to read, default is all remaining bytes")),
	), handleReadFileContent)

	// 添加获取视频信息的工具
	mcpServer.AddTool(mcp.NewTool(
		ToolGetVideoInfo,
		mcp.WithDescription("Get information about a video file"),
		mcp.WithString("path", mcp.Description("The path of the video file"), mcp.Required()),
	), handleGetVideoInfo)

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
