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

func TestSheets_ResizeSizesDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	const payload = `[80,"120","standard"]`
	path := filepath.Join(t.TempDir(), "widths.json")
	require.NoError(t, os.WriteFile(path, []byte(payload), 0o600))
	for _, tc := range []struct{ name, flag, stdin string }{
		{"inline", payload, ""}, {"file", "@" + path, ""}, {"stdin", "-", payload},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{"sheets", "+cols-resize", "--spreadsheet-token", "shtDryRun", "--sheet-name", "Main",
					"--range", "Z:AB", "--widths", tc.flag, "--dry-run"},
				Stdin: []byte(tc.stdin), DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
			require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_write", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
			require.Equal(t, "batch_update", clie2e.DryRunGet(result.Stdout, "api.0.body.tool_name").String())
			input := clie2e.DryRunGet(result.Stdout, "api.0.body.input").String()
			ops := gjson.Get(input, "operations").Array()
			require.Len(t, ops, 3)
			for i, target := range []string{"Z:Z", "AA:AA", "AB:AB"} {
				require.Equal(t, "resize_range", ops[i].Get("tool_name").String())
				require.Equal(t, "Main", ops[i].Get("input.sheet_name").String())
				require.Equal(t, target, ops[i].Get("input.range").String())
			}
			require.Equal(t, int64(80), ops[0].Get("input.resize_width.value").Int())
			require.Equal(t, int64(120), ops[1].Get("input.resize_width.value").Int())
			require.Equal(t, "standard", ops[2].Get("input.resize_width.type").String())
			require.False(t, ops[2].Get("input.resize_width.value").Exists())
		})
	}
}

func TestSheets_RangeCopyAliasDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	for _, tc := range []struct {
		name, tool string
		args       []string
	}{
		{"standalone", "transform_range", []string{"+range-copy", "--source-sheet-id", "source", "--range", "B2:C4", "--copy-to-range", "G7", "--target-sheet-id", "other", "--paste-type", "formats"}},
		{"batch", "batch_update", []string{"+batch-update", "--operations", `[{"shortcut":"+range-copy","input":{"source_sheet_id":"source","range":"B2:C4","copy_to_range":"G7","target_sheet_id":"other","paste_type":"formats"}}]`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)
			args := append([]string{"sheets"}, tc.args...)
			args = append(args, "--spreadsheet-token", "shtDryRun", "--dry-run")
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "user"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
			require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_write", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
			require.Equal(t, tc.tool, clie2e.DryRunGet(result.Stdout, "api.0.body.tool_name").String())
			input := clie2e.DryRunGet(result.Stdout, "api.0.body.input").String()
			if tc.name == "batch" {
				require.Equal(t, "transform_range", gjson.Get(input, "operations.0.tool_name").String())
				input = gjson.Get(input, "operations.0.input").Raw
			}
			for key, want := range map[string]string{"operation": "copy", "sheet_id": "source", "range": "B2:C4", "destination_range": "G7", "destination_sheet_id": "other", "paste_type": "format_only"} {
				require.Equal(t, want, gjson.Get(input, key).String())
			}
		})
	}
}
