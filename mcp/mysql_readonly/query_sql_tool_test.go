package main

import (
	"fmt"
	"testing"
)

func TestExecuteReadOnlyQuery(t *testing.T) {
	// 初始化mysql连接
	if err := initGlobalDB(); err != nil {
		t.Fatalf("failed to initialize MySQL: %v", err)
	}

	// 测试查询语句
	query := "SELECT * FROM sales LIMIT 10"
	result, err := executeReadOnlyQuery(globalDB, query)
	if err != nil {
		t.Fatalf("failed to execute read-only query: %v", err)
	}

	fmt.Println("Query result:", result.Content)
}
