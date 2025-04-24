package tool_group

import (
	"Lib/mcp/middleware"
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

	// 默认添加logging中间件
	options = append(options, server.WithToolHandlerMiddleware(middleware.LoggingMiddleware))
	// 默认添加资源能力
	options = append(options, server.WithResourceCapabilities(true, true))
	// 默认添加提示能力
	options = append(options, server.WithPromptCapabilities(true))
	// 默认添加工具能力
	options = append(options, server.WithToolCapabilities(true))

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
	go func() {
		if err := sseServer.Start(addr); err != nil {
			log.Printf("服务器启动失败: %v\n", err)
			return
		}
	}()

	// 等待服务器完全启动
	// 这里可以添加一些逻辑来确保服务器完全启动
	// 例如，等待一段时间或检查某个状态
	time.Sleep(100 * time.Millisecond)
	log.Printf("服务器已启动，可以通过 %s 访问\n", baseURL+sseEndpoint)

	// 处理系统信号
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	<-signalChan
	sseServer.Shutdown(context.Background())
	log.Println("服务器已安全关闭")

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
