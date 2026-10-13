// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCellsSetSparseUsesExactWriteTargets(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, payload := range []string{
		`[{"cell":"A2","value":"00123"},{"range":"Z900","formula":"=SUM(B2:B4)","cell_styles":{"bold":false}}]`,
		`{"cells":[{"cell":"A2","value":"00123"},{"range":"Z900","formula":"=SUM(B2:B4)","cell_styles":{"bold":false}}]}`,
	} {
		calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+cells-set"), []string{
			"--url", testURL, "--sheet-name", "S", "--cells", payload,
		})
		body := calls[0].(map[string]interface{})["body"].(map[string]interface{})
		require.Equal(t, "batch_update", body["tool_name"])
		input := body["input"].(string)
		ops := gjson.Get(input, "operations").Array()
		require.Len(t, ops, 2)
		for i, target := range []string{"A2", "Z900"} {
			require.Equal(t, "set_cell_range", ops[i].Get("tool_name").String())
			require.Equal(t, "S", ops[i].Get("input.sheet_name").String())
			require.Equal(t, target, ops[i].Get("input.range").String())
			require.Len(t, ops[i].Get("input.cells").Array(), 1)
			require.Len(t, ops[i].Get("input.cells.0").Array(), 1)
			require.False(t, ops[i].Get("input.cells.0.0.cell").Exists())
			require.False(t, ops[i].Get("input.cells.0.0.range").Exists())
		}
		require.Equal(t, "00123", ops[0].Get("input.cells.0.0.value").String())
		require.Equal(t, "=SUM(B2:B4)", ops[1].Get("input.cells.0.0.formula").String())
		require.Equal(t, "normal", ops[1].Get("input.cells.0.0.cell_styles.font_weight").String())
	}
}

func TestCellsSetPreservesNumbersInFinalRequests(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name, tool, path string
		args             []string
	}{
		{"sparse value", "batch_update", "operations.0.input.cells.0", []string{"--cells", `[{"cell":"A1","value":9007199254740993},{"cell":"B1","value":"00123"}]`}},
		{"sparse typed value", "batch_update", "operations.0.input.cells.0", []string{"--cells", `[{"cell":"A1","type":"number","value":9007199254740993},{"cell":"B1","type":"string","value":"00123"}]`}},
		{"sparse values block", "batch_update", "operations.0.input.cells.0", []string{"--cells", `[{"range":"A1:B1","values":[[9007199254740993,"00123"]]}]`}},
		{"sparse typed block", "batch_update", "operations.0.input.cells.0", []string{"--cells", `[{"range":"A1:B1","values":[[{"type":"number","value":9007199254740993},{"type":"string","value":"00123"}]]}]`}},
		{"repaired sparse block", "batch_update", "operations.0.input.cells.0", []string{"--cells", `[{"range":"A1:B1","values":[[9007199254740993,"00123",]],}]`}},
		{"dense cells", "set_cell_range", "cells.0", []string{"--range", "A1:B1", "--cells", `[[{"type":"number","value":9007199254740993},{"value":"00123"}]]`}},
		{"repaired dense cells", "set_cell_range", "cells.0", []string{"--range", "A1:B1", "--cells", `[[{"type":"number","value":9007199254740993},{"value":"00123"},],]`}},
		{"styled cells", "set_cell_range", "cells.0", []string{"--range", "A1:B1", "--cells", `[[{"type":"number","value":9007199254740993,"cell_styles":{"hAlign":2,"vAlign":1,"font_size":16},"border_styles":{"top":{"style":"solid","weight":2}}},{"value":"00123"}]]`}},
		{"writes", "batch_update", "operations.0.input.cells.0", []string{"--writes", `[{"range":"A1:B1","cells":[[{"type":"number","value":9007199254740993},{"value":"00123"}]]}]`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--url", testURL, "--sheet-id", "sheetId"}, tc.args...)
			api := parseDryRunAPI(t, shortcutFromRegistry(t, "+cells-set"), args)
			body := api[0].(map[string]interface{})["body"].(map[string]interface{})
			require.Equal(t, tc.tool, body["tool_name"])
			preview := body["input"].(string)
			write := toolOutputStub(testToken, "write", `{}`)
			_, err := runShortcutWithStubs(t, shortcutFromRegistry(t, "+cells-set"), args, write)
			require.NoError(t, err)
			require.Len(t, write.CapturedBodies, 1)
			require.Equal(t, tc.tool, gjson.GetBytes(write.CapturedBody, "tool_name").String())
			for _, input := range []string{preview, gjson.GetBytes(write.CapturedBody, "input").String()} {
				cell := gjson.Get(input, tc.path+".0")
				require.Equal(t, gjson.Number, cell.Get("value").Type)
				require.Equal(t, "9007199254740993", cell.Get("value").Raw)
				require.False(t, cell.Get("type").Exists())
				textPath := tc.path + ".1.value"
				if tc.name == "sparse value" || tc.name == "sparse typed value" {
					textPath = "operations.1.input.cells.0.0.value"
				}
				require.Equal(t, gjson.String, gjson.Get(input, textPath).Type)
				require.Equal(t, "00123", gjson.Get(input, textPath).String())
				if tc.name == "styled cells" {
					require.Equal(t, "right", cell.Get("cell_styles.horizontal_alignment").String())
					require.Equal(t, "middle", cell.Get("cell_styles.vertical_alignment").String())
					require.Equal(t, "16", cell.Get("cell_styles.font_size").Raw)
					require.Equal(t, "medium", cell.Get("border_styles.top.weight").String())
				}
			}
		})
	}
}

