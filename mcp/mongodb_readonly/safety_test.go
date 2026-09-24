package main

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestReadOnlyPipeline(t *testing.T) {
	// 场景：常用统计及关联查询应通过；写入阶段、嵌套写入阶段与脚本表达式必须在发送到 MongoDB 前被拒绝。
	cases := []struct {
		name    string
		input   string
		allowed bool
	}{
		{"group and sort", `[{"$match":{"status":"ready"}},{"$group":{"_id":"$type","count":{"$sum":1}}},{"$sort":{"count":-1}}]`, true},
		{"lookup with read pipeline", `[{"$lookup":{"from":"users","pipeline":[{"$match":{"active":true}}],"as":"users"}}]`, true},
		{"facet branches", `[{"$facet":{"totals":[{"$count":"n"}],"sample":[{"$limit":5}]}}]`, true},
		{"output writes", `[{"$out":"backup"}]`, false},
		{"merge writes", `[{"$merge":"backup"}]`, false},
		{"nested output writes", `[{"$lookup":{"from":"users","pipeline":[{"$out":"backup"}],"as":"users"}}]`, false},
		{"nested merge writes", `[{"$facet":{"bad":[{"$merge":"backup"}]}}]`, false},
		{"duplicate nested pipeline", `[{"$lookup":{"from":"users","pipeline":[{"$limit":1}],"pipeline":[{"$out":"backup"}],"as":"users"}}]`, false},
		{"server script", `[{"$project":{"value":{"$function":{"body":"function(){}","args":[],"lang":"js"}}}}]`, false},
		{"unknown stage", `[{"$search":{"text":{"query":"a","path":"name"}}}]`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parsePipeline(tc.input)
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v, err=%v", tc.allowed, err)
			}
		})
	}
}

func TestExtendedJSONFilterAndBounds(t *testing.T) {
	// 场景：ObjectId 和日期保留 BSON 类型；普通筛选可用，服务端脚本和超额读取在执行前失败。
	doc, err := parseDocument(`{"_id":{"$oid":"507f1f77bcf86cd799439011"},"created_at":{"$gte":{"$date":"2026-01-01T00:00:00Z"}}}`, "filter")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := doc[0].Value.(primitive.ObjectID); !ok {
		t.Fatalf("_id type = %T, want primitive.ObjectID", doc[0].Value)
	}
	if _, err := parseDocument(`{"$where":"return true"}`, "filter"); err == nil {
		t.Fatal("$where must be rejected")
	}
	if _, err := boundedInteger(float64(maxLimit+1), defaultLimit, maxLimit, "limit"); err == nil {
		t.Fatal("limit above maximum must be rejected")
	}
	if _, err := parseDocument(`{"x":"`+strings.Repeat("a", maxInputBytes)+`"}`, "filter"); err == nil {
		t.Fatal("oversized input must be rejected")
	}
}

func TestSingleSourceSelection(t *testing.T) {
	// 场景：单源可省略 source，数据库由请求指定；多源环境必须明确 source，避免误查。
	one := &sourceRegistry{sources: map[string]mongoSource{"main": {defaultDatabase: "default"}}}
	name, _, database, err := one.resolve("", "requested")
	if err != nil || name != "main" || database != "requested" {
		t.Fatalf("name=%q database=%q err=%v", name, database, err)
	}
	many := &sourceRegistry{sources: map[string]mongoSource{"a": {}, "b": {}}}
	if _, _, _, err := many.resolve("", "requested"); err == nil {
		t.Fatal("multi-source call without source must fail")
	}
}
