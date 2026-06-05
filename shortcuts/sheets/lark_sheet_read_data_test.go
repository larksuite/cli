// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

func TestReadDataShortcuts_DryRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sc        common.Shortcut
		args      []string
		toolName  string
		wantInput map[string]interface{}
	}{
		{
			name:     "+cells-get single range + include=style,formula",
			sc:       CellsGet,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:B2", "--include", "style,formula"},
			toolName: "get_cell_ranges",
			wantInput: map[string]interface{}{
				"excel_id":            testToken,
				"sheet_id":            testSheetID,
				"ranges":              []interface{}{"A1:B2"},
				"include_styles":      true,
				"value_render_option": "formula",
				"cell_limit":          float64(unboundedReadLimit), // pinned high; --max-chars is the only cap
			},
		},
		{
			// Canonical form: --sheet-id + bare --range. Aligned with
			// +cells-get / +csv-get; before the e2e BUG-019 fix this
			// shortcut was the odd one out (range-prefix required).
			name:     "+dropdown-get with --sheet-id",
			sc:       DropdownGet,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "C2:C6"},
			toolName: "get_cell_ranges",
			wantInput: map[string]interface{}{
				"excel_id":            testToken,
				"sheet_id":            testSheetID,
				"ranges":              []interface{}{"C2:C6"},
				"include_styles":      false,
				"value_render_option": "formatted_value",
			},
		},
		{
			name:     "+dropdown-get with --sheet-name",
			sc:       DropdownGet,
			args:     []string{"--url", testURL, "--sheet-name", "Sheet1", "--range", "C2:C6"},
			toolName: "get_cell_ranges",
			wantInput: map[string]interface{}{
				"excel_id":            testToken,
				"sheet_name":          "Sheet1",
				"ranges":              []interface{}{"C2:C6"},
				"include_styles":      false,
				"value_render_option": "formatted_value",
			},
		},
		{
			// --rows-json is post-processing on +csv-get's response; it must
			// NOT leak into the get_range_as_csv input.
			name:     "+csv-get --rows-json builds the same input (flag is post-process)",
			sc:       CsvGet,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:C10", "--rows-json"},
			toolName: "get_range_as_csv",
			wantInput: map[string]interface{}{
				"excel_id": testToken,
				"sheet_id": testSheetID,
				"range":    "A1:C10",
				"max_rows": float64(unboundedReadLimit),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := parseDryRunBody(t, tt.sc, tt.args)
			got := decodeToolInput(t, body, tt.toolName)
			assertInputEquals(t, got, tt.wantInput)
		})
	}
}

// TestDropdownGet_RequiresSheetSelector locks the +cells-get-style
// selector contract: at least one of --sheet-id / --sheet-name must be
// supplied. Before BUG-019 fix this shortcut required a "Sheet!A1"
// prefix inside --range instead; the canonical selector pair is what
// every other get_cell_ranges wrapper uses.
func TestDropdownGet_RequiresSheetSelector(t *testing.T) {
	t.Parallel()
	stdout, stderr, err := runShortcutCapturingErr(t, DropdownGet, []string{
		"--url", testURL, "--range", "A2:A100", "--dry-run",
	})
	if err == nil {
		t.Fatalf("expected validation error; stdout=%s stderr=%s", stdout, stderr)
	}
	combined := stdout + stderr + err.Error()
	if !strings.Contains(combined, "sheet-id") && !strings.Contains(combined, "sheet-name") {
		t.Errorf("expected --sheet-id/--sheet-name guard; got=%s|%s|%v", stdout, stderr, err)
	}
}

