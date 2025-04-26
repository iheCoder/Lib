package knowbase

import "testing"

func TestPdfToChunks(t *testing.T) {
	pdfPath := "./testdata/redbook-5th-edition.pdf"
	chunks, err := ExtractPDFTextIntoChunks(pdfPath, 10)
	if err != nil {
		t.Errorf("error: %v", err)
		return
	}

	for _, chunk := range chunks {
		t.Logf("chunk: %v", chunk)
	}
}
