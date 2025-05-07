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

var (
	files = map[string]*excelize.File{}

	ExcelToolGroup = tool_group.ToolGroup{
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

	filepathToolOption = mcp.WithString("filepath", mcp.Required(), mcp.Description("path of the Excel file"))
	sheetToolOption    = mcp.WithString("sheet", mcp.Required(), mcp.Description("Sheet name (default is 'Sheet1')"))
	rowsArgOption      = mcp.WithString("rows", mcp.Required(), mcp.Description("Comma-separated row numbers (e.g. '1,2,3')"))
	colsArgOption      = mcp.WithString("columns", mcp.Required(), mcp.Description("Comma-separated column letters (e.g. 'A,B,C')"))
	rowsDataArgOption  = mcp.WithString("rows_data", mcp.Required(), mcp.Description("JSON of {row_number: [values]}, json will unmarshal into map[string][]string. (e.g. '{\"1\": [\"A\", \"B\"]}')"))
	colsDataArgOption  = mcp.WithString("cols_data", mcp.Required(), mcp.Description("JSON of {column_letter: [values]}, json will unmarshal into map[string][]string. (e.g. '{\"A\": [\"Name\", \"Tom\"]}')"))
)

func getExcelFile(filename string) (*excelize.File, error) {
	if f, ok := files[filename]; ok {
		return f, nil
	}

	f, err := excelize.OpenFile(filename)
	if err != nil {
		return nil, fmt.Errorf("file not found and failed to open: %s", filename)
	}
	files[filename] = f

	return f, nil
}

var newExcelTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool("new_excel_file",
		mcp.WithDescription("Create a new Excel file with a given path"),
		filepathToolOption,
	),
	Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filepath := req.Params.Arguments["filepath"].(string)

		files[filepath] = excelize.NewFile()
		if err := files[filepath].SaveAs(filepath); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to save Excel file: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Excel file %q created.", filepath)), nil
	},
}

var readRowsTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool("read_rows",
		mcp.WithDescription("Read specific rows from a sheet"),
		filepathToolOption,
		sheetToolOption,
		rowsArgOption,
	),
	Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filepath := req.Params.Arguments["filepath"].(string)
		sheet := req.Params.Arguments["sheet"].(string)
		rowsStr := req.Params.Arguments["rows"].(string)

		f, err := getExcelFile(filepath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to open Excel file: %v", err)), nil
		}

		var result = map[string][]string{}
		for _, r := range splitCSV(rowsStr) {
			rowIdx, _ := parseInt(r)
			row, err := f.GetRows(sheet)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get rows: %v", err)), nil
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
		filepathToolOption,
		sheetToolOption,
		colsArgOption,
	),
	Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filepath := req.Params.Arguments["filepath"].(string)
		sheet := req.Params.Arguments["sheet"].(string)
		colsStr := req.Params.Arguments["columns"].(string)

		f, err := getExcelFile(filepath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to open Excel file: %v", err)), nil
		}

		result := map[string][]string{}
		for _, col := range splitCSV(colsStr) {
			colIdx, _ := parseInt(col)
			cols, err := f.GetCols(sheet)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get columns: %v", err)), nil
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
		filepathToolOption,
		sheetToolOption,
		rowsDataArgOption,
	),
	Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filepath := req.Params.Arguments["filepath"].(string)
		sheet := req.Params.Arguments["sheet"].(string)
		rowsJSON := req.Params.Arguments["rows_data"].(string)

		f, err := getExcelFile(filepath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to open Excel file: %v", err)), nil
		}

		var rows map[string][]string
		if err := json.Unmarshal([]byte(rowsJSON), &rows); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to parse JSON: %v", err)), nil
		}

		for r, vals := range rows {
			ridx, err := parseInt(r)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid row number: %s", r)), nil
			}

			for i, val := range vals {
				cell, _ := excelize.CoordinatesToCellName(i+1, ridx)
				if err := f.SetCellValue(sheet, cell, val); err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("failed to set cell value: %v", err)), nil
				}
			}
		}

		if err := f.Save(); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to save Excel file: %v", err)), nil
		}

		return mcp.NewToolResultText("Rows written successfully."), nil
	},
}

var writeColsTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool("write_columns",
		mcp.WithDescription("Write content to specific columns"),
		filepathToolOption,
		sheetToolOption,
		colsDataArgOption,
	),
	Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filepath := req.Params.Arguments["filepath"].(string)
		sheet := req.Params.Arguments["sheet"].(string)
		colsJSON := req.Params.Arguments["cols_data"].(string)

		f, err := getExcelFile(filepath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to open Excel file: %v", err)), nil
		}

		var cols map[string][]string
		if err := json.Unmarshal([]byte(colsJSON), &cols); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to parse JSON: %v", err)), nil
		}

		for col, vals := range cols {
			colIdx, err := excelize.ColumnNameToNumber(col)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid column letter: %s", col)), nil
			}

			for i, val := range vals {
				cell, _ := excelize.CoordinatesToCellName(colIdx, i+1)
				if err := f.SetCellValue(sheet, cell, val); err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("failed to set cell value: %v", err)), nil
				}
			}
		}

		if err := f.Save(); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to save Excel file: %v", err)), nil
		}

		return mcp.NewToolResultText("Columns written successfully."), nil
	},
}

var addSheetTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool("add_sheet",
		mcp.WithDescription("Add a new sheet"),
		filepathToolOption,
		mcp.WithString("sheet_name", mcp.Required(), mcp.Description("Name of the new sheet")),
	),
	Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filepath := req.Params.Arguments["filepath"].(string)
		sheetName := req.Params.Arguments["sheet_name"].(string)
		f, err := getExcelFile(filepath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to open Excel file: %v", err)), nil
		}

		if _, err = f.NewSheet(sheetName); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to add new sheet: %v", err)), nil
		}

		return mcp.NewToolResultText("Sheet added successfully."), nil
	},
}

var deleteSheetTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool("delete_sheet",
		mcp.WithDescription("Delete a sheet"),
		filepathToolOption,
		mcp.WithString("sheet_name", mcp.Required(), mcp.Description("Name of the sheet to delete")),
	),
	Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filepath := req.Params.Arguments["filepath"].(string)
		sheetName := req.Params.Arguments["sheet_name"].(string)
		f, err := getExcelFile(filepath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to open Excel file: %v", err)), nil
		}

		if err := f.DeleteSheet(sheetName); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to delete sheet: %v", err)), nil
		}

		return mcp.NewToolResultText("Sheet deleted."), nil
	},
}

var renameSheetTool = tool_group.MCPToolItem{
	Tool: mcp.NewTool("rename_sheet",
		mcp.WithDescription("Rename a sheet"),
		filepathToolOption,
		mcp.WithString("old_name", mcp.Required(), mcp.Description("Old name of the sheet")),
		mcp.WithString("new_name", mcp.Required(), mcp.Description("New name of the sheet")),
	),
	Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filepath := req.Params.Arguments["filepath"].(string)
		oldName := req.Params.Arguments["old_name"].(string)
		newName := req.Params.Arguments["new_name"].(string)
		f, err := getExcelFile(filepath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to open Excel file: %v", err)), nil
		}

		if err := f.SetSheetName(oldName, newName); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to rename sheet: %v", err)), nil
		}

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
