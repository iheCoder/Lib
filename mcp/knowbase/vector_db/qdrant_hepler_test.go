package vector_db

import (
	"testing"
)

func TestQdrantHelper(t *testing.T) {
	helper, err := NewQdrantHelper("localhost:6334")
	if err != nil {
		t.Fatalf("Failed to connect Qdrant: %v", err)
	}
	defer helper.Close()

	collectionName := "test_collection"
	vectorSize := 4

	// 创建 Collection
	err = helper.CreateCollection(collectionName, vectorSize)
	if err != nil {
		t.Fatalf("Failed to create collection: %v", err)
	}

	// 插入向量
	points := []InsertPoint{
		{
			ID:     1,
			Vector: []float32{0.1, 0.2, 0.3, 0.4},
			Payload: map[string]interface{}{
				"tag": "test1",
			},
		},
		{
			ID:     2,
			Vector: []float32{0.2, 0.1, 0.4, 0.3},
			Payload: map[string]interface{}{
				"tag": "test2",
			},
		},
	}
	err = helper.InsertVectors(collectionName, points)
	if err != nil {
		t.Fatalf("Failed to insert vectors: %v", err)
	}

	// 搜索向量
	results, err := helper.Search(collectionName, []float32{0.1, 0.2, 0.3, 0.4}, 2)
	if err != nil {
		t.Fatalf("Failed to search vectors: %v", err)
	}

	if len(results) == 0 {
		t.Fatalf("No search results found")
	}

	t.Logf("Search results: %+v", results)
}
