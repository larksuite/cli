// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"strings"
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
		  {"shortcut":"+dim-insert","input":{"sheet_id":"sh1","range":"1:3"}}
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

// TestDropdownUpdate_BatchPayload verifies the multi-range dropdown
// update fans out into a single batch_update with one set_cell_range
// op per range.
func TestDropdownUpdate_BatchPayload(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, DropdownUpdate, []string{
		"--url", testURL,
		"--ranges", `["sheet1!A2:A5","sheet1!C2:C5"]`,
		"--options", `["a","b","c"]`,
		"--multiple",
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
		if dv["multiple_values"] != true {
			t.Errorf("op[%d] multiple_values = %v, want true", i, dv["multiple_values"])
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
	stdout, stderr, err := runShortcutCapturingErr(t, DropdownUpdate, []string{
		"--url", testURL,
		"--ranges", `["A2:A5"]`,
		"--options", `["a"]`,
		"--dry-run",
	})
	if err == nil || !strings.Contains(stdout+stderr+err.Error(), "must include a sheet prefix") {
		t.Errorf("expected sheet-prefix guard for +dropdown-update; got=%s|%s|%v", stdout, stderr, err)
	}

	// batch-update with empty operations
	stdout, stderr, err = runShortcutCapturingErr(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[]`,
		"--yes",
		"--dry-run",
	})
	if err == nil || !strings.Contains(stdout+stderr+err.Error(), "non-empty JSON array") {
		t.Errorf("expected empty-operations guard; got=%s|%s|%v", stdout, stderr, err)
	}

	// dropdown-update with non-array --options (object instead) → array guard
	stdout, stderr, err = runShortcutCapturingErr(t, DropdownUpdate, []string{
		"--url", testURL,
		"--ranges", `["sheet1!A1:A2"]`,
		"--options", `{"not":"array"}`,
		"--dry-run",
	})
	if err == nil || !strings.Contains(stdout+stderr+err.Error(), "must be a JSON array") {
		t.Errorf("expected JSON array guard; got=%s|%s|%v", stdout, stderr, err)
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
			name:      "legacy v2 +dim-move rejected",
			opsJSON:   `[{"shortcut":"+dim-move","input":{}}]`,
			wantMatch: "not allowed in +batch-update",
		},
		{
			name:      "user filled operation manually",
			opsJSON:   `[{"shortcut":"+dim-insert","input":{"operation":"delete","range":"1:1"}}]`,
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stdout, stderr, err := runShortcutCapturingErr(t, BatchUpdate, []string{
				"--url", testURL,
				"--operations", tc.opsJSON,
				"--yes",
				"--dry-run",
			})
			if err == nil {
				t.Fatalf("expected error containing %q; got stdout=%s stderr=%s", tc.wantMatch, stdout, stderr)
			}
			if !strings.Contains(stdout+stderr+err.Error(), tc.wantMatch) {
				t.Errorf("expected error containing %q; got: %s | %s | %v", tc.wantMatch, stdout, stderr, err)
			}
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
		"--operations", `[{"shortcut":"+dim-freeze","input":{"sheet_id":"sh1","freeze_rows":2}}]`,
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
		"--operations", `[{"shortcut":"+rows-resize","input":{"sheet_id":"sh1","range":"1:3","resize_height":{"type":"pixel","value":30}}}]`,
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
