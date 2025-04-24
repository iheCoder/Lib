package mcp

import (
	bg "Lib/mcp/badgerdb"
	fs "Lib/mcp/filesystem"
	"Lib/mcp/tool_group"
	"testing"
)

func TestMergeServerToolGroups(t *testing.T) {
	// 创建一个新的MCPServer实例
	server := tool_group.NewMCPServer("fs-badger-server", "1.0.0")

	// 添加工具组
	server.AddToolGroup(bg.BadgerDBToolGroup)
	server.AddToolGroup(fs.FileSystemToolGroup)

	// 启动服务器
	err := server.Run("http://localhost:8080", ":8080", "/sse", "/message")
	if err != nil {
		t.Fatalf("Failed to run server: %v", err)
	}
}
