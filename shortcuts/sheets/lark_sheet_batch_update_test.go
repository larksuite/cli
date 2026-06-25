// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"testing"
)

// TestBatchUpdate_TranslatesShortcutToToolName verifies +batch-update
// translates each CLI-shape sub-op ({shortcut, input}) to the MCP-shape
// ({tool_name, input(+operation, +excel_id)}) before threading into
// the underlying batch_update tool. Covers continue_on_error too.
func TestBatchUpdate_TranslatesShortcutToToolName(t *testing.T) {
	t.Parallel()

	body := parseDryRunBody(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[
		  {"shortcut":"+cells-set","input":{"sheet_id":"sh1","range":"A1","cells":[[{"value":42}]]}},
		  {"shortcut":"+dim-insert","input":{"sheet_id":"sh1","position":"1","count":3}}
		]`,
		"--continue-on-error",
		"--yes",
	})
	input := decodeToolInput(t, body, "batch_update")
	ops, _ := input["operations"].([]interface{})
	if len(ops) != 2 {
		t.Fatalf("operations length = %d, want 2", len(ops))
	}
	if input["continue_on_error"] != true {
		t.Errorf("continue_on_error = %v, want true", input["continue_on_error"])
	}

	// op[0]: +cells-set → set_cell_range, no operation field
	op0 := ops[0].(map[string]interface{})
	if op0["tool_name"] != "set_cell_range" {
		t.Errorf("op[0].tool_name = %v, want set_cell_range", op0["tool_name"])
	}
	in0, _ := op0["input"].(map[string]interface{})
	if in0["excel_id"] == nil {
		t.Errorf("op[0].input.excel_id missing (translator should inject)")
	}
	if _, has := in0["operation"]; has {
		t.Errorf("op[0].input.operation present, +cells-set should not inject one: %#v", in0)
	}

	// op[1]: +dim-insert → modify_sheet_structure + operation:"insert"
	op1 := ops[1].(map[string]interface{})
	if op1["tool_name"] != "modify_sheet_structure" {
		t.Errorf("op[1].tool_name = %v, want modify_sheet_structure", op1["tool_name"])
	}
	in1, _ := op1["input"].(map[string]interface{})
	if in1["operation"] != "insert" {
		t.Errorf("op[1].input.operation = %v, want \"insert\"", in1["operation"])
	}
}

func TestBatchUpdate_HighRiskWriteRequiresYes(t *testing.T) {
	t.Parallel()
	stdout, stderr, err := runShortcutCapturingErr(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[{"shortcut":"+cells-set","input":{}}]`,
	})
	if err == nil {
		t.Fatalf("expected confirmation_required; stdout=%s stderr=%s", stdout, stderr)
	}
}

// TestCellsBatchSetStyle_FansOutOps verifies multiple ranges produce one
// set_cell_range op each, sharing the same style flags.
func TestCellsBatchSetStyle_FansOutOps(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, CellsBatchSetStyle, []string{
		"--url", testURL,
		"--ranges", `["sheet1!A1:B2","sheet1!D1:E2","sheet1!A5:A6"]`,
		"--font-weight", "bold",
		"--background-color", "#ffff00",
	})
	input := decodeToolInput(t, body, "batch_update")
	ops, _ := input["operations"].([]interface{})
	if len(ops) != 3 {
		t.Fatalf("operations length = %d, want 3 (one per range)", len(ops))
	}
	for i, raw := range ops {
		op, _ := raw.(map[string]interface{})
		if op["tool_name"] != "set_cell_range" {
			t.Errorf("op[%d].tool_name = %v, want set_cell_range", i, op["tool_name"])
		}
		params, _ := op["input"].(map[string]interface{})
		if params["sheet_name"] != "sheet1" {
			t.Errorf("op[%d].sheet_name = %v, want sheet1", i, params["sheet_name"])
		}
		cells, _ := params["cells"].([]interface{})
		row, _ := cells[0].([]interface{})
		cell, _ := row[0].(map[string]interface{})
		style, _ := cell["cell_styles"].(map[string]interface{})
		if style["font_weight"] != "bold" || style["background_color"] != "#ffff00" {
			t.Errorf("op[%d] cell_styles wrong: %#v", i, style)
		}
	}
}

