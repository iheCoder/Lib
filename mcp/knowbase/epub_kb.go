package knowbase

import (
	"Lib/mcp/knowbase/embedding"
	"Lib/mcp/knowbase/resource_handle"
	"Lib/mcp/knowbase/vector_db"
	"fmt"
)

// EPUBToVectorImporter 管理从 EPUB 到向量库的导入流程
type EPUBToVectorImporter struct {
	ChunkSize      int                        // 每个chunk的大小（行数/字符数/段落智能分块）
	CollectionName string                     // 存到哪个collection
	VectorSize     int                        // 向量维度
	Qdrant         *vector_db.QdrantHelper    // Qdrant客户端
	Mode           resource_handle.HandleMode // 分块模式
}

// NewEPUBToVectorImporter 创建一个新的EPUBToVectorImporter
func NewEPUBToVectorImporter(chunkSize int, collectionName string, vectorSize int, qdrant *vector_db.QdrantHelper, mode resource_handle.HandleMode) *EPUBToVectorImporter {
	return &EPUBToVectorImporter{
		ChunkSize:      chunkSize,
		CollectionName: collectionName,
		VectorSize:     vectorSize,
		Qdrant:         qdrant,
		Mode:           mode,
	}
}

// Import 完成一整套流程
func (e *EPUBToVectorImporter) Import(epubPath string) error {
	// 1. 提取EPUB文本切片
	chunks, err := resource_handle.ExtractEPUBChunksByPython(epubPath, e.ChunkSize, e.Mode)
	if err != nil {
		return fmt.Errorf("failed to extract EPUB text: %w", err)
	}

	if len(chunks) == 0 {
		return fmt.Errorf("no content extracted from EPUB")
	}

	// 2. 调用本地embedding服务，生成向量
	embeddings, err := knowbase.GenerateLocalEmbedding(chunks)
	if err != nil {
		return fmt.Errorf("failed to generate embedding: %w", err)
	}

	if len(embeddings) != len(chunks) {
		return fmt.Errorf("embedding length mismatch: got %d embeddings for %d chunks", len(embeddings), len(chunks))
	}

	// 3. 创建collection（如果不存在）
	err = e.Qdrant.CreateCollection(e.CollectionName, e.VectorSize)
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}

	// 4. 组装成InsertPoint
	var points []vector_db.InsertPoint
	for i, emb := range embeddings {
		vec32 := make([]float32, len(emb))
		for j, v := range emb {
			vec32[j] = float32(v)
		}

		point := vector_db.InsertPoint{
			ID:     int64(i),
			Vector: vec32,
			Payload: map[string]interface{}{
				"chunk_text":  chunks[i],
				"source_epub": epubPath,
				"chunk_idx":   i,
			},
		}
		points = append(points, point)
	}

	// 5. 写入Qdrant
	err = e.Qdrant.InsertVectors(e.CollectionName, points)
	if err != nil {
		return fmt.Errorf("failed to insert vectors: %w", err)
	}

	return nil
}
