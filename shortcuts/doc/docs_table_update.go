// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/util"
	"github.com/larksuite/cli/shortcuts/common"
)

var validActions = map[string]bool{
	"update-cell": true,
	"insert-row":  true,
	"delete-rows": true,
	"insert-col":  true,
	"delete-cols": true,
}

var DocsTableUpdate = common.Shortcut{
	Service:     "docs",
	Command:     "+table-update",
	Description: "Update a table inside a Lark document (cell edit, row/col insert/delete)",
	Risk:        "write",
	Scopes:      []string{"docx:document:write_only", "docx:document:readonly"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "doc", Desc: "document URL or token", Required: true},
		{Name: "action", Desc: "operation type", Default: "update-cell", Enum: []string{"update-cell", "insert-row", "delete-rows", "insert-col", "delete-cols"}},
		{Name: "table-index", Desc: "table index in document (0-based)", Default: "0"},
		{Name: "table-id", Desc: "table block_id (overrides --table-index)"},
		{Name: "row", Desc: "row index (0-based, for update-cell)"},
		{Name: "col", Desc: "column index (0-based, for update-cell)"},
		{Name: "at", Desc: "insert position index (for insert-row/insert-col)"},
		{Name: "from", Desc: "start index inclusive (for delete-rows/delete-cols)"},
		{Name: "to", Desc: "end index exclusive (for delete-rows/delete-cols)"},
		{Name: "markdown", Desc: "new cell content (for update-cell)", Input: []string{common.File, common.Stdin}},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		action := runtime.Str("action")
		if !validActions[action] {
			return common.FlagErrorf("invalid --action %q", action)
		}

		switch action {
		case "update-cell":
			if runtime.Str("row") == "" || runtime.Str("col") == "" {
				return common.FlagErrorf("--action update-cell requires --row and --col")
			}
			if runtime.Str("markdown") == "" {
				return common.FlagErrorf("--action update-cell requires --markdown")
			}
		case "insert-row", "insert-col":
			if runtime.Str("at") == "" {
				return common.FlagErrorf("--action %s requires --at", action)
			}
		case "delete-rows", "delete-cols":
			if runtime.Str("from") == "" || runtime.Str("to") == "" {
				return common.FlagErrorf("--action %s requires --from and --to", action)
			}
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ref, err := parseDocumentRef(runtime.Str("doc"))
		if err != nil {
			return err
		}

		docID := ref.Token
		// For wiki URLs, resolve the actual document ID first.
		if ref.Kind == "wiki" {
			resolved, err := resolveWikiToDocID(runtime, ref.Token)
			if err != nil {
				return err
			}
			docID = resolved
		}

		action := runtime.Str("action")

		// Find the target table block ID.
		tableBlockID := runtime.Str("table-id")
		if tableBlockID == "" {
			idx, err := strconv.Atoi(runtime.Str("table-index"))
			if err != nil || idx < 0 {
				return common.FlagErrorf("--table-index must be a non-negative integer")
			}
			tableBlockID, err = findTableBlockID(runtime, docID, idx)
			if err != nil {
				return err
			}
		}

		switch action {
		case "update-cell":
			return execUpdateCell(runtime, docID, tableBlockID)
		case "insert-row":
			return execInsertRow(runtime, docID, tableBlockID)
		case "delete-rows":
			return execDeleteRows(runtime, docID, tableBlockID)
		case "insert-col":
			return execInsertCol(runtime, docID, tableBlockID)
		case "delete-cols":
			return execDeleteCols(runtime, docID, tableBlockID)
		default:
			return common.FlagErrorf("unknown action %q", action)
		}
	},
}

// resolveWikiToDocID calls the wiki API to get the real document token.
func resolveWikiToDocID(runtime *common.RuntimeContext, wikiToken string) (string, error) {
	data, err := runtime.CallAPI(http.MethodGet,
		fmt.Sprintf("/open-apis/wiki/v2/spaces/get_node?token=%s", wikiToken),
		nil, nil)
	if err != nil {
		return "", fmt.Errorf("resolve wiki token: %w", err)
	}
	node := common.GetMap(data, "node")
	objToken := common.GetString(node, "obj_token")
	if objToken == "" {
		return "", output.ErrValidation("wiki node has no obj_token")
	}
	return objToken, nil
}

// findTableBlockID lists document blocks and returns the block_id of the N-th table.
func findTableBlockID(runtime *common.RuntimeContext, docID string, tableIndex int) (string, error) {
	blocks, err := listAllBlocks(runtime, docID)
	if err != nil {
		return "", err
	}

	tableCount := 0
	for _, b := range blocks {
		bm, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		blockType, _ := util.ToFloat64(bm["block_type"])
		// Block type 31 = table in Lark docx API.
		if int(blockType) == 31 {
			if tableCount == tableIndex {
				blockID, _ := bm["block_id"].(string)
				if blockID == "" {
					return "", fmt.Errorf("table block has no block_id")
				}
				return blockID, nil
			}
			tableCount++
		}
	}
	return "", output.ErrValidation("document has only %d table(s), but --table-index is %d", tableCount, tableIndex)
}