// TestReadData_RequiresRange covers the trim-based --range guard on the
// single-range readers (--range "" slips past cobra's MarkFlagRequired but
// must still be rejected by Validate).
func TestReadData_RequiresRange(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		sc   common.Shortcut
	}{
		{"+cells-get", CellsGet},
		{"+csv-get", CsvGet},
		{"+dropdown-get", DropdownGet},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			stdout, stderr, err := runShortcutCapturingErr(t, c.sc, []string{
				"--url", testURL, "--sheet-id", testSheetID, "--range", "  ", "--dry-run",
			})
			if err == nil {
				t.Fatalf("expected validation error; stdout=%s stderr=%s", stdout, stderr)
			}
			if !strings.Contains(stdout+stderr+err.Error(), "--range is required") {
				t.Errorf("expected --range guard; got=%s|%s|%v", stdout, stderr, err)
			}
		})
	}
}

// TestInfoTypeFromInclude exercises the fine-grained → coarse mapping
// directly (white-box).
func TestInfoTypeFromInclude(t *testing.T) {
	t.Parallel()
	// Caller (sheetInfoInput) skips infoTypeFromInclude when len(include)==0,
	// so the helper only ever sees non-empty input.
	cases := []struct {
		include []string
		want    string
	}{
		{[]string{"row_heights"}, "row_heights_column_widths"},
		{[]string{"row_heights", "col_widths"}, "row_heights_column_widths"},
		{[]string{"hidden_rows", "hidden_cols"}, "hidden_infos"},
		{[]string{"groups"}, "group_infos"},
		{[]string{"merges"}, "merged_cells_infos"},
		{[]string{"row_heights", "merges"}, "all"}, // mixed
		{[]string{"frozen"}, "all"},                // frozen alone falls back to all
		{[]string{"unknown"}, "all"},               // unknown → all
	}
	for _, c := range cases {
		if got := infoTypeFromInclude(c.include); got != c.want {
			t.Errorf("infoTypeFromInclude(%v) = %q, want %q", c.include, got, c.want)
		}
	}
}

// TestCsvGet_StripRowPrefix verifies the client-side post-process for
// --include-row-prefix=false.
func TestCsvGet_StripRowPrefix(t *testing.T) {
	t.Parallel()
	in := map[string]interface{}{
		"annotated_csv": "[row=1] a,b,c\n[row=2] d,e,f",
		"other":         "untouched",
	}
	out := stripRowPrefixFromCsvOutput(in).(map[string]interface{})
	csv := out["annotated_csv"].(string)
	if csv != " a,b,c\n d,e,f" {
		t.Errorf("annotated_csv = %q, want stripped prefix", csv)
	}
	if out["other"] != "untouched" {
		t.Errorf("other field corrupted: %v", out["other"])
	}
}

// TestAssembleRowsJSON covers the --rows-json reshaping: every logical row
// emitted (no header singled out), integer row_number, column-letter keyed
// values, embedded newlines inside quoted fields, and current_region passthrough.
func TestAssembleRowsJSON(t *testing.T) {
	t.Parallel()
	in := map[string]interface{}{
		"annotated_csv":   "[row=1] 姓名,备注,时间差_分钟\n[row=2] 张三,\"line1\nline2\",8.5\n[row=3] 李四,ok,3",
		"current_region":  "A1:C3",
		"col_indices":     []interface{}{"A", "B", "C"},
		"row_indices":     []interface{}{1, 2, 3},
		"warning_message": "①定位行号…②定位列字母…",
	}
	out, ok := assembleRowsJSON(in, "A1:C3").(map[string]interface{})
	if !ok {
		t.Fatalf("assembleRowsJSON did not return a map")
	}

	// Fields whose info rows-json carries elsewhere are dropped (annotated_csv /
	// indices → rows; warning_message → moot static nag + structured
	// data_not_fully_read). Unrelated metadata like current_region is preserved.
	if _, exists := out["annotated_csv"]; exists {
		t.Errorf("annotated_csv should be dropped")
	}
	if _, exists := out["col_indices"]; exists {
		t.Errorf("col_indices should be dropped")
	}
	if _, exists := out["warning_message"]; exists {
		t.Errorf("warning_message should be dropped in rows-json mode")
	}
	if _, exists := out["columns"]; exists {
		t.Errorf("columns field should not exist (no header assumption)")
	}
	if out["current_region"] != "A1:C3" {
		t.Errorf("current_region passthrough lost: %v", out["current_region"])
	}

	rows, _ := out["rows"].([]map[string]interface{})
	if len(rows) != 3 {
		t.Fatalf("want all 3 rows (incl. row 1), got %d: %+v", len(rows), rows)
	}
	// Row 1 is emitted as a normal row, not consumed as a header.
	if rows[0]["row_number"].(int) != 1 {
		t.Errorf("first row_number = %v, want 1", rows[0]["row_number"])
	}
	if v := rows[0]["values"].(map[string]interface{}); v["A"] != "姓名" || v["C"] != "时间差_分钟" {
		t.Errorf("row 1 values wrong: %+v", v)
	}
	// Row 2 keeps its embedded newline inside a single cell.
	v1 := rows[1]["values"].(map[string]interface{})
	if rows[1]["row_number"].(int) != 2 || v1["A"] != "张三" || v1["B"] != "line1\nline2" || v1["C"] != "8.5" {
		t.Errorf("row 2 wrong (embedded newline?): %+v", rows[1])
	}
}

