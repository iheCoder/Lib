package knowbase

import (
	knowbase "Lib/mcp/knowbase/embedding"
	"Lib/mcp/knowbase/vector_db"
	"testing"
)

func TestPDFKB(t *testing.T) {
	helper, err := vector_db.NewQdrantHelper("localhost:6334")
	if err != nil {
		t.Fatalf("Failed to connect Qdrant: %v", err)
	}

	chunkLines := 1000
	collectionName := "redbook_5th_edition_collection"
	vectorSize := 384
	importer := NewPDFToVectorImporter(chunkLines, collectionName, vectorSize, helper)
	err = importer.Import("./testdata/redbook-5th-edition.pdf")
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// 测试查询
	query := "what's new DBMS Architectures ?"
	// 生成embedding
	queryEmbedding, err := knowbase.GenerateLocalEmbedding([]string{query})
	if err != nil {
		t.Fatalf("GenerateLocalEmbedding failed: %v", err)
	}

	if len(queryEmbedding) == 0 {
		t.Fatalf("Empty embedding returned")
	}

	// 把 []float64 转成 []float32
	queryVec := make([]float32, len(queryEmbedding[0]))
	for i, v := range queryEmbedding[0] {
		queryVec[i] = float32(v)
	}

	resp, err := importer.Qdrant.Search(collectionName, queryVec, 1)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(resp) == 0 {
		t.Fatalf("No results returned")
	}

	t.Logf("Top result: %s", resp[0].Payload["chunk_text"])
}
