package filesystem

import (
	"Lib/mcp/tool_group"
	"context"
	"errors"
	"fmt"
	"github.com/mark3labs/mcp-go/mcp"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// 文件系统工具：列出文件
var listFilesTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool(
		ToolListFiles,
		mcp.WithDescription("List files in ~/Documents/mcp"),
	),
	Handler: handleListFiles,
}

// 文件系统工具：创建文件
var createFileTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool(
		ToolCreateFile,
		mcp.WithDescription("Create a file in ~/Documents/mcp"),
		mcp.WithString("filename", mcp.Description("The name of the file to create")),
	),
	Handler: handleCreateFile,
}

// 文件系统工具：删除文件
var deleteFileTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool(
		ToolDeleteFile,
		mcp.WithDescription("Delete a file in ~/Documents/mcp"),
		mcp.WithString("filename", mcp.Description("The name of the file to delete")),
	),
	Handler: handleDeleteFile,
}

// 文件系统工具：写入文件
var writeToFileTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool(
		ToolWriteToFile,
		mcp.WithDescription("Write content to a file in ~/Documents/mcp"),
		mcp.WithString("filename", mcp.Description("The name of the file to write to"), mcp.Required()),
		mcp.WithString("content", mcp.Description("The content to write into the file"), mcp.Required()),
		mcp.WithString("mode", mcp.Description("Write mode: append (default) or overwrite")),
	),
	Handler: handleWriteToFile,
}

// 文件系统工具：读取文件内容
var readFileTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool(
		ToolReadFile,
		mcp.WithDescription("Read content from a file from local filesystem"),
		mcp.WithString("path", mcp.Description("The path of the file to read"), mcp.Required()),
		mcp.WithNumber("offset", mcp.Description("The offset to start reading from, default is 0")),
		mcp.WithNumber("size", mcp.Description("The number of bytes to read, default is all remaining bytes")),
	),
	Handler: handleReadFileContent,
}

// 文件系统工具：按分隔符读取内容
var readFileByDelimiterTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool(
		ToolReadFileByDelimiter,
		mcp.WithDescription("Read content from a file using a delimiter"),
		mcp.WithString("path", mcp.Description("Path of the file to read"), mcp.Required()),
		mcp.WithString("delimiter", mcp.Description("Delimiter to split the file content (default is \\n)")),
		mcp.WithNumber("offset", mcp.Description("Offset line number to start reading")),
		mcp.WithNumber("size", mcp.Description("Number of segments to return from offset")),
	),
	Handler: handleReadFileByDelimiter,
}

// FileSystemToolGroup 组合成工具组
var FileSystemToolGroup = tool_group.ToolGroup{
	Name: "filesystem",
	Items: []tool_group.MCPToolItem{
		listFilesTool,
		createFileTool,
		deleteFileTool,
		writeToFileTool,
		readFileTool,
		readFileByDelimiterTool,
	},
}

// 定义工具名称
const (
	ToolListFiles           = "list_files"
	ToolCreateFile          = "create_file"
	ToolDeleteFile          = "delete_file"
	ToolWriteToFile         = "write_to_file"
	ToolReadFile            = "read_file"
	ToolReadFileByDelimiter = "read_file_by_delimiter"
)

// handleReadFileByDelimiter 处理读取根据分隔符读取文件的工具请求
func handleReadFileByDelimiter(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, ok := request.Params.Arguments["path"].(string)
	if !ok {
		return nil, fmt.Errorf("missing 'path' parameter")
	}

	offset := 0
	if v, ok := request.Params.Arguments["offset"].(float64); ok {
		offset = int(v)
	}

	size := -1
	if v, ok := request.Params.Arguments["size"].(float64); ok {
		size = int(v)
	}

	delimiter := "\n"
	if d, ok := request.Params.Arguments["delimiter"].(string); ok {
		delimiter = d
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	chunks := strings.Split(string(data), delimiter)
	if offset >= len(chunks) {
		return nil, fmt.Errorf("offset out of range")
	}

	end := len(chunks)
	if size > 0 && offset+size < len(chunks) {
		end = offset + size
	}

	selected := chunks[offset:end]
	result := strings.Join(selected, delimiter)

	return mcp.NewToolResultText(result), nil
}

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
