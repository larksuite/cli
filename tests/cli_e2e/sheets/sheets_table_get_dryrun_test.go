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

// TestSheets_TableGetDefaultDryRun pins the request structure +table-get emits
// when no --range is given: it must first read get_workbook_structure (to learn
// each sheet's grid dimensions, which anchor the used-range probe over the full
// grid) and then read cells via get_cell_ranges. This guards the pro016 / pro025
// fix — the default read must span internal blank rows/columns, not stop at the
// A1 current region.
func TestSheets_TableGetDefaultDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+table-get",
			"--spreadsheet-token", "shtDryRun",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout

	// api.0 — the structure read that supplies the grid dimensions.
	require.Equal(t, "get_workbook_structure", gjson.Get(out, "api.0.body.tool_name").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_read",
		gjson.Get(out, "api.0.url").String(), "stdout:\n%s", out)

	// api.1 — the cells read.
	require.Equal(t, "get_cell_ranges", gjson.Get(out, "api.1.body.tool_name").String(), "stdout:\n%s", out)
}

// TestSheets_TableGetSingleSheetDryRun confirms the single-sheet selector path
// also reads get_workbook_structure now (previously it did not): the grid
// dimensions are needed even when only one sheet is read, so the used-range
// probe can anchor over the full grid.
func TestSheets_TableGetSingleSheetDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+table-get",
			"--spreadsheet-token", "shtDryRun",
			"--sheet-name", "Sheet1",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "get_workbook_structure", gjson.Get(out, "api.0.body.tool_name").String(),
		"single-sheet path must still read the structure for grid dimensions; stdout:\n%s", out)
	require.Equal(t, "get_cell_ranges", gjson.Get(out, "api.1.body.tool_name").String(), "stdout:\n%s", out)
}
