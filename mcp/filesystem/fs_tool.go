package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/mark3labs/mcp-go/mcp"
	"io/ioutil"
	"os"
	"path/filepath"
)

// 定义工具名称
const (
	ToolListFiles   = "list_files"
	ToolCreateFile  = "create_file"
	ToolDeleteFile  = "delete_file"
	ToolWriteToFile = "write_to_file"
	ToolReadFile    = "read_file"
)

// 读取文件内容
func handleReadFileContent(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := request.Params.Arguments["path"].(string)
	foffset, _ := request.Params.Arguments["offset"].(float64)
	fsize, _ := request.Params.Arguments["size"].(float64)
	offset := int64(foffset)
	size := int64(fsize)

	// Open file
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %v", err)
	}

	// Validate file size
	if fileInfo.Size() == 0 {
		return nil, errors.New("file is empty")
	} else if fileInfo.IsDir() {
		return nil, errors.New("path is a directory")
	} else if size > fileInfo.Size()+offset || size <= 0 {
		size = fileInfo.Size() - offset
	}

	// Validate offset
	if offset < 0 || offset > fileInfo.Size() {
		return nil, errors.New("invalid offset")
	}

	// Calculate read size
	if size < 0 || offset+size > fileInfo.Size() {
		size = fileInfo.Size() - offset
	}

	// Seek to offset
	_, err = file.Seek(offset, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to seek file: %v", err)
	}

	// Read content
	buffer := make([]byte, size)
	_, err = file.Read(buffer)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	// Return result
	return mcp.NewToolResultText(string(buffer)), nil
}

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
