// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRemainingStyleFormsPreserveStamps(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	style := `{"fontWeight":"bold","textAlign":"right","verticalAlign":"center","fillColor":"#123456","borderColor":"#654321","borderStyle":"thin"}`
	for _, tc := range []struct {
		command, cell string
		args          []string
	}{
		{"+styles-put", "operations.0.input.cells.0.0", []string{"--styles", `{"styles":[{"name":"S","cell_styles":[{"range":"B2","style":` + style + `}]}]}`}},
		{"+cells-set", "cells.0.0", []string{"--sheet-name", "S", "--range", "B2", "--cells", `[[{"value":"00123","cell_styles":` + style + `}]]`}},
		{"+cells-set-style", "cells.0.0", []string{"--sheet-name", "S", "--range", "B2", "--style", style}},
		{"+batch-update", "operations.0.input.cells.0.0", []string{"--operations", `[{"shortcut":"+cells-set","input":{"sheet_name":"S","range":"B2","cells":[[{"value":"00123","cell_styles":` + style + `}]]}}]`}},
	} {
		t.Run(tc.command, func(t *testing.T) {
			calls := parseDryRunAPI(t, shortcutFromRegistry(t, tc.command), append([]string{"--url", testURL}, tc.args...))
			input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
			cell := gjson.Get(input, tc.cell)
			for key, want := range map[string]string{"font_weight": "bold", "horizontal_alignment": "right", "vertical_alignment": "middle", "background_color": "#123456"} {
				require.Equal(t, want, cell.Get("cell_styles."+key).String(), input)
			}
			for _, side := range []string{"top", "bottom", "left", "right"} {
				require.Equal(t, "solid", cell.Get("border_styles."+side+".style").String(), input)
				require.Equal(t, "thin", cell.Get("border_styles."+side+".weight").String(), input)
				require.Equal(t, "#654321", cell.Get("border_styles."+side+".color").String(), input)
			}
			require.False(t, cell.Get("cell_styles.style").Exists())
			if tc.command == "+cells-set" || tc.command == "+batch-update" {
				require.Equal(t, "00123", cell.Get("value").Value())
			} else {
				require.False(t, cell.Get("value").Exists())
			}
		})
	}
}

func TestRemainingStyleFormsRejectConflictsAndContent(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, fields := range []string{
		`"style":null`, `"style":[]`, `"style":{}`, `"style":{"style":{"font_weight":"bold"}}`,
		`"style":{"range":"C2","font_weight":"bold"}`, `"style":{"value":"overwrite","font_weight":"bold"}`,
		`"style":{"font_weight":"bold"},"font_weight":"normal"`,
		`"style":{"fontWeight":"bold"},"font_weight":"normal"`,
		`"style":{"text_align":"right"},"horizontal_alignment":"left"`,
		`"style":{"borderColor":"#123456"},"border_color":"#654321"`,
		`"textAlign":true`, `"fillColor":true`, `"style":{"shadow":true}`,
	} {
		t.Run(fields, func(t *testing.T) {
			out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+styles-put"), []string{"--url", testURL, "--styles", `{"styles":[{"name":"S","cell_styles":[{"range":"B2",` + fields + `}]}]}`, "--dry-run"})
			var validation *errs.ValidationError
			require.ErrorAs(t, err, &validation)
			require.Equal(t, errs.SubtypeInvalidArgument, validation.Subtype)
			require.Empty(t, out)
		})
	}
}

func TestTypedCellsKeepExistingStructuredForeignStyles(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+cells-set"), []string{"--url", testURL, "--sheet-name", "S", "--range", "A1", "--cells", `[[{"value":"keep","cell_styles":{"fontColor":{"red":0.12,"green":0.31,"blue":0.47},"textColor":{"a":1,"r":255,"g":255,"b":255}}}]]`})
	input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
	style := gjson.Get(input, "cells.0.0.cell_styles")
	require.Equal(t, float64(0.12), style.Get("fontColor.red").Value())
	require.Equal(t, float64(255), style.Get("textColor.r").Value())
	require.False(t, style.Get("font_color").Exists())
	require.Equal(t, "keep", gjson.Get(input, "cells.0.0.value").Value())
}

