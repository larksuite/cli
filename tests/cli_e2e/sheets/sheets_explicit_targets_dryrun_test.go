// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSheets_ExplicitIDStyleStampsDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	const payload = `[{"range":"B2:C2","style":{"fontWeight":"bold","hAlign":"right"}}]`
	path := filepath.Join(t.TempDir(), "styles.json")
	require.NoError(t, os.WriteFile(path, []byte(payload), 0600))
	for _, tc := range []struct{ value, stdin string }{{payload, ""}, {"@" + path, ""}, {"-", payload}} {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)
		r, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"sheets", "+styles-put", "--spreadsheet-token", "shtDryRun", "--sheet-id", "subExplicit", "--styles", tc.value, "--dry-run"}, Stdin: []byte(tc.stdin), DefaultAs: "user"})
		require.NoError(t, err)
		r.AssertExitCode(t, 0)
		require.Equal(t, "POST", clie2e.DryRunGet(r.Stdout, "api.0.method").String())
		require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_read", clie2e.DryRunGet(r.Stdout, "api.0.url").String())
		require.Equal(t, "get_workbook_structure", clie2e.DryRunGet(r.Stdout, "api.0.body.tool_name").String())
		require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_write", clie2e.DryRunGet(r.Stdout, "api.1.url").String())
		input := clie2e.DryRunGet(r.Stdout, "api.1.body.input").String()
		require.Equal(t, "subExplicit", gjson.Get(input, "operations.0.input.sheet_id").String())
		require.Equal(t, "B2:C2", gjson.Get(input, "operations.0.input.range").String())
		require.Equal(t, "bold", gjson.Get(input, "operations.0.input.cells.0.0.cell_styles.font_weight").String())
		require.False(t, gjson.Get(input, "operations.0.input.cells.0.0.value").Exists())
	}
}

func TestSheets_ExplicitBatchTargetsDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	for _, tc := range []struct {
		command, tool string
		extra         []string
	}{
		{"+cells-batch-clear", "clear_cell_range", []string{"--scope", "formats"}},
		{"+dropdown-delete", "set_cell_range", nil},
		{"+dropdown-update", "set_cell_range", []string{"--options", `["a","b"]`}},
		{"+cells-batch-set-style", "set_cell_range", []string{"--font-weight", "bold"}},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)
		args := append([]string{"sheets", tc.command, "--spreadsheet-token", "shtDryRun", "--sheet-id", "subExplicit", "--range", `A1:B2,D4`, "--dry-run"}, tc.extra...)
		r, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "user"})
		require.NoError(t, err)
		r.AssertExitCode(t, 0)
		require.Equal(t, "POST", clie2e.DryRunGet(r.Stdout, "api.0.method").String())
		require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_write", clie2e.DryRunGet(r.Stdout, "api.0.url").String())
		require.Equal(t, "batch_update", clie2e.DryRunGet(r.Stdout, "api.0.body.tool_name").String())
		input := clie2e.DryRunGet(r.Stdout, "api.0.body.input").String()
		for _, op := range gjson.Get(input, "operations").Array() {
			require.Equal(t, tc.tool, op.Get("tool_name").String())
			require.Equal(t, "subExplicit", op.Get("input.sheet_id").String())
			require.False(t, op.Get("input.sheet_name").Exists())
		}
		require.Equal(t, "A1:B2", gjson.Get(input, "operations.0.input.range").String())
		require.Equal(t, "D4", gjson.Get(input, "operations.1.input.range").String())
		if tc.command == "+cells-batch-clear" {
			require.Equal(t, "formats", gjson.Get(input, "operations.0.input.clear_type").String())
		}
	}
	for _, args := range [][]string{
		{"--sheet-name", "S", "--ranges", `["Other!A1"]`},
		{"--sheet-id", "id", "--ranges", `["S!A1"]`},
		{"--sheet-id", "id", "--sheet-name", "S", "--ranges", `["A1"]`},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)
		r, err := clie2e.RunCmd(ctx, clie2e.Request{Args: append([]string{"sheets", "+cells-batch-clear", "--spreadsheet-token", "shtDryRun", "--dry-run"}, args...), DefaultAs: "user"})
		require.NoError(t, err)
		r.AssertExitCode(t, 2)
		require.Empty(t, r.Stdout)
		require.Equal(t, "validation", gjson.Get(r.Stderr, "error.type").String())
	}
}

func TestSheets_ResizeBoundaryAndDeleteEndDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	for _, args := range [][]string{
		{"+rows-resize", "--start-row", "5", "--end-row", "8", "--height", "120"},
		{"+dim-delete", "--start", "5", "--end", "8"},
	} {
		r, err := clie2e.RunCmd(ctx, clie2e.Request{Args: append([]string{"sheets"}, append(args, "--spreadsheet-token", "shtDryRun", "--sheet-name", "S", "--dry-run")...), DefaultAs: "user"})
		require.NoError(t, err)
		r.AssertExitCode(t, 0)
		require.Equal(t, "POST", clie2e.DryRunGet(r.Stdout, "api.0.method").String())
		require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_write", clie2e.DryRunGet(r.Stdout, "api.0.url").String())
		input := clie2e.DryRunGet(r.Stdout, "api.0.body.input").String()
		require.Equal(t, "5:8", gjson.Get(input, "range").String())
		require.Equal(t, "S", gjson.Get(input, "sheet_name").String())
	}
}