// TestCellsBatchClear_FansOutOps verifies multiple ranges produce one
// clear_cell_range op each, all sharing the same --scope-derived clear_type,
// with the sheet prefix split into sheet_name + bare range.
func TestCellsBatchClear_FansOutOps(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, CellsBatchClear, []string{
		"--url", testURL,
		"--ranges", `["sheet1!A1:A10","sheet2!C1:D5","sheet1!F3"]`,
		"--scope", "all",
		"--yes",
	})
	input := decodeToolInput(t, body, "batch_update")
	ops, _ := input["operations"].([]interface{})
	if len(ops) != 3 {
		t.Fatalf("operations length = %d, want 3 (one per range)", len(ops))
	}
	wantSheet := []string{"sheet1", "sheet2", "sheet1"}
	wantRange := []string{"A1:A10", "C1:D5", "F3"}
	for i, raw := range ops {
		op, _ := raw.(map[string]interface{})
		if op["tool_name"] != "clear_cell_range" {
			t.Errorf("op[%d].tool_name = %v, want clear_cell_range", i, op["tool_name"])
		}
		params, _ := op["input"].(map[string]interface{})
		if params["sheet_name"] != wantSheet[i] {
			t.Errorf("op[%d].sheet_name = %v, want %s", i, params["sheet_name"], wantSheet[i])
		}
		if params["range"] != wantRange[i] {
			t.Errorf("op[%d].range = %v, want %s", i, params["range"], wantRange[i])
		}
		if params["clear_type"] != "all" {
			t.Errorf("op[%d].clear_type = %v, want all", i, params["clear_type"])
		}
	}
}

// TestCellsBatchClear_ScopeDefaultsToContents verifies the default --scope
// (content) maps to the tool's clear_type "contents" — identical to the
// standalone +cells-clear normalization.
func TestCellsBatchClear_ScopeDefaultsToContents(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, CellsBatchClear, []string{
		"--url", testURL,
		"--ranges", `["sheet1!A1:B2"]`,
		"--yes",
	})
	input := decodeToolInput(t, body, "batch_update")
	ops, _ := input["operations"].([]interface{})
	if len(ops) != 1 {
		t.Fatalf("operations length = %d, want 1", len(ops))
	}
	params, _ := ops[0].(map[string]interface{})["input"].(map[string]interface{})
	if params["clear_type"] != "contents" {
		t.Errorf("clear_type = %v, want contents (default scope)", params["clear_type"])
	}
}

// TestCellsBatchClear_Guards covers the sheet-prefix requirement and the
// high-risk-write confirmation gate.
func TestCellsBatchClear_Guards(t *testing.T) {
	t.Parallel()

	// sheetless range → prefix guard (shared with the dropdown fan-outs).
	_, _, err := runShortcutCapturingErr(t, CellsBatchClear, []string{
		"--url", testURL,
		"--ranges", `["A1:A10"]`,
		"--yes",
		"--dry-run",
	})
	requireValidation(t, err, "must include a sheet prefix")

	// missing --yes → confirmation_required (high-risk-write).
	stdout, stderr, err := runShortcutCapturingErr(t, CellsBatchClear, []string{
		"--url", testURL,
		"--ranges", `["sheet1!A1:A10"]`,
	})
	if err == nil {
		t.Errorf("expected confirmation_required without --yes; stdout=%s stderr=%s", stdout, stderr)
	}
}

// TestDropdownUpdate_BatchPayload verifies the multi-range dropdown
// update fans out into a single batch_update with one set_cell_range
// op per range. Also covers --colors / --highlight -> highlight_colors
// / enable_highlight propagation through dropdownBatchInput.
func TestDropdownUpdate_BatchPayload(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, DropdownUpdate, []string{
		"--url", testURL,
		"--ranges", `["sheet1!A2:A5","sheet1!C2:C5"]`,
		"--options", `["a","b","c"]`,
		"--colors", `["#FFE699","#bff7d9","#ffb3b3"]`,
		"--multiple", "--highlight",
	})
	input := decodeToolInput(t, body, "batch_update")
	ops, _ := input["operations"].([]interface{})
	if len(ops) != 2 {
		t.Fatalf("operations length = %d, want 2", len(ops))
	}
	for i, raw := range ops {
		op, _ := raw.(map[string]interface{})
		params, _ := op["input"].(map[string]interface{})
		cells, _ := params["cells"].([]interface{})
		if len(cells) != 4 {
			t.Errorf("op[%d] cells rows = %d, want 4 (A2:A5 / C2:C5)", i, len(cells))
		}
		row0, _ := cells[0].([]interface{})
		cell, _ := row0[0].(map[string]interface{})
		dv, _ := cell["data_validation"].(map[string]interface{})
		if dv == nil || dv["type"] != "list" {
			t.Errorf("op[%d] missing data_validation list: %#v", i, cell)
		}
		items, _ := dv["items"].([]interface{})
		if len(items) != 3 {
			t.Errorf("op[%d] data_validation.items length = %d, want 3", i, len(items))
		}
		if dv["support_multiple_values"] != true {
			t.Errorf("op[%d] support_multiple_values = %v, want true", i, dv["support_multiple_values"])
		}
		colors, _ := dv["highlight_colors"].([]interface{})
		if len(colors) != 3 {
			t.Errorf("op[%d] highlight_colors length = %d, want 3", i, len(colors))
		}
		if dv["enable_highlight"] != true {
			t.Errorf("op[%d] enable_highlight = %v, want true", i, dv["enable_highlight"])
		}
	}
}

