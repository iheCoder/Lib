package resource_handle

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

func ExtractPDFChunksByPython(pdfPath string, sizePerChunk int, mode HandleMode) ([]string, error) {
	cmd := exec.Command("python3", "pdf_handle.py", pdfPath, fmt.Sprintf("%d", sizePerChunk), fmt.Sprintf("%d", mode))
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var chunks []string
	err = json.Unmarshal(out, &chunks)
	if err != nil {
		return nil, err
	}

	return chunks, nil
}

type HandleMode int32

const (
	// HandleModeByLines 按行分割
	HandleModeByLines HandleMode = iota
	// HandleModeByChars 按字符分割
	HandleModeByChars
)

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