func TestCellsSetSparseSupportsExplicitBlocksAndQualifiedTargets(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+cells-set"), []string{
		"--url", testURL, "--cells",
		`[{"range":"'First sheet'!B3:C3","values":[[1,2]]},{"cell":"Second!D5","value":"keep"}]`,
		"--allow-overwrite=false",
	})
	input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
	for i, want := range []struct{ sheet, target string }{{"First sheet", "B3:C3"}, {"Second", "D5"}} {
		op := gjson.Get(input, "operations").Array()[i].Get("input")
		require.Equal(t, want.sheet, op.Get("sheet_name").String())
		require.Equal(t, want.target, op.Get("range").String())
		require.False(t, op.Get("allow_overwrite").Bool())
	}
	require.Equal(t, float64(2), gjson.Get(input, "operations.0.input.cells.0.1.value").Float())
}

func TestCellsSetSparseRejectsAmbiguousTargetsAndPayloads(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		payload, param string
		extra          []string
	}{
		{`[{"cell":"A1","value":1}]`, "--range", []string{"--range", "B1"}},
		{`[{"cell":"A1","value":1}]`, "--start-cell", []string{"--start-cell", "B1"}},
		{`[{"cell":"A1","value":1}]`, "--copy-to-range", []string{"--copy-to-range", "B1:B2"}},
		{`[{"cell":"A1","formula":"=1+2"}]`, "--formula", []string{"--formula", "=1+2"}},
		{`[{"cell":"A1","values":[[1]],"cells":[[2]]}]`, "--cells", nil},
		{`[{"cell":"A1","value":1},{"value":2}]`, "--cells", nil},
		{`[{"cell":"A1","range":"B1","value":1}]`, "--cells", nil},
		{`[{"cell":"A1:B2","value":1}]`, "--cells", nil},
		{`[{"range":"A1:B2","value":1}]`, "--cells", nil},
		{`[{"cell":"A0","value":1}]`, "--cells", nil},
		{`[{"cell":3,"value":1}]`, "--cells", nil},
		{`[{"cell":"A1","values":[[1]],"value":2}]`, "--cells", nil},
		{`[{"cell":"A1","values":[[1,2]]}]`, "--cells", nil},
		{`[{"cell":"A1","cells":[[1],[2]]}]`, "--cells", nil},
		{`[{"cell":"A1","value":1,"unexpected":true}]`, "--cells", nil},
	} {
		t.Run(tc.payload+tc.param, func(t *testing.T) {
			args := append([]string{"--url", testURL, "--sheet-name", "S", "--cells", tc.payload, "--dry-run"}, tc.extra...)
			out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-set"), args)
			var validation *errs.ValidationError
			require.ErrorAs(t, err, &validation)
			require.Equal(t, errs.SubtypeInvalidArgument, validation.Subtype)
			require.Equal(t, tc.param, validation.Param)
			require.Empty(t, out)
			if tc.payload == `[{"cell":"A0","value":1}]` {
				require.NotNil(t, errors.Unwrap(validation), "the coordinate parser's cause must survive the typed wrapper")
			}
		})
	}
}

func TestCellsSetSparseCellBlockStaysWithinOneCell(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	api := parseDryRunAPI(t, shortcutFromRegistry(t, "+cells-set"), []string{
		"--url", testURL, "--sheet-name", "S", "--cells", `[{"cell":"A1","values":[[7]]}]`,
	})
	input := api[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
	require.Equal(t, "A1", gjson.Get(input, "operations.0.input.range").String())
	require.Equal(t, float64(7), gjson.Get(input, "operations.0.input.cells.0.0.value").Float())
}

func TestCellsSetSparseBlockPreservesJSONStringAsCellValue(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	api := parseDryRunAPI(t, shortcutFromRegistry(t, "+cells-set"), []string{
		"--url", testURL, "--sheet-name", "S", "--cells", `[{"range":"A1","values":"[[1,2]]"}]`,
	})
	input := api[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
	require.Equal(t, "A1", gjson.Get(input, "operations.0.input.range").String())
	require.Equal(t, `[[{"value":"[[1,2]]"}]]`, gjson.Get(input, "operations.0.input.cells").Raw)
}

func TestCellsSetSparseCannotNestInBatchUpdate(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+batch-update"), []string{
		"--url", testURL, "--operations",
		`[{"shortcut":"+cells-set","input":{"sheet_name":"S","range":"B1","cells":[{"cell":"A1","value":1}]}}]`,
		"--dry-run",
	})
	validation := requireValidation(t, err, "own batch request")
	require.Equal(t, "--operations", validation.Param)
}

func TestCellsSetSparseKeepsWriteLimitsAndErrorAttribution(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	items := make([]map[string]interface{}, maxBatchOperations+1)
	for i := range items {
		items[i] = map[string]interface{}{"cell": "A1", "value": i}
	}
	payload, err := json.Marshal(items)
	require.NoError(t, err)
	_, _, err = runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-set"), []string{
		"--url", testURL, "--sheet-name", "S", "--cells", string(payload), "--dry-run",
	})
	validation := requireValidation(t, err, "at most")
	require.Equal(t, "--cells", validation.Param)
}