func TestTableSheetNameAliasPreservesTypedData(t *testing.T) {
	for _, command := range []string{"+table-put", "+workbook-create"} {
		fv := newMapFlagViewForCommand(command, map[string]interface{}{"sheets": `{"sheets":[{"sheet_name":"S","start_cell":"C7","header":false,"allow_overwrite":false,"columns":["id","n","b"],"dtypes":{"n":"int64","b":"bool"},"data":[["00123",9007199254740993,false]]}]}`})
		p, err := parseTablePutPayload(fv)
		require.NoError(t, err)
		require.Equal(t, "S", p.Sheets[0].Name)
		require.Equal(t, "C7", p.Sheets[0].StartCell)
		require.Equal(t, false, *p.Sheets[0].Header)
		require.Equal(t, false, *p.Sheets[0].AllowOverwrite)
		require.Equal(t, []interface{}{"00123", json.Number("9007199254740993"), false}, p.Sheets[0].Rows[0])
	}
	for _, names := range []string{`"name":"S","sheet_name":"T"`, `"name":"","sheet_name":"S"`, `"sheet_name":null`, `"sheet_name":7`, `"sheet_name":""`} {
		_, err := parseTablePutPayload(newMapFlagViewForCommand("+workbook-create", map[string]interface{}{"sheets": `{"sheets":[{` + names + `,"columns":["a"],"data":[["x"]]}]}`}))
		var validation *errs.ValidationError
		require.ErrorAs(t, err, &validation, names)
	}
}

func TestSortDirectionFormsPreserveOrderAndHeader(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	keys := `[{"column":"B","order":"ASC"},{"column":"C","order":"descending","ascending":false}]`
	for _, command := range []string{"+range-sort", "+batch-update"} {
		args := []string{"--url", testURL}
		path := "sort_conditions"
		if command == "+range-sort" {
			args = append(args, "--sheet-name", "S", "--range", "A3:D8", "--has-header", "--sort-keys", keys)
		} else {
			args = append(args, "--operations", `[{"shortcut":"+range-sort","input":{"sheet_name":"S","range":"A3:D8","has_header":true,"sort_keys":`+keys+`}}]`)
			path = "operations.0.input.sort_conditions"
		}
		calls := parseDryRunAPI(t, shortcutFromRegistry(t, command), args)
		input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
		got := gjson.Get(input, path).Array()
		require.Len(t, got, 2)
		require.Equal(t, "B", got[0].Get("column").String())
		require.Equal(t, true, got[0].Get("ascending").Value())
		require.Equal(t, "C", got[1].Get("column").String())
		require.Equal(t, false, got[1].Get("ascending").Value())
		require.False(t, got[0].Get("order").Exists())
	}
	for _, key := range []string{`{"column":"B","order":"asc","ascending":false}`, `{"column":"B","order":"random","ascending":true}`, `{"column":"B","order":7,"ascending":true}`} {
		out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+range-sort"), []string{"--url", testURL, "--sheet-name", "S", "--range", "A3:D8", "--sort-keys", "[" + key + "]", "--dry-run"})
		var validation *errs.ValidationError
		require.ErrorAs(t, err, &validation)
		require.Equal(t, "--sort-keys", validation.Param)
		require.Empty(t, out)
	}
}

func TestSingletonConditionalThreshold(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, value := range []string{`[4]`, `["4"]`, `[false]`} {
		calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+cond-format-create"), []string{"--url", testURL, "--sheet-name", "S", "--ranges", `["B2"]`, "--rule-type", "cellIs", "--properties", `{"style":{"back_color":"#FF0000"},"attrs":[{"operator":"equal","value":` + value + `}]}`})
		input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
		want := "4"
		if value == `[false]` {
			want = "false"
		}
		require.Equal(t, want, gjson.Get(input, "properties.attrs.0.value").String(), input)
	}
}

