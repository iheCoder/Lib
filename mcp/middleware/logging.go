package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// LoggingMiddleware 中间件函数，用于打印工具调用的输入和输出
func LoggingMiddleware(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// 打印工具名称和输入
		fmt.Printf("Tool name: %s\n", request.Params.Name)
		fmt.Printf("Tool call input: %+v\n", request.Params.Arguments)

		// 调用下一个处理函数
		result, err := next(ctx, request)

		// 打印输出
		if result != nil {
			// 将输出转换为 JSON 字符串
			resultJSON, _ := json.Marshal(result)
			fmt.Printf("Tool call output: %s\n", string(resultJSON))
		} else {
			fmt.Printf("Tool call output: nil\n")
		}

		return result, err
	}
}
