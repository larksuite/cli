// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSheets_SheetShortcutsWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	spreadsheetToken := ""
	originalSheetID := ""
	createdSheetID := ""
	copiedSheetID := ""

	t.Run("create spreadsheet with +create as bot", func(t *testing.T) {
		spreadsheetToken = createSpreadsheet(t, parentT, ctx, "lark-cli-e2e-sheet-shortcuts-"+suffix, "bot")
	})

	t.Run("get initial sheet info as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"sheets", "+info", "--spreadsheet-token", spreadsheetToken},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		originalSheetID = gjson.Get(result.Stdout, "data.sheets.sheets.0.sheet_id").String()
		require.NotEmpty(t, originalSheetID, "sheet_id should not be empty, stdout: %s", result.Stdout)
	})

	t.Run("create sheet with +create-sheet as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+create-sheet",
				"--spreadsheet-token", spreadsheetToken,
				"--title", "data-" + suffix,
				"--index", "1",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		createdSheetID = gjson.Get(result.Stdout, "data.sheet_id").String()
		require.NotEmpty(t, createdSheetID, "created sheet_id should not be empty, stdout: %s", result.Stdout)
		assert.Equal(t, "data-"+suffix, gjson.Get(result.Stdout, "data.sheet.title").String())
	})

	t.Run("copy sheet with +copy-sheet as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, createdSheetID, "created sheet_id is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+copy-sheet",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", createdSheetID,
				"--title", "copy-" + suffix,
				"--index", "2",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		copiedSheetID = gjson.Get(result.Stdout, "data.sheet_id").String()
		require.NotEmpty(t, copiedSheetID, "copied sheet_id should not be empty, stdout: %s", result.Stdout)
		assert.NotEqual(t, createdSheetID, copiedSheetID)
		assert.Equal(t, "copy-"+suffix, gjson.Get(result.Stdout, "data.sheet.title").String())
	})

	t.Run("update sheet with +update-sheet as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, createdSheetID, "created sheet_id is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+update-sheet",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", createdSheetID,
				"--title", "renamed-" + suffix,
				"--hidden=true",
				"--frozen-row-count", "2",
				"--frozen-col-count", "1",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		assert.Equal(t, createdSheetID, gjson.Get(result.Stdout, "data.sheet_id").String())
		assert.Equal(t, "renamed-"+suffix, gjson.Get(result.Stdout, "data.sheet.title").String())
		assert.Equal(t, true, gjson.Get(result.Stdout, "data.sheet.hidden").Bool())
	})

	t.Run("verify updated sheet through +info as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, createdSheetID, "created sheet_id is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"sheets", "+info", "--spreadsheet-token", spreadsheetToken},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		var matched gjson.Result
		for _, item := range gjson.Get(result.Stdout, "data.sheets.sheets").Array() {
			if gjson.Get(item.Raw, "sheet_id").String() == createdSheetID {
				matched = item
				break
			}
		}
		require.True(t, matched.Exists(), "updated sheet %s should exist, stdout: %s", createdSheetID, result.Stdout)
		assert.Equal(t, "renamed-"+suffix, gjson.Get(matched.Raw, "title").String())
		assert.Equal(t, true, gjson.Get(matched.Raw, "hidden").Bool())
		assert.Equal(t, int64(2), gjson.Get(matched.Raw, "grid_properties.frozen_row_count").Int())
		assert.Equal(t, int64(1), gjson.Get(matched.Raw, "grid_properties.frozen_column_count").Int())
	})

	t.Run("delete sheet with +delete-sheet as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, copiedSheetID, "copied sheet_id is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+delete-sheet",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", copiedSheetID,
			},
			DefaultAs: "bot",
			Yes:       true,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		assert.Equal(t, true, gjson.Get(result.Stdout, "data.deleted").Bool())
		assert.Equal(t, copiedSheetID, gjson.Get(result.Stdout, "data.sheet_id").String())
	})

	t.Run("verify deleted sheet through +info as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, copiedSheetID, "copied sheet_id is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"sheets", "+info", "--spreadsheet-token", spreadsheetToken},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		for _, item := range gjson.Get(result.Stdout, "data.sheets.sheets").Array() {
			if gjson.Get(item.Raw, "sheet_id").String() == copiedSheetID {
				t.Fatalf("deleted sheet %s should not exist, stdout: %s", copiedSheetID, result.Stdout)
			}
		}
	})
}
