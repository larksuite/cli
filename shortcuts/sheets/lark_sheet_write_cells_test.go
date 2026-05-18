// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

func TestWriteCellsShortcuts_DryRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sc        common.Shortcut
		args      []string
		toolName  string
		wantInput map[string]interface{}
	}{
		{
			name: "+cells-set with --cells bare 2D array",
			sc:   CellsSet,
			args: []string{
				"--url", testURL, "--sheet-id", testSheetID,
				"--range", "A1:B2",
				"--cells", `[[{"value":1},{"value":2}],[{"value":3},{"value":4}]]`,
			},
			toolName: "set_cell_range",
			wantInput: map[string]interface{}{
				"excel_id": testToken,
				"sheet_id": testSheetID,
				"range":    "A1:B2",
				"cells":    []interface{}{[]interface{}{map[string]interface{}{"value": float64(1)}, map[string]interface{}{"value": float64(2)}}, []interface{}{map[string]interface{}{"value": float64(3)}, map[string]interface{}{"value": float64(4)}}},
			},
		},
		{
			name: "+cells-set --allow-overwrite=false sends false explicitly",
			sc:   CellsSet,
			args: []string{
				"--url", testURL, "--sheet-id", testSheetID,
				"--range", "A1",
				"--cells", `[[{"value":1}]]`,
				"--allow-overwrite=false",
			},
			toolName: "set_cell_range",
			wantInput: map[string]interface{}{
				"excel_id":        testToken,
				"sheet_id":        testSheetID,
				"range":           "A1",
				"cells":           []interface{}{[]interface{}{map[string]interface{}{"value": float64(1)}}},
				"allow_overwrite": false,
			},
		},
		{
			name: "+csv-put inline csv",
			sc:   CsvPut,
			args: []string{
				"--url", testURL, "--sheet-id", testSheetID,
				"--csv", "a,b,c\n1,2,3",
				"--start-cell", "B3",
			},
			toolName: "set_range_from_csv",
			wantInput: map[string]interface{}{
				"excel_id":   testToken,
				"sheet_id":   testSheetID,
				"csv":        "a,b,c\n1,2,3",
				"start_cell": "B3",
			},
		},
		{
			name: "+dropdown-set fans out cells matrix",
			sc:   DropdownSet,
			args: []string{
				"--url", testURL, "--sheet-id", testSheetID,
				"--range", "A2:A4",
				"--options", `["a","b"]`,
				"--multiple", "--highlight",
			},
			toolName: "set_cell_range",
			wantInput: map[string]interface{}{
				"excel_id": testToken,
				"sheet_id": testSheetID,
				"range":    "A2:A4",
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := parseDryRunBody(t, tt.sc, tt.args)
			got := decodeToolInput(t, body, tt.toolName)
			assertInputEquals(t, got, tt.wantInput)
		})
	}
}

// TestDropdownSet_CellsShape inspects the 3×1 matrix produced from
// --range A2:A4 to confirm the data_validation prototype is replicated.
func TestDropdownSet_CellsShape(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, DropdownSet, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A2:A4", "--options", `["a","b"]`, "--multiple",
	})
	input := decodeToolInput(t, body, "set_cell_range")
	cells, _ := input["cells"].([]interface{})
	if len(cells) != 3 {
		t.Fatalf("cells rows = %d, want 3 (A2:A4)", len(cells))
	}
	for i, row := range cells {
		r, _ := row.([]interface{})
		if len(r) != 1 {
			t.Errorf("row %d cols = %d, want 1", i, len(r))
		}
		cell, _ := r[0].(map[string]interface{})
		dv, _ := cell["data_validation"].(map[string]interface{})
		if dv == nil {
			t.Errorf("row %d cell missing data_validation: %#v", i, cell)
			continue
		}
		if dv["type"] != "list" {
			t.Errorf("row %d data_validation.type = %v, want list", i, dv["type"])
		}
		if dv["multiple_values"] != true {
			t.Errorf("row %d data_validation.multiple_values = %v, want true", i, dv["multiple_values"])
		}
	}
}

// TestCellsSetStyle_FlatFlags verifies that the 11 flat style flags +
// --border-styles compose into cell_styles + border_styles per cell.
func TestCellsSetStyle_FlatFlags(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, CellsSetStyle, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A1:B1",
		"--font-weight", "bold",
		"--background-color", "#ffff00",
		"--horizontal-alignment", "center",
		"--border-styles", `{"top":{"style":"thick"}}`,
	})
	input := decodeToolInput(t, body, "set_cell_range")
	cells, _ := input["cells"].([]interface{})
	row, _ := cells[0].([]interface{})
	cell, _ := row[0].(map[string]interface{})
	style, _ := cell["cell_styles"].(map[string]interface{})
	if style["font_weight"] != "bold" || style["background_color"] != "#ffff00" || style["horizontal_alignment"] != "center" {
		t.Errorf("cell_styles wrong: %#v", style)
	}
	if cell["border_styles"] == nil {
		t.Fatalf("border_styles missing on cell: %#v", cell)
	}
	if _, leaked := style["border_styles"]; leaked {
		t.Errorf("border_styles leaked into cell_styles: %#v", style)
	}
}

