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

// TestSheets_SheetListWorkflow exercises +sheet-list against a real workbook.
// It is the only coverage that sees the actual backend response: the package
// tests stub get_workbook_structure, so they can only prove the projection is
// consistent with the shape the stub asserts, not with the shape the server
// sends.
//
// The contract under test is the reason +sheet-list exists as its own shortcut
// rather than an alias: its data is the bare sheets array, entry-for-entry
// identical to what +workbook-info nests under `sheets`. A projection that
// re-serialized or filtered per-sheet fields would still pass a "has sheet_id"
// assertion, so the last subtest compares the two payloads verbatim.
func TestSheets_SheetListWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	spreadsheetToken := ""
	createdSheetID := ""

	t.Run("create spreadsheet as bot", func(t *testing.T) {
		spreadsheetToken = createSpreadsheet(t, parentT, ctx, "lark-cli-e2e-sheet-list-"+suffix, "bot")
	})

	// A second sub-sheet makes the list non-trivial: a one-entry array would pass
	// an ordering-blind assertion either way.
	t.Run("add a second sub-sheet as bot", func(t *testing.T) {
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
	})

	t.Run("+sheet-list emits a bare sheets array as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")
		require.NotEmpty(t, createdSheetID, "created sheet_id is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"sheets", "+sheet-list", "--spreadsheet-token", spreadsheetToken},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		data := gjson.Get(result.Stdout, "data")
		require.True(t, data.IsArray(), "data must be the sheets array itself, stdout:\n%s", result.Stdout)

		// At least the workbook's default sheet plus the one just created. Not an
		// exact count: how many sheets +create seeds a new workbook with is that
		// command's contract, and pinning it here would misattribute its change to
		// +sheet-list.
		entries := data.Array()
		require.GreaterOrEqual(t, len(entries), 2, "stdout:\n%s", result.Stdout)

		ids := make([]string, 0, len(entries))
		for _, entry := range entries {
			id := entry.Get("sheet_id").String()
			require.NotEmpty(t, id, "every entry carries sheet_id, stdout:\n%s", result.Stdout)
			assert.NotEmpty(t, entry.Get("sheet_name").String(), "stdout:\n%s", result.Stdout)
			ids = append(ids, id)
		}
		assert.Contains(t, ids, createdSheetID, "the sub-sheet just created must be listed, stdout:\n%s", result.Stdout)
	})

	t.Run("+sheet-list entries match +workbook-info's sheets as bot", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")

		listResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"sheets", "+sheet-list", "--spreadsheet-token", spreadsheetToken},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		listResult.AssertExitCode(t, 0)
		listResult.AssertStdoutStatus(t, true)

		infoResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"sheets", "+workbook-info", "--spreadsheet-token", spreadsheetToken},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		infoResult.AssertExitCode(t, 0)
		infoResult.AssertStdoutStatus(t, true)

		assert.JSONEq(t,
			gjson.Get(infoResult.Stdout, "data.sheets").Raw,
			gjson.Get(listResult.Stdout, "data").Raw,
			"+sheet-list must forward +workbook-info's sheets entries verbatim\nlist:\n%s\ninfo:\n%s",
			listResult.Stdout, infoResult.Stdout)
	})
}
