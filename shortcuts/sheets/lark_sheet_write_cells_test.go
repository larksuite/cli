// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"fmt"
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
			name: "+cells-set --copy-to-range passes copy_to_range",
			sc:   CellsSet,
			args: []string{
				"--url", testURL, "--sheet-id", testSheetID,
				"--range", "H2",
				"--cells", `[[{"formula":"=A2*B2"}]]`,
				"--copy-to-range", "H2:H100",
			},
			toolName: "set_cell_range",
			wantInput: map[string]interface{}{
				"excel_id":      testToken,
				"sheet_id":      testSheetID,
				"range":         "H2",
				"cells":         []interface{}{[]interface{}{map[string]interface{}{"formula": "=A2*B2"}}},
				"copy_to_range": "H2:H100",
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
				"--multiple",
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
// Also covers --colors / --highlight emitting the canonical
// `highlight_colors` / `enable_highlight` field names (not the legacy
// `colors` / `highlight_options`).
func TestDropdownSet_CellsShape(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, DropdownSet, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A2:A4", "--options", `["a","b"]`, "--multiple",
		"--colors", `["#FFE699","#bff7d9"]`, "--highlight",
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
		items, _ := dv["items"].([]interface{})
		if len(items) != 2 || items[0] != "a" || items[1] != "b" {
			t.Errorf("row %d data_validation.items = %#v, want [\"a\",\"b\"]", i, dv["items"])
		}
		if dv["support_multiple_values"] != true {
			t.Errorf("row %d data_validation.support_multiple_values = %v, want true", i, dv["support_multiple_values"])
		}
		if _, hasLegacy := dv["multiple_values"]; hasLegacy {
			t.Errorf("row %d data_validation should not emit legacy `multiple_values`", i)
		}
		colors, _ := dv["highlight_colors"].([]interface{})
		if len(colors) != 2 || colors[0] != "#FFE699" || colors[1] != "#bff7d9" {
			t.Errorf("row %d data_validation.highlight_colors = %#v, want [\"#FFE699\",\"#bff7d9\"]", i, dv["highlight_colors"])
		}
		if dv["enable_highlight"] != true {
			t.Errorf("row %d data_validation.enable_highlight = %v, want true", i, dv["enable_highlight"])
		}
		if _, hasLegacy := dv["colors"]; hasLegacy {
			t.Errorf("row %d data_validation should not emit legacy `colors`", i)
		}
		if _, hasLegacy := dv["highlight_options"]; hasLegacy {
			t.Errorf("row %d data_validation should not emit legacy `highlight_options`", i)
		}
	}
}

// TestDropdownSet_HighlightTriState pins down the tri-state semantics of
// --highlight after the server flipped enable_highlight's default from false
// to true. The translator uses runtime.Changed() to tell "user did not pass
// the flag" apart from "user passed --highlight=false":
//
//   - omitted          → no enable_highlight key in body (server applies its
//     new default = true)
//   - --highlight      → enable_highlight=true  (presence-only cobra form)
//   - --highlight=true → enable_highlight=true  (explicit form)
//   - --highlight=false → enable_highlight=false (the only way to opt out;
//     the documented "plain dropdown" path)
func TestDropdownSet_HighlightTriState(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		args      []string
		wantKey   bool
		wantValue bool
	}{
		{
			name:    "omitted leaves enable_highlight off the body",
			args:    []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A2", "--options", `["a","b"]`},
			wantKey: false,
		},
		{
			name:      "presence form (--highlight) stamps true",
			args:      []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A2", "--options", `["a","b"]`, "--highlight"},
			wantKey:   true,
			wantValue: true,
		},
		{
			name:      "explicit --highlight=true stamps true",
			args:      []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A2", "--options", `["a","b"]`, "--highlight=true"},
			wantKey:   true,
			wantValue: true,
		},
		{
			name:      "explicit --highlight=false stamps false (the opt-out path)",
			args:      []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A2", "--options", `["a","b"]`, "--highlight=false"},
			wantKey:   true,
			wantValue: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := parseDryRunBody(t, DropdownSet, tc.args)
			input := decodeToolInput(t, body, "set_cell_range")
			cells, _ := input["cells"].([]interface{})
			row0, _ := cells[0].([]interface{})
			cell, _ := row0[0].(map[string]interface{})
			dv, _ := cell["data_validation"].(map[string]interface{})
			got, has := dv["enable_highlight"]
			if has != tc.wantKey {
				t.Fatalf("enable_highlight key present = %v, want %v (dv = %#v)", has, tc.wantKey, dv)
			}
			if tc.wantKey && got != tc.wantValue {
				t.Errorf("enable_highlight = %v (%T), want %v", got, got, tc.wantValue)
			}
		})
	}
}

