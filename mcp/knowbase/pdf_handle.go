package knowbase

import (
	"github.com/ledongthuc/pdf"
	"strings"
)

func ExtractPDFTextIntoChunks(filepath string, linesPerChunk int) ([]string, error) {
	text, err := extractTextFromPDF(filepath)
	if err != nil {
		return nil, err
	}

	return splitTextIntoChunks(text, linesPerChunk), nil
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

func splitTextIntoChunks(text string, linesPerChunk int) []string {
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