// TestDropdownDelete_BatchClearsValidation verifies delete sets
// data_validation: null on every cell.
func TestDropdownDelete_BatchClearsValidation(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, DropdownDelete, []string{
		"--url", testURL,
		"--ranges", `["sheet1!A2:A4"]`,
		"--yes",
	})
	input := decodeToolInput(t, body, "batch_update")
	ops, _ := input["operations"].([]interface{})
	if len(ops) != 1 {
		t.Fatalf("operations length = %d, want 1", len(ops))
	}
	op := ops[0].(map[string]interface{})
	params, _ := op["input"].(map[string]interface{})
	cells, _ := params["cells"].([]interface{})
	for i, raw := range cells {
		row, _ := raw.([]interface{})
		cell, _ := row[0].(map[string]interface{})
		if _, present := cell["data_validation"]; !present {
			t.Errorf("row %d: data_validation key missing", i)
			continue
		}
		if cell["data_validation"] != nil {
			t.Errorf("row %d: data_validation = %v, want null", i, cell["data_validation"])
		}
	}
}

func TestBatchUpdate_ValidationGuards(t *testing.T) {
	t.Parallel()

	// dropdown-update with sheetless range
	_, _, err := runShortcutCapturingErr(t, DropdownUpdate, []string{
		"--url", testURL,
		"--ranges", `["A2:A5"]`,
		"--options", `["a"]`,
		"--dry-run",
	})
	requireValidation(t, err, "must include a sheet prefix")

	// batch-update with empty operations
	_, _, err = runShortcutCapturingErr(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[]`,
		"--yes",
		"--dry-run",
	})
	requireValidation(t, err, "non-empty JSON array")

	// dropdown-update with non-array --options (object instead) → array guard
	// (now via schema validator at parseJSONFlag time)
	_, _, err = runShortcutCapturingErr(t, DropdownUpdate, []string{
		"--url", testURL,
		"--ranges", `["sheet1!A1:A2"]`,
		"--options", `{"not":"array"}`,
		"--dry-run",
	})
	requireValidation(t, err, `expected type "array"`)
}

// TestValidateDropdownRanges_RejectsMalformedRange locks the up-front sheet!range
// validation: entries that merely contain "!" but are otherwise malformed (empty
// sheet, empty range, or an unparseable A1 ref) must fail at Validate rather than
// slip through to DryRun/Execute. Covers +dropdown-update / +dropdown-delete,
// which fan out over --ranges.
func TestValidateDropdownRanges_RejectsMalformedRange(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		ranges string
		want   string
	}{
		{"no sheet prefix at all", `["A1:A5"]`, "must include a sheet prefix"},
		{"empty sheet name", `["!A1:A5"]`, "must use sheet!range form"},
		{"empty range after prefix", `["Sheet1!"]`, "must use sheet!range form"},
		{"unparseable ref", `["Sheet1!bad"]`, "invalid cell ref"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, DropdownUpdate, []string{
				"--url", testURL,
				"--ranges", tc.ranges,
				"--options", `["a"]`,
				"--dry-run",
			})
			requireValidation(t, err, tc.want)
		})
	}
}

// TestBatchUpdate_TranslatorRejects covers per-op shape errors caught by
// translateBatchOp: unknown shortcut, missing shortcut, banned (read /
// fan-out / legacy v2) shortcuts, hand-filled reserved keys, etc.
func TestBatchUpdate_TranslatorRejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		opsJSON   string
		wantMatch string
	}{
		{
			name:      "missing shortcut field",
			opsJSON:   `[{"input":{"range":"A1"}}]`,
			wantMatch: "'shortcut' field is required",
		},
		{
			name:      "empty shortcut string",
			opsJSON:   `[{"shortcut":"","input":{}}]`,
			wantMatch: "'shortcut' must be a non-empty string",
		},
		{
			name:      "unknown shortcut",
			opsJSON:   `[{"shortcut":"+cells-set-magic","input":{}}]`,
			wantMatch: "not allowed in +batch-update",
		},
		{
			name:      "read op rejected",
			opsJSON:   `[{"shortcut":"+cells-get","input":{}}]`,
			wantMatch: "not allowed in +batch-update",
		},
		{
			name:      "nested batch-update rejected",
			opsJSON:   `[{"shortcut":"+batch-update","input":{}}]`,
			wantMatch: "not allowed in +batch-update",
		},
		{
			name:      "fan-out wrapper rejected",
			opsJSON:   `[{"shortcut":"+cells-batch-set-style","input":{}}]`,
			wantMatch: "not allowed in +batch-update",
		},
		{
			name:      "fan-out wrapper +cells-batch-clear rejected",
			opsJSON:   `[{"shortcut":"+cells-batch-clear","input":{}}]`,
			wantMatch: "not allowed in +batch-update",
		},
		{
			name:      "legacy v2 +dim-move rejected",
			opsJSON:   `[{"shortcut":"+dim-move","input":{}}]`,
			wantMatch: "not allowed in +batch-update",
		},
		{
			name:      "user filled operation manually",
			opsJSON:   `[{"shortcut":"+dim-insert","input":{"operation":"delete","position":"1","count":1}}]`,
			wantMatch: "do not pass input.operation",
		},
		{
			name:      "user filled excel_id",
			opsJSON:   `[{"shortcut":"+cells-set","input":{"excel_id":"shtcnX","range":"A1"}}]`,
			wantMatch: "do not pass input.excel_id",
		},
		{
			name:      "user filled url",
			opsJSON:   `[{"shortcut":"+cells-set","input":{"url":"https://x.feishu.cn/sheets/sh","range":"A1"}}]`,
			wantMatch: "do not pass input.url",
		},
		{
			name:      "extra top-level key",
			opsJSON:   `[{"shortcut":"+cells-set","input":{"range":"A1"},"tool_name":"oops"}]`,
			wantMatch: "unknown top-level key",
		},
		{
			name:      "sub-op not an object",
			opsJSON:   `["not-an-object"]`,
			wantMatch: "must be a JSON object",
		},
		{
			name:      "input not an object",
			opsJSON:   `[{"shortcut":"+cells-set","input":"not-an-object"}]`,
			wantMatch: "'input' must be a JSON object",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, BatchUpdate, []string{
				"--url", testURL,
				"--operations", tc.opsJSON,
				"--yes",
				"--dry-run",
			})
			requireValidation(t, err, tc.wantMatch)
		})
	}
}

// TestBatchUpdate_DimFreezeInjectsFreeze covers the static-freeze-only
// path: +dim-freeze always injects operation=freeze (count==0 unfreeze
// path of the single shortcut is intentionally not supported in batch).
func TestBatchUpdate_DimFreezeInjectsFreeze(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[{"shortcut":"+dim-freeze","input":{"sheet_id":"sh1","dimension":"row","count":2}}]`,
		"--yes",
	})
	input := decodeToolInput(t, body, "batch_update")
	ops, _ := input["operations"].([]interface{})
	op := ops[0].(map[string]interface{})
	if op["tool_name"] != "modify_sheet_structure" {
		t.Errorf("tool_name = %v, want modify_sheet_structure", op["tool_name"])
	}
	in, _ := op["input"].(map[string]interface{})
	if in["operation"] != "freeze" {
		t.Errorf("operation = %v, want \"freeze\"", in["operation"])
	}
}

