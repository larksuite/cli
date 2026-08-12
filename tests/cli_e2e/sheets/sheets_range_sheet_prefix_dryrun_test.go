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

// TestSheets_RangeSheetPrefixDryRun pins the wire effect of a sheet-prefixed
// --range at the binary level: the prefix becomes sheet_name and the tool
// receives the bare A1 range, so a call that names its sheet only inside
// --range no longer dies on the selector check.
func TestSheets_RangeSheetPrefixDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)

	tests := []struct {
		name      string
		args      []string
		toolName  string
		rangePath string // where the bare range lands in the tool input
		wantSheet string
		wantRange string
	}{
		{
			name: "cells-get plain prefix",
			args: []string{
				"sheets", "+cells-get",
				"--spreadsheet-token", "shtDryRun",
				"--range", "Sheet1!A1:D20",
			},
			toolName:  "get_cell_ranges",
			rangePath: "ranges.0",
			wantSheet: "Sheet1",
			wantRange: "A1:D20",
		},
		{
			// Quoted names are lexed, not split on the first "!": a sheet whose
			// own name contains a bang must survive intact.
			name: "cells-get quoted name containing a bang",
			args: []string{
				"sheets", "+cells-get",
				"--spreadsheet-token", "shtDryRun",
				"--range", "'Q1!Sales'!A1:B2",
			},
			toolName:  "get_cell_ranges",
			rangePath: "ranges.0",
			wantSheet: "Q1!Sales",
			wantRange: "A1:B2",
		},
		{
			name: "cells-clear quoted name with a space",
			args: []string{
				"sheets", "+cells-clear",
				"--spreadsheet-token", "shtDryRun",
				"--range", "'My Sheet'!A1:B2",
			},
			toolName:  "clear_cell_range",
			rangePath: "range",
			wantSheet: "My Sheet",
			wantRange: "A1:B2",
		},
		{
			name: "cells-set carries the prefix into a write",
			args: []string{
				"sheets", "+cells-set",
				"--spreadsheet-token", "shtDryRun",
				"--range", "Sheet1!A1",
				"--cells", `[[{"value":"x"}]]`,
			},
			toolName:  "set_cell_range",
			rangePath: "range",
			wantSheet: "Sheet1",
			wantRange: "A1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      append(append([]string{}, tt.args...), "--dry-run"),
				DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := clie2e.DryRunData(result.Stdout)
			require.Equal(t, "POST", gjson.Get(out, "api.0.method").String(), "stdout:\n%s", out)
			require.Equal(t, tt.toolName, gjson.Get(out, "api.0.body.tool_name").String(), "stdout:\n%s", out)

			input := gjson.Get(out, "api.0.body.input").String()
			require.Equal(t, tt.wantSheet, gjson.Get(input, "sheet_name").String(), "input:\n%s", input)
			require.Equal(t, tt.wantRange, gjson.Get(input, tt.rangePath).String(), "input:\n%s", input)
		})
	}
}

// An explicit selector stays authoritative end to end: --range is forwarded
// verbatim, so a prefix that disagrees with --sheet-name can never silently
// retarget the call.
func TestSheets_RangeSheetPrefixExplicitSelectorWinsDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+cells-get",
			"--spreadsheet-token", "shtDryRun",
			"--sheet-name", "Other",
			"--range", "Sheet1!A1:D20",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	input := gjson.Get(clie2e.DryRunData(result.Stdout), "api.0.body.input").String()
	require.Equal(t, "Other", gjson.Get(input, "sheet_name").String(), "input:\n%s", input)
	require.Equal(t, "Sheet1!A1:D20", gjson.Get(input, "ranges.0").String(), "input:\n%s", input)
}

