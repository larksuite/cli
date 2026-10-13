// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestStyleStampCarriersKeepExactTargetsAndContent(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		selector, payload string
		extra             []string
		index             int
	}{
		{"sheet-name", `{"hAlign":"right","fontWeight":"bold"}`, []string{"--range", "B2:C2"}, 0},
		{"sheet-id", `[{"range":"B2:C2","style":{"hAlign":"right","fontWeight":"bold"}}]`, nil, 1},
		{"sheet-name", `[{"name":"target","range":"B2:C2","font_weight":"bold","horizontal_alignment":"right"}]`, nil, 0},
	} {
		args := append([]string{"--url", testURL, "--" + tc.selector, "target", "--styles", tc.payload}, tc.extra...)
		calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+styles-put"), args)
		body := calls[tc.index].(map[string]interface{})["body"].(map[string]interface{})
		input := gjson.Parse(body["input"].(string)).Get("operations.0.input")
		key := "sheet_name"
		if tc.selector == "sheet-id" {
			key = "sheet_id"
		}
		require.Equal(t, "target", input.Get(key).String())
		require.Equal(t, "B2:C2", input.Get("range").String())
		for _, cell := range input.Get("cells.0").Array() {
			require.Equal(t, "bold", cell.Get("cell_styles.font_weight").String())
			require.Equal(t, "right", cell.Get("cell_styles.horizontal_alignment").String())
			require.False(t, cell.Get("value").Exists())
			require.False(t, cell.Get("formula").Exists())
		}
	}
}

func TestStyleStampCarriersRejectAmbiguity(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, args := range [][]string{
		{"--sheet-name", "S", "--styles", `{"range":null,"font_weight":"bold"}`},
		{"--sheet-name", "S", "--styles", `[{"range":null,"font_weight":"bold"}]`},
		{"--sheet-name", "S", "--styles", `[{"range":"B2","font_weight":"bold"},null]`},
		{"--styles", `[{"range":"B2","font_weight":"bold"}]`},
		{"--sheet-name", "S", "--styles", `[{"name":"Other","range":"B2","font_weight":"bold"}]`},
		{"--sheet-name", "S", "--range", "A1", "--styles", `{"range":"B2","font_weight":"bold"}`},
		{"--sheet-name", "S", "--range", "A1", "--styles", `{"font_weight":"bold","value":"drop"}`},
		{"--sheet-name", "S", "--range", "A1", "--styles", `{"cell_styles":[{"range":"B2","font_weight":"bold"}]}`},
		{"--sheet-id", "id", "--styles", `{"styles":[{"name":"","cell_styles":[{"range":"A1","font_weight":"bold"}]}]}`},
		{"--sheet-id", "id", "--styles", `[{"name":"A","range":"A1","font_weight":"bold"},{"name":"B","range":"A1","font_weight":"bold"}]`},
		{"--sheet-id", "id", "--sheet-name", "S", "--styles", `[{"range":"A1","font_weight":"bold"}]`},
	} {
		out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+styles-put"), append([]string{"--url", testURL, "--dry-run"}, args...))
		var validation *errs.ValidationError
		require.ErrorAs(t, err, &validation, args)
		require.Empty(t, out)
	}
}

func TestStyleStampsPreserveTargetAndStampOrder(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+styles-put"), []string{"--url", testURL, "--styles", `[{"name":"Z","range":"B2","font_weight":"bold"},{"name":"A","range":"C3","font_weight":"bold"},{"name":"Z","range":"D4","font_weight":"bold"}]`})
	input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
	ops := gjson.Get(input, "operations").Array()
	require.Len(t, ops, 3)
	for i, want := range []struct{ name, rangeText string }{{"Z", "B2:B2"}, {"Z", "D4:D4"}, {"A", "C3"}} {
		require.Equal(t, "set_cell_range", ops[i].Get("tool_name").String())
		require.Equal(t, want.name, ops[i].Get("input.sheet_name").String())
		require.Equal(t, want.rangeText, ops[i].Get("input.range").String())
		require.Equal(t, "bold", ops[i].Get("input.cells.0.0.cell_styles.font_weight").String())
		require.False(t, ops[i].Get("input.cells.0.0.value").Exists())
	}
}

