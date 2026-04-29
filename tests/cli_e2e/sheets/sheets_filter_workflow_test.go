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

	t.Run("create spreadsheet with initial data as bot", func(t *testing.T) {
		spreadsheetToken = createSpreadsheet(t, parentT, ctx, "lark-cli-e2e-sheets-filter-"+suffix, "bot")
	})

	t.Run("get sheet info as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"sheets", "+info", "--spreadsheet-token", spreadsheetToken},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		sheetID = gjson.Get(result.Stdout, "data.sheets.sheets.0.sheet_id").String()
		require.NotEmpty(t, sheetID, "sheet_id should not be empty, stdout: %s", result.Stdout)
	})

	t.Run("write test data for filtering as bot", func(t *testing.T) {
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
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("create filter with spreadsheet.sheet.filters create as bot", func(t *testing.T) {
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
			Args:      []string{"sheets", "spreadsheet.sheet.filters", "create"},
			DefaultAs: "bot",
			Params: map[string]any{
				"spreadsheet_token": spreadsheetToken,
				"sheet_id":          sheetID,
			},
			Data: filterData,
			Yes:  true,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
	})

	t.Run("get filter with spreadsheet.sheet.filters get as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, sheetID, "sheet_id is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"sheets", "spreadsheet.sheet.filters", "get"},
			DefaultAs: "bot",
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

	t.Run("update filter with spreadsheet.sheet.filters update as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, sheetID, "sheet_id is required")

		filterData := map[string]any{
			"col":         "C",
			"filter_type": "multiValue",
			"condition": map[string]any{
				"filter_type": "multiValue",
				"expected":    []any{"A"},
			},
		}

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"sheets", "spreadsheet.sheet.filters", "update"},
			DefaultAs: "bot",
			Params: map[string]any{
				"spreadsheet_token": spreadsheetToken,
				"sheet_id":          sheetID,
			},
			Data: filterData,
			Yes:  true,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
	})

	t.Run("delete filter with spreadsheet.sheet.filters delete as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, sheetID, "sheet_id is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"sheets", "spreadsheet.sheet.filters", "delete"},
			DefaultAs: "bot",
			Params: map[string]any{
				"spreadsheet_token": spreadsheetToken,
				"sheet_id":          sheetID,
			},
			Yes: true,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
	})
}