func TestSheets_QualifiedDeleteIndexesRejectConflictingDimensionDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	r, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"sheets", "+dim-delete", "--spreadsheet-token", "shtDryRun", "--start-index", "S!3", "--end-index", "5", "--dimension", "column", "--dry-run"}, DefaultAs: "user"})
	require.NoError(t, err)
	r.AssertExitCode(t, 2)
	require.Empty(t, r.Stdout)
	require.Equal(t, "validation", gjson.Get(r.Stderr, "error.type").String())
	require.Equal(t, "invalid_argument", gjson.Get(r.Stderr, "error.subtype").String())
	require.Equal(t, "--dimension", gjson.Get(r.Stderr, "error.param").String())
}

func TestSheets_WriteIntentConflictsRejectBeforeAPIDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	for _, tc := range []struct {
		command, param string
		args           []string
	}{
		{"+styles-put", "--sheet-name", []string{"--sheet-name", "Expected", "--styles", `{"name":"Other","cell_styles":[{"range":"B2","font_weight":"bold"}]}`}},
		{"+cells-set", "--cells", []string{"--sheet-name", "S", "--range", "A1", "--cells", `[[{"type":"text","text":"=SUM(1,2)"}]]`}},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)
		r, err := clie2e.RunCmd(ctx, clie2e.Request{Args: append([]string{"sheets", tc.command, "--spreadsheet-token", "shtDryRun", "--dry-run"}, tc.args...), DefaultAs: "user"})
		require.NoError(t, err)
		r.AssertExitCode(t, 2)
		require.Empty(t, r.Stdout)
		require.Equal(t, "validation", gjson.Get(r.Stderr, "error.type").String())
		require.Equal(t, "invalid_argument", gjson.Get(r.Stderr, "error.subtype").String())
		require.Equal(t, tc.param, gjson.Get(r.Stderr, "error.param").String())
	}
}

func TestSheets_DropdownRangeHelpMatchesSelectorsDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	for _, command := range []string{"+dropdown-update", "+dropdown-delete"} {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)
		r, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"sheets", command, "--help"}})
		require.NoError(t, err)
		r.AssertExitCode(t, 0)
		help := strings.Join(strings.Fields(r.Stdout), " ")
		require.Contains(t, help, "explicit --sheet-id / --sheet-name with unqualified ranges")
		require.Contains(t, help, "--sheet-id cannot be combined with sheet-prefixed ranges")
	}
}

func TestSheets_TypedTablePayloadAnchorDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	r, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"sheets", "+table-put", "--spreadsheet-token", "shtDryRun", "--sheet-id", "id", "--sheets", `[{"name":"S","start_cell":"C5","columns":["label","n"],"dtypes":{"n":"int64"},"data":[["00123",1001]],"allow_overwrite":false}]`, "--dry-run"}, DefaultAs: "user"})
	require.NoError(t, err)
	r.AssertExitCode(t, 0)
	require.Equal(t, "POST", clie2e.DryRunGet(r.Stdout, "api.0.method").String())
	require.Equal(t, "get_workbook_structure", clie2e.DryRunGet(r.Stdout, "api.0.body.tool_name").String())
	require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_write", clie2e.DryRunGet(r.Stdout, "api.1.url").String())
	require.Equal(t, "set_cell_range", clie2e.DryRunGet(r.Stdout, "api.1.body.tool_name").String())
	input := clie2e.DryRunGet(r.Stdout, "api.1.body.input").String()
	require.Equal(t, "C5:D6", gjson.Get(input, "range").String())
	require.Equal(t, "id", gjson.Get(input, "sheet_id").String())
	require.Equal(t, "label", gjson.Get(input, "cells.0.0.value").Value())
	require.Equal(t, "00123", gjson.Get(input, "cells.1.0.value").Value())
	require.Equal(t, "1001", gjson.Get(input, "cells.1.1.value").Raw)
	require.Equal(t, false, gjson.Get(input, "allow_overwrite").Value())
}

func TestSheets_MetadataOutputPathIsUnsupportedDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	for _, command := range []string{
		"+workbook-info", "+sheet-info", "+dropdown-get", "+chart-list", "+pivot-list",
		"+cond-format-list", "+filter-list", "+filter-view-list", "+sparkline-list", "+float-image-list",
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)
		output := filepath.Join(t.TempDir(), "metadata.json")
		args := []string{"sheets", command, "--spreadsheet-token", "shtDryRun", "--output-path", output, "--dry-run"}
		r, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "user"})
		require.NoError(t, err)
		r.AssertExitCode(t, 2)
		require.Empty(t, r.Stdout)
		require.Equal(t, "validation", gjson.Get(r.Stderr, "error.type").String())
		require.Equal(t, "--output-path", gjson.Get(r.Stderr, "error.params.0.name").String())
		require.Equal(t, "unknown flag", gjson.Get(r.Stderr, "error.params.0.reason").String())
		_, err = os.Stat(output)
		require.True(t, os.IsNotExist(err), "dry-run must not create the output file")
	}
}
