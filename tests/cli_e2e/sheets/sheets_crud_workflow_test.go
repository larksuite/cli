// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	drivee2e "github.com/larksuite/cli/tests/cli_e2e/drive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestSheets_CRUDE2EWorkflow tests the full lifecycle of spreadsheet operations
// using all shortcut methods: +create, +read, +write, +append, +find, +info, +export
func TestSheets_CRUDE2EWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	spreadsheetToken := ""
	sheetID := ""

	t.Run("create spreadsheet with +create as bot", func(t *testing.T) {
		spreadsheetToken = createSpreadsheet(t, parentT, ctx, "lark-cli-e2e-sheets-"+suffix, "bot")
	})

	t.Run("get spreadsheet info with +info as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"sheets", "+info", "--spreadsheet-token", spreadsheetToken},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		assert.Equal(t, spreadsheetToken, gjson.Get(result.Stdout, "data.spreadsheet.spreadsheet.token").String())
		sheetID = gjson.Get(result.Stdout, "data.sheets.sheets.0.sheet_id").String()
		require.NotEmpty(t, sheetID, "sheet_id should not be empty, stdout: %s", result.Stdout)
	})

	t.Run("write data with +write as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, sheetID, "sheet_id is required")

		values := [][]any{
			{"Name", "Age", "City"},
			{"Alice", 25, "Beijing"},
			{"Bob", 30, "Shanghai"},
		}
		valuesJSON, _ := json.Marshal(values)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+write",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", sheetID,
				"--range", "A1:C3",
				"--values", string(valuesJSON),
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("read data with +read as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, sheetID, "sheet_id is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+read",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", sheetID,
				"--range", "A1:C3",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		// Verify the data was written correctly
		values := gjson.Get(result.Stdout, "data.valueRange.values")
		require.True(t, values.IsArray(), "values should be an array, stdout: %s", result.Stdout)
		assert.Equal(t, "Name", values.Array()[0].Array()[0].String())
		assert.Equal(t, "Alice", values.Array()[1].Array()[0].String())
	})

	t.Run("append rows with +append as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, sheetID, "sheet_id is required")

		values := [][]any{{"Charlie", 28, "Guangzhou"}}
		valuesJSON, _ := json.Marshal(values)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+append",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", sheetID,
				"--range", "A4:C4",
				"--values", string(valuesJSON),
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("find cells with +find as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, sheetID, "sheet_id is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+find",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", sheetID,
				"--find", "Alice",
				"--range", fmt.Sprintf("%s!A1:C10", sheetID),
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, true, gjson.Get(result.Stdout, "ok").Bool(), "stdout:\n%s", result.Stdout)

		matchedCells := gjson.Get(result.Stdout, "data.find_result.matched_cells")
		require.True(t, matchedCells.IsArray(), "matched_cells should be an array, stdout: %s", result.Stdout)
		assert.True(t, len(matchedCells.Array()) > 0, "should find at least one cell containing 'Alice'")
	})

	t.Run("export spreadsheet with +export as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		outputDir := t.TempDir()
		outputPath := filepath.Join(outputDir, "export.xlsx")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+export",
				"--spreadsheet-token", spreadsheetToken,
				"--file-extension", "xlsx",
				"--output-path", "./export.xlsx",
			},
			DefaultAs: "bot",
			WorkDir:   outputDir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		savedPath := gjson.Get(result.Stdout, "data.saved_path").String()
		require.NotEmpty(t, savedPath, "stdout:\n%s", result.Stdout)
		savedPathReal, err := filepath.EvalSymlinks(savedPath)
		require.NoError(t, err, "stdout:\n%s", result.Stdout)
		outputPathReal, err := filepath.EvalSymlinks(outputPath)
		require.NoError(t, err, "stdout:\n%s", result.Stdout)
		assert.Equal(t, outputPathReal, savedPathReal, "stdout:\n%s", result.Stdout)
		assert.FileExists(t, outputPath, "stdout:\n%s", result.Stdout)
	})
}

// TestSheets_SpreadsheetsResource tests the spreadsheets resource methods
func TestSheets_SpreadsheetsResource(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	spreadsheetToken := ""

	t.Run("create spreadsheet with spreadsheets create as bot", func(t *testing.T) {
		folderToken := drivee2e.CreateDriveFolder(t, parentT, ctx, "lark-cli-e2e-sheets-resource-folder-"+suffix, "bot", "")
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"sheets", "spreadsheets", "create"},
			DefaultAs: "bot",
			Data: map[string]any{
				"title":        "lark-cli-e2e-sheets-resource-" + suffix,
				"folder_token": folderToken,
			},
			Yes: true,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		spreadsheetToken = gjson.Get(result.Stdout, "data.spreadsheet.spreadsheet_token").String()
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token should not be empty, stdout: %s", result.Stdout)

		parentT.Cleanup(func() {
			cleanupCtx, cancel := clie2e.CleanupContext()
			defer cancel()

			deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
				Args: []string{
					"drive", "+delete",
					"--file-token", spreadsheetToken,
					"--type", "sheet",
					"--yes",
				},
				DefaultAs: "bot",
			})
			clie2e.ReportCleanupFailure(parentT, "delete spreadsheet "+spreadsheetToken, deleteResult, deleteErr)
		})
	})

	t.Run("get spreadsheet with spreadsheets get as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"sheets", "spreadsheets", "get"},
			DefaultAs: "bot",
			Params:    map[string]any{"spreadsheet_token": spreadsheetToken},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		assert.Equal(t, spreadsheetToken, gjson.Get(result.Stdout, "data.spreadsheet.token").String())
		assert.NotEmpty(t, gjson.Get(result.Stdout, "data.spreadsheet.url").String())
	})

	t.Run("patch spreadsheet with spreadsheets patch as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")

		updatedTitle := "lark-cli-e2e-sheets-patched-" + suffix
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"sheets", "spreadsheets", "patch"},
			DefaultAs: "bot",
			Params:    map[string]any{"spreadsheet_token": spreadsheetToken},
			Data:      map[string]any{"title": updatedTitle},
			Yes:       true,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		// Verify the title was updated by fetching the spreadsheet
		getResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"sheets", "spreadsheets", "get"},
			DefaultAs: "bot",
			Params:    map[string]any{"spreadsheet_token": spreadsheetToken},
		})
		require.NoError(t, err)
		getResult.AssertExitCode(t, 0)
		getResult.AssertStdoutStatus(t, 0)

		// Verify the title was actually updated
		assert.Equal(t, updatedTitle, gjson.Get(getResult.Stdout, "data.spreadsheet.title").String())
	})
}