// TestDropdownSet_ColorsLongerThanOptions checks the early Validate-time
// error when --colors length exceeds the dropdown source size (options
// length in list mode). Equal-or-shorter lengths are accepted (server
// cycles the rest through a built-in palette).
func TestDropdownSet_ColorsLongerThanOptions(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, DropdownSet, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A2:A4",
		"--options", `["a","b"]`,
		"--colors", `["#FFE699","#bff7d9","#ffb3b3"]`,
		"--dry-run",
	})
	ve := requireValidation(t, err, "must not exceed dropdown source size")
	if ve.Param != "--colors" {
		t.Errorf("param = %q, want --colors", ve.Param)
	}
}

// TestDropdownSet_ColorsShorterAccepted verifies the partial-colors case:
// fewer colors than options is legal — array is forwarded as-is and the
// server fills remaining slots from its default palette.
func TestDropdownSet_ColorsShorterAccepted(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, DropdownSet, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A2:A4",
		"--options", `["a","b","c","d"]`,
		"--colors", `["#FFE699","#bff7d9"]`,
	})
	input := decodeToolInput(t, body, "set_cell_range")
	cells, _ := input["cells"].([]interface{})
	row0, _ := cells[0].([]interface{})
	cell, _ := row0[0].(map[string]interface{})
	dv, _ := cell["data_validation"].(map[string]interface{})
	colors, _ := dv["highlight_colors"].([]interface{})
	if len(colors) != 2 {
		t.Errorf("highlight_colors length = %d, want 2 (forwarded as-is)", len(colors))
	}
}

// TestDropdownSet_ListFromRange verifies --source-range emits
// data_validation.type=listFromRange + data_validation.range, paired with
// --colors / --highlight propagating to highlight_colors / enable_highlight.
func TestDropdownSet_ListFromRange(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, DropdownSet, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "B2:B21",
		"--source-range", "Sheet1!T1:T3",
		"--colors", `["#cce8ff","#ffd6e7","#e6e6e6"]`,
		"--highlight",
	})
	input := decodeToolInput(t, body, "set_cell_range")
	cells, _ := input["cells"].([]interface{})
	row0, _ := cells[0].([]interface{})
	cell, _ := row0[0].(map[string]interface{})
	dv, _ := cell["data_validation"].(map[string]interface{})
	if dv["type"] != "listFromRange" {
		t.Errorf("data_validation.type = %v, want listFromRange", dv["type"])
	}
	if dv["range"] != "Sheet1!T1:T3" {
		t.Errorf("data_validation.range = %v, want Sheet1!T1:T3 (verbatim, server normalizes)", dv["range"])
	}
	if _, hasItems := dv["items"]; hasItems {
		t.Errorf("listFromRange mode should not emit `items`: %#v", dv)
	}
	if dv["enable_highlight"] != true {
		t.Errorf("data_validation.enable_highlight = %v, want true", dv["enable_highlight"])
	}
	colors, _ := dv["highlight_colors"].([]interface{})
	if len(colors) != 3 {
		t.Errorf("highlight_colors length = %d, want 3", len(colors))
	}
}

// TestDropdownSet_ListFromRange_ColorsLongerThanCells rejects --colors
// longer than the source range cell count (T1:T3 has 3 cells, 4 colors
// must be refused).
func TestDropdownSet_ListFromRange_ColorsLongerThanCells(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, DropdownSet, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "B2:B21",
		"--source-range", "Sheet1!T1:T3",
		"--colors", `["#a","#b","#c","#d"]`,
		"--highlight",
		"--dry-run",
	})
	ve := requireValidation(t, err, "must not exceed dropdown source size")
	if ve.Param != "--colors" {
		t.Errorf("param = %q, want --colors", ve.Param)
	}
}

// TestDropdownSet_XorBothSet rejects passing both --options and
// --source-range.
func TestDropdownSet_XorBothSet(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, DropdownSet, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "B2:B21",
		"--options", `["a","b"]`,
		"--source-range", "Sheet1!T1:T3",
		"--dry-run",
	})
	requireValidation(t, err, "mutually exclusive")
}

// TestDropdownSet_XorNeitherSet rejects passing neither --options nor
// --source-range.
func TestDropdownSet_XorNeitherSet(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, DropdownSet, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "B2:B21",
		"--dry-run",
	})
	requireValidation(t, err, "one of --options")
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
		"--border-styles", `{"top":{"style":"solid"}}`,
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
	_, _, err := runShortcutCapturingErr(t, CellsSetStyle, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A1:B2", "--dry-run",
	})
	requireValidation(t, err, "at least one style flag")
}

func TestCellsSet_RequiresJSONArray(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, CellsSet, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A1", "--cells", `{"foo":"bar"}`, "--dry-run",
	})
	// Schema validator fires first now: "--cells: expected type \"array\", got \"object\"".
	// Either the schema phrasing or the legacy requireJSONArray phrasing is
	// acceptable — both pin the same contract.
	ve := requireValidation(t, err, "")
	if !strings.Contains(ve.Message, `expected type "array"`) && !strings.Contains(ve.Message, "must be a JSON array") {
		t.Errorf("expected array-type guard; got message=%q", ve.Message)
	}
}