func TestDeclaredAliasesMatchCanonicalBodies(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		command          string
		args             []string
		alias, canonical string
	}{
		{"+cols-resize", []string{"--sheet-name", "S", "--columns", `[{"column":"B","width":123}]`}, "columns", "widths"},
		{"+rows-resize", []string{"--sheet-name", "S", "--range", "7:8", "--sizes", `[44,52]`}, "sizes", "heights"},
		{"+sheet-copy", []string{"--sheet-name", "S", "--new-sheet-name", "new S"}, "new-sheet-name", "title"},
		{"+csv-get", []string{"--worksheet-id", "sheetId", "--range", "B2"}, "worksheet-id", "sheet-id"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			alias := append([]string{"--url", testURL}, tc.args...)
			canonical := append([]string(nil), alias...)
			for i, arg := range canonical {
				if arg == "--"+tc.alias {
					canonical[i] = "--" + tc.canonical
				}
			}
			sc := shortcutFromRegistry(t, tc.command)
			require.Equal(t, parseDryRunAPI(t, sc, canonical), parseDryRunAPI(t, sc, alias))
			var found bool
			for _, flag := range sc.Flags {
				if flag.Name == tc.canonical {
					require.Contains(t, flag.Aliases, tc.alias)
					found = true
				}
			}
			require.True(t, found)
		})
	}
}

func TestTypedTextCarriersRetainLiteralAndFormulaIntent(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	payload := `[[{"type":"text","text":"00123"},{"type":"text","text":"literal"},{"type":"formula","text":"=SUM(1,2)"},{"type":"string","text":""}]]`
	calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+cells-set"), []string{"--url", testURL, "--sheet-name", "S", "--range", "B2:E2", "--cells", payload, "--allow-overwrite=false"})
	input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
	require.Equal(t, "00123", gjson.Get(input, "cells.0.0.value").Value())
	require.Equal(t, "literal", gjson.Get(input, "cells.0.1.value").Value())
	require.False(t, gjson.Get(input, "cells.0.1.formula").Exists())
	require.Equal(t, "=SUM(1,2)", gjson.Get(input, "cells.0.2.formula").Value())
	require.Equal(t, "", gjson.Get(input, "cells.0.3.value").Value())
	require.Equal(t, false, gjson.Get(input, "allow_overwrite").Value())
	for _, content := range []string{`{"type":"text","text":"x","value":"y"}`, `{"type":"text","text":7}`, `{"type":"formula","text":"plain"}`} {
		out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-set"), []string{"--url", testURL, "--sheet-name", "S", "--range", "B2", "--cells", "[[" + content + "]]", "--dry-run"})
		var validation *errs.ValidationError
		require.ErrorAs(t, err, &validation)
		require.Empty(t, out)
	}
}

func TestSingleTargetPayloadsUseOnlyExplicitNames(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, payload := range []string{
		`{"styles":[{"cell_styles":[{"range":"B2","font_weight":"bold"}]}]}`,
		`{"cell_styles":[{"range":"B2","font_weight":"bold"}]}`,
	} {
		calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+styles-put"), []string{"--url", testURL, "--sheet-name", "Explicit", "--styles", payload})
		input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
		require.Equal(t, "Explicit", gjson.Get(input, "operations.0.input.sheet_name").String())
	}
	p, err := parseTablePutPayload(newMapFlagViewForCommand("+table-put", map[string]interface{}{"sheet-name": "Explicit", "sheets": `{"columns":["a"],"data":[["x"]]}`}))
	// A table still needs its own sheet envelope; it is not a cell write.
	require.Error(t, err)
	require.Nil(t, p)
	p, err = parseTablePutPayload(newMapFlagViewForCommand("+table-put", map[string]interface{}{"sheet-name": "Explicit", "sheets": `[{"columns":["a"],"data":[["x"]]}]`}))
	require.NoError(t, err)
	require.Equal(t, "Explicit", p.Sheets[0].Name)
	for _, payload := range []string{`{"styles":[{"name":"Other","cell_styles":[{"range":"B2","font_weight":"bold"}]}]}`, `{"styles":[{"name":"","cell_styles":[{"range":"B2","font_weight":"bold"}]}]}`, `{"styles":[{"cell_styles":[{"range":"B2","font_weight":"bold"}]},{"cell_styles":[{"range":"C2","font_weight":"bold"}]}]}`} {
		out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+styles-put"), []string{"--url", testURL, "--sheet-name", "Explicit", "--styles", payload, "--dry-run"})
		var validation *errs.ValidationError
		require.ErrorAs(t, err, &validation)
		require.Empty(t, out)
	}
	for _, payload := range []string{`[{"sheet_name":"Other","columns":["a"],"data":[["x"]]}]`, `[{"name":"","columns":["a"],"data":[["x"]]}]`, `[{"columns":["a"],"data":[["x"]]},{"columns":["a"],"data":[["y"]]}]`} {
		out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+table-put"), []string{"--url", testURL, "--sheet-name", "Explicit", "--sheets", payload, "--dry-run"})
		var validation *errs.ValidationError
		require.ErrorAs(t, err, &validation)
		require.Empty(t, out)
	}
	for _, payload := range []string{`[{"sheet_name":"Explicit","range":"C7","values":[["keep"]]}]`, `[{"range":"C7","values":[["keep"]]}]`, `[{"columns":["a"],"data":[["keep"]],"range":"C7"}]`, `[{"sheet_name":"Explicit","columns":["a"],"data":[["keep"]],"foo":true}]`} {
		out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+table-put"), []string{"--url", testURL, "--sheet-name", "Explicit", "--sheets", payload, "--dry-run"})
		var validation *errs.ValidationError
		require.ErrorAs(t, err, &validation)
		require.Empty(t, out)
	}
}

