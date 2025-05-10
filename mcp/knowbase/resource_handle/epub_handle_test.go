package resource_handle

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestExtractEPUBChunksByPython(t *testing.T) {
	epubPath := "../testdata/100 Go Mistakes and How to Avoid Them.epub"
	sizePerChunk := 100
	mode := HandleModeByLines // 使用导出的 HandleMode 常量

	chunks, err := ExtractEPUBChunksByPython(epubPath, sizePerChunk, mode)

	assert.NoError(t, err, "ExtractEPUBChunksByPython 应该成功执行")
	assert.NotEmpty(t, chunks, "返回的 chunks 不应为空")
	assert.True(t, len(chunks) > 0, "chunks 的数量应大于 0")

	for _, chunk := range chunks {
		assert.NotEmpty(t, chunk, "每个 chunk 不应为空")
	}
}
