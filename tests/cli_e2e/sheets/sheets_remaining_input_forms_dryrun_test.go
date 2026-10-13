// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSheets_RemainingStyleFormsDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	const payload = `{"styles":[{"name":"S","cell_styles":[{"range":"B2","style":{"fontWeight":"bold","textAlign":"right","vertical":"center","fillColor":"#123456","borderStyle":"full","borderColor":"#654321"}}]}]}`
	path := filepath.Join(t.TempDir(), "styles.json")
	require.NoError(t, os.WriteFile(path, []byte(payload), 0o600))
	for _, tc := range []struct{ name, value, stdin string }{{"inline", payload, ""}, {"file", "@" + path, ""}, {"stdin", "-", payload}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)
			r, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"sheets", "+styles-put", "--spreadsheet-token", "shtDryRun", "--styles", tc.value, "--dry-run"}, Stdin: []byte(tc.stdin), DefaultAs: "user"})
			require.NoError(t, err)
			r.AssertExitCode(t, 0)
			require.Equal(t, "POST", clie2e.DryRunGet(r.Stdout, "api.0.method").String())
			require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_write", clie2e.DryRunGet(r.Stdout, "api.0.url").String())
			require.Equal(t, "batch_update", clie2e.DryRunGet(r.Stdout, "api.0.body.tool_name").String())
			input := clie2e.DryRunGet(r.Stdout, "api.0.body.input").String()
			require.Equal(t, "S", gjson.Get(input, "operations.0.input.sheet_name").String())
			require.Equal(t, "B2", gjson.Get(input, "operations.0.input.range").String())
			cell := gjson.Get(input, "operations.0.input.cells.0.0")
			require.False(t, cell.Get("value").Exists())
			require.False(t, cell.Get("formula").Exists())
			for key, want := range map[string]string{"font_weight": "bold", "horizontal_alignment": "right", "vertical_alignment": "middle", "background_color": "#123456"} {
				require.Equal(t, want, cell.Get("cell_styles."+key).String())
			}
			for _, side := range []string{"top", "bottom", "left", "right"} {
				require.Equal(t, "solid", cell.Get("border_styles."+side+".style").String())
				require.Equal(t, "#654321", cell.Get("border_styles."+side+".color").String())
			}
		})
	}
}