// TestAssembleRowsJSON_DerivedLetters verifies cell letters are derived from the
// range start when the tool omits col_indices (e.g. a C-anchored read).
func TestAssembleRowsJSON_DerivedLetters(t *testing.T) {
	t.Parallel()
	in := map[string]interface{}{
		"annotated_csv": "[row=5] h1,h2\n[row=6] a,b",
	}
	out := assembleRowsJSON(in, "C5:D6").(map[string]interface{})
	rows := out["rows"].([]map[string]interface{})
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	if rows[0]["row_number"].(int) != 5 {
		t.Errorf("first row_number = %v, want 5", rows[0]["row_number"])
	}
	if v := rows[0]["values"].(map[string]interface{}); v["C"] != "h1" || v["D"] != "h2" {
		t.Errorf("derived-letter values wrong: %+v", v)
	}
	if v := rows[1]["values"].(map[string]interface{}); v["C"] != "a" || v["D"] != "b" {
		t.Errorf("row 6 values wrong: %+v", v)
	}
}

// TestAssembleRowsJSON_DataNotFullyRead verifies the structured under-read hint:
// when current_region extends past actual_range, rows-json surfaces the true data
// range as a first-class field (mirroring the backend's prose warning).
func TestAssembleRowsJSON_DataNotFullyRead(t *testing.T) {
	t.Parallel()
	// Read only A1:D2, but the data region reaches D4 → 2 rows unread.
	in := map[string]interface{}{
		"annotated_csv":  "[row=1] 序号,姓名\n[row=2] 101,张三",
		"actual_range":   "A1:D2",
		"current_region": "A1:D4",
	}
	out := assembleRowsJSON(in, "A1:D2").(map[string]interface{})
	hint, ok := out["data_not_fully_read"].(map[string]interface{})
	if !ok {
		t.Fatalf("data_not_fully_read missing; out=%+v", out)
	}
	if hint["read_through_row"] != 2 || hint["data_extends_through_row"] != 4 ||
		hint["unread_rows"] != 2 || hint["reread_range"] != "A1:D4" {
		t.Errorf("data_not_fully_read wrong: %+v", hint)
	}

	// Fully-read case: no hint emitted.
	in2 := map[string]interface{}{
		"annotated_csv":  "[row=1] 序号,姓名\n[row=2] 101,张三",
		"actual_range":   "A1:D2",
		"current_region": "A1:D2",
	}
	out2 := assembleRowsJSON(in2, "A1:D2").(map[string]interface{})
	if _, exists := out2["data_not_fully_read"]; exists {
		t.Errorf("data_not_fully_read should be absent when fully read")
	}
}
