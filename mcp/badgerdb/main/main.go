package main

import (
	"Lib/mcp/badgerdb"
	"Lib/mcp/tool_group"
	"log"
)

func main() {
	// 创建并启动 MCPServer
	server := tool_group.NewMCPServer("badger-server", "1.0.0")
	server.AddToolGroup(badgerdb.BadgerDBToolGroup)
	if err := server.Run("http://localhost:8080", ":8080", "/sse", "/message"); err != nil {
		log.Fatalf("server run error: %v", err)
	}
}
