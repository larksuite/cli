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

func TestSheets_AlignmentVocabularyDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	const payload = `[[{"value":"keep","cell_styles":{"alignment":{"horizontal":"right","vertical":"bottom","wrapText":false}}},{"value":0,"cell_styles":{"h_align":0.0,"v_align":0e0}},{"value":false,"cell_styles":{"h_align":1e0,"v_align":1.0}}]]`
	path := filepath.Join(t.TempDir(), "cells.json")
	require.NoError(t, os.WriteFile(path, []byte(payload), 0o600))
	for _, tc := range []struct{ name, flag, stdin string }{
		{"inline", payload, ""}, {"file", "@" + path, ""}, {"stdin", "-", payload},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{"sheets", "+cells-set", "--spreadsheet-token", "shtDryRun", "--sheet-name", "S",
					"--range", "A2:C2", "--cells", tc.flag, "--allow-overwrite=false", "--dry-run"},
				Stdin: []byte(tc.stdin), DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
			require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_write", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
			require.Equal(t, "set_cell_range", clie2e.DryRunGet(result.Stdout, "api.0.body.tool_name").String())
			input := clie2e.DryRunGet(result.Stdout, "api.0.body.input").String()
			require.Equal(t, "S", gjson.Get(input, "sheet_name").String())
			require.Equal(t, "A2:C2", gjson.Get(input, "range").String())
			require.Equal(t, false, gjson.Get(input, "allow_overwrite").Value())
			cells := gjson.Get(input, "cells.0").Array()
			require.Len(t, cells, 3)
			for i, want := range []struct{ horizontal, vertical string }{{"right", "bottom"}, {"left", "top"}, {"center", "middle"}} {
				require.Equal(t, want.horizontal, cells[i].Get("cell_styles.horizontal_alignment").Value())
				require.Equal(t, want.vertical, cells[i].Get("cell_styles.vertical_alignment").Value())
				require.False(t, cells[i].Get("cell_styles.alignment").Exists())
				require.False(t, cells[i].Get("cell_styles.h_align").Exists())
				require.False(t, cells[i].Get("cell_styles.v_align").Exists())
			}
			require.Equal(t, "overflow", cells[0].Get("cell_styles.word_wrap").Value())
			require.Equal(t, "keep", cells[0].Get("value").Value())
			require.Equal(t, float64(0), cells[1].Get("value").Value())
			require.Equal(t, false, cells[2].Get("value").Value())
		})
	}
}

func TestSheets_AlignmentExplicitPrecedenceDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	for _, tc := range []struct {
		name, path string
		args       []string
	}{
		{"standalone", "cells.0.0", []string{"+cells-set-style", "--sheet-name", "S", "--range", "A1", "--horizontal-alignment", "center", "--style", `{"Alignment":{"horizontal":null,"vertical":"top"}}`}},
		{"batch", "operations.0.input.cells.0.0", []string{"+batch-update", "--operations", `[{"shortcut":"+cells-set-style","input":{"sheet_name":"S","range":"A1","horizontal_alignment":"center","style":{"ALIGNMENT":{"horizontal":null,"vertical":"top"}}}}]`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)
			args := append([]string{"sheets"}, tc.args...)
			args = append(args, "--spreadsheet-token", "shtDryRun", "--dry-run")
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "user"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			input := clie2e.DryRunGet(result.Stdout, "api.0.body.input").String()
			require.Equal(t, "center", gjson.Get(input, tc.path+".cell_styles.horizontal_alignment").String())
			require.Equal(t, "top", gjson.Get(input, tc.path+".cell_styles.vertical_alignment").String())
		})
	}
}

func TestSheets_NumericAlignmentRejectsRoundingDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	for _, number := range []string{"1.0000000000000000001", "2.0000000000000000001", "-1e-400"} {
		for _, command := range []string{"+cells-set", "+cells-set-style"} {
			t.Run(command+"/"+number, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				t.Cleanup(cancel)
				flag, payload := "--cells", `[[{"value":"keep","cell_styles":{"hAlign":`+number+`}}]]`
				if command == "+cells-set-style" {
					flag, payload = "--style", `{"hAlign":`+number+`}`
				}
				result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"sheets", command, "--spreadsheet-token", "shtDryRun", "--sheet-name", "S", "--range", "A1", flag, payload, "--dry-run"}, DefaultAs: "user"})
				require.NoError(t, err)
				result.AssertExitCode(t, 2)
				require.Empty(t, result.Stdout)
				require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
				require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
				require.Equal(t, flag, gjson.Get(result.Stderr, "error.param").String())
			})
		}
	}
}
