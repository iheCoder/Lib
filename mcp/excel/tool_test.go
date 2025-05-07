package excel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestNewExcelTool(t *testing.T) {
	_, err := newExcelTool.Handler(context.TODO(), mockRequest(map[string]any{
		"filepath": "./testdata/test.xlsx",
	}))
	if err != nil {
		t.Fatalf("failed to create new excel file: %v", err)
	}
}

func TestReadWriteRowsTool(t *testing.T) {
	// Create file and write rows
	TestNewExcelTool(t)
	fp := "./testdata/test.xlsx"
	_, _ = addSheetTool.Handler(context.TODO(), mockRequest(map[string]any{
		"filepath":   fp,
		"sheet_name": "Sheet2",
	}))

	data := map[string][]string{"1": {"A", "B", "C"}}
	raw, _ := json.Marshal(data)

	_, err := writeRowsTool.Handler(context.TODO(), mockRequest(map[string]any{
		"filepath":  fp,
		"sheet":     "Sheet1",
		"rows_data": string(raw),
	}))
	if err != nil {
		t.Fatalf("writeRowsTool failed: %v", err)
	}

	res, err := readRowsTool.Handler(context.TODO(), mockRequest(map[string]any{
		"filepath": fp,
		"sheet":    "Sheet1",
		"rows":     "1",
	}))
	if err != nil {
		t.Fatalf("readRowsTool failed: %v", err)
	}

	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "A") {
		t.Errorf("expected result to contain 'A', got %v", text)
	}
}

func mockRequest(params map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = params
	return req
}