func TestSheets_RemainingParameterFormsDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	for _, tc := range []struct {
		name, tool string
		args       []string
		check      func(*testing.T, string)
	}{
		{"freeze complete state", "modify_sheet_structure", []string{"+dim-freeze", "--worksheet-name", "S", "--frozen-rows", "0", "--frozen-cols", "2"}, func(t *testing.T, input string) {
			require.Equal(t, "freeze", gjson.Get(input, "operation").String())
			require.Equal(t, int64(2), gjson.Get(input, "freeze_columns").Int())
			require.False(t, gjson.Get(input, "freeze_rows").Exists())
		}},
		{"selected border attributes", "set_cell_range", []string{"+cells-set-style", "--sheet-name", "S", "--range", "B2", "--border-type", "LEFT_BORDER", "--border-color", "#123456", "--border-weight", "medium"}, func(t *testing.T, input string) {
			border := gjson.Get(input, "cells.0.0.border_styles")
			require.Equal(t, "solid", border.Get("left.style").String())
			require.Equal(t, "#123456", border.Get("left.color").String())
			require.Equal(t, "medium", border.Get("left.weight").String())
			require.False(t, border.Get("right").Exists())
			require.False(t, gjson.Get(input, "cells.0.0.value").Exists())
		}},
		{"sort directions", "transform_range", []string{"+range-sort", "--sheet-name", "S", "--range", "A3:D8", "--has-header", "--sort-keys", `[{"column":"B","order":"asc"},{"column":"C","order":"desc"}]`}, func(t *testing.T, input string) {
			require.Equal(t, "A3:D8", gjson.Get(input, "range").String())
			require.Equal(t, true, gjson.Get(input, "has_header").Value())
			require.Equal(t, true, gjson.Get(input, "sort_conditions.0.ascending").Value())
			require.Equal(t, false, gjson.Get(input, "sort_conditions.1.ascending").Value())
		}},
		{"typed text and formula", "set_cell_range", []string{"+cells-set", "--sheet-name", "S", "--range", "B2:C2", "--cells", `[[{"type":"text","text":"literal"},{"type":"formula","text":"=SUM(1,2)"}]]`, "--allow-overwrite=false"}, func(t *testing.T, input string) {
			require.Equal(t, "literal", gjson.Get(input, "cells.0.0.value").Value())
			require.False(t, gjson.Get(input, "cells.0.0.formula").Exists())
			require.Equal(t, "=SUM(1,2)", gjson.Get(input, "cells.0.1.formula").Value())
			require.Equal(t, false, gjson.Get(input, "allow_overwrite").Value())
		}},
		{"dimension list alias", "batch_update", []string{"+cols-resize", "--sheet-name", "S", "--columns", `[{"column":"Z","width":123}]`}, func(t *testing.T, input string) {
			require.Equal(t, "resize_range", gjson.Get(input, "operations.0.tool_name").String())
			require.Equal(t, "Z:Z", gjson.Get(input, "operations.0.input.range").String())
			require.Equal(t, int64(123), gjson.Get(input, "operations.0.input.resize_width.value").Int())
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)
			args := append([]string{"sheets"}, tc.args...)
			args = append(args, "--spreadsheet-token", "shtDryRun", "--dry-run")
			r, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "user"})
			require.NoError(t, err)
			r.AssertExitCode(t, 0)
			require.Equal(t, "POST", clie2e.DryRunGet(r.Stdout, "api.0.method").String())
			require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_write", clie2e.DryRunGet(r.Stdout, "api.0.url").String())
			require.Equal(t, tc.tool, clie2e.DryRunGet(r.Stdout, "api.0.body.tool_name").String())
			tc.check(t, clie2e.DryRunGet(r.Stdout, "api.0.body.input").String())
		})
	}
}

func TestSheets_BatchFreezeInvalidMixedCountsDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	for _, axis := range []struct{ canonical, alias string }{{"rows", "frozen_cols"}, {"cols", "frozen_rows"}} {
		for _, invalid := range []string{`"2"`, `null`, `true`, `1.5`} {
			t.Run(axis.canonical+"/"+invalid, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				t.Cleanup(cancel)
				operations := `[{"shortcut":"+dim-freeze","input":{"sheet_name":"S","` + axis.canonical + `":` + invalid + `,"` + axis.alias + `":1}}]`
				result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"sheets", "+batch-update", "--spreadsheet-token", "shtDryRun", "--operations", operations, "--dry-run"}, DefaultAs: "user"})
				require.NoError(t, err)
				result.AssertExitCode(t, 2)
				require.Empty(t, result.Stdout)
				require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
				require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
				require.Equal(t, "--operations", gjson.Get(result.Stderr, "error.param").String())
			})
		}
	}
}

func TestSheets_TableNameAliasesKeepSelectorGuardDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	const payload = `[{"sheet_name":"Other","columns":["a"],"data":[["x"]]}]`
	path := filepath.Join(t.TempDir(), "table.json")
	require.NoError(t, os.WriteFile(path, []byte(payload), 0o600))
	for _, tc := range []struct{ value, stdin string }{{payload, ""}, {"@" + path, ""}, {"-", payload}} {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)
		r, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"sheets", "+table-put", "--spreadsheet-token", "shtDryRun", "--sheet-name", "Expected", "--sheets", tc.value, "--dry-run"}, Stdin: []byte(tc.stdin), DefaultAs: "user"})
		require.NoError(t, err)
		r.AssertExitCode(t, 2)
		require.Empty(t, r.Stdout)
		require.Equal(t, "validation", gjson.Get(r.Stderr, "error.type").String())
		require.Equal(t, "--sheet-name", gjson.Get(r.Stderr, "error.param").String())
	}
}
