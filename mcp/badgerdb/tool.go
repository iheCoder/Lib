package badgerdb

import (
	"Lib/mcp/tool_group"
	"context"
	"encoding/json"
	"fmt"
	"github.com/mark3labs/mcp-go/mcp"
)

var (
	db *Database
)

func init() {
	var err error
	db, err = NewDatabase()
	if err != nil {
		panic(fmt.Sprintf("Failed to open database: %v", err))
	}
}

// BadgerDBToolGroup is a group of tools for interacting with BadgerDB.
var BadgerDBToolGroup = tool_group.ToolGroup{
	Name: "BadgerDB Tools",
	Items: []tool_group.MCPToolItem{
		addKeyValueTool,
		getValueByKeyTool,
		listKeysAndValuesTool,
		searchKeysAndValuesTool,
	},
}

var addKeyValueTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool("add_key_value",
		mcp.WithDescription("Add a key-value pair to the BadgerDB"),
		mcp.WithString("key", mcp.Required(), mcp.Description("The key")),
		mcp.WithString("value", mcp.Required(), mcp.Description("The value")),
	),
	Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key := req.Params.Arguments["key"].(string)
		value := req.Params.Arguments["value"].(string)
		err := db.Add(key, value)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(fmt.Sprintf("Key %q added successfully.", key)), nil
	},
}

var getValueByKeyTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool("get_value_by_key",
		mcp.WithDescription("Get a value by key from the BadgerDB"),
		mcp.WithString("key", mcp.Required(), mcp.Description("The key")),
	),
	Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key := req.Params.Arguments["key"].(string)
		val, err := db.Get(key)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(val), nil
	},
}

var listKeysAndValuesTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool("list_keys_and_values",
		mcp.WithDescription("List all key-value pairs in the BadgerDB"),
	),
	Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		results, err := db.ListKeysAndValues()
		if err != nil {
			return nil, err
		}

		// Convert the map to a string representation
		resultStr, _ := json.Marshal(results)
		return mcp.NewToolResultText(string(resultStr)), nil
	},
}

var searchKeysAndValuesTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool("search_keys_and_values",
		mcp.WithDescription("Search key-value pairs by keyword with offset and limit"),
		mcp.WithString("keyword", mcp.Required(), mcp.Description("Keyword to search")),
		mcp.WithNumber("offset", mcp.Description("Offset for pagination, default is 0")),
		mcp.WithNumber("limit", mcp.Description("Limit number of results, default is 10")),
	),
	Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		keyword := req.Params.Arguments["keyword"].(string)
		offset := 0
		if v, ok := req.Params.Arguments["offset"].(float64); ok {
			offset = int(v)
		}
		limit := 10
		if v, ok := req.Params.Arguments["limit"].(float64); ok {
			limit = int(v)
		}
		results, err := db.SearchKeysAndValuesWithFilter(keyword, offset, limit)
		if err != nil {
			return nil, err
		}

		// Convert the map to a string representation
		resultStr, _ := json.Marshal(results)
		return mcp.NewToolResultText(string(resultStr)), nil
	},
}