// listAllBlocks fetches all blocks of a document with pagination.
func listAllBlocks(runtime *common.RuntimeContext, docID string) ([]interface{}, error) {
	var allBlocks []interface{}
	pageToken := ""

	for {
		url := fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks?page_size=500", docID)
		if pageToken != "" {
			url += "&page_token=" + pageToken
		}
		data, err := runtime.CallAPI(http.MethodGet, url, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("list blocks: %w", err)
		}
		items := common.GetSlice(data, "items")
		allBlocks = append(allBlocks, items...)

		hasMore := common.GetBool(data, "has_more")
		if !hasMore {
			break
		}
		pageToken = common.GetString(data, "page_token")
		if pageToken == "" {
			break
		}
	}
	return allBlocks, nil
}

// getTableInfo retrieves the table block to get row/col count and cell block IDs.
func getTableInfo(runtime *common.RuntimeContext, docID, tableBlockID string) (rows int, cols int, cells []string, err error) {
	url := fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/%s", docID, tableBlockID)
	data, err := runtime.CallAPI(http.MethodGet, url, nil, nil)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("get table block: %w", err)
	}
	block := common.GetMap(data, "block")
	table := common.GetMap(block, "table")
	if table == nil {
		return 0, 0, nil, fmt.Errorf("block %s is not a table", tableBlockID)
	}

	prop := common.GetMap(table, "property")
	rowSize := int(common.GetFloat(prop, "row_size"))
	colSize := int(common.GetFloat(prop, "column_size"))

	cellItems := common.GetSlice(table, "cells")
	cellIDs := make([]string, 0, len(cellItems))
	for _, c := range cellItems {
		if s, ok := c.(string); ok {
			cellIDs = append(cellIDs, s)
		}
	}

	return rowSize, colSize, cellIDs, nil
}

// execUpdateCell updates a single cell's content via MCP.
func execUpdateCell(runtime *common.RuntimeContext, docID, tableBlockID string) error {
	row, _ := strconv.Atoi(runtime.Str("row"))
	col, _ := strconv.Atoi(runtime.Str("col"))
	markdown := runtime.Str("markdown")

	rows, cols, cells, err := getTableInfo(runtime, docID, tableBlockID)
	if err != nil {
		return err
	}
	if row < 0 || row >= rows {
		return output.ErrValidation("--row %d out of range [0, %d)", row, rows)
	}
	if col < 0 || col >= cols {
		return output.ErrValidation("--col %d out of range [0, %d)", col, cols)
	}

	cellIndex := row*cols + col
	if cellIndex >= len(cells) {
		return output.ErrValidation("cell index %d out of range (table has %d cells)", cellIndex, len(cells))
	}
	cellBlockID := cells[cellIndex]

	// Step 1: Delete all existing children of the cell.
	if err := deleteCellChildren(runtime, docID, cellBlockID); err != nil {
		return fmt.Errorf("clear cell: %w", err)
	}

	// Step 2: Create new content in the cell.
	// For simple text, directly create a text block. For complex markdown, use MCP.
	if err := createCellContent(runtime, docID, cellBlockID, markdown); err != nil {
		return fmt.Errorf("write cell content: %w", err)
	}

	runtime.Out(map[string]interface{}{
		"success":    true,
		"action":     "update-cell",
		"doc_id":     docID,
		"table_id":   tableBlockID,
		"cell_id":    cellBlockID,
		"row":        row,
		"col":        col,
		"message":    fmt.Sprintf("cell [%d,%d] updated successfully", row, col),
	}, nil)
	return nil
}

// deleteCellChildren removes all child blocks from a cell.
func deleteCellChildren(runtime *common.RuntimeContext, docID, cellBlockID string) error {
	// List children of the cell block.
	url := fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/%s/children", docID, cellBlockID)
	data, err := runtime.CallAPI(http.MethodGet, url, nil, nil)
	if err != nil {
		return err
	}
	items := common.GetSlice(data, "items")
	if len(items) == 0 {
		return nil
	}

	// Collect child block IDs.
	childIDs := make([]string, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			if id := common.GetString(m, "block_id"); id != "" {
				childIDs = append(childIDs, id)
			}
		}
	}
	if len(childIDs) == 0 {
		return nil
	}

	// Batch delete children.
	deleteURL := fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/%s/children/batch_delete", docID, cellBlockID)
	body := map[string]interface{}{
		"start_index": 0,
		"end_index":   len(childIDs),
	}
	_, err = runtime.CallAPI(http.MethodDelete, deleteURL, nil, body)
	return err
}

