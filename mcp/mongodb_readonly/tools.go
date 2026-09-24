package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const queryTimeout = 10 * time.Second

type readTools struct {
	registry *sourceRegistry
}

func stringArgument(request mcp.CallToolRequest, name string) (string, error) {
	value, present := request.Params.Arguments[name]
	if !present || value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s 必须是字符串", name)
	}
	return text, nil
}

func requiredArgument(request mcp.CallToolRequest, name string) (string, error) {
	value, err := stringArgument(request, name)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("缺少 %s", name)
	}
	return value, nil
}

func (t *readTools) target(request mcp.CallToolRequest, requireDatabase, requireCollection bool) (string, string, *mongo.Client, string, error) {
	source, err := stringArgument(request, "source")
	if err != nil {
		return "", "", nil, "", err
	}
	database, err := stringArgument(request, "database")
	if err != nil {
		return "", "", nil, "", err
	}
	source, client, database, err := t.registry.resolve(source, database)
	if err != nil {
		return "", "", nil, "", err
	}
	if requireDatabase && database == "" {
		return "", "", nil, "", errors.New("缺少 database，且 source 未配置 default_database")
	}
	collection := ""
	if requireCollection {
		collection, err = requiredArgument(request, "collection")
		if err != nil {
			return "", "", nil, "", err
		}
	}
	return source, database, client, collection, nil
}

// handle 把客户端参数错误和数据库错误转为 MCP 工具结果；
// 日志只记录定位字段，不打印 URI、筛选条件或查询结果。
func (t *readTools) handle(operation string, run func(context.Context, mcp.CallToolRequest) (string, error)) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(parent context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, cancel := context.WithTimeout(parent, queryTimeout)
		defer cancel()
		result, err := run(ctx, request)
		if err != nil {
			// 记录定位范围，但不记录包含业务数据的 filter、pipeline 或返回内容。
			fmt.Fprintf(os.Stderr, "MongoDB %s source=%v database=%v collection=%v failed: %v\n",
				operation, request.Params.Arguments["source"], request.Params.Arguments["database"],
				request.Params.Arguments["collection"], err)
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	}
}

func (t *readTools) listDatabases(ctx context.Context, request mcp.CallToolRequest) (string, error) {
	_, _, client, _, err := t.target(request, false, false)
	if err != nil {
		return "", err
	}
	names, err := client.ListDatabaseNames(ctx, bson.D{})
	return jsonResult(names, err)
}

func (t *readTools) listCollections(ctx context.Context, request mcp.CallToolRequest) (string, error) {
	_, database, client, _, err := t.target(request, true, false)
	if err != nil {
		return "", err
	}
	names, err := client.Database(database).ListCollectionNames(ctx, bson.D{})
	return jsonResult(names, err)
}

func (t *readTools) listIndexes(ctx context.Context, request mcp.CallToolRequest) (string, error) {
	_, database, client, collection, err := t.target(request, true, true)
	if err != nil {
		return "", err
	}
	cursor, err := client.Database(database).Collection(collection).Indexes().List(ctx)
	if err != nil {
		return "", err
	}
	return cursorResult(ctx, cursor, maxLimit)
}

func (t *readTools) find(ctx context.Context, request mcp.CallToolRequest) (string, error) {
	_, database, client, collection, err := t.target(request, true, true)
	if err != nil {
		return "", err
	}
	filterText, err := stringArgument(request, "filter")
	if err != nil {
		return "", err
	}
	filter, err := parseDocument(filterText, "filter")
	if err != nil {
		return "", err
	}
	limit, err := boundedInteger(request.Params.Arguments["limit"], defaultLimit, maxLimit, "limit")
	if err != nil {
		return "", err
	}
	if limit == 0 {
		return "[]", nil
	}
	skip, err := boundedInteger(request.Params.Arguments["skip"], 0, maxSkip, "skip")
	if err != nil {
		return "", err
	}
	findOptions := options.Find().SetLimit(int64(limit)).SetSkip(int64(skip)).SetBatchSize(int32(limit)).SetMaxTime(queryTimeout)
	if projectionText, err := stringArgument(request, "projection"); err != nil {
		return "", err
	} else if projectionText != "" {
		projection, err := parseDocument(projectionText, "projection")
		if err != nil {
			return "", err
		}
		findOptions.SetProjection(projection)
	}
	if sortText, err := stringArgument(request, "sort"); err != nil {
		return "", err
	} else if sortText != "" {
		sort, err := parseDocument(sortText, "sort")
		if err != nil {
			return "", err
		}
		findOptions.SetSort(sort)
	}
	cursor, err := client.Database(database).Collection(collection).Find(ctx, filter, findOptions)
	if err != nil {
		return "", err
	}
	return cursorResult(ctx, cursor, limit)
}