func TestFreezeCountFormsRequireBothAxes(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, args := range [][]string{
		{"--frozen-rows", "0", "--frozen-cols", "2"},
		{"--frozen-row-count", "0", "--cols", "2"},
		{"--rows", "0", "--col-count", "2"},
	} {
		canonical := parseDryRunAPI(t, shortcutFromRegistry(t, "+dim-freeze"), []string{"--url", testURL, "--sheet-name", "S", "--rows", "0", "--cols", "2"})
		got := parseDryRunAPI(t, shortcutFromRegistry(t, "+dim-freeze"), append([]string{"--url", testURL, "--sheet-name", "S"}, args...))
		require.Equal(t, canonical, got)
	}
	for _, counts := range []string{`"frozen_rows":0,"frozen_cols":2`, `"frozenRows":0,"frozenCols":2`, `"rows":0,"frozen_col_count":2`, `"frozen_rows":0,"cols":2`} {
		calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+batch-update"), []string{"--url", testURL, "--operations", `[{"shortcut":"+dim-freeze","input":{"sheet_name":"S",` + counts + `}}]`})
		input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
		require.Equal(t, float64(2), gjson.Get(input, "operations.0.input.freeze_columns").Value(), input)
		require.False(t, gjson.Get(input, "operations.0.input.freeze_rows").Exists(), input)
	}
	for _, counts := range []string{`"rows":0,"frozen_cols":0`, `"frozen_rows":0,"cols":0`} {
		calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+batch-update"), []string{"--url", testURL, "--operations", `[{"shortcut":"+dim-freeze","input":{"sheet_name":"S",` + counts + `}}]`})
		input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
		require.Equal(t, "unfreeze", gjson.Get(input, "operations.0.input.operation").String())
		require.False(t, gjson.Get(input, "operations.0.input.freeze_rows").Exists())
		require.False(t, gjson.Get(input, "operations.0.input.freeze_columns").Exists())
	}
	for _, counts := range []string{`"frozen_rows":2`, `"frozen_columns":2`, `"frozen_rows":1,"rows":2,"cols":3`, `"frozen_rows":1.5,"cols":2`, `"frozen_rows":1,"cols":2,"dimension":"row","count":1`} {
		out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+batch-update"), []string{"--url", testURL, "--operations", `[{"shortcut":"+dim-freeze","input":{"sheet_name":"S",` + counts + `}}]`, "--dry-run"})
		var validation *errs.ValidationError
		require.ErrorAs(t, err, &validation, counts)
		require.Empty(t, out)
	}
}

