package vector_db

import (
	"context"
	"fmt"
	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
)

type QdrantHelper struct {
	conn   *grpc.ClientConn
	client qdrant.PointsClient
	ctx    context.Context
}

// 初始化连接
func NewQdrantHelper(host string) (*QdrantHelper, error) {
	conn, err := grpc.Dial(host, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("connect Qdrant failed: %v", err)
	}

	client := qdrant.NewPointsClient(conn)

	return &QdrantHelper{
		conn:   conn,
		client: client,
		ctx:    context.Background(),
	}, nil
}

// 关闭连接
func (q *QdrantHelper) Close() {
	q.conn.Close()
}

// 创建 Collection
func (q *QdrantHelper) CreateCollection(collectionName string, vectorSize int) error {
	collectionsClient := qdrant.NewCollectionsClient(q.conn)

	_, err := collectionsClient.Create(q.ctx, &qdrant.CreateCollection{
		CollectionName: collectionName,
		VectorsConfig: &qdrant.VectorsConfig{
			Config: &qdrant.VectorsConfig_Params{
				Params: &qdrant.VectorParams{
					Size:     uint64(vectorSize),
					Distance: qdrant.Distance_Cosine,
				},
			},
		},
	})
	return err
}

// 插入向量
func (q *QdrantHelper) InsertVectors(collectionName string, points []InsertPoint) error {
	var qPoints []*qdrant.PointStruct
	for _, pt := range points {
		qPoints = append(qPoints, &qdrant.PointStruct{
			Id: &qdrant.PointId{
				PointIdOptions: &qdrant.PointId_Num{Num: uint64(pt.ID)},
			},
			Vectors: &qdrant.Vectors{
				VectorsOptions: &qdrant.Vectors_Vector{
					Vector: &qdrant.Vector{Data: pt.Vector},
				},
			},
			Payload: toPayload(pt.Payload),
		})
	}

	_, err := q.client.Upsert(q.ctx, &qdrant.UpsertPoints{
		CollectionName: collectionName,
		Points:         qPoints,
	})
	return err
}

// 搜索相似向量
func (q *QdrantHelper) Search(collectionName string, vector []float32, topK int) ([]SearchResult, error) {
	resp, err := q.client.Search(q.ctx, &qdrant.SearchPoints{
		CollectionName: collectionName,
		Vector:         vector,
		Limit:          uint64(topK),
	})
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	for _, res := range resp.Result {
		results = append(results, SearchResult{
			ID:      res.Id,
			Score:   float64(res.Score),
			Payload: fromPayload(res.Payload),
		})
	}

	return results, nil
}

// InsertPoint 辅助结构
type InsertPoint struct {
	ID      int64
	Vector  []float32
	Payload map[string]interface{}
}

// SearchResult 辅助结构
type SearchResult struct {
	ID      interface{}
	Score   float64
	Payload map[string]interface{}
}

// payload 转换 helpers
func toPayload(m map[string]interface{}) map[string]*qdrant.Value {
	payload := make(map[string]*qdrant.Value)
	for k, v := range m {
		switch val := v.(type) {
		case string:
			payload[k] = &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: val}}
		case float64:
			payload[k] = &qdrant.Value{Kind: &qdrant.Value_DoubleValue{DoubleValue: val}}
		}
	}
	return payload
}

func fromPayload(p map[string]*qdrant.Value) map[string]interface{} {
	payload := make(map[string]interface{})
	for k, v := range p {
		switch val := v.Kind.(type) {
		case *qdrant.Value_StringValue:
			payload[k] = val.StringValue
		case *qdrant.Value_DoubleValue:
			payload[k] = val.DoubleValue
		}
	}
	return payload
}
