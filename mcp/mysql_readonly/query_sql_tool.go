package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/mark3labs/mcp-go/mcp"
	"gopkg.in/yaml.v3"
	"os"
	"regexp"
	"strings"
)

// Define the tool name
const (
	ToolExecuteMySQLQuery = "execute_mysql_query"
	defaultMaxTokens      = 60000 // 默认最大tokens数量
)

var dbSources = make(map[string]*sql.DB)

type DatabaseConfig struct {
	Databases map[string]string `yaml:"databases"`
}

// 连接 MySQL 数据库
func initGlobalDB() error {
	data, err := os.ReadFile("config/database.yaml")
	if err != nil {
		return fmt.Errorf("读取数据库配置文件失败: %v", err)
	}

	var config DatabaseConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析数据库配置文件失败: %v", err)
	}

	for name, dsn := range config.Databases {
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			return fmt.Errorf("连接数据库 %s 失败: %v", name, err)
		}
		if err := db.Ping(); err != nil {
			return fmt.Errorf("ping 数据库 %s 失败: %v", name, err)
		}
		dbSources[name] = db
	}

	return nil
}

// getAvailableSources 返回当前已加载的数据库源名称列表
func getAvailableSources() []string {
	sources := make([]string, 0, len(dbSources))
	for name := range dbSources {
		sources = append(sources, name)
	}
	return sources
}

func querySqlTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 获取查询参数
	query, ok := request.Params.Arguments["query"].(string)
	if !ok || query == "" {
		// 如果没有提供查询参数，返回错误
		return mcp.NewToolResultError("missing or invalid 'query' parameter"), nil
	}

	source := "learn"
	if val, ok := request.Params.Arguments["source"].(string); ok && val != "" {
		source = val
	}
	db, ok := dbSources[source]
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("未知的数据源: %s", source)), nil
	}

	// 获取最大tokens数量，若未提供，则不限制
	maxTokens, ok := request.Params.Arguments["max_tokens"].(float64)
	if !ok {
		maxTokens = defaultMaxTokens
	}

	// 执行只读 MySQL 查询
	result, err := executeReadOnlyQuery(db, query)
	if err != nil {
		fmt.Printf("Error executing query %s error %v\n", query, err)
		return mcp.NewToolResultError(err.Error()), nil
	}

	// 检查结果长度是否超过最大tokens限制
	if maxTokens > 0 {
		resultLength := calculateTokenCount(result)
		if resultLength > int(maxTokens) {
			errMsg := fmt.Sprintf("result exceeds max_tokens limit: actual %d > expected %d", resultLength, int(maxTokens))
			fmt.Println(errMsg)
			return mcp.NewToolResultError(errMsg), nil
		}
	}

	return mcp.NewToolResultText(result), nil
}

// 执行只读 MySQL 查询
func executeReadOnlyQuery(db *sql.DB, query string) (string, error) {
	// 检查查询是否为只读操作
	if IsDestructiveSQL(query) {
		return "", errors.New("update, delete, drop, truncate, alter, replace, load data are not allowed contained in the query")
	}

	rows, err := db.Query(query)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	// 获取列名
	columns, err := rows.Columns()
	if err != nil {
		return "", err
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
			return "", err
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
		return "", err
	}

	return string(resultJSON), nil
}

// destructiveSQLKeywords 是一个包含潜在破坏性SQL操作关键字的列表（小写）
// 注意：这个列表可能需要根据具体需求调整
var destructiveSQLKeywords = []string{
	"update",
	"delete",
	"drop",      // 删库/删表/删索引等
	"truncate",  // 清空表内容
	"alter",     // 修改表结构
	"replace",   // 插入或替换，可能覆盖数据
	"load data", // 加载数据，可能覆盖或修改
	// 可以根据需要添加更多关键字，例如 'grant', 'revoke', 'create user', 'set', etc.
	// 但要注意 'create table'/'create database' 通常不被视为“破坏性”，除非目标已存在且没有 'IF NOT EXISTS'
}

// destructiveSQLPattern 是用于匹配破坏性关键字的预编译正则表达式
// (?i) - 不区分大小写
// \b   - 匹配单词边界，避免匹配像 'updates' 或 'fordelete' 这样的词
// 使用 strings.Join 将关键字列表组合成 OR 模式
var destructiveSQLPattern = regexp.MustCompile(`(?i)\b(` + strings.Join(destructiveSQLKeywords, "|") + `)\b`)

// IsDestructiveSQL checks if a SQL query string contains potentially destructive keywords.
//
// 它使用正则表达式检查是否存在预定义的破坏性关键字（如 UPDATE, DELETE, DROP 等）。
//
// 注意：
// 1. 这是基于关键字匹配的简单检查，不是完整的SQL解析。
// 2. 可能会在 SQL 注释或字符串字面量中错误地匹配到关键字（False Positive）。
// 3. 可能无法检测到通过存储过程或函数执行的破坏性操作。
// 4. 检查区分单词边界（例如，不会将 'updates' 匹配为 'update'）。
// 5. 检查不区分大小写。
//
// 对于需要更高安全性的场景，应考虑使用更健壮的SQL解析库或数据库用户权限控制。
//
// 参数:
//
//	query string - 要检查的 SQL 查询语句。
//
// 返回值:
//
//	bool - 如果查询包含任何破坏性关键字，则返回 true，否则返回 false。
func IsDestructiveSQL(query string) bool {
	// 使用预编译的正则表达式进行匹配
	return destructiveSQLPattern.MatchString(query)
}

// calculateTokenCount 计算给定字符串的 token 数量
// 这里假设每 3 个字符算一个 token，实际情况可能因编码和具体实现而异
func calculateTokenCount(s string) int {
	return (len(s) + 2) / 3
}
