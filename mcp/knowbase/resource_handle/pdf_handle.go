package resource_handle

import (
	"github.com/ledongthuc/pdf"
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
// either by lines or by characters depending on mode ("lines" or "chars").
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

func extractTextFromPDF(filePath string) (string, error) {
	f, r, err := pdf.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var textBuilder strings.Builder
	totalPage := r.NumPage()
	for pageIndex := 1; pageIndex <= totalPage; pageIndex++ {
		p := r.Page(pageIndex)
		if p.V.IsNull() {
			continue
		}
		content, err := p.GetPlainText(nil)
		if err != nil {
			return "", err
		}
		textBuilder.WriteString(content)
	}

	return textBuilder.String(), nil
}

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

// splitTextIntoChunksByChars splits text into chunks of n characters (runes).
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