// A single-cell --range is an anchor sized from the payload — but not when it
// carries a sheet prefix. Such a range only survives the rewrite beside an
// explicit selector it contradicts, and sizing it would ship a range naming
// one sheet next to a sheet_name naming another. It fails locally instead,
// with the mismatch the caller can act on.
func TestSheets_QualifiedAnchorNotExpandedDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+cells-set",
			"--spreadsheet-token", "shtDryRun",
			"--sheet-name", "Other",
			"--range", "Sheet1!A1",
			"--cells", `[[{"value":"a"},{"value":"b"}],[{"value":"c"},{"value":"d"}]]`,
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	combined := result.Stdout + "\n" + result.Stderr
	if !strings.Contains(combined, "2 rows") {
		t.Fatalf("expected a cells-vs-range mismatch, got:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	}
}

// The +batch-update sub-op is the second entry point: translateBatchOp reaches
// the rewrite through the sub-op's own flag view rather than cobra's PreRunE,
// so a prefixed range has to fill the selector there too.
func TestSheets_BatchUpdateSheetPrefixDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+batch-update",
			"--spreadsheet-token", "shtDryRun",
			"--operations", `[
				{"shortcut":"+cells-set","input":{"range":"Sheet1!A1","cells":[[{"value":"x"}]]}},
				{"shortcut":"+cells-set","input":{"range":"'My Sheet'!B2","cells":[[{"value":"y"}]]}}
			]`,
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := clie2e.DryRunData(result.Stdout)
	require.Equal(t, "batch_update", gjson.Get(out, "api.0.body.tool_name").String(), "stdout:\n%s", out)

	input := gjson.Get(out, "api.0.body.input").String()
	require.Equal(t, "set_cell_range", gjson.Get(input, "operations.0.tool_name").String(), "input:\n%s", input)
	require.Equal(t, "Sheet1", gjson.Get(input, "operations.0.input.sheet_name").String(), "input:\n%s", input)
	require.Equal(t, "A1", gjson.Get(input, "operations.0.input.range").String(), "input:\n%s", input)
	require.Equal(t, "My Sheet", gjson.Get(input, "operations.1.input.sheet_name").String(), "input:\n%s", input)
	require.Equal(t, "B2", gjson.Get(input, "operations.1.input.range").String(), "input:\n%s", input)
}

// The --writes plural form builds its own per-item flag view, which makes it a
// third entry point for the prefix rewrite alongside the standalone command and
// the +batch-update sub-op. Each item resolves independently, so one batch can
// mix a plain name with a quoted one.
func TestSheets_CellsSetWritesSheetPrefixDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+cells-set",
			"--spreadsheet-token", "shtDryRun",
			"--writes", `[
				{"range":"Sheet1!A1","cells":[[{"value":"x"}]]},
				{"range":"'My Sheet'!B2","cells":[[{"value":"y"}]]}
			]`,
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := clie2e.DryRunData(result.Stdout)
	require.Equal(t, "POST", gjson.Get(out, "api.0.method").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_write",
		gjson.Get(out, "api.0.url").String(), "stdout:\n%s", out)
	require.Equal(t, "batch_update", gjson.Get(out, "api.0.body.tool_name").String(), "stdout:\n%s", out)

	input := gjson.Get(out, "api.0.body.input").String()
	require.Equal(t, "set_cell_range", gjson.Get(input, "operations.0.tool_name").String(), "input:\n%s", input)
	require.Equal(t, "Sheet1", gjson.Get(input, "operations.0.input.sheet_name").String(), "input:\n%s", input)
	require.Equal(t, "A1", gjson.Get(input, "operations.0.input.range").String(), "input:\n%s", input)
	require.Equal(t, "My Sheet", gjson.Get(input, "operations.1.input.sheet_name").String(), "input:\n%s", input)
	require.Equal(t, "B2", gjson.Get(input, "operations.1.input.range").String(), "input:\n%s", input)
}

// A --writes item with no sheet anywhere — not in a selector key, not as a
// range prefix — still fails, and the error names the flags that fix it.
func TestSheets_CellsSetWritesWithoutAnySheetRejectedDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+cells-set",
			"--spreadsheet-token", "shtDryRun",
			"--writes", `[{"range":"A1","cells":[[{"value":"x"}]]}]`,
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	combined := result.Stdout + "\n" + result.Stderr
	if !strings.Contains(combined, "sheet-id or --sheet-name") {
		t.Fatalf("expected a selector error, got:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	}
}
