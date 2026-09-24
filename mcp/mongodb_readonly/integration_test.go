package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestLiveReadTools(t *testing.T) {
	// 场景：仅在显式给出本地配置时连接真实 MongoDB。
	// 依次验证 source、库、集合和索引可读，再用一条实际文档验证查询、计数、去重与聚合。
	configPath := os.Getenv("MONGODB_READONLY_TEST_CONFIG")
	if configPath == "" {
		t.Skip("set MONGODB_READONLY_TEST_CONFIG to run against a MongoDB instance")
	}
	registry, err := loadSources(configPath)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.close()
	tools := &readTools{registry: registry}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	request := func(args map[string]any) mcp.CallToolRequest {
		var call mcp.CallToolRequest
		call.Params.Arguments = args
		return call
	}
	if _, err := tools.listDatabases(ctx, request(nil)); err != nil {
		t.Fatalf("list databases: %v", err)
	}
	collectionsText, err := tools.listCollections(ctx, request(nil))
	if err != nil {
		t.Fatalf("list collections: %v", err)
	}
	var collections []string
	if err := json.Unmarshal([]byte(collectionsText), &collections); err != nil {
		t.Fatal(err)
	}
	if len(collections) == 0 {
		t.Skip("default database has no collections")
	}
	args := map[string]any{"collection": collections[0], "limit": float64(1)}
	if _, err := tools.listIndexes(ctx, request(args)); err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	if _, err := tools.estimatedCount(ctx, request(args)); err != nil {
		t.Fatalf("estimated count: %v", err)
	}

	// 查找一条样本文档，并只用它的 _id 做后续精确读取，避免全表扫描。
	var sample map[string]json.RawMessage
	for _, collection := range collections {
		args["collection"] = collection
		found, err := tools.find(ctx, request(args))
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		var documents []map[string]json.RawMessage
		if err := json.Unmarshal([]byte(found), &documents); err != nil {
			t.Fatal(err)
		}
		if len(documents) > 0 {
			sample = documents[0]
			break
		}
	}
	if sample == nil || sample["_id"] == nil {
		t.Skip("no document with _id found")
	}
	filter, err := json.Marshal(map[string]json.RawMessage{"_id": sample["_id"]})
	if err != nil {
		t.Fatal(err)
	}
	args["filter"] = string(filter)
	if _, err := tools.count(ctx, request(args)); err != nil {
		t.Fatalf("count: %v", err)
	}
	args["field"] = "_id"
	if _, err := tools.distinct(ctx, request(args)); err != nil {
		t.Fatalf("distinct: %v", err)
	}
	args["pipeline"] = `[{"$match":` + string(filter) + `}]`
	if _, err := tools.aggregate(ctx, request(args)); err != nil {
		t.Fatalf("aggregate: %v", err)
	}
}
