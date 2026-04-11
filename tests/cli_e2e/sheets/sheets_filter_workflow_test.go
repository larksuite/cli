// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestSheets_FilterWorkflow tests the spreadsheet sheet filter operations
func TestSheets_FilterWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	spreadsheetToken := ""
	sheetID := ""

	// First create a spreadsheet and add some data for filtering
	t.Run("create spreadsheet with initial data", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"sheets", "+create", "--title", "lark-cli-e2e-sheets-filter-" + suffix},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		spreadsheetToken = gjson.Get(result.Stdout, "data.spreadsheet_token").String()
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token should not be empty, stdout: %s", result.Stdout)

		parentT.Cleanup(func() {
			// Best-effort cleanup
		})
	})

	t.Run("get sheet info", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"sheets", "+info", "--spreadsheet-token", spreadsheetToken},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		sheetID = gjson.Get(result.Stdout, "data.sheets.sheets.0.sheet_id").String()
		require.NotEmpty(t, sheetID, "sheet_id should not be empty, stdout: %s", result.Stdout)
	})

	t.Run("write test data for filtering", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, sheetID, "sheet_id is required")

		values := [][]any{
			{"Name", "Score", "Grade"},
			{"Alice", 85, "B"},
			{"Bob", 92, "A"},
			{"Charlie", 78, "C"},
			{"Diana", 95, "A"},
		}
		valuesJSON, _ := json.Marshal(values)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+write",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", sheetID,
				"--values", string(valuesJSON),
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("create filter with spreadsheet.sheet.filters create", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, sheetID, "sheet_id is required")

		filterData := map[string]any{
			"range":       fmt.Sprintf("%s!A1:D5", sheetID),
			"col":         "C",
			"filter_type": "multiValue",
			"condition": map[string]any{
				"filter_type": "multiValue",
				"expected":    []any{"A", "B"},
			},
		}

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"sheets", "spreadsheet.sheet.filters", "create"},
			Params: map[string]any{
				"spreadsheet_token": spreadsheetToken,
				"sheet_id":          sheetID,
			},
			Data: filterData,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
	})

	t.Run("get filter with spreadsheet.sheet.filters get", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, sheetID, "sheet_id is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"sheets", "spreadsheet.sheet.filters", "get"},
			Params: map[string]any{
				"spreadsheet_token": spreadsheetToken,
				"sheet_id":          sheetID,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		filterInfo := gjson.Get(result.Stdout, "data.sheet_filter_info")
		require.True(t, filterInfo.Exists(), "filter info should exist, stdout: %s", result.Stdout)
	})

	t.Run("update filter with spreadsheet.sheet.filters update", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, sheetID, "sheet_id is required")

		filterData := map[string]any{
			"col":         "B",
			"filter_type": "number",
			"condition": map[string]any{
				"filter_type":  "number",
				"compare_type": "greater",
				"expected":     []any{80},
			},
		}

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"sheets", "spreadsheet.sheet.filters", "update"},
			Params: map[string]any{
				"spreadsheet_token": spreadsheetToken,
				"sheet_id":          sheetID,
			},
			Data: filterData,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
	})

	t.Run("delete filter with spreadsheet.sheet.filters delete", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, sheetID, "sheet_id is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"sheets", "spreadsheet.sheet.filters", "delete"},
			Params: map[string]any{
				"spreadsheet_token": spreadsheetToken,
				"sheet_id":          sheetID,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
	})
}

// TestSheets_FindWorkflow tests the spreadsheet.sheets find operation
func TestSheets_FindWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	spreadsheetToken := ""
	sheetID := ""

	// Create spreadsheet and add data for finding
	t.Run("create spreadsheet", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"sheets", "+create", "--title", "lark-cli-e2e-sheets-find-" + suffix},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		spreadsheetToken = gjson.Get(result.Stdout, "data.spreadsheet_token").String()
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token should not be empty, stdout: %s", result.Stdout)

		parentT.Cleanup(func() {
			// Best-effort cleanup
		})
	})

	t.Run("get sheet info", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"sheets", "+info", "--spreadsheet-token", spreadsheetToken},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		sheetID = gjson.Get(result.Stdout, "data.sheets.sheets.0.sheet_id").String()
		require.NotEmpty(t, sheetID, "sheet_id should not be empty, stdout: %s", result.Stdout)
	})

	t.Run("write test data for finding", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, sheetID, "sheet_id is required")

		values := [][]any{
			{"apple", "banana", "cherry"},
			{"Apple", "BANANA", "Cherry"},
			{"APPLE", "banana", "CHERRY"},
		}
		valuesJSON, _ := json.Marshal(values)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+write",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", sheetID,
				"--values", string(valuesJSON),
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("find cells with spreadsheet.sheets find", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, sheetID, "sheet_id is required")

		findData := map[string]any{
			"find": "apple",
			"find_condition": map[string]any{
				"range":             fmt.Sprintf("%s!A1:C3", sheetID),
				"match_case":        false,
				"match_entire_cell": false,
			},
		}

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"sheets", "spreadsheet.sheets", "find"},
			Params: map[string]any{
				"spreadsheet_token": spreadsheetToken,
				"sheet_id":          sheetID,
			},
			Data: findData,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		findResult := gjson.Get(result.Stdout, "data.find_result")
		require.True(t, findResult.Exists(), "find_result should exist, stdout: %s", result.Stdout)
	})
}