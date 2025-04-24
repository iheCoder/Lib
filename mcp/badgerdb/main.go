package main

import (
	"Lib/mcp/middleware"
	"log"
)

func main() {
	// 创建工具组
	dbGroup := middleware.ToolGroup{
		Name: "badgerdb tools",
		Items: []middleware.MCPToolItem{
			addKeyValueTool,
			getValueByKeyTool,
			listKeysAndValuesTool,
			searchKeysAndValuesTool,
		},
	}

	// 创建并启动 MCPServer
	server := middleware.NewMCPServer("badger-server", "1.0.0")
	server.AddToolGroup(dbGroup)
	if err := server.Run("http://localhost:8080", ":8080", "/sse", "/message"); err != nil {
		log.Fatalf("server run error: %v", err)
	}
}
