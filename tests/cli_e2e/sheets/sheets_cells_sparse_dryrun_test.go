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

func TestSheets_CellsSetSparseDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	payload := `[{"cell":"A2","value":"00123"},{"range":"'Other sheet'!C9:D9","values":[[{"type":"number","value":9007199254740993},{"formula":"=SUM(A1:A3)"}]]}]`
	path := filepath.Join(t.TempDir(), "cells.json")
	require.NoError(t, os.WriteFile(path, []byte(payload), 0o600))
	for _, tc := range []struct{ name, flag, stdin string }{
		{"inline", payload, ""}, {"file", "@" + path, ""}, {"stdin", "-", payload},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:  []string{"sheets", "+cells-set", "--spreadsheet-token", "shtDryRun", "--sheet-name", "Main", "--cells", tc.flag, "--allow-overwrite=false", "--dry-run"},
				Stdin: []byte(tc.stdin), DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			out := clie2e.DryRunData(result.Stdout)
			require.Equal(t, "POST", gjson.Get(out, "api.0.method").String())
			require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_write", gjson.Get(out, "api.0.url").String())
			require.Equal(t, "batch_update", gjson.Get(out, "api.0.body.tool_name").String())
			input := gjson.Get(out, "api.0.body.input").String()
			ops := gjson.Get(input, "operations").Array()
			require.Len(t, ops, 2)
			require.Equal(t, "A2", ops[0].Get("input.range").String())
			require.Equal(t, "Main", ops[0].Get("input.sheet_name").String())
			require.Equal(t, "00123", ops[0].Get("input.cells.0.0.value").String())
			require.Equal(t, "C9:D9", ops[1].Get("input.range").String())
			require.Equal(t, "Other sheet", ops[1].Get("input.sheet_name").String())
			require.Equal(t, gjson.Number, ops[1].Get("input.cells.0.0.value").Type)
			require.Equal(t, "9007199254740993", ops[1].Get("input.cells.0.0.value").Raw)
			require.False(t, ops[1].Get("input.cells.0.0.type").Exists())
			require.Equal(t, "=SUM(A1:A3)", ops[1].Get("input.cells.0.1.formula").String())
			for _, op := range ops {
				require.Equal(t, "set_cell_range", op.Get("tool_name").String())
				require.False(t, op.Get("input.allow_overwrite").Bool())
			}
		})
	}
}