func (t *readTools) count(ctx context.Context, request mcp.CallToolRequest) (string, error) {
	_, database, client, collection, err := t.target(request, true, true)
	if err != nil {
		return "", err
	}
	filterText, err := stringArgument(request, "filter")
	if err != nil {
		return "", err
	}
	filter, err := parseDocument(filterText, "filter")
	if err != nil {
		return "", err
	}
	count, err := client.Database(database).Collection(collection).CountDocuments(ctx, filter, options.Count().SetMaxTime(queryTimeout))
	return jsonResult(map[string]int64{"count": count}, err)
}

func (t *readTools) estimatedCount(ctx context.Context, request mcp.CallToolRequest) (string, error) {
	_, database, client, collection, err := t.target(request, true, true)
	if err != nil {
		return "", err
	}
	count, err := client.Database(database).Collection(collection).EstimatedDocumentCount(ctx,
		options.EstimatedDocumentCount().SetMaxTime(queryTimeout))
	return jsonResult(map[string]int64{"estimated_count": count}, err)
}

func (t *readTools) distinct(ctx context.Context, request mcp.CallToolRequest) (string, error) {
	_, database, client, collection, err := t.target(request, true, true)
	if err != nil {
		return "", err
	}
	field, err := requiredArgument(request, "field")
	if err != nil {
		return "", err
	}
	filterText, err := stringArgument(request, "filter")
	if err != nil {
		return "", err
	}
	filter, err := parseDocument(filterText, "filter")
	if err != nil {
		return "", err
	}
	values, err := client.Database(database).Collection(collection).Distinct(ctx, field, filter,
		options.Distinct().SetMaxTime(queryTimeout))
	if err != nil {
		return "", err
	}
	return extendedJSONResult(values)
}

func (t *readTools) aggregate(ctx context.Context, request mcp.CallToolRequest) (string, error) {
	_, database, client, collection, err := t.target(request, true, true)
	if err != nil {
		return "", err
	}
	pipelineText, err := requiredArgument(request, "pipeline")
	if err != nil {
		return "", err
	}
	pipeline, err := parsePipeline(pipelineText)
	if err != nil {
		return "", err
	}
	limit, err := boundedInteger(request.Params.Arguments["limit"], defaultLimit, maxLimit, "limit")
	if err != nil {
		return "", err
	}
	if limit == 0 {
		return "[]", nil
	}
	// 最终输出另加一个限量阶段。它控制返回量，整条管道仍受超时限制。
	pipeline = append(pipeline, bson.D{{Key: "$limit", Value: limit}})
	cursor, err := client.Database(database).Collection(collection).Aggregate(ctx, pipeline,
		options.Aggregate().SetAllowDiskUse(false).SetBatchSize(int32(limit)).SetMaxTime(queryTimeout))
	if err != nil {
		return "", err
	}
	return cursorResult(ctx, cursor, limit)
}

func cursorResult(ctx context.Context, cursor *mongo.Cursor, limit int) (string, error) {
	defer cursor.Close(ctx)
	documents := make([]json.RawMessage, 0)
	usedBytes := 2 // JSON 数组的方括号
	for len(documents) < limit && cursor.Next(ctx) {
		var doc bson.D
		if err := cursor.Decode(&doc); err != nil {
			return "", err
		}
		encoded, err := bson.MarshalExtJSON(doc, false, false)
		if err != nil {
			return "", err
		}
		usedBytes += len(encoded) + 1
		if usedBytes > maxOutputBytes {
			return "", fmt.Errorf("查询结果超过 %d 字节，请缩小筛选范围或投影字段", maxOutputBytes)
		}
		documents = append(documents, encoded)
	}
	if err := cursor.Err(); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(documents)
	return string(encoded), err
}

func extendedJSONResult(value any) (string, error) {
	// v1 驱动的 Extended JSON 编码器要求顶层是文档；包装后只返回 values 数组。
	encoded, err := bson.MarshalExtJSON(bson.D{{Key: "values", Value: value}}, false, false)
	if err != nil {
		return "", err
	}
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &wrapper); err != nil {
		return "", err
	}
	values := wrapper["values"]
	if len(values) > maxOutputBytes {
		return "", fmt.Errorf("查询结果超过 %d 字节，请缩小筛选范围", maxOutputBytes)
	}
	return string(values), nil
}

func jsonResult(value any, operationError error) (string, error) {
	if operationError != nil {
		return "", operationError
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if len(encoded) > maxOutputBytes {
		return "", fmt.Errorf("查询结果超过 %d 字节", maxOutputBytes)
	}
	return string(encoded), nil
}