// TestBatchUpdate_ResizeNoOperationField covers the resize_range dispatch:
// mapping has no operationField, so input.operation must NOT be injected.
func TestBatchUpdate_ResizeNoOperationField(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[{"shortcut":"+rows-resize","input":{"sheet_id":"sh1","range":"1:3","type":"pixel","size":30}}]`,
		"--yes",
	})
	input := decodeToolInput(t, body, "batch_update")
	op := input["operations"].([]interface{})[0].(map[string]interface{})
	if op["tool_name"] != "resize_range" {
		t.Errorf("tool_name = %v, want resize_range", op["tool_name"])
	}
	in, _ := op["input"].(map[string]interface{})
	if _, has := in["operation"]; has {
		t.Errorf("operation should NOT be injected for resize_range; got %#v", in)
	}
}

// TestSplitSheetPrefixedRange exercises the helper directly.
func TestSplitSheetPrefixedRange(t *testing.T) {
	t.Parallel()
	sheet, sub, err := splitSheetPrefixedRange("sheet1!A2:A100")
	if err != nil || sheet != "sheet1" || sub != "A2:A100" {
		t.Errorf("split = (%q,%q,%v), want (sheet1, A2:A100, nil)", sheet, sub, err)
	}
	if _, _, err := splitSheetPrefixedRange("A2:A100"); err == nil {
		t.Error("expected error on missing prefix")
	}
	if _, _, err := splitSheetPrefixedRange("!A2"); err == nil {
		t.Error("expected error on empty sheet name")
	}
	// Compile-time use of json import
	_ = json.Marshal
}
