// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"strconv"
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

func TestBatchUpdate_DimInsertInheritAfterCopiesFollowingStyle(t *testing.T) {
	t.Parallel()

	body := parseDryRunBody(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[
		  {"shortcut":"+dim-insert","input":{"sheet_id":"sh1","position":"D","count":1,"inherit_style":"after"}}
		]`,
		"--yes",
	})
	input := decodeToolInput(t, body, "batch_update")
	ops, _ := input["operations"].([]interface{})
	if len(ops) != 1 {
		t.Fatalf("operations length = %d, want 1", len(ops))
	}
	op := ops[0].(map[string]interface{})
	if op["tool_name"] != "modify_sheet_structure" {
		t.Fatalf("tool_name = %v, want modify_sheet_structure", op["tool_name"])
	}
	in, _ := op["input"].(map[string]interface{})
	// inherit_style=after copies the following column's style via a plain
	// before-insert at the same position (the backend anchors on the following
	// column), so position stays D with side=before.
	assertInputEquals(t, in, map[string]interface{}{
		"excel_id":  testToken,
		"sheet_id":  "sh1",
		"operation": "insert",
		"position":  "D",
		"count":     float64(1),
		"side":      "before",
	})
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
		{
			name:      "wrapped cell_styles structure",
			opsJSON:   `[{"shortcut":"+cells-set-style","input":{"sheet_name":"s","range":"A1","cell_styles":{"background_color":"#EBF1F8"}}}]`,
			wantMatch: "do not wrap in cell_styles",
		},
		{
			name:      "wrapped styles structure",
			opsJSON:   `[{"shortcut":"+cells-set-style","input":{"sheet_name":"s","range":"A1","styles":{"font_weight":"bold"}}}]`,
			wantMatch: "do not wrap in styles",
		},
		{
			name:      "wrapped cell_merges structure",
			opsJSON:   `[{"shortcut":"+cells-set-style","input":{"sheet_name":"s","range":"A1","cell_merges":[{"range":"A1:B1"}]}}]`,
			wantMatch: "do not wrap in cell_merges",
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

// TestBatchUpdate_FlattenedStyleKeysNotMistakenForWrapper guards the
// wrapped-structure rejection against overreach: the same style fields in
// their correct flattened form must translate cleanly — only the wrapper
// container keys (cell_styles / styles / cell_merges) are rejected.
func TestBatchUpdate_FlattenedStyleKeysNotMistakenForWrapper(t *testing.T) {
	t.Parallel()
	got, err := translateBatchOp(map[string]interface{}{
		"shortcut": "+cells-set-style",
		"input": map[string]interface{}{
			"sheet_name":       "s",
			"range":            "A1",
			"background_color": "#EBF1F8",
			"font_weight":      "bold",
		},
	}, testToken, 0)
	if err != nil {
		t.Fatalf("flattened style keys must pass the wrapper check, got %v", err)
	}
	input := got["input"].(map[string]interface{})
	cells := input["cells"].([][]interface{})
	style := cells[0][0].(map[string]interface{})["cell_styles"].(map[string]interface{})
	if style["background_color"] != "#EBF1F8" || style["font_weight"] != "bold" {
		t.Fatalf("translated style = %#v", style)
	}
}

// TestBatchUpdate_WrapperKeysDisjointFromSubOpFlags locks the static
// assumption wrappedSubOpInputKeys relies on: no shortcut registered in
// batchOpDispatch declares a flag named cell_styles / cell_merges / styles.
// If a future dispatch-table addition (e.g. +table-put) carries one of these
// flags, its legitimate input would be silently rejected by the wrapper
// check — this test turns that silent breakage into a build-time failure.
func TestBatchUpdate_WrapperKeysDisjointFromSubOpFlags(t *testing.T) {
	t.Parallel()
	wrapped := make(map[string]struct{}, len(wrappedSubOpInputKeys))
	for _, k := range wrappedSubOpInputKeys {
		wrapped[k] = struct{}{}
	}
	for shortcut := range batchOpDispatch {
		for _, f := range flagsFor(shortcut) {
			key := strings.ReplaceAll(f.Name, "-", "_")
			if _, clash := wrapped[key]; clash {
				t.Errorf("%s declares flag --%s which collides with wrappedSubOpInputKeys; "+
					"exempt this shortcut from the wrapper check before adding it to batchOpDispatch",
					shortcut, f.Name)
			}
		}
	}
}

// TestBatchUpdate_AggregatesMultipleOpErrors pins op-level aggregation: when
// several operations are invalid, one reply names them all (numbered, with
// each op's own error) instead of failing on the first bad op only. A single
// bad op keeps the historical single-error message (no aggregate wrapper).
func TestBatchUpdate_AggregatesMultipleOpErrors(t *testing.T) {
	t.Parallel()

	t.Run("two bad ops reported together", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, BatchUpdate, []string{
			"--url", testURL,
			"--operations", `[
				{"shortcut":"+cells-set-magic","input":{}},
				{"shortcut":"+cells-set","input":{"sheet_name":"s","range":"A1"}},
				{"shortcut":"+cells-clear","input":{"sheet_name":"s","range":"A1"}}
			]`,
			"--yes", "--dry-run",
		})
		requireValidation(t, err, "2 of 3 operations failed validation")
		for _, want := range []string{"1) ", "2) ", "operations[0]", "operations[1]"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("aggregated op error should contain %q, got %q", want, err.Error())
			}
		}
	})

	t.Run("single bad op keeps plain message", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, BatchUpdate, []string{
			"--url", testURL,
			"--operations", `[
				{"shortcut":"+cells-set-magic","input":{}},
				{"shortcut":"+cells-clear","input":{"sheet_name":"s","range":"A1"}}
			]`,
			"--yes", "--dry-run",
		})
		requireValidation(t, err, "not allowed in +batch-update")
		if strings.Contains(err.Error(), "operations failed validation") {
			t.Errorf("single bad op must not get the aggregate wrapper, got %q", err.Error())
		}
	})
}

// TestBatchUpdate_PrescriptiveHints pins the recovery hints that ride on the
// highest-frequency batch failures, so an agent can repair its payload in a
// single retry without --help / --print-schema round trips.
func TestBatchUpdate_PrescriptiveHints(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		opsJSON    string
		wantMatch  string
		wantInHint []string
	}{
		{
			name:       "missing shortcut gets entry template",
			opsJSON:    `[{"input":{"range":"A1"}}]`,
			wantMatch:  "'shortcut' field is required",
			wantInHint: []string{`{"shortcut":"+cells-set"`, `"input"`},
		},
		{
			name:       "disallowed shortcut lists the allow-list inline",
			opsJSON:    `[{"shortcut":"+cells-batch-set-style","input":{}}]`,
			wantMatch:  "not allowed in +batch-update",
			wantInHint: []string{"allowed shortcuts:", "+cells-set-style", "+range-copy"},
		},
		{
			name:       "translator failure lists full key contract",
			opsJSON:    `[{"shortcut":"+dim-insert","input":{"sheet_name":"s"}}]`,
			wantMatch:  "--position is required",
			wantInHint: []string{"+dim-insert input keys:", "sheet_id|sheet_name (choose one)", "position (required)", "count (required)", "inherit_style"},
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
			ve := requireValidation(t, err, tc.wantMatch)
			for _, want := range tc.wantInHint {
				if !strings.Contains(ve.Hint, want) {
					t.Errorf("hint should contain %q, got %q", want, ve.Hint)
				}
			}
		})
	}
}

// TestTranslateBatchOperations_OverLimitSplitHint pins the split
// prescription on the 100-entry cap: the hint must say how many batches
// the caller should re-issue.
func TestTranslateBatchOperations_OverLimitSplitHint(t *testing.T) {
	t.Parallel()
	ops := make([]interface{}, 185)
	for i := range ops {
		ops[i] = map[string]interface{}{"shortcut": "+cells-set", "input": map[string]interface{}{}}
	}
	_, err := translateBatchOperations(ops, "shtcnX")
	ve := requireValidation(t, err, "accepts at most 100 entries; got 185")
	for _, want := range []string{"2 separate +batch-update calls", "at most 100 entries each"} {
		if !strings.Contains(ve.Hint, want) {
			t.Errorf("hint should contain %q, got %q", want, ve.Hint)
		}
	}
}

func TestTranslateBatchOperations_AggregateCellCap(t *testing.T) {
	ops := []interface{}{
		map[string]interface{}{
			"shortcut": "+cells-set-style",
			"input": map[string]interface{}{
				"sheet-id": "sh1", "range": "A1:A100001", "font-weight": "bold",
			},
		},
		map[string]interface{}{
			"shortcut": "+cells-set-style",
			"input": map[string]interface{}{
				"sheet-id": "sh1", "range": "A1:A100000", "font-weight": "bold",
			},
		},
	}
	_, err := translateBatchOperations(ops, "shtcnX")
	ve := requireValidation(t, err, "materialize 200001 cells total")
	if ve.Param != "--operations" {
		t.Fatalf("param = %q, want --operations", ve.Param)
	}
}

// TestSubOpInputContract pins the contract line derivation from flag-defs:
// reserved spreadsheet locators are omitted, the sheet selector collapses
// to a choose-one, and required flags are marked.
func TestSubOpInputContract(t *testing.T) {
	t.Parallel()
	got := subOpInputContract("+dim-insert")
	for _, want := range []string{"sheet_id|sheet_name (choose one)", "position (required)", "count (required)", "inherit_style"} {
		if !strings.Contains(got, want) {
			t.Errorf("contract should contain %q, got %q", want, got)
		}
	}
	for _, banned := range []string{"url", "spreadsheet_token", "dry_run"} {
		if strings.Contains(got, banned) {
			t.Errorf("contract must not expose %q, got %q", banned, got)
		}
	}
	if got := subOpInputContract("+no-such-shortcut"); got != "" {
		t.Errorf("unknown shortcut should yield empty contract, got %q", got)
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
		"--operations", `[{"shortcut":"+rows-resize","input":{"sheet_id":"sh1","range":"1:3","height":30}}]`,
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

// TestSplitSheetPrefixedRange exercises the helper directly, including the
// grammar it shares with --range's selector rewrite: the sheet it returns is
// written verbatim into a sub-op's "sheet_name", so a quoted name has to come
// back unwrapped and every spelling of the separator has to be recognized.
func TestSplitSheetPrefixedRange(t *testing.T) {
	t.Parallel()
	ok := []struct{ in, sheet, sub string }{
		{"sheet1!A2:A100", "sheet1", "A2:A100"},
		{"工作表1！A1:B2", "工作表1", "A1:B2"},           // full-width separator
		{`sheet1\!A2`, "sheet1", "A2"},            // survived shell history expansion
		{"'My Sheet'!A1:B2", "My Sheet", "A1:B2"}, // quotes are the delimiter, not the name
		{"'Q1!Sales'!A1", "Q1!Sales", "A1"},       // separator inside a quoted name
		{"  sheet1!A2  ", "sheet1", "A2"},
	}
	for _, tc := range ok {
		sheet, sub, err := splitSheetPrefixedRange(tc.in)
		if err != nil || sheet != tc.sheet || sub != tc.sub {
			t.Errorf("split(%q) = (%q,%q,%v), want (%q,%q,nil)", tc.in, sheet, sub, err, tc.sheet, tc.sub)
		}
	}
	for _, in := range []string{"A2:A100", "!A2", "sheet1!", "'unclosed!A1"} {
		_, _, err := splitSheetPrefixedRange(in)
		// Typed metadata, not just "an error": the flag attribution is what
		// lets a caller find what to fix, and a plain error would otherwise
		// pass this regression test.
		ve := requireValidation(t, err, "must use sheet!range form")
		if ve.Param != "--range" {
			t.Errorf("split(%q): Param = %q, want --range", in, ve.Param)
		}
		if !strings.Contains(ve.Message, strconv.Quote(in)) {
			t.Errorf("split(%q): message %q does not name the offending input", in, ve.Message)
		}
	}
	// Compile-time use of json import
	_ = json.Marshal
}

// TestValidateDropdownRanges_AcceptsPrefixGrammar pins the two --ranges items
// the literal-"!" scan got wrong: a full-width separator was rejected as
// carrying no prefix at all, and a quoted name shipped its quotes inside
// sheet_name for the backend to fail on as sheet-not-found.
func TestValidateDropdownRanges_AcceptsPrefixGrammar(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, ranges, wantSheet string }{
		{"full-width separator", `["工作表1！A1:B2"]`, "工作表1"},
		{"quoted sheet name", `["'My Sheet'!A1:B2"]`, "My Sheet"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := parseDryRunBody(t, CellsBatchClear, []string{
				"--url", testURL,
				"--ranges", tc.ranges,
				"--yes",
			})
			input := decodeToolInput(t, body, "batch_update")
			ops, _ := input["operations"].([]interface{})
			if len(ops) != 1 {
				t.Fatalf("operations length = %d, want 1", len(ops))
			}
			params, _ := ops[0].(map[string]interface{})["input"].(map[string]interface{})
			if params["sheet_name"] != tc.wantSheet {
				t.Errorf("sheet_name = %q, want %q", params["sheet_name"], tc.wantSheet)
			}
			if params["range"] != "A1:B2" {
				t.Errorf("range = %q, want A1:B2", params["range"])
			}
		})
	}
}

// TestBatchUpdate_CollidingDimFreezeWarns covers the failure mode the legacy
// deprecation note alone could not surface: two +dim-freeze sub-ops on one
// sheet. Freeze is full-state replacement, so the second silently discards the
// first — and BOTH report success, which is why it goes unnoticed. Per-op
// "equivalent to --rows 1" / "equivalent to --cols 2" notes do not say that;
// the caller has to infer the interaction. This pins that the CLI states it.
func TestBatchUpdate_CollidingDimFreezeWarns(t *testing.T) {
	t.Parallel()

	t.Run("two per-axis freezes on one sheet", func(t *testing.T) {
		t.Parallel()
		warning := dryRunWarning(t, BatchUpdate, []string{
			"--url", testURL,
			"--operations", `[
			  {"shortcut":"+dim-freeze","input":{"sheet_name":"S1","rows":1}},
			  {"shortcut":"+dim-freeze","input":{"sheet_name":"S1","cols":2}}
			]`,
			"--yes",
		})
		for _, want := range []string{
			"operations[0], operations[1]",
			"only operations[1] survives",
			"--cols 2)",                     // the state actually reached
			"ONE sub-op: --rows 1 --cols 2", // the fix
		} {
			if !strings.Contains(warning, want) {
				t.Errorf("collision warning should contain %q, got %q", want, warning)
			}
		}
	})

	t.Run("legacy spelling collides the same way and keeps its own note", func(t *testing.T) {
		t.Parallel()
		warning := dryRunWarning(t, BatchUpdate, []string{
			"--url", testURL,
			"--operations", `[
			  {"shortcut":"+dim-freeze","input":{"sheet_name":"S1","dimension":"row","count":1}},
			  {"shortcut":"+dim-freeze","input":{"sheet_name":"S1","dimension":"column","count":2}}
			]`,
			"--yes",
		})
		if !strings.Contains(warning, "ONE sub-op: --rows 1 --cols 2") {
			t.Errorf("legacy spelling should collide too, got %q", warning)
		}
		if !strings.Contains(warning, "superseded by --rows/--cols") {
			t.Errorf("per-op deprecation note should still ride along, got %q", warning)
		}
	})

	t.Run("different sheets do not collide", func(t *testing.T) {
		t.Parallel()
		warning := dryRunWarning(t, BatchUpdate, []string{
			"--url", testURL,
			"--operations", `[
			  {"shortcut":"+dim-freeze","input":{"sheet_name":"S1","rows":1}},
			  {"shortcut":"+dim-freeze","input":{"sheet_name":"S2","cols":2}}
			]`,
			"--yes",
		})
		if strings.Contains(warning, "same sheet") {
			t.Errorf("freezes on different sheets are independent, got %q", warning)
		}
	})

	t.Run("a single freeze warns about nothing", func(t *testing.T) {
		t.Parallel()
		warning := dryRunWarning(t, BatchUpdate, []string{
			"--url", testURL,
			"--operations", `[{"shortcut":"+dim-freeze","input":{"sheet_name":"S1","rows":1,"cols":2}}]`,
			"--yes",
		})
		if warning != "" {
			t.Errorf("one combined freeze is the correct form, got warning %q", warning)
		}
	})
}

// TestBatchOpAliasCollidesWithTarget pins the message for a sub-op carrying
// BOTH an intuitive alias and the flag it aliases. The key is recognized, so
// reporting it as "unknown input key" (which it did, because keys are walked
// in sorted order and "size" sorts before "width", leaving nothing to conflict
// with yet) sent the caller looking for a typo that was not there.
func TestBatchOpAliasCollidesWithTarget(t *testing.T) {
	t.Parallel()

	t.Run("conflicting values name both spellings", func(t *testing.T) {
		t.Parallel()
		input := map[string]interface{}{"sheet_name": "S1", "range": "A:C", "size": float64(100), "width": float64(120)}
		err := normalizeSubOpInputKeys("+cols-resize", input)
		if err == nil {
			t.Fatal("want an error for size + width with different values")
		}
		for _, want := range []string{`"size"`, `"width"`, "same flag"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error should contain %q, got %q", want, err.Error())
			}
		}
		if strings.Contains(err.Error(), "unknown input key") {
			t.Errorf("an aliased key is not unknown, got %q", err.Error())
		}
	})

	t.Run("same value under both spellings drops the alias", func(t *testing.T) {
		t.Parallel()
		input := map[string]interface{}{"sheet_name": "S1", "range": "A:C", "size": float64(120), "width": float64(120)}
		if err := normalizeSubOpInputKeys("+cols-resize", input); err != nil {
			t.Fatalf("identical values are harmless, got %v", err)
		}
		if _, still := input["size"]; still {
			t.Errorf("the alias should be dropped, got %#v", input)
		}
		if input["width"] != float64(120) {
			t.Errorf("width = %#v, want 120", input["width"])
		}
	})
}

// TestBatchUpdate_AggregatedErrorsKeepHints pins that folding several bad
// sub-ops into one message does not cost the caller the per-shortcut key
// contract each single-op error carries — otherwise the more mistakes you
// make, the less guidance you get.
func TestBatchUpdate_AggregatedErrorsKeepHints(t *testing.T) {
	t.Parallel()

	_, _, err := runShortcutCapturingErr(t, BatchUpdate, []string{
		"--url", testURL, "--yes",
		"--operations", `[
		  {"shortcut":"+cells-set","input":{"sheet_name":"S1","bogus":1}},
		  {"shortcut":"+cells-clear","input":{"sheet_name":"S1","nope":2}}
		]`,
	})
	ve := requireValidation(t, err, "2 of 2 operations failed validation")
	for _, want := range []string{
		"+cells-set input keys:",
		"+cells-clear input keys:",
	} {
		if !strings.Contains(ve.Message, want) {
			t.Errorf("aggregated message should inline %q, got %q", want, ve.Message)
		}
	}
}
