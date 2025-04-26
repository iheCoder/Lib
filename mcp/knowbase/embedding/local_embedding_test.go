package knowbase

import (
	"fmt"
	"testing"
)

func TestLocalEmbedding(t *testing.T) {
	texts := []string{"你好，世界", "hello, world"}

	embeddings, err := generateLocalEmbedding(texts)
	if err != nil {
		t.Errorf("Embedding error: %v\n", err)
		return
	}

	for i, emb := range embeddings {
		fmt.Printf("Embedding #%d (len=%d): %.4f %.4f ...\n", i+1, len(emb), emb[0], emb[1])
	}
}
