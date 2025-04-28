package main

import (
	"Lib/mcp/filesystem"
	"Lib/mcp/tool_group"
	"log"
)

func main() {
	// 创建并启动 MCPServer
	server := tool_group.NewMCPServer("file-server", "1.0.0")
	server.AddToolGroup(filesystem.FileSystemToolGroup)
	if err := server.Run("http://localhost:8080", ":8080", "/sse", "/message"); err != nil {
		log.Fatalf("server run error: %v", err)
	}
}
