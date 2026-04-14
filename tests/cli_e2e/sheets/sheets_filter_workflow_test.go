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

	suffix := clie2e.GenerateSuffix()
	spreadsheetToken := ""
	sheetID := ""

	t.Run("create spreadsheet with initial data", func(t *testing.T) {
		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{"sheets", "+create", "--title", "lark-cli-e2e-sheets-filter-" + suffix},
		}, clie2e.RetryOptions{})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		spreadsheetToken = gjson.Get(result.Stdout, "data.spreadsheet_token").String()
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token should not be empty, stdout: %s", result.Stdout)

		parentT.Cleanup(func() {
			// No sheets delete command is currently available in lark-cli,
			// so created spreadsheets are intentionally left in the test account.
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
				"--range", "A1:C5",
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