func TestSingleStyleObjectChecksExplicitSheetNameAfterInputResolution(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, key := range []string{"name", "sheet_name"} {
		payload := `{"` + key + `":"Other","cell_styles":[{"range":"B2","font_weight":"bold"}]}`
		path := filepath.Join(t.TempDir(), "styles.json")
		require.NoError(t, os.WriteFile(path, []byte(payload), 0600))
		for _, source := range []string{payload, "@" + path, "-"} {
			out, _, err := runShortcutCapturingErrWithStdin(t, shortcutFromRegistry(t, "+styles-put"), []string{"--url", testURL, "--sheet-name", "Expected", "--styles", source, "--dry-run"}, payload)
			var validation *errs.ValidationError
			require.ErrorAs(t, err, &validation)
			require.Equal(t, "--sheet-name", validation.Param)
			require.Empty(t, out, "conflicting target must not produce an API plan")
		}
		write := toolOutputStub(testToken, "write", `{"ok":true}`)
		write.Optional = true
		out, err := runShortcutWithStubs(t, shortcutFromRegistry(t, "+styles-put"), []string{"--url", testURL, "--sheet-name", "Expected", "--styles", payload}, write)
		var validation *errs.ValidationError
		require.ErrorAs(t, err, &validation)
		require.Equal(t, "--sheet-name", validation.Param)
		require.Empty(t, write.CapturedBodies, "conflicting target must not be sent")
		require.Empty(t, out)
		calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+styles-put"), []string{"--url", testURL, "--sheet-name", "Other", "--styles", payload})
		input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
		require.Equal(t, "Other", gjson.Get(input, "operations.0.input.sheet_name").String())
	}
}

func TestExplicitSheetIDIsVerifiedBeforeAnyMutation(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct{ command, payload, flag string }{
		{"+table-put", `{"sheets":[{"name":"Wrong","columns":["a"],"data":[["keep"]]}]}`, "sheets"},
		{"+styles-put", `{"styles":[{"name":"Wrong","cell_styles":[{"range":"A1","font_weight":"bold"}]}]}`, "styles"},
		{"+styles-put", `[{"range":"Wrong!A1","font_weight":"bold"}]`, "styles"},
	} {
		read := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"id","sheet_name":"S","row_count":200,"column_count":20}]}`)
		write := toolOutputStub(testToken, "write", `{"ok":true}`)
		write.Optional = true
		out, err := runShortcutWithStubs(t, shortcutFromRegistry(t, tc.command), []string{"--url", testURL, "--sheet-id", "id", "--" + tc.flag, tc.payload}, read, write)
		var validation *errs.ValidationError
		require.ErrorAs(t, err, &validation)
		require.Equal(t, errs.SubtypeInvalidArgument, validation.Subtype)
		require.Len(t, read.CapturedBodies, 1)
		require.Empty(t, write.CapturedBodies)
		require.Empty(t, out)
	}
}

func TestExplicitSheetIDWritesUsingVerifiedID(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct{ command, payload, flag, path string }{
		{"+table-put", `{"sheets":[{"name":"S","header":false,"columns":["a"],"data":[["00123"]]}]}`, "sheets", "sheet_id"},
		{"+styles-put", `{"styles":[{"name":"S","cell_styles":[{"range":"A1","font_weight":"bold"}]}]}`, "styles", "operations.0.input.sheet_id"},
		{"+styles-put", `[{"range":"S!A:C","font_weight":"bold"}]`, "styles", "operations.0.input.sheet_id"},
	} {
		read := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"id","sheet_name":"S","row_count":2,"column_count":3}]}`)
		write := toolOutputStub(testToken, "write", `{"ok":true}`)
		_, err := runShortcutWithStubs(t, shortcutFromRegistry(t, tc.command), []string{"--url", testURL, "--sheet-id", "id", "--" + tc.flag, tc.payload}, read, write)
		require.NoError(t, err)
		require.Len(t, read.CapturedBodies, 1)
		require.Len(t, write.CapturedBodies, 1)
		input := gjson.GetBytes(write.CapturedBody, "input").String()
		require.Equal(t, "id", gjson.Get(input, tc.path).String(), input)
		if tc.command == "+table-put" {
			require.Equal(t, "00123", gjson.Get(input, "cells.0.0.value").Value())
		}
		if tc.payload[0] == '[' {
			require.Equal(t, "A1:C2", gjson.Get(input, "operations.0.input.range").String())
		}
	}
}

