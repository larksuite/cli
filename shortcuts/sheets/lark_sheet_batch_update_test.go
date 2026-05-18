// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBatchUpdate_RawPassthrough verifies +batch-update threads
// --data.operations into the tool input as-is and honors
// --continue-on-error.
func TestBatchUpdate_RawPassthrough(t *testing.T) {
	t.Parallel()

	body := parseDryRunBody(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[{"tool":"set_cell_range","params":{"excel_id":"shtcnTOK","sheet_id":"sh1","range":"A1","cells":[[{"value":42}]]}}]`,
		"--continue-on-error",
		"--yes",
	})
	input := decodeToolInput(t, body, "batch_update")
	ops, _ := input["operations"].([]interface{})
	if len(ops) != 1 {
		t.Fatalf("operations length = %d, want 1", len(ops))
	}
	if input["continue_on_error"] != true {
		t.Errorf("continue_on_error = %v, want true", input["continue_on_error"])
	}
}

func TestBatchUpdate_HighRiskWriteRequiresYes(t *testing.T) {
	t.Parallel()
	stdout, stderr, err := runShortcutCapturingErr(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[{"tool":"set_cell_range","params":{}}]`,
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
		if op["tool"] != "set_cell_range" {
			t.Errorf("op[%d].tool = %v, want set_cell_range", i, op["tool"])
		}
		params, _ := op["params"].(map[string]interface{})
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
		params, _ := op["params"].(map[string]interface{})
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
	params, _ := op["params"].(map[string]interface{})
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
	cases := []struct {
		name string
		sc   interface{ shortcut() }
		args []string
		want string
	}{}
	_ = cases

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