// TestCellsSet_RejectsUnsupportedMentionType pins the mention_type enum in
// data/flag-schemas.json (synced from the upstream tool schema): a rich_text
// mention whose mention_type is outside MENTION_FILE_TYPE (here 6 = cloud
// shared folder) is rejected by the schema validator at flag-parse time,
// before it can reach the server and blow up pb serialization
// ("mentionFileInfo.fileType: enum value expected").
func TestCellsSet_RejectsUnsupportedMentionType(t *testing.T) {
	t.Parallel()
	cells := `[[{"rich_text":[{"type":"mention","text":"x","mention_type":6,"mention_token":"t"}]}]]`
	_, _, err := runShortcutCapturingErr(t, CellsSet, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A1", "--cells", cells, "--dry-run",
	})
	ve := requireValidation(t, err, "mention_type")
	if !strings.Contains(ve.Message, "not in enum") {
		t.Errorf("expected enum guard; got message=%q", ve.Message)
	}
}

// TestCellsSet_AllowsValidMentionTypes confirms the guard lets through a
// user @mention (mention_type 0) and a render-supported file type (22 = DOCX).
func TestCellsSet_AllowsValidMentionTypes(t *testing.T) {
	t.Parallel()
	for _, mt := range []int{0, 22} {
		cells := fmt.Sprintf(`[[{"rich_text":[{"type":"mention","text":"x","mention_type":%d,"mention_token":"t"}]}]]`, mt)
		stdout, stderr, err := runShortcutCapturingErr(t, CellsSet, []string{
			"--url", testURL, "--sheet-id", testSheetID,
			"--range", "A1", "--cells", cells, "--dry-run",
		})
		if err != nil {
			t.Errorf("mention_type %d: unexpected error: stdout=%s stderr=%s err=%v", mt, stdout, stderr, err)
		}
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
	if item["image_token"] != "<file_token>" {
		t.Errorf("image_token = %v, want <file_token>", item["image_token"])
	}
	if item["text"] != "" {
		t.Errorf("text = %v, want empty string", item["text"])
	}
	if item["image_width"] != "<image_width>" {
		t.Errorf("image_width = %v, want <image_width>", item["image_width"])
	}
	if item["image_height"] != "<image_height>" {
		t.Errorf("image_height = %v, want <image_height>", item["image_height"])
	}
}

// TestCellsSetImage_DryRunOfficeParentType confirms that an imported "office"
// spreadsheet (token prefixed with "fake_office_") uploads with
// parent_type=office_sheet_file instead of the native sheet_image, and that the
// preview's parent_node carries the same token.
func TestCellsSetImage_DryRunOfficeParentType(t *testing.T) {
	t.Parallel()
	const officeToken = "fake_office_abc123"
	calls := parseDryRunAPI(t, CellsSetImage, []string{
		"--spreadsheet-token", officeToken, "--sheet-id", testSheetID,
		"--range", "A1",
		"--image", "./README.md", // any existing-shaped path; dry-run skips stat
	})
	if len(calls) != 2 {
		t.Fatalf("api calls = %d, want 2 (upload + set_cell_range)", len(calls))
	}
	upload := calls[0].(map[string]interface{})
	ubody, _ := upload["body"].(map[string]interface{})
	if ubody["parent_type"] != officeSheetFileParentType {
		t.Errorf("parent_type = %v, want %s", ubody["parent_type"], officeSheetFileParentType)
	}
	if ubody["parent_node"] != officeToken {
		t.Errorf("parent_node = %v, want %s", ubody["parent_node"], officeToken)
	}
}

func TestCellsSetImage_RangeMustBeSingleCell(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, CellsSetImage, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A1:B2", "--image", "./foo.png", "--dry-run",
	})
	ve := requireValidation(t, err, "must be exactly one cell")
	if ve.Param != "--range" {
		t.Errorf("param = %q, want --range", ve.Param)
	}
}

// TestCellsSetImage_DryRunRejectsUnsafePath guards that an unsafe --image path
// (e.g. an absolute path) is rejected during Validate, so --dry-run fails the
// same way as a real run instead of printing a misleading success preview.
func TestCellsSetImage_DryRunRejectsUnsafePath(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, CellsSetImage, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A1", "--image", "/etc/hosts", "--dry-run",
	})
	ve := requireValidation(t, err, "must be a relative path")
	if ve.Param != "--image" {
		t.Errorf("param = %q, want --image", ve.Param)
	}
}

// TestRangeDimensions exercises the A1 parser's corner cases used by
// cells-set-style / dropdown-set / rows-resize / cols-resize.
func TestRangeDimensions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in       string
		wantRows int
		wantCols int
		wantErr  bool
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