func TestBatchFreezeMixedCountsRejectInvalidTypesBeforeWriting(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, axis := range []struct{ canonical, alias string }{{"rows", "frozen_cols"}, {"cols", "frozen_rows"}} {
		for _, invalid := range []string{`"2"`, `null`, `true`, `1.5`} {
			t.Run(axis.canonical+"/"+invalid, func(t *testing.T) {
				operations := `[{"shortcut":"+dim-freeze","input":{"sheet_id":"sheetId","` + axis.canonical + `":` + invalid + `,"` + axis.alias + `":1}}]`
				write := toolOutputStub(testToken, "write", `{}`)
				write.Optional = true
				out, err := runShortcutWithStubs(t, shortcutFromRegistry(t, "+batch-update"), []string{"--url", testURL, "--operations", operations}, write)
				var validation *errs.ValidationError
				require.ErrorAs(t, err, &validation)
				require.Equal(t, errs.SubtypeInvalidArgument, validation.Subtype)
				require.Equal(t, "--operations", validation.Param)
				require.Empty(t, write.CapturedBodies)
				require.Empty(t, out)
			})
		}
	}
}

func TestUnsupportedFreezeCountSpellings(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, name := range []string{"frozen-columns", "frozen-column-count", "column-count"} {
		out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+dim-freeze"), []string{"--url", testURL, "--sheet-name", "S", "--rows", "0", "--" + name, "2", "--dry-run"})
		var validation *errs.ValidationError
		require.ErrorAs(t, err, &validation)
		require.Len(t, validation.Params, 1)
		require.Equal(t, "--"+name, validation.Params[0].Name)
		require.Equal(t, "unknown flag", validation.Params[0].Reason)
		require.Empty(t, out)
		out, _, err = runShortcutCapturingErr(t, shortcutFromRegistry(t, "+batch-update"), []string{"--url", testURL, "--operations", `[{"shortcut":"+dim-freeze","input":{"sheet_name":"S","rows":0,"` + name + `":2}}]`, "--dry-run"})
		require.ErrorAs(t, err, &validation)
		require.Empty(t, out)
	}
}

func TestBorderAttributeFlagsKeepExplicitSideSelection(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, command := range []string{"+cells-set-style", "+cells-batch-set-style", "+batch-update"} {
		args := []string{"--url", testURL}
		path := "cells.0.0"
		switch command {
		case "+cells-set-style":
			args = append(args, "--sheet-name", "S", "--range", "B2")
		case "+cells-batch-set-style":
			args = append(args, "--ranges", `["S!B2"]`)
			path = "operations.0.input.cells.0.0"
		case "+batch-update":
			args = append(args, "--operations", `[{"shortcut":"+cells-set-style","input":{"sheet_name":"S","range":"B2","border_styles":{"left":{"style":"dashed"}},"border_color":"#123456","border_weight":"medium"}}]`)
			path = "operations.0.input.cells.0.0"
		}
		if command != "+batch-update" {
			args = append(args, "--border-styles", `{"left":{"style":"dashed"}}`, "--border-color", "#123456", "--border-weight", "medium")
		}
		calls := parseDryRunAPI(t, shortcutFromRegistry(t, command), args)
		input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
		border := gjson.Get(input, path+".border_styles")
		require.Equal(t, "dashed", border.Get("left.style").String(), input)
		require.Equal(t, "#123456", border.Get("left.color").String(), input)
		require.Equal(t, "medium", border.Get("left.weight").String(), input)
		for _, side := range []string{"top", "bottom", "right"} {
			require.False(t, border.Get(side).Exists(), input)
		}
	}
	for _, args := range [][]string{
		{"--border-color", "#123456"},
		{"--border-styles", `{"all":{"style":"solid","color":"#654321"}}`, "--border-color", "#123456"},
		{"--border-styles", `{"left":{"style":"solid","weight":"thin"}}`, "--border-weight", "thick"},
		{"--border-styles", `{"left":{"style":"solid"}}`, "--border-weight", "invalid"},
	} {
		out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-set-style"), append([]string{"--url", testURL, "--sheet-name", "S", "--range", "B2", "--dry-run"}, args...))
		var validation *errs.ValidationError
		require.ErrorAs(t, err, &validation)
		require.Contains(t, []string{"--border-color", "--border-weight"}, validation.Param)
		require.Empty(t, out)
	}
}
