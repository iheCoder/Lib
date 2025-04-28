package resource_handle

import "testing"

func TestExtractPDFChunksByPython(t *testing.T) {
	pdfPath := "../testdata/redbook-5th-edition.pdf"
	chunks, err := ExtractPDFChunksByPython(pdfPath, 1000, HandleModeByChars)
	if err != nil {
		t.Errorf("error: %v", err)
		return
	}

	for _, chunk := range chunks {
		t.Logf("chunk: %v", chunk)
	}
}