// createCellContent creates a text block inside a cell.
func createCellContent(runtime *common.RuntimeContext, docID, cellBlockID, markdown string) error {
	// Create a simple text block as child of the cell.
	url := fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/%s/children", docID, cellBlockID)
	body := map[string]interface{}{
		"children": []map[string]interface{}{
			{
				"block_type": 2, // text block
				"text": map[string]interface{}{
					"elements": []map[string]interface{}{
						{
							"text_run": map[string]interface{}{
								"content": markdown,
							},
						},
					},
					"style": map[string]interface{}{},
				},
			},
		},
	}

	resp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    url,
		Body:       body,
	})
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("create cell content: HTTP %d: %s", resp.StatusCode, string(resp.RawBody))
	}

	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(resp.RawBody, &envelope); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	if envelope.Code != 0 {
		return output.ErrAPI(envelope.Code, fmt.Sprintf("create cell content: %s", envelope.Msg), nil)
	}
	return nil
}

// execInsertRow inserts a row at the specified index.
func execInsertRow(runtime *common.RuntimeContext, docID, tableBlockID string) error {
	at, _ := strconv.Atoi(runtime.Str("at"))

	body := map[string]interface{}{
		"insert_table_row": map[string]interface{}{
			"row_index": at,
		},
	}
	if err := patchBlock(runtime, docID, tableBlockID, body); err != nil {
		return err
	}

	runtime.Out(map[string]interface{}{
		"success":  true,
		"action":   "insert-row",
		"doc_id":   docID,
		"table_id": tableBlockID,
		"at":       at,
		"message":  fmt.Sprintf("row inserted at index %d", at),
	}, nil)
	return nil
}

// execDeleteRows deletes rows in [from, to) range.
func execDeleteRows(runtime *common.RuntimeContext, docID, tableBlockID string) error {
	from, _ := strconv.Atoi(runtime.Str("from"))
	to, _ := strconv.Atoi(runtime.Str("to"))

	body := map[string]interface{}{
		"delete_table_rows": map[string]interface{}{
			"row_start_index": from,
			"row_end_index":   to,
		},
	}
	if err := patchBlock(runtime, docID, tableBlockID, body); err != nil {
		return err
	}

	runtime.Out(map[string]interface{}{
		"success":  true,
		"action":   "delete-rows",
		"doc_id":   docID,
		"table_id": tableBlockID,
		"from":     from,
		"to":       to,
		"message":  fmt.Sprintf("rows [%d, %d) deleted", from, to),
	}, nil)
	return nil
}

// execInsertCol inserts a column at the specified index.
func execInsertCol(runtime *common.RuntimeContext, docID, tableBlockID string) error {
	at, _ := strconv.Atoi(runtime.Str("at"))

	body := map[string]interface{}{
		"insert_table_column": map[string]interface{}{
			"column_index": at,
		},
	}
	if err := patchBlock(runtime, docID, tableBlockID, body); err != nil {
		return err
	}

	runtime.Out(map[string]interface{}{
		"success":  true,
		"action":   "insert-col",
		"doc_id":   docID,
		"table_id": tableBlockID,
		"at":       at,
		"message":  fmt.Sprintf("column inserted at index %d", at),
	}, nil)
	return nil
}

// execDeleteCols deletes columns in [from, to) range.
func execDeleteCols(runtime *common.RuntimeContext, docID, tableBlockID string) error {
	from, _ := strconv.Atoi(runtime.Str("from"))
	to, _ := strconv.Atoi(runtime.Str("to"))

	body := map[string]interface{}{
		"delete_table_columns": map[string]interface{}{
			"column_start_index": from,
			"column_end_index":   to,
		},
	}
	if err := patchBlock(runtime, docID, tableBlockID, body); err != nil {
		return err
	}

	runtime.Out(map[string]interface{}{
		"success":  true,
		"action":   "delete-cols",
		"doc_id":   docID,
		"table_id": tableBlockID,
		"from":     from,
		"to":       to,
		"message":  fmt.Sprintf("columns [%d, %d) deleted", from, to),
	}, nil)
	return nil
}

// patchBlock calls the docx block patch API.
func patchBlock(runtime *common.RuntimeContext, docID, blockID string, body map[string]interface{}) error {
	apiPath := fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/%s", docID, blockID)
	resp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodPatch,
		ApiPath:    apiPath,
		Body:       body,
	})
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("patch block: HTTP %d: %s", resp.StatusCode, string(resp.RawBody))
	}

	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(resp.RawBody, &envelope); err != nil {
		return fmt.Errorf("parse patch response: %w", err)
	}
	if envelope.Code != 0 {
		return output.ErrAPI(envelope.Code, fmt.Sprintf("patch block: %s", envelope.Msg), nil)
	}
	return nil
}
