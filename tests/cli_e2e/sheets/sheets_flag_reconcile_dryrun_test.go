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

func TestSheets_FlagReconciliationDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	for _, tc := range []struct {
		name, command, tool string
		args                []string
		want                map[string]any
		param               string
	}{
		{"zero-based insertion", "+dim-insert", "modify_sheet_structure", []string{"--start-index", "3", "--end-index", "7"}, map[string]any{"position": "4", "count": float64(4)}, ""},
		{"first-row insertion", "+dim-insert", "modify_sheet_structure", []string{"--start-index", "0", "--end-index", "1"}, map[string]any{"position": "1", "count": float64(1)}, ""},
		{"batch insertion index", "+batch-update", "batch_update", []string{"--operations", `[{"shortcut":"+dim-insert","input":{"sheet_name":"S","start_index":3,"count":2}}]`}, map[string]any{"operations.0.input.position": "4", "operations.0.input.count": float64(2)}, ""},
		{"batch insertion end index", "+batch-update", "batch_update", []string{"--operations", `[{"shortcut":"+dim-insert","input":{"sheet_name":"S","start_index":3,"end_index":7}}]`}, map[string]any{"operations.0.input.position": "4", "operations.0.input.count": float64(4)}, ""},
		{"batch formula", "+batch-update", "batch_update", []string{"--operations", `[{"shortcut":"+cells-set","input":{"sheet_name":"S","range":"A1","formula":"=SUM(1,2)"}}]`}, map[string]any{"operations.0.input.cells.0.0.formula": "=SUM(1,2)"}, ""},
		{"batch deletion index and count", "+batch-update", "batch_update", []string{"--operations", `[{"shortcut":"+dim-delete","input":{"sheet_name":"S","start_index":3,"count":2}}]`}, map[string]any{"operations.0.input.range": "3:4"}, ""},
		{"batch deletion camel-case end index", "+batch-update", "batch_update", []string{"--operations", `[{"shortcut":"+dim-delete","input":{"sheet_name":"S","start_index":3,"endIndex":7}}]`}, map[string]any{"operations.0.input.range": "3:7", "operations.0.input.operation": "delete"}, ""},
		{"batch deletion invalid camel-case end index", "+batch-update", "", []string{"--operations", `[{"shortcut":"+dim-delete","input":{"sheet_name":"S","start_index":3,"endIndex":2}}]`}, nil, "--operations"},
		{"inclusive deletion", "+dim-delete", "modify_sheet_structure", []string{"--start-index", "3", "--end-index", "7"}, map[string]any{"range": "3:7"}, ""},
		{"single-row deletion", "+dim-delete", "modify_sheet_structure", []string{"--start-index", "3", "--end-index", "3"}, map[string]any{"range": "3:3"}, ""},
		{"deletion count agrees", "+dim-delete", "modify_sheet_structure", []string{"--start-index", "6", "--end-index", "22", "--count", "17"}, map[string]any{"range": "6:22"}, ""},
		{"bold false", "+cells-set-style", "set_cell_range", []string{"--range", "A1", "--bold=false"}, map[string]any{"cells.0.0.cell_styles.font_weight": "normal"}, ""},
		{"font-bold false", "+cells-set-style", "set_cell_range", []string{"--range", "A1", "--font-bold=false"}, map[string]any{"cells.0.0.cell_styles.font_weight": "normal"}, ""},
		{"style envelope false", "+cells-set-style", "set_cell_range", []string{"--range", "A1", "--style", `{"bold":false}`}, map[string]any{"cells.0.0.cell_styles.font_weight": "normal"}, ""},
		{"nested font and border envelope", "+cells-set-style", "set_cell_range", []string{"--range", "A1", "--style", `{"font":{"bold":false,"size":"14","color":"#123456"},"h_align":"center","v_align":"center","wrap_text":true,"border":{"bottom":{"style":"dashed"}}}`}, map[string]any{"cells.0.0.cell_styles.font_weight": "normal", "cells.0.0.cell_styles.font_size": float64(14), "cells.0.0.cell_styles.font_color": "#123456", "cells.0.0.cell_styles.horizontal_alignment": "center", "cells.0.0.cell_styles.vertical_alignment": "middle", "cells.0.0.cell_styles.word_wrap": "auto-wrap", "cells.0.0.border_styles.bottom.style": "dashed"}, ""},
		{"nested font in typed cells", "+cells-set", "set_cell_range", []string{"--range", "A1", "--cells", `[[{"value":"keep","cell_styles":{"font":{"bold":true,"size":14},"border":{"bottom":{"style":"dashed"}}}}]]`}, map[string]any{"cells.0.0.value": "keep", "cells.0.0.cell_styles.font_weight": "bold", "cells.0.0.cell_styles.font_size": float64(14), "cells.0.0.border_styles.bottom.style": "dashed"}, ""},
		{"batch style keeps numeric font size", "+batch-update", "batch_update", []string{"--operations", `[{"shortcut":"+cells-set-style","input":{"sheet_name":"S","range":"A1","style":{"font":{"bold":true,"size":14},"border_styles":{"bottom":{"style":"dashed"}}}}}]`}, map[string]any{"operations.0.input.cells.0.0.cell_styles.font_weight": "bold", "operations.0.input.cells.0.0.cell_styles.font_size": float64(14), "operations.0.input.cells.0.0.border_styles.bottom.style": "dashed"}, ""},
		{"unsupported font field", "+cells-set-style", "", []string{"--range", "A1", "--styles", `{"font":{"bold":true,"shadow":true}}`}, nil, "--styles"},
		{"nested font conflicts with flat font weight", "+cells-set-style", "", []string{"--range", "A1", "--style", `{"font":{"bold":true},"font_weight":"normal"}`}, nil, "--style"},
		{"unsupported batch font field", "+batch-update", "", []string{"--operations", `[{"shortcut":"+cells-set-style","input":{"sheet_name":"S","range":"A1","style":{"font":{"bold":true,"shadow":true}}}}]`}, nil, "--operations"},
		{"plural style envelope", "+cells-set-style", "set_cell_range", []string{"--range", "A1", "--styles", `{"font_weight":"bold"}`}, map[string]any{"cells.0.0.cell_styles.font_weight": "bold"}, ""},
		{"explicit bold outranks envelope", "+cells-set-style", "set_cell_range", []string{"--range", "A1", "--style", `{"font_weight":"bold"}`, "--bold=false"}, map[string]any{"cells.0.0.cell_styles.font_weight": "normal"}, ""},
		{"explicit font-bold outranks envelope", "+cells-set-style", "set_cell_range", []string{"--range", "A1", "--style", `{"font_weight":"bold"}`, "--font-bold=false"}, map[string]any{"cells.0.0.cell_styles.font_weight": "normal"}, ""},
		{"batch style explicit bold outranks envelope", "+cells-batch-set-style", "batch_update", []string{"--ranges", `["S!A1"]`, "--style", `{"font_weight":"bold"}`, "--bold=false"}, map[string]any{"operations.0.input.cells.0.0.cell_styles.font_weight": "normal"}, ""},
		{"batch style explicit font-bold outranks envelope", "+cells-batch-set-style", "batch_update", []string{"--ranges", `["S!A1"]`, "--style", `{"font_weight":"bold"}`, "--font-bold=false"}, map[string]any{"operations.0.input.cells.0.0.cell_styles.font_weight": "normal"}, ""},
		{"agreeing formula inputs", "+cells-set", "set_cell_range", []string{"--range", "A1", "--cells", `[[{"formula":"=SUM(1,2)"}]]`, "--formula", "=SUM(1,2)", "--formulas", "=SUM(1,2)"}, map[string]any{"cells.0.0.formula": "=SUM(1,2)"}, ""},
		{"formula conflicts with cells", "+cells-set", "", []string{"--range", "A1", "--cells", `[[{"value":1}]]`, "--formula", "=SUM(B1:B2)"}, nil, "--formula"},
		{"formula aliases conflict", "+cells-set", "", []string{"--range", "A1", "--formula", "=1", "--formulas", "=2"}, nil, "--formulas"},
		{"dimension conflict", "+dim-insert", "", []string{"--position", "3", "--count", "1", "--dimension", "column"}, nil, "--dimension"},
		{"reversed insertion range", "+dim-insert", "", []string{"--range", "6:3", "--count", "2"}, nil, "--range"},
		{"invalid insertion range end", "+dim-insert", "", []string{"--range", "3:garbage", "--count", "1"}, nil, "--range"},
		{"non-singleton insertion range", "+dim-insert", "", []string{"--range", "3:6", "--count", "1"}, nil, "--range"},
		{"insertion count conflicts", "+dim-insert", "", []string{"--start-index", "3", "--end-index", "7", "--count", "1"}, nil, "--end-index"},
		{"batch insertion count conflicts", "+batch-update", "", []string{"--operations", `[{"shortcut":"+dim-insert","input":{"sheet_name":"S","start_index":3,"end_index":7,"count":1}}]`}, nil, "--operations"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)
			args := []string{"sheets", tc.command, "--spreadsheet-token", "shtDryRun", "--dry-run"}
			if tc.command != "+batch-update" && tc.command != "+cells-batch-set-style" {
				args = append(args, "--sheet-name", "S")
			}
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      append(args, tc.args...),
				DefaultAs: "user",
			})
			require.NoError(t, err)
			if tc.param != "" {
				result.AssertExitCode(t, 2)
				require.Empty(t, result.Stdout)
				require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
				require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
				require.Equal(t, tc.param, gjson.Get(result.Stderr, "error.param").String())
				return
			}
			result.AssertExitCode(t, 0)
			require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
			require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_write", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
			require.Equal(t, tc.tool, clie2e.DryRunGet(result.Stdout, "api.0.body.tool_name").String())
			input := clie2e.DryRunGet(result.Stdout, "api.0.body.input").String()
			selector := "sheet_name"
			if tc.command == "+batch-update" || tc.command == "+cells-batch-set-style" {
				selector = "operations.0.input.sheet_name"
			}
			require.Equal(t, "S", gjson.Get(input, selector).String())
			for path, want := range tc.want {
				require.Equal(t, want, gjson.Get(input, path).Value(), "input: %s", input)
			}
		})
	}
}
