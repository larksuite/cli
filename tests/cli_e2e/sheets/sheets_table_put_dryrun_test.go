// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestSheets_TablePutStylesDryRun pins the request structure +table-put emits
// when --styles is supplied: the set_cell_range write carries cell_styles merged
// into the matrix, and the structural styles (merge / resize) render as their
// own invoke_write tool calls afterward — the same shape +workbook-create uses.
func TestSheets_TablePutStylesDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+table-put",
			"--spreadsheet-token", "shtDryRun",
			"--sheets", `{"sheets":[{"name":"数据","columns":["a","b"],"data":[["x","y"]]}]}`,
			"--styles", `{"styles":[{"name":"数据","cell_styles":[{"range":"A1:B1","font_weight":"bold"}],"cell_merges":[{"range":"A1:B1"}],"col_sizes":[{"range":"A:A","type":"pixel","size":120}]}]}`,
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := clie2e.DryRunData(result.Stdout)

	// api.0 — the typed write, with cell_styles merged into the cells matrix.
	require.Equal(t, "POST", gjson.Get(out, "api.0.method").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_write",
		gjson.Get(out, "api.0.url").String(), "stdout:\n%s", out)
	require.Equal(t, "set_cell_range", gjson.Get(out, "api.0.body.tool_name").String(), "stdout:\n%s", out)
	firstInput := gjson.Get(out, "api.0.body.input").String()
	require.Contains(t, firstInput, `"font_weight":"bold"`, "cell_styles should merge into the matrix; input:\n%s", firstInput)
	require.Contains(t, firstInput, `"range":"A1:B2"`, "write range should cover header + data; input:\n%s", firstInput)

	// api.1 — the merge op.
	require.Equal(t, "merge_cells", gjson.Get(out, "api.1.body.tool_name").String(), "stdout:\n%s", out)
	require.Contains(t, gjson.Get(out, "api.1.body.input").String(), `"range":"A1:B1"`, "stdout:\n%s", out)

	// api.2 — the column resize.
	require.Equal(t, "resize_range", gjson.Get(out, "api.2.body.tool_name").String(), "stdout:\n%s", out)
	require.Contains(t, gjson.Get(out, "api.2.body.input").String(), `"type":"pixel"`, "stdout:\n%s", out)
}

// TestSheets_TablePutStylesNameMismatchRejected confirms a --styles item whose
// name does not match the --sheets payload sheet is rejected up front (no write
// lands), so a typo surfaces as a validation error rather than a silent skip.
func TestSheets_TablePutStylesNameMismatchRejected(t *testing.T) {
	setSheetsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+table-put",
			"--spreadsheet-token", "shtDryRun",
			"--sheets", `{"sheets":[{"name":"数据","columns":["a"],"data":[["x"]]}]}`,
			"--styles", `{"styles":[{"name":"其他","cell_styles":[{"range":"A1","font_weight":"bold"}]}]}`,
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	combined := result.Stdout + "\n" + result.Stderr
	if !strings.Contains(combined, "must match") {
		t.Fatalf("expected name-mismatch error, got:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	}
}
