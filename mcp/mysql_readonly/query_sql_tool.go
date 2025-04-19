package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/mark3labs/mcp-go/mcp"
)

// Define the tool name
const (
	ToolExecuteMySQLQuery = "execute_mysql_query"
)

var globalDB *sql.DB

// 连接 MySQL 数据库
func initGlobalDB() error {
	// 请根据实际情况修改数据库连接信息
	dsn := "root:123456@tcp(127.0.0.1:3306)/learn"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	// 测试数据库连接
	if err := db.Ping(); err != nil {
		db.Close()
		return err
	}

	globalDB = db
	return nil
}

func querySqlTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 打印请求参数
	fmt.Printf("Received request: %v\n", request.Params.Arguments)

	// 获取查询参数
	query, ok := request.Params.Arguments["query"].(string)
	if !ok || query == "" {
		return mcp.NewToolResultError("missing or invalid 'query' parameter"), nil
	}

	// 执行只读 MySQL 查询
	result, err := executeReadOnlyQuery(globalDB, query)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return result, nil
}

// 执行只读 MySQL 查询
func executeReadOnlyQuery(db *sql.DB, query string) (*mcp.CallToolResult, error) {
	// 检查查询是否为只读操作
	if !isReadOnlyQuery(query) {
		return nil, errors.New("only read-only queries are allowed")
	}

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 获取列名
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	// 存储查询结果
	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}
		row := make(map[string]interface{})
		for i, col := range columns {
			var v interface{}
			val := values[i]
			b, ok := val.([]byte)
			if ok {
				v = string(b)
			} else {
				v = val
			}
			row[col] = v
		}
		results = append(results, row)
	}

	// 将结果转换为 JSON 字符串
	resultJSON, err := json.Marshal(results)
	if err != nil {
		return nil, err
	}

	return mcp.NewToolResultText(string(resultJSON)), nil
}

// 检查查询是否为只读操作
func isReadOnlyQuery(query string) bool {
	// 简单检查查询语句是否以 SELECT 开头
	// 这里只是一个简单示例，实际应用中可能需要更复杂的检查
	// 例如处理注释、大小写等情况
	// 还可以使用 SQL 解析器来更准确地判断
	return len(query) >= 6 && query[:6] == "SELECT"
}
