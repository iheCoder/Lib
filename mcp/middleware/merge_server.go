package middleware

import (
	"context"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type MCPMode int32

const (

	// ModeSSE 表示服务器推送事件模式
	ModeSSE MCPMode = iota
	// ModeStdIO 表示标准输入输出模式
	ModeStdIO
	// ModeHTTP 表示HTTP模式
	ModeHTTP
)

type MCPServer struct {
	ToolGroups []ToolGroup
	Name       string
	Version    string
	Mode       MCPMode
	server     *server.MCPServer
}

func NewMCPServer(name, version string, options ...server.ServerOption) *MCPServer {
	ms := &MCPServer{
		Name:    name,
		Version: version,
	}

	ms.server = server.NewMCPServer(name, version, options...)
	return ms
}

// AddToolGroup 添加一个工具组
func (ms *MCPServer) AddToolGroup(group ToolGroup) {
	ms.ToolGroups = append(ms.ToolGroups, group)

	for _, item := range group.Items {
		ms.server.AddTool(item.Tool, item.Handler)
	}
}

func (ms *MCPServer) Run(baseURL, addr, sseEndpoint, messageEndpoint string) error {
	sseServer := server.NewSSEServer(ms.server,
		server.WithBaseURL(baseURL),
		server.WithSSEEndpoint(sseEndpoint),
		server.WithMessageEndpoint(messageEndpoint),
	)

	// 启动服务器
	if err := sseServer.Start(addr); err != nil {
		return err
	}

	// 处理系统信号
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChan
		sseServer.Shutdown(context.Background())
	}()

	// 等待服务器完全启动
	// 这里可以添加一些逻辑来确保服务器完全启动
	// 例如，等待一段时间或检查某个状态
	time.Sleep(100 * time.Millisecond)
	log.Printf("服务器已启动，可以通过 %s 访问\n", baseURL+sseEndpoint)

	return nil
}

// ToolGroup 代表一个工具组，包含多个工具项
type ToolGroup struct {
	Name  string
	Items []MCPToolItem
}

func (tg *ToolGroup) AddToolItem(tool mcp.Tool, handler server.ToolHandlerFunc) {
	tg.Items = append(tg.Items, MCPToolItem{
		Tool:    tool,
		Handler: handler,
	})
}

type MCPToolItem struct {
	Tool    mcp.Tool
	Handler server.ToolHandlerFunc
}
