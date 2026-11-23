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

// TestSheets_CellsSetHabitualShapesDryRun pins the --cells shapes that are now
// accepted verbatim: every spelling below reaches the tool as the one canonical
// matrix, so the rewrite happens before the request is built rather than being
// prescribed back to the caller.
func TestSheets_CellsSetHabitualShapesDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)

	tests := []struct {
		name string
		args []string
	}{
		{
			// openpyxl / gspread write plain values; the scalar is lifted into
			// the {"value":…} the cell contract expects.
			name: "bare scalar matrix",
			args: []string{"--range", "A1", "--cells", `[["x"]]`},
		},
		{
			// `json.dump({"cells": cells}, f)` inside a payload-generating
			// script: the flag name leaks in as a JSON key.
			name: "cells envelope",
			args: []string{"--range", "A1", "--cells", `{"cells":[[{"value":"x"}]]}`},
		},
		{
			name: "lone cell object without the 2D wrapper",
			args: []string{"--range", "A1", "--cells", `{"value":"x"}`},
		},
		{
			// gspread spells the payload "values", and this CLI's own
			// +workbook-create uses --values for untyped data.
			name: "values alias",
			args: []string{"--range", "A1", "--values", `[["x"]]`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			args := append([]string{
				"sheets", "+cells-set",
				"--spreadsheet-token", "shtDryRun",
				"--sheet-name", "Sheet1",
			}, tt.args...)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      append(args, "--dry-run"),
				DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := clie2e.DryRunData(result.Stdout)
			require.Equal(t, "POST", gjson.Get(out, "api.0.method").String(), "stdout:\n%s", out)
			require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_write",
				gjson.Get(out, "api.0.url").String(), "stdout:\n%s", out)
			require.Equal(t, "set_cell_range", gjson.Get(out, "api.0.body.tool_name").String(), "stdout:\n%s", out)

			input := gjson.Get(out, "api.0.body.input").String()
			require.Equal(t, "shtDryRun", gjson.Get(input, "excel_id").String(), "input:\n%s", input)
			require.Equal(t, "Sheet1", gjson.Get(input, "sheet_name").String(), "input:\n%s", input)
			require.Equal(t, "A1", gjson.Get(input, "range").String(), "input:\n%s", input)
			require.Equal(t, `[[{"value":"x"}]]`, gjson.Get(input, "cells").Raw, "input:\n%s", input)
		})
	}
}

// A row may mix plain values with typed cells — the shape a caller produces the
// moment one formula shows up in an otherwise plain matrix. Only the scalars
// are lifted; the typed cell rides through untouched.
func TestSheets_CellsSetMixedScalarAndTypedRowDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+cells-set",
			"--spreadsheet-token", "shtDryRun",
			"--sheet-name", "Sheet1",
			"--range", "A1:C1",
			"--cells", `[["名称",10331.5,{"formula":"=D2*E2"}]]`,
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	input := gjson.Get(clie2e.DryRunData(result.Stdout), "api.0.body.input").String()
	require.Equal(t, `[[{"value":"名称"},{"value":10331.5},{"formula":"=D2*E2"}]]`,
		gjson.Get(input, "cells").Raw, "input:\n%s", input)
}

// A --writes item gets the same rewrites as the standalone flag and the
// +batch-update sub-op. The rewrite runs before the writes array is
// schema-validated, which is what it takes for the array's own "cells is
// required" check to see the payload under the name it expects.
func TestSheets_CellsSetWritesHabitualShapesDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)

	tests := []struct {
		name  string
		items string
	}{
		{"values alias", `[{"sheet_name":"S1","range":"A1","values":[["x"]]}]`},
		{"cells envelope", `[{"sheet_name":"S1","range":"A1","cells":{"cells":[[{"value":"x"}]]}}]`},
		{"bare scalar matrix", `[{"sheet_name":"S1","range":"A1","cells":[["x"]]}]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"sheets", "+cells-set",
					"--spreadsheet-token", "shtDryRun",
					"--writes", tt.items,
					"--dry-run",
				},
				DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			input := gjson.Get(clie2e.DryRunData(result.Stdout), "api.0.body.input").String()
			require.Equal(t, "S1", gjson.Get(input, "operations.0.input.sheet_name").String(), "input:\n%s", input)
			require.Equal(t, `[[{"value":"x"}]]`,
				gjson.Get(input, "operations.0.input.cells").Raw, "input:\n%s", input)
		})
	}
}

// null is deliberately NOT lifted: {} (leave the cell alone) and {"value":""}
// (write an empty string) are both plausible readings, so the caller is asked
// which one they meant instead of having one guessed for them.
func TestSheets_CellsSetNullCellRejectedDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+cells-set",
			"--spreadsheet-token", "shtDryRun",
			"--sheet-name", "Sheet1",
			"--range", "A1:B1",
			"--cells", `[["x",null]]`,
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	combined := result.Stdout + "\n" + result.Stderr
	if !strings.Contains(combined, `got \"null\"`) && !strings.Contains(combined, `got "null"`) {
		t.Fatalf("expected a null-cell prescription, got:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	}
}
