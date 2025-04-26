package knowbase

import "testing"

func TestGenerateEmbedding(t *testing.T) {
	text := "Hello, world!"
	embedding, err := generateEmbedding(text)
	if err != nil {
		t.Errorf("error generating embedding: %v", err)
		return
	}

	if len(embedding) != len(text) {
		t.Errorf("len(embedding) != len(text)")
	}
	for _, v := range embedding {
		if v < -1 || v > 1 {
			t.Errorf("embedding value out of range: %f", v)
		}
	}
}
