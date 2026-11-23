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

// TestSheets_SheetShortcutsWorkflow drives the sub-sheet lifecycle against a
// real workbook: create, copy, rename, hide, freeze, delete.
//
// Each step is proved by reading the workbook back through +workbook-info:
// rename and hide answer with nothing but a revision counter, so the command's
// own response cannot carry the claim, and even where one does return a
// sheet_id the listing is what shows the name and the hidden flag actually
// landed.
func TestSheets_SheetShortcutsWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	spreadsheetToken := ""
	originalSheetID := ""
	createdSheetID := ""
	copiedSheetID := ""

	// sheetsOf returns the workbook's sub-sheet entries as +workbook-info sees
	// them. The structure payload spells the display name sheet_name in some
	// shapes and title in others, so nameOf accepts both.
	sheetsOf := func(t *testing.T) []gjson.Result {
		t.Helper()
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"sheets", "+workbook-info", "--spreadsheet-token", spreadsheetToken},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		return gjson.Get(result.Stdout, "data.sheets").Array()
	}
	nameOf := func(entry gjson.Result) string {
		if name := entry.Get("sheet_name").String(); name != "" {
			return name
		}
		return entry.Get("title").String()
	}
	findByID := func(t *testing.T, sheetID string) (gjson.Result, bool) {
		t.Helper()
		for _, entry := range sheetsOf(t) {
			if entry.Get("sheet_id").String() == sheetID {
				return entry, true
			}
		}
		return gjson.Result{}, false
	}

	t.Run("create spreadsheet with +workbook-create as bot", func(t *testing.T) {
		spreadsheetToken = createSpreadsheet(t, parentT, ctx, "lark-cli-e2e-sheet-shortcuts-"+suffix, "bot")
	})

	t.Run("get initial sheet info as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")

		entries := sheetsOf(t)
		require.NotEmpty(t, entries, "a new workbook must list at least its default sheet")
		originalSheetID = entries[0].Get("sheet_id").String()
		require.NotEmpty(t, originalSheetID, "sheet_id should not be empty")
	})

	t.Run("create sheet with +sheet-create as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+sheet-create",
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
		assert.NotEqual(t, originalSheetID, createdSheetID)

		entry, ok := findByID(t, createdSheetID)
		require.True(t, ok, "the created sheet must be listed")
		assert.Equal(t, "data-"+suffix, nameOf(entry))
	})

	t.Run("copy sheet with +sheet-copy as bot", func(t *testing.T) {
		require.NotEmpty(t, createdSheetID, "created sheet_id is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+sheet-copy",
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
		assert.NotEqual(t, createdSheetID, copiedSheetID, "the copy is a distinct sub-sheet")

		entry, ok := findByID(t, copiedSheetID)
		require.True(t, ok, "the copy must be listed")
		assert.Equal(t, "copy-"+suffix, nameOf(entry))
	})

	t.Run("rename sheet with +sheet-rename as bot", func(t *testing.T) {
		require.NotEmpty(t, createdSheetID, "created sheet_id is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+sheet-rename",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", createdSheetID,
				"--title", "renamed-" + suffix,
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		entry, ok := findByID(t, createdSheetID)
		require.True(t, ok, "the renamed sheet must still be listed under the same sheet_id")
		assert.Equal(t, "renamed-"+suffix, nameOf(entry))
	})

	// The pre-refactor +update-sheet set title, hidden and the freeze counts in
	// one call; the refactored surface splits them across three commands.
	t.Run("hide sheet with +sheet-hide as bot", func(t *testing.T) {
		require.NotEmpty(t, createdSheetID, "created sheet_id is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+sheet-hide",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", createdSheetID,
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		entry, ok := findByID(t, createdSheetID)
		require.True(t, ok, "a hidden sheet is still listed")
		assert.True(t, entry.Get("is_hidden").Bool(), "sheet should be hidden, entry: %s", entry.Raw)
	})

	t.Run("freeze rows and columns with +dim-freeze as bot", func(t *testing.T) {
		require.NotEmpty(t, originalSheetID, "original sheet_id is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+dim-freeze",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", originalSheetID,
				"--rows", "2",
				"--cols", "1",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		assert.Equal(t, int64(2), gjson.Get(result.Stdout, "data.frozen_rows").Int(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, int64(1), gjson.Get(result.Stdout, "data.frozen_columns").Int(), "stdout:\n%s", result.Stdout)
	})

	t.Run("delete sheet with +sheet-delete as bot", func(t *testing.T) {
		require.NotEmpty(t, copiedSheetID, "copied sheet_id is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+sheet-delete",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", copiedSheetID,
			},
			DefaultAs: "bot",
			Yes:       true,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		_, ok := findByID(t, copiedSheetID)
		assert.False(t, ok, "deleted sheet %s should no longer be listed", copiedSheetID)
	})
}
