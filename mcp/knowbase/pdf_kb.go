package knowbase

import (
	"Lib/mcp/knowbase/embedding"
	"Lib/mcp/knowbase/resource_handle"
	"Lib/mcp/knowbase/vector_db"
	"fmt"
)

// PDFToVectorImporter 管理从 PDF 到向量库的导入流程
type PDFToVectorImporter struct {
	ChunkLines     int                     // 每个chunk多少行
	CollectionName string                  // 存到哪个collection
	VectorSize     int                     // 向量维度（比如384）
	Qdrant         *vector_db.QdrantHelper // Qdrant客户端
}

// NewPDFToVectorImporter 创建一个新的PDFToVectorImporter
func NewPDFToVectorImporter(chunkLines int, collectionName string, vectorSize int, qdrant *vector_db.QdrantHelper) *PDFToVectorImporter {
	return &PDFToVectorImporter{
		ChunkLines:     chunkLines,
		CollectionName: collectionName,
		VectorSize:     vectorSize,
		Qdrant:         qdrant,
	}
}

// Import 完成一整套流程
func (p *PDFToVectorImporter) Import(pdfPath string) error {
	// 1. 提取PDF文本切片
	chunks, err := resource_handle.ExtractPDFChunksByPython(pdfPath, p.ChunkLines, resource_handle.HandleModeByChars)
	if err != nil {
		return fmt.Errorf("failed to extract PDF text: %w", err)
	}

	if len(chunks) == 0 {
		return fmt.Errorf("no content extracted from PDF")
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
	err = p.Qdrant.CreateCollection(p.CollectionName, p.VectorSize)
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
				"chunk_text": chunks[i],
				"source_pdf": pdfPath,
				"chunk_idx":  i,
			},
		}
		points = append(points, point)
	}

	// 5. 写入Qdrant
	err = p.Qdrant.InsertVectors(p.CollectionName, points)
	if err != nil {
		return fmt.Errorf("failed to insert vectors: %w", err)
	}

	return nil
}
