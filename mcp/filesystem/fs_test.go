package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

func TestHandleWriteToFile(t *testing.T) {
	// 构造请求
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"filename": "test_write.txt",
		"content":  "Hello, MCP!",
		"mode":     "overwrite",
	}

	// 调用写入方法
	result, err := handleWriteToFile(context.Background(), req)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// 验证文件内容
	home, _ := os.UserHomeDir()
	filePath := filepath.Join(home, "Documents/mcp", "test_write.txt")
	data, err := os.ReadFile(filePath)
	assert.NoError(t, err)
	assert.Equal(t, "Hello, MCP!", string(data))

	// 清理测试文件
	_ = os.Remove(filePath)
}