func TestCellsSetStyle_RequiresAtLeastOneFlag(t *testing.T) {
	t.Parallel()
	stdout, stderr, err := runShortcutCapturingErr(t, CellsSetStyle, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A1:B2", "--dry-run",
	})
	if err == nil || !strings.Contains(stdout+stderr+err.Error(), "at least one style flag") {
		t.Errorf("expected style-flag guard; got=%s|%s|%v", stdout, stderr, err)
	}
}

func TestCellsSet_RequiresJSONArray(t *testing.T) {
	t.Parallel()
	stdout, stderr, err := runShortcutCapturingErr(t, CellsSet, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A1", "--cells", `{"foo":"bar"}`, "--dry-run",
	})
	if err == nil {
		t.Fatalf("expected validation error; stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout+stderr+err.Error(), "must be a JSON array") {
		t.Errorf("expected JSON-array guard; got=%s|%s|%v", stdout, stderr, err)
	}
}

// TestCellsSetImage_DryRun verifies the 2-step plan (upload + embed) is
// rendered, including the parent_type=sheet_image upload metadata.
func TestCellsSetImage_DryRun(t *testing.T) {
	t.Parallel()
	calls := parseDryRunAPI(t, CellsSetImage, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A1",
		"--image", "./README.md", // any existing-shaped path; dry-run skips stat
	})
	if len(calls) != 2 {
		t.Fatalf("api calls = %d, want 2 (upload + set_cell_range)", len(calls))
	}
	upload := calls[0].(map[string]interface{})
	if upload["url"] != "/open-apis/drive/v1/medias/upload_all" {
		t.Errorf("upload url = %v", upload["url"])
	}
	ubody, _ := upload["body"].(map[string]interface{})
	if ubody["parent_type"] != "sheet_image" {
		t.Errorf("parent_type = %v, want sheet_image", ubody["parent_type"])
	}
	if ubody["parent_node"] != testToken {
		t.Errorf("parent_node = %v, want token", ubody["parent_node"])
	}

	embed := calls[1].(map[string]interface{})
	body, _ := embed["body"].(map[string]interface{})
	input := decodeToolInput(t, body, "set_cell_range")
	cells, _ := input["cells"].([]interface{})
	row, _ := cells[0].([]interface{})
	cell, _ := row[0].(map[string]interface{})
	rt, _ := cell["rich_text"].([]interface{})
	if len(rt) != 1 {
		t.Fatalf("rich_text len = %d, want 1", len(rt))
	}
	item, _ := rt[0].(map[string]interface{})
	if item["type"] != "embed-image" {
		t.Errorf("rich_text.type = %v, want embed-image", item["type"])
	}
	if item["attachment_name"] != "README.md" {
		t.Errorf("attachment_name = %v, want README.md (basename)", item["attachment_name"])
	}
}

func TestCellsSetImage_RangeMustBeSingleCell(t *testing.T) {
	t.Parallel()
	stdout, stderr, err := runShortcutCapturingErr(t, CellsSetImage, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A1:B2", "--image", "./foo.png", "--dry-run",
	})
	if err == nil || !strings.Contains(stdout+stderr+err.Error(), "must be exactly one cell") {
		t.Errorf("expected single-cell guard; got=%s|%s|%v", stdout, stderr, err)
	}
}

// TestRangeDimensions exercises the A1 parser's corner cases used by
// cells-set-style / dropdown-set / dim-resize.
func TestRangeDimensions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in        string
		wantRows  int
		wantCols  int
		wantErr   bool
	}{
		{"A1", 1, 1, false},
		{"A1:B2", 2, 2, false},
		{"sheet1!C3:E10", 8, 3, false},
		{"A:C", 0, 0, true},   // whole column not supported
		{"3:6", 0, 0, true},   // whole row not supported
		{"B2:A1", 0, 0, true}, // end before start
		{"", 0, 0, true},
	}
	var unusedSheet common.Shortcut = CellsSet // touch the common import
	_ = unusedSheet
	for _, c := range cases {
		rows, cols, err := rangeDimensions(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("rangeDimensions(%q): want error, got rows=%d cols=%d", c.in, rows, cols)
			}
			continue
		}
		if err != nil {
			t.Errorf("rangeDimensions(%q) unexpected error: %v", c.in, err)
		}
		if rows != c.wantRows || cols != c.wantCols {
			t.Errorf("rangeDimensions(%q) = (%d,%d), want (%d,%d)", c.in, rows, cols, c.wantRows, c.wantCols)
		}
	}
}