func TestNamedResizeBoundariesUseA1UnitsAndKeepConflicts(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct{ command, start, end, rng, size string }{
		{"+rows-resize", "start-row", "end-row", "5:8", "height"},
		{"+cols-resize", "col-start", "col-end", "B:D", "width"},
	} {
		first, last := "5", "8"
		if tc.command == "+cols-resize" {
			first, last = "B", "D"
		}
		base := []string{"--url", testURL, "--sheet-name", "S", "--" + tc.size, "120"}
		want := parseDryRunAPI(t, shortcutFromRegistry(t, tc.command), append(append([]string(nil), base...), "--range", tc.rng))
		got := parseDryRunAPI(t, shortcutFromRegistry(t, tc.command), append(append([]string(nil), base...), "--"+tc.start, first, "--"+tc.end, last))
		require.Equal(t, want, got)
	}
	for _, args := range [][]string{
		{"--start-row", "0"}, {"--end-row", "5"}, {"--start-row", "8", "--end-row", "5"},
		{"--start-row", "5", "--end-row", "8", "--range", "2:3"}, {"--start-row", "A"},
	} {
		out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+rows-resize"), append([]string{"--url", testURL, "--sheet-name", "S", "--height", "120", "--dry-run"}, args...))
		var validation *errs.ValidationError
		require.ErrorAs(t, err, &validation)
		require.Empty(t, out)
	}
	canon := parseDryRunAPI(t, shortcutFromRegistry(t, "+batch-update"), []string{"--url", testURL, "--operations", `[{"shortcut":"+rows-resize","input":{"sheet_name":"S","range":"5:8","height":120}}]`})
	alias := parseDryRunAPI(t, shortcutFromRegistry(t, "+batch-update"), []string{"--url", testURL, "--operations", `[{"shortcut":"+rows-resize","input":{"sheet_name":"S","start_row":5,"end_row":8,"height":120}}]`})
	// Numeric batch row boundaries use the same A1 row numbers.
	require.Equal(t, canon, alias)
}

func TestRemainingExactAliasesPreserveRequestGrammar(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		command, alias, canonical, value string
		extra                            []string
	}{
		{"+chart-create-basic", "position", "anchor-cell", "F2", []string{"--chart-type", "column", "--data-range", "A1:B3"}},
		{"+cols-resize", "col", "range", "D", []string{"--width", "120"}},
		{"+dim-delete", "end", "end-index", "8", []string{"--start", "5", "--yes"}},
	} {
		base := append([]string{"--url", testURL, "--sheet-name", "S"}, tc.extra...)
		got := parseDryRunAPI(t, shortcutFromRegistry(t, tc.command), append(append([]string(nil), base...), "--"+tc.alias, tc.value))
		want := parseDryRunAPI(t, shortcutFromRegistry(t, tc.command), append(append([]string(nil), base...), "--"+tc.canonical, tc.value))
		require.Equal(t, want, got)
	}
}

func TestTableIDCompatibilityDoesNotDropUnsupportedPayload(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, payload := range []string{
		`[{"name":"S","range":"B2","values":[["keep"]]}]`,
		`[{"name":"S","columns":["a"],"data":[["keep"]],"rows":[["other"]]}]`,
	} {
		out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+table-put"), []string{"--url", testURL, "--sheet-id", "id", "--sheets", payload, "--dry-run"})
		var validation *errs.ValidationError
		require.ErrorAs(t, err, &validation)
		require.Equal(t, "--sheets", validation.Param)
		require.Empty(t, out)
	}
}

func TestBatchRangeSelectorsDoNotRequireStructureReadScope(t *testing.T) {
	for _, command := range []string{"+cells-batch-clear", "+cells-batch-set-style", "+dropdown-update", "+dropdown-delete"} {
		sc := shortcutFromRegistry(t, command)
		require.NotContains(t, sc.ConditionalScopes, sheetsStructureReadScope)
		require.NotContains(t, sc.Scopes, sheetsStructureReadScope)
	}
}

func TestTypedTablePayloadAnchorKeepsHeaderTypesAndExtent(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	payload := `[{"name":"S","start_cell":"C5","columns":["label","n"],"dtypes":{"n":"int64"},"data":[["00123",1001]],"allow_overwrite":false}]`
	calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+table-put"), []string{"--url", testURL, "--sheets", payload})
	body := calls[0].(map[string]interface{})["body"].(map[string]interface{})
	input := body["input"].(string)
	require.Equal(t, "C5:D6", gjson.Get(input, "range").String())
	require.Equal(t, "S", gjson.Get(input, "sheet_name").String())
	require.Equal(t, "label", gjson.Get(input, "cells.0.0.value").Value())
	require.Equal(t, "00123", gjson.Get(input, "cells.1.0.value").Value())
	require.Equal(t, "1001", gjson.Get(input, "cells.1.1.value").Raw)
	require.Equal(t, false, gjson.Get(input, "allow_overwrite").Value())
	for _, flag := range []string{"range", "start-cell"} {
		out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+table-put"), []string{"--url", testURL, "--" + flag, "C5", "--sheets", payload, "--dry-run"})
		var validation *errs.ValidationError
		require.ErrorAs(t, err, &validation)
		require.Len(t, validation.Params, 1)
		require.Equal(t, "--"+flag, validation.Params[0].Name)
		require.Equal(t, "unknown flag", validation.Params[0].Reason)
		require.Contains(t, validation.Hint, "start_cell")
		require.Empty(t, out)
	}
}
