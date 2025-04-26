package resource_handle

import (
	"errors"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"io"
	"os"
	"strings"
)

type HandleMode int32

const (
	// HandleModeByLines 按行分割
	HandleModeByLines HandleMode = iota
	// HandleModeByChars 按字符分割
	HandleModeByChars
)

// ExtractPDFTextIntoChunks extracts text from a PDF and splits it into chunks,
// either by lines or by characters depending on mode.
func ExtractPDFTextIntoChunks(filepath string, sizePerChunk int, mode HandleMode) ([]string, error) {
	text, err := extractTextFromPDF(filepath)
	if err != nil {
		return nil, err
	}

	switch mode {
	case HandleModeByLines:
		return splitTextIntoChunksByLines(text, sizePerChunk), nil
	case HandleModeByChars:
		return splitTextIntoChunksByChars(text, sizePerChunk), nil
	default:
		return splitTextIntoChunksByLines(text, sizePerChunk), nil
	}
}

// extractTextFromPDF 从指定的 PDF 文件中提取文本内容
func extractTextFromPDF(filePath string) (string, error) {
	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", errors.New("文件不存在")
	}

	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// 创建默认配置
	conf := model.NewDefaultConfiguration()
	conf.Cmd = model.EXTRACTCONTENT

	// 读取、验证和优化 PDF 文件
	ctx, err := api.ReadValidateAndOptimize(file, conf)
	if err != nil {
		return "", err
	}

	// 获取所有页面
	pages, err := api.PagesForPageSelection(ctx.PageCount, nil, true, true)
	if err != nil {
		return "", err
	}

	// 用于存储提取的文本
	var text string

	// 遍历所有页面
	for p, v := range pages {
		if !v {
			continue
		}

		// 提取页面内容
		r, err := pdfcpu.ExtractPageContent(ctx, p)
		if err != nil {
			return "", err
		}
		if r == nil {
			continue
		}

		// 读取页面内容
		content, err := io.ReadAll(r)
		if err != nil {
			return "", err
		}

		// 将页面内容添加到文本中
		text += string(content)
	}

	return text, nil
}

// splitTextIntoChunksByLines 按行分块
func splitTextIntoChunksByLines(text string, linesPerChunk int) []string {
	lines := strings.Split(text, "\n")
	var chunks []string

	for i := 0; i < len(lines); i += linesPerChunk {
		end := i + linesPerChunk
		if end > len(lines) {
			end = len(lines)
		}
		chunk := strings.Join(lines[i:end], "\n")
		chunks = append(chunks, chunk)
	}
	return chunks
}

// splitTextIntoChunksByChars 按字符分块
func splitTextIntoChunksByChars(text string, charsPerChunk int) []string {
	var chunks []string
	runes := []rune(text)
	for i := 0; i < len(runes); i += charsPerChunk {
		end := i + charsPerChunk
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}
