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

	fmt.Println("Query result:", result)
}

func TestIsDestructiveSQL(t *testing.T) {
	// 定义测试用例结构体
	testCases := []struct {
		name     string // 测试用例名称
		query    string // 输入的 SQL 查询
		expected bool   // 期望的结果 (true 表示检测为破坏性)
	}{
		// --- 安全查询 ---
		{
			name:     "Simple SELECT",
			query:    "SELECT id, name FROM users WHERE country = 'CN';",
			expected: false,
		},
		//{
		//	name:  "SELECT with keyword in string literal",
		//	query: "SELECT description FROM articles WHERE description LIKE '%update logs%';",
		//	// 注意：基于简单正则的局限性，这里可能会误报为 true，取决于你的关键字列表和正则
		//	// 如果你的正则不处理注释/字符串，这里预期可能是 true
		//	// 如果你的需求是忽略注释/字符串中的关键字，则需要更复杂的解析器
		//	// 这里我们假设当前的简单正则会匹配到，所以预期是 true (尽管理想是 false)
		//	// 如果你改进了正则或函数来处理注释/字符串，需要更新这里的 expected 值
		//	// **更新：基于提供的正则 \b(update|...)\b，它不会匹配 '%update logs%' 中的 update，所以是 false**
		//	expected: false,
		//},
		{
			name: "SELECT with keyword in single-line comment",
			query: `-- This is a comment mentioning DELETE
SELECT * FROM products;`,
			// 同上，简单正则可能匹配到注释中的 DELETE，预期是 true (理想是 false)
			// **更新：基于提供的正则 \b(update|...)\b，它会匹配注释中的 DELETE，所以是 true**
			expected: true, // Regex limitation - will detect keyword in comment
		},
		{
			name:  "SELECT with keyword in block comment",
			query: `SELECT /* DROP TABLE users; */ id FROM config;`,
			// 同上，简单正则可能匹配到注释中的 DROP，预期是 true (理想是 false)
			// **更新：基于提供的正则 \b(update|...)\b，它会匹配注释中的 DROP，所以是 true**
			expected: true, // Regex limitation - will detect keyword in comment
		},
		{
			name:     "Simple INSERT",
			query:    "INSERT INTO logs (message) VALUES ('User logged in');",
			expected: false, // INSERT 通常不视为破坏性
		},
		{
			name:     "Simple CREATE TABLE",
			query:    "CREATE TABLE new_data (id INT);",
			expected: false, // CREATE 通常不视为破坏性
		},
		{
			name:     "Empty Query",
			query:    "",
			expected: false,
		},
		{
			name:     "Whitespace Query",
			query:    "   \t\n ",
			expected: false,
		},
		// --- 破坏性查询 ---
		{
			name:     "Simple UPDATE",
			query:    "UPDATE customers SET status = 'inactive' WHERE last_order_date < '2023-01-01';",
			expected: true,
		},
		{
			name:     "Simple DELETE",
			query:    "DELETE FROM sessions WHERE expiry < NOW();",
			expected: true,
		},
		{
			name:     "Lowercase DROP TABLE",
			query:    "drop table old_logs;",
			expected: true,
		},
		{
			name:     "TRUNCATE TABLE",
			query:    "TRUNCATE TABLE temp_results;",
			expected: true,
		},
		{
			name:     "ALTER TABLE",
			query:    "ALTER TABLE products ADD COLUMN price DECIMAL(10, 2);",
			expected: true,
		},
		{
			name:     "REPLACE INTO",
			query:    "REPLACE INTO user_settings (user_id, setting_key, value) VALUES (1, 'theme', 'dark');",
			expected: true,
		},
		{
			name:     "LOAD DATA",
			query:    "LOAD DATA INFILE '/path/to/data.csv' INTO TABLE sales_data;",
			expected: true,
		},
		{
			name:     "Multiple statements with destructive",
			query:    "SELECT * from users; DELETE from audit_log;",
			expected: true, // 包含 DELETE
		},
		{
			name:     "Uppercase DELETE",
			query:    "DELETE FROM users WHERE id = 1;",
			expected: true,
		},
		{
			name:     "Mixed Case UPDATE",
			query:    "UpDaTe products SET name = 'New Name' WHERE id = 5;",
			expected: true,
		},
	}
	// 遍历所有测试用例
	for _, tc := range testCases {
		// 使用 t.Run 为每个测试用例创建一个子测试，方便管理和查看结果
		t.Run(tc.name, func(t *testing.T) {
			// 调用被测试函数
			actual := IsDestructiveSQL(tc.query)
			// 断言：检查实际结果是否符合预期
			if actual != tc.expected {
				// 如果不符合，使用 t.Errorf 报告错误
				// t.Errorf 不会中止测试，会继续执行其他测试用例
				// 如果希望失败时立即中止当前子测试，可以使用 t.Fatalf
				t.Errorf("IsDestructiveSQL(%q) = %v; want %v", tc.query, actual, tc.expected)
			}
		})
	}
}
