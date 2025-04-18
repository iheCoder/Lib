package main

import (
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
	ToolListFiles  = "list_files"
	ToolCreateFile = "create_file"
	ToolDeleteFile = "delete_file"
)

// 处理查看文件列表的工具请求
func handleListFiles(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	documentsPath, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	documentsPath = filepath.Join(documentsPath, "Documents")

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
	documentsPath = filepath.Join(documentsPath, "Documents")

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
	)

	// 添加查看文件列表的工具
	mcpServer.AddTool(mcp.NewTool(
		ToolListFiles,
		mcp.WithDescription("List files in ~/Documents"),
	), handleListFiles)

	// 添加创建文件的工具
	mcpServer.AddTool(mcp.NewTool(
		ToolCreateFile,
		mcp.WithDescription("Create a file in ~/Documents"),
		mcp.WithString("filename", mcp.Description("The name of the file to create")),
	), handleCreateFile)

	// 添加删除文件的工具
	mcpServer.AddTool(mcp.NewTool(
		ToolDeleteFile,
		mcp.WithDescription("Delete a file in ~/Documents"),
		mcp.WithString("filename", mcp.Description("The name of the file to delete")),
	), handleDeleteFile)

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
