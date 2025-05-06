package excel

import (
	"Lib/mcp/tool_group"
	"context"
	"encoding/json"
	"fmt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/xuri/excelize/v2"
	"strings"
)

var files = map[string]*excelize.File{}

var ExcelToolGroup = tool_group.ToolGroup{
	Name: "Excel Tools",
	Items: []tool_group.MCPToolItem{
		newExcelTool,
		readRowsTool,
		readColsTool,
		writeRowsTool,
		writeColsTool,
		addSheetTool,
		deleteSheetTool,
		renameSheetTool,
	},
}

var newExcelTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool("new_excel_file",
		mcp.WithDescription("Create a new Excel file with a given name"),
		mcp.WithString("filename", mcp.Required(), mcp.Description("Name of the Excel file")),
	),
	Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filename := req.Params.Arguments["filename"].(string)

		files[filename] = excelize.NewFile()
		if err := files[filename].SaveAs(filename); err != nil {
			return nil, fmt.Errorf("failed to save Excel file: %v", err)
		}

		return mcp.NewToolResultText(fmt.Sprintf("Excel file %q created.", filename)), nil
	},
}

var readRowsTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool("read_rows",
		mcp.WithDescription("Read specific rows from a sheet"),
		mcp.WithString("filename", mcp.Required(), mcp.Description("Excel filename")),
		mcp.WithString("sheet", mcp.Required(), mcp.Description("Sheet name")),
		mcp.WithString("rows", mcp.Required(), mcp.Description("Comma-separated row numbers")),
	),
	Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filename := req.Params.Arguments["filename"].(string)
		sheet := req.Params.Arguments["sheet"].(string)
		rowsStr := req.Params.Arguments["rows"].(string)

		f, ok := files[filename]
		if !ok {
			return nil, fmt.Errorf("file not found: %s", filename)
		}

		var result = map[string][]string{}
		for _, r := range splitCSV(rowsStr) {
			rowIdx, _ := parseInt(r)
			row, err := f.GetRows(sheet)
			if err != nil {
				return nil, err
			}
			if rowIdx-1 >= 0 && rowIdx-1 < len(row) {
				result[r] = row[rowIdx-1]
			}
		}
		b, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(b)), nil
	},
}

var readColsTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool("read_columns",
		mcp.WithDescription("Read specific columns from a sheet"),
		mcp.WithString("filename", mcp.Required()),
		mcp.WithString("sheet", mcp.Required()),
		mcp.WithString("columns", mcp.Required(), mcp.Description("Comma-separated column letters")),
	),
	Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filename := req.Params.Arguments["filename"].(string)
		sheet := req.Params.Arguments["sheet"].(string)
		colsStr := req.Params.Arguments["columns"].(string)

		f, ok := files[filename]
		if !ok {
			return nil, fmt.Errorf("file not found: %s", filename)
		}

		result := map[string][]string{}
		for _, col := range splitCSV(colsStr) {
			colIdx, _ := parseInt(col)
			cols, err := f.GetCols(sheet)
			if err != nil {
				return nil, err
			}

			if colIdx-1 >= 0 && colIdx-1 < len(cols) {
				result[col] = cols[colIdx-1]
			}
		}
		b, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(b)), nil
	},
}

var writeRowsTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool("write_rows",
		mcp.WithDescription("Write content to specific rows"),
		mcp.WithString("filename", mcp.Required()),
		mcp.WithString("sheet", mcp.Required()),
		mcp.WithString("rows_data", mcp.Required(), mcp.Description("JSON of {row_number: [values]}")),
	),
	Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filename := req.Params.Arguments["filename"].(string)
		sheet := req.Params.Arguments["sheet"].(string)
		rowsJSON := req.Params.Arguments["rows_data"].(string)

		f, ok := files[filename]
		if !ok {
			return nil, fmt.Errorf("file not found: %s", filename)
		}
		var rows map[string][]string
		if err := json.Unmarshal([]byte(rowsJSON), &rows); err != nil {
			return nil, err
		}

		for r, vals := range rows {
			ridx, _ := parseInt(r)
			for i, val := range vals {
				cell, _ := excelize.CoordinatesToCellName(i+1, ridx)
				f.SetCellValue(sheet, cell, val)
			}
		}

		if err := f.Save(); err != nil {
			return nil, fmt.Errorf("failed to save Excel file: %v", err)
		}

		return mcp.NewToolResultText("Rows written successfully."), nil
	},
}

var writeColsTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool("write_columns",
		mcp.WithDescription("Write content to specific columns"),
		mcp.WithString("filename", mcp.Required()),
		mcp.WithString("sheet", mcp.Required()),
		mcp.WithString("cols_data", mcp.Required(), mcp.Description("JSON of {column_letter: [values]}")),
	),
	Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filename := req.Params.Arguments["filename"].(string)
		sheet := req.Params.Arguments["sheet"].(string)
		colsJSON := req.Params.Arguments["cols_data"].(string)

		f, ok := files[filename]
		if !ok {
			return nil, fmt.Errorf("file not found: %s", filename)
		}
		var cols map[string][]string
		if err := json.Unmarshal([]byte(colsJSON), &cols); err != nil {
			return nil, err
		}
		for col, vals := range cols {
			colIdx, _ := excelize.ColumnNameToNumber(col)
			for i, val := range vals {
				cell, _ := excelize.CoordinatesToCellName(colIdx, i+1)
				f.SetCellValue(sheet, cell, val)
			}
		}
		return mcp.NewToolResultText("Columns written successfully."), nil
	},
}

var addSheetTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool("add_sheet",
		mcp.WithDescription("Add a new sheet"),
		mcp.WithString("filename", mcp.Required()),
		mcp.WithString("sheet_name", mcp.Required()),
	),
	Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filename := req.Params.Arguments["filename"].(string)
		sheetName := req.Params.Arguments["sheet_name"].(string)
		f, ok := files[filename]
		if !ok {
			return nil, fmt.Errorf("file not found: %s", filename)
		}
		f.NewSheet(sheetName)
		return mcp.NewToolResultText("Sheet added successfully."), nil
	},
}

var deleteSheetTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool("delete_sheet",
		mcp.WithDescription("Delete a sheet"),
		mcp.WithString("filename", mcp.Required()),
		mcp.WithString("sheet_name", mcp.Required()),
	),
	Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filename := req.Params.Arguments["filename"].(string)
		sheetName := req.Params.Arguments["sheet_name"].(string)
		f, ok := files[filename]
		if !ok {
			return nil, fmt.Errorf("file not found: %s", filename)
		}
		f.DeleteSheet(sheetName)
		return mcp.NewToolResultText("Sheet deleted."), nil
	},
}

var renameSheetTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool("rename_sheet",
		mcp.WithDescription("Rename a sheet"),
		mcp.WithString("filename", mcp.Required()),
		mcp.WithString("old_name", mcp.Required()),
		mcp.WithString("new_name", mcp.Required()),
	),
	Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filename := req.Params.Arguments["filename"].(string)
		oldName := req.Params.Arguments["old_name"].(string)
		newName := req.Params.Arguments["new_name"].(string)
		f, ok := files[filename]
		if !ok {
			return nil, fmt.Errorf("file not found: %s", filename)
		}
		f.SetSheetName(oldName, newName)
		return mcp.NewToolResultText("Sheet renamed."), nil
	},
}

func splitCSV(s string) []string {
	var r []string
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v) // 去掉首尾空格
		if v != "" {
			r = append(r, v)
		}
	}
	return r
}

func parseInt(s string) (int, error) {
	var i int
	_, err := fmt.Sscanf(s, "%d", &i)
	return i, err
}
