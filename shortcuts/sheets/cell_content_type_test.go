// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCellContentTypePreservesValuesAndFormulaIntent(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	api := parseDryRunAPI(t, shortcutFromRegistry(t, "+cells-set"), []string{
		"--url", testURL, "--sheet-name", "S", "--range", "A1:G1", "--cells",
		`[[{"type":"string","value":"00123"},{"type":"text","value":"literal"},{"type":"number","value":0},{"type":"boolean","value":false},{"type":"formula","value":"=SUM(A2:A4)"},{"type":"formula","formula":"=1+2"},{"rich_text":[{"type":"text","text":"keep"}]}]]`,
	})
	body := api[0].(map[string]interface{})["body"].(map[string]interface{})
	require.Equal(t, "set_cell_range", body["tool_name"])
	input := body["input"].(string)
	for i := range 6 {
		require.False(t, gjson.Get(input, "cells.0").Array()[i].Get("type").Exists())
	}
	require.Equal(t, "00123", gjson.Get(input, "cells.0.0.value").Value())
	require.Equal(t, "literal", gjson.Get(input, "cells.0.1.value").Value())
	require.False(t, gjson.Get(input, "cells.0.1.formula").Exists())
	require.Equal(t, float64(0), gjson.Get(input, "cells.0.2.value").Value())
	require.Equal(t, false, gjson.Get(input, "cells.0.3.value").Value())
	require.Equal(t, "=SUM(A2:A4)", gjson.Get(input, "cells.0.4.formula").Value())
	require.False(t, gjson.Get(input, "cells.0.4.value").Exists())
	require.Equal(t, "=1+2", gjson.Get(input, "cells.0.5.formula").Value())
	require.Equal(t, "text", gjson.Get(input, "cells.0.6.rich_text.0.type").Value())
}

func TestTypedEqualsTextIsRejectedBeforeFormulaAutodetection(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, field := range []string{"text", "value"} {
		for _, kind := range []string{"text", "string"} {
			cell := `{"type":"` + kind + `","` + field + `":"=SUM(1,2)"}`
			for _, tc := range []struct {
				command, param string
				args           []string
			}{
				{"+cells-set", "--cells", []string{"--sheet-name", "S", "--range", "A1", "--cells", "[[" + cell + "]]"}},
				{"+cells-set", "--writes", []string{"--writes", `[{"sheet_name":"S","range":"A1","cells":[[` + cell + `]]}]`}},
				{"+cells-set", "--cells", []string{"--sheet-name", "S", "--cells", `[{"cell":"A1","type":"` + kind + `","` + field + `":"=SUM(1,2)"}]`}},
				{"+batch-update", "--operations", []string{"--operations", `[{"shortcut":"+cells-set","input":{"sheet_name":"S","range":"A1","cells":[[` + cell + `]]}}]`}},
			} {
				out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, tc.command), append([]string{"--url", testURL, "--dry-run"}, tc.args...))
				var validation *errs.ValidationError
				require.ErrorAs(t, err, &validation)
				require.Equal(t, tc.param, validation.Param)
				require.Empty(t, out, "literal text must not become a formula write")
			}
		}
	}
}

func TestCellContentTypeCarrierParity(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name, command, path, tool string
		args                      []string
	}{
		{"bare cell", "+cells-set", "cells.0.0", "set_cell_range", []string{"--sheet-name", "S", "--range", "A2", "--cells", `{"type":"formula","value":"=SUM(B2:B4)"}`}},
		{"writes", "+cells-set", "operations.0.input.cells.0.0", "batch_update", []string{"--writes", `[{"sheet_name":"S","range":"A2","cells":[[{"type":"formula","value":"=SUM(B2:B4)"}]]}]`}},
		{"sparse cells", "+cells-set", "operations.0.input.cells.0.0", "batch_update", []string{"--sheet-name", "S", "--cells", `[{"cell":"A2","type":"formula","value":"=SUM(B2:B4)"},{"cell":"Z900","type":"number","value":0}]`}},
		{"batch", "+batch-update", "operations.0.input.cells.0.0", "batch_update", []string{"--operations", `[{"shortcut":"+cells-set","input":{"sheet_name":"S","range":"A2","cells":[[{"type":"formula","value":"=SUM(B2:B4)"}]]}}]`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := parseDryRunAPI(t, shortcutFromRegistry(t, tc.command), append([]string{"--url", testURL}, tc.args...))
			body := api[0].(map[string]interface{})["body"].(map[string]interface{})
			require.Equal(t, tc.tool, body["tool_name"])
			input := body["input"].(string)
			cell := gjson.Get(input, tc.path)
			require.Equal(t, "=SUM(B2:B4)", cell.Get("formula").Value())
			require.False(t, cell.Get("value").Exists())
			require.False(t, cell.Get("type").Exists())
			if tc.name == "sparse cells" {
				require.Len(t, gjson.Get(input, "operations").Array(), 2)
				require.Equal(t, "A2", gjson.Get(input, "operations.0.input.range").String())
				require.Equal(t, "Z900", gjson.Get(input, "operations.1.input.range").String())
				require.Equal(t, float64(0), gjson.Get(input, "operations.1.input.cells.0.0.value").Value())
			}
		})
	}
}

func TestCellContentTypeRejectsConflictsAndUnknownTypes(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, payload := range []string{
		`[[{"type":"number","value":"007"}]]`,
		`[[{"type":"string","value":7}]]`,
		`[[{"type":"boolean","value":0}]]`,
		`[[{"type":"number","value":null}]]`,
		`[[{"type":1,"value":7}]]`,
		`[[{"type":"date","value":"2026-10-12"}]]`,
		`[[{"type":"formula","value":"SUM(A1:A2)"}]]`,
		`[[{"type":"formula","value":7}]]`,
		`[[{"type":"formula","value":"=1","formula":"=2"}]]`,
		`[[{"type":"string","value":"x","formula":"=2"}]]`,
		`[[{"type":"text","value":"x","rich_text":[{"type":"text","text":"y"}]}]]`,
		`[[{"type":"number","cell_styles":{"bold":true}}]]`,
		`[[{"type":"string","multiple_values":[["x"]]}]]`,
		`[{"cell":"A2","type":"number","value":"007"}]`,
	} {
		t.Run(payload, func(t *testing.T) {
			args := []string{"--url", testURL, "--sheet-name", "S", "--cells", payload, "--dry-run"}
			if payload[1] == '[' {
				args = append(args, "--range", "A2")
			}
			stdout, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-set"), args)
			var validation *errs.ValidationError
			require.ErrorAs(t, err, &validation)
			require.Equal(t, errs.SubtypeInvalidArgument, validation.Subtype)
			require.Equal(t, "--cells", validation.Param)
			require.Empty(t, stdout)
		})
	}
}
