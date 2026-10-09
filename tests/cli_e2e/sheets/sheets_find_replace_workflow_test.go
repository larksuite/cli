// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestSheets_FindReplaceWorkflowAsUser_AATCompatible ports the find/replace scenario from
// LarkQA/lark-cli-e2e-tests at 68614d7d2106 onto the in-repo harness. The user
// lane works with either UAT or AAT; the internal AAT job verifies the principal
// and requires this workflow to pass without skipped steps.
func TestSheets_FindReplaceWorkflowAsUser_AATCompatible(t *testing.T) {
	clie2e.SkipWithoutUserToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	const identity = "user"
	spreadsheetToken := createSpreadsheet(t, t, ctx, "lark-cli-e2e-find-replace-"+clie2e.GenerateSuffix(), identity)
	info, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"sheets", "+workbook-info", "--spreadsheet-token", spreadsheetToken}, DefaultAs: identity,
	})
	require.NoError(t, err)
	info.AssertExitCode(t, 0)
	info.AssertStdoutStatus(t, true)
	sheetID := gjson.Get(info.Stdout, "data.sheets.0.sheet_id").String()
	require.NotEmpty(t, sheetID)
	write, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"sheets", "+cells-set", "--spreadsheet-token", spreadsheetToken,
			"--sheet-id", sheetID, "--range", "A1:C3", "--cells", `[["Name","Status","Score"],["Alice","Todo",85],["Bob","Done",92]]`},
		DefaultAs: identity,
	})
	require.NoError(t, err)
	write.AssertExitCode(t, 0)
	write.AssertStdoutStatus(t, true)

	if !t.Run("find original cell", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"sheets", "spreadsheet.sheets", "find"}, DefaultAs: identity,
			Params: map[string]any{"spreadsheet_token": spreadsheetToken, "sheet_id": sheetID},
			Data: map[string]any{"find": "Alice", "find_condition": map[string]any{
				"range": sheetID + "!A1:C3", "match_entire_cell": true,
			}},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		matches := gjson.Get(result.Stdout, "data.find_result.matched_cells").Array()
		require.Len(t, matches, 1)
		require.Equal(t, "A2", matches[0].String())
	}) {
		return
	}
	if !t.Run("replace original value", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"sheets", "+cells-replace", "--spreadsheet-token", spreadsheetToken,
				"--sheet-id", sheetID, "--range", "A1:C3", "--find", "Alice", "--replacement", "Alicia", "--match-entire-cell"},
			DefaultAs: identity,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	}) {
		return
	}
	t.Run("read replaced cell and unchanged neighbor", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"sheets", "+cells-get", "--spreadsheet-token", spreadsheetToken,
				"--sheet-id", sheetID, "--range", "A2:A3"}, DefaultAs: identity,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, "Alicia", gjson.Get(result.Stdout, "data.ranges.0.cells.0.0.value").String())
		require.Equal(t, "Bob", gjson.Get(result.Stdout, "data.ranges.0.cells.1.0.value").String())
	})
}
