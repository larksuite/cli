// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/larksuite/cli/tests/cli_e2e/drive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestSheets_CRUDE2EWorkflow tests the full lifecycle of spreadsheet operations
// using all shortcut methods: +workbook-create, +workbook-info, +cells-set,
// +cells-get, +cells-search, +workbook-export
func TestSheets_CRUDE2EWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	spreadsheetToken := ""
	sheetID := ""

	t.Run("create spreadsheet with +workbook-create as bot", func(t *testing.T) {
		spreadsheetToken = createSpreadsheet(t, parentT, ctx, "lark-cli-e2e-sheets-"+suffix, "bot")
	})

	t.Run("get spreadsheet info with +workbook-info as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"sheets", "+workbook-info", "--spreadsheet-token", spreadsheetToken},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		sheetID = gjson.Get(result.Stdout, "data.sheets.0.sheet_id").String()
		require.NotEmpty(t, sheetID, "sheet_id should not be empty, stdout: %s", result.Stdout)
	})

	t.Run("write data with +cells-set as bot", func(t *testing.T) {
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
				"sheets", "+cells-set",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", sheetID,
				"--range", "A1:C3",
				"--cells", string(valuesJSON),
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("read data with +cells-get as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, sheetID, "sheet_id is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+cells-get",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", sheetID,
				"--range", "A1:C3",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		// Collected out of the decoded payload rather than matched against a
		// fixed path: get_cell_ranges' response nesting is the backend's to
		// change and is pinned nowhere in this repo, while the values having
		// survived the round trip is the actual claim.
		got := scalarsIn(gjson.Get(result.Stdout, "data"))
		for _, want := range []string{"Name", "Alice", "Beijing"} {
			require.Contains(t, got, want, "read-back lost %q; stdout:\n%s", want, result.Stdout)
		}
	})

	// The pre-refactor +append is gone; writing past the block already on the
	// sheet is a plain +cells-set at the next row, which also exercises the
	// auto-expand path the old command relied on.
	t.Run("append a row with +cells-set as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, sheetID, "sheet_id is required")

		values := [][]any{{"Charlie", 28, "Guangzhou"}}
		valuesJSON, _ := json.Marshal(values)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+cells-set",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", sheetID,
				"--range", "A4:C4",
				"--cells", string(valuesJSON),
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		// Read the row back rather than trusting the ok envelope: an
		// auto-expanding write that reported success without persisting is
		// exactly the failure this subtest exists to catch, and the
		// +cells-search below only looks at row 2.
		readBack, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+cells-get",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", sheetID,
				"--range", "A4:C4",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		readBack.AssertExitCode(t, 0)
		readBack.AssertStdoutStatus(t, true)

		got := scalarsIn(gjson.Get(readBack.Stdout, "data"))
		for _, want := range []string{"Charlie", "Guangzhou"} {
			require.Contains(t, got, want, "appended row lost %q; stdout:\n%s", want, readBack.Stdout)
		}
	})

	t.Run("find cells with +cells-search as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, sheetID, "sheet_id is required")

		search := func(t *testing.T, term string) string {
			t.Helper()
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"sheets", "+cells-search",
					"--spreadsheet-token", spreadsheetToken,
					"--sheet-id", sheetID,
					"--find", term,
					"--range", "A1:C10",
				},
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			result.AssertStdoutStatus(t, true)
			return result.Stdout
		}

		// Both halves are asserted: a hit alone would still pass if the search
		// silently matched everything, which is the failure mode a
		// non-matching term catches.
		hit := search(t, "Alice")
		assert.Equal(t, int64(1), gjson.Get(hit, "data.total_matches").Int(), "stdout:\n%s", hit)
		assert.Equal(t, "A2", gjson.Get(hit, "data.matches.0.address").String(), "stdout:\n%s", hit)

		miss := search(t, "no-such-value-"+suffix)
		assert.Equal(t, int64(0), gjson.Get(miss, "data.total_matches").Int(), "stdout:\n%s", miss)
	})

	t.Run("export spreadsheet with +workbook-export as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		outputDir := t.TempDir()
		outputPath := filepath.Join(outputDir, "export.xlsx")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+workbook-export",
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
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	spreadsheetToken := ""
	const defaultAs = "bot"

	t.Run("create spreadsheet with spreadsheets create as bot", func(t *testing.T) {
		folderToken := drive.CreateDriveFolder(t, parentT, ctx, "lark-cli-e2e-sheets-resource-folder-"+suffix, defaultAs, "")
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"sheets", "spreadsheets", "create"},
			DefaultAs: defaultAs,
			Data: map[string]any{
				"title":        "lark-cli-e2e-sheets-resource-" + suffix,
				"folder_token": folderToken,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		spreadsheetToken = gjson.Get(result.Stdout, "data.spreadsheet.spreadsheet_token").String()
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token should not be empty, stdout: %s", result.Stdout)

		parentT.Cleanup(func() {
			cleanupCtx, cancel := clie2e.CleanupContext()
			defer cancel()

			deleteResult, deleteErr := drive.DeleteDriveResourceAndVerify(cleanupCtx, spreadsheetToken, "sheet", defaultAs)
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
		result.AssertStdoutStatus(t, true)

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
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		// Verify the title was updated by fetching the spreadsheet
		getResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"sheets", "spreadsheets", "get"},
			DefaultAs: "bot",
			Params:    map[string]any{"spreadsheet_token": spreadsheetToken},
		})
		require.NoError(t, err)
		getResult.AssertExitCode(t, 0)
		getResult.AssertStdoutStatus(t, true)

		// Verify the title was actually updated
		assert.Equal(t, updatedTitle, gjson.Get(getResult.Stdout, "data.spreadsheet.title").String())
	})
}
