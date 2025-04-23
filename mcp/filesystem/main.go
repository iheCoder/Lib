package main

import (
	"Lib/mcp/middleware"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// 定义工具名称
const (
	ToolListFiles   = "list_files"
	ToolCreateFile  = "create_file"
	ToolDeleteFile  = "delete_file"
	ToolWriteToFile = "write_to_file"
)

// 处理查看文件列表的工具请求
func handleListFiles(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentsPath, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	documentsPath = filepath.Join(documentsPath, "Documents/mcp")

	files, err := ioutil.ReadDir(documentsPath)
	if err != nil {
		return nil, err
	}

	var fileNames []string
	for _, file := range files {
		fileNames = append(fileNames, file.Name())
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("%v", fileNames),
			},
		},
	}, nil
}

// 处理创建文件的工具请求
func handleCreateFile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentsPath, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	documentsPath = filepath.Join(documentsPath, "Documents/mcp")

	fileName, ok := request.Params.Arguments["filename"].(string)
	if !ok {
		return nil, fmt.Errorf("missing 'filename' parameter")
	}

	filePath := filepath.Join(documentsPath, fileName)
	file, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("File %s created successfully", fileName),
			},
		},
	}, nil
}

// 处理写入文件的工具请求
func handleWriteToFile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentsPath, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	documentsPath = filepath.Join(documentsPath, "Documents/mcp")

	fileName, ok := request.Params.Arguments["filename"].(string)
	if !ok {
		return nil, fmt.Errorf("missing 'filename' parameter")
	}

	content, ok := request.Params.Arguments["content"].(string)
	if !ok {
		return nil, fmt.Errorf("missing 'content' parameter")
	}

	mode := "append"
	if m, ok := request.Params.Arguments["mode"].(string); ok && (m == "overwrite" || m == "append") {
		mode = m
	}

	filePath := filepath.Join(documentsPath, fileName)

	var file *os.File
	if mode == "overwrite" {
		file, err = os.Create(filePath)
	} else {
		file, err = os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if _, err := file.WriteString(content); err != nil {
		return nil, err
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("File %s written successfully in %s mode", fileName, mode),
			},
		},
	}, nil
}

// 处理删除文件的工具请求
func handleDeleteFile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentsPath, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	documentsPath = filepath.Join(documentsPath, "Documents")

	fileName, ok := request.Params.Arguments["filename"].(string)
	if !ok {
		return nil, fmt.Errorf("missing 'filename' parameter")
	}

	filePath := filepath.Join(documentsPath, fileName)
	err = os.Remove(filePath)
	if err != nil {
		return nil, err
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("File %s deleted successfully", fileName),
			},
		},
	}, nil
}

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
		mcp.WithString("filename", mcp.Description("The name of the file to write to")),
		mcp.WithString("content", mcp.Description("The content to write into the file")),
		mcp.WithString("mode", mcp.Description("Write mode: append (default) or overwrite")),
	), handleWriteToFile)

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
