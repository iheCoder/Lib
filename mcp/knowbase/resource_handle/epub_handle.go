package resource_handle

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

// ExtractEPUBChunksByPython extracts text chunks from an EPUB file using Python
func ExtractEPUBChunksByPython(epubPath string, sizePerChunk int, mode HandleMode) ([]string, error) {
	cmd := exec.Command("python", "epub_handle.py", epubPath, fmt.Sprintf("%d", sizePerChunk), fmt.Sprintf("%d", mode))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("error: %v, output: %s", err, string(out))
	}

	var chunks []string
	err = json.Unmarshal(out, &chunks)
	if err != nil {
		return nil, err
	}

	return chunks, nil
}
