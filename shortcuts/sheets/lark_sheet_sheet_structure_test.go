// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

// TestSheetStructureShortcuts_DryRun covers all 8 shortcuts in
// lark_sheet_sheet_structure (sheet-info + 7 dim-*) and verifies that the
// CLI's A1-style --range / --position / --count flags map straight through
// to the tool's `range` / `position` / `count` fields (or are normalised
// per shortcut's wire shape).
func TestSheetStructureShortcuts_DryRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sc        common.Shortcut
		args      []string
		toolName  string
		wantInput map[string]interface{}
	}{
		{
			name:     "+sheet-info with include single category → narrow info_type",
			sc:       SheetInfo,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--include", "row_heights,col_widths"},
			toolName: "get_sheet_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"sheet_id":  testSheetID,
				"info_type": "row_heights_column_widths",
			},
		},
		{
			name:     "+sheet-info with mixed include → all",
			sc:       SheetInfo,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--include", "row_heights,merges"},
			toolName: "get_sheet_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"sheet_id":  testSheetID,
				"info_type": "all",
			},
		},
		{
			// --inherit-style before copies the preceding row: anchor row 5 and
			// insert after it (side=after), so the blank still lands before row 6.
			name:     "+dim-insert row position=6 count=3 inherit-before",
			sc:       DimInsert,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--position", "6", "--count", "3", "--inherit-style", "before"},
			toolName: "modify_sheet_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "insert",
				"sheet_id":  testSheetID,
				"position":  "5",
				"count":     float64(3),
				"side":      "after",
			},
		},
		{
			name:     "+dim-insert column position=C count=2",
			sc:       DimInsert,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--position", "C", "--count", "2"},
			toolName: "modify_sheet_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "insert",
				"sheet_id":  testSheetID,
				"position":  "C",
				"count":     float64(2),
			},
		},
		{
			name:     "+dim-delete column B:D",
			sc:       DimDelete,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "B:D"},
			toolName: "modify_sheet_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "delete",
				"sheet_id":  testSheetID,
				"range":     "B:D",
			},
		},
		{
			name:     "+dim-hide row 3:5",
			sc:       DimHide,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "3:5"},
			toolName: "modify_sheet_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "hide",
				"sheet_id":  testSheetID,
				"range":     "3:5",
			},
		},
		{
			name:     "+dim-unhide column AA:AC",
			sc:       DimUnhide,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "AA:AC"},
			toolName: "modify_sheet_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "unhide",
				"sheet_id":  testSheetID,
				"range":     "AA:AC",
			},
		},
		{
			name:     "+dim-freeze row count=2 → freeze_rows",
			sc:       DimFreeze,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--dimension", "row", "--count", "2"},
			toolName: "modify_sheet_structure",
			wantInput: map[string]interface{}{
				"excel_id":    testToken,
				"operation":   "freeze",
				"sheet_id":    testSheetID,
				"freeze_rows": float64(2),
			},
		},
		{
			name:     "+dim-freeze count=0 → unfreeze",
			sc:       DimFreeze,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--dimension", "column", "--count", "0"},
			toolName: "modify_sheet_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "unfreeze",
				"sheet_id":  testSheetID,
			},
		},
		{
			// The whole point of --rows/--cols: both axes in ONE operation.
			// Two single-axis calls would leave only the last axis frozen,
			// because freeze is full-state replacement server-side.
			name:     "+dim-freeze --rows 1 --cols 2 → one combined op",
			sc:       DimFreeze,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--rows", "1", "--cols", "2"},
			toolName: "modify_sheet_structure",
			wantInput: map[string]interface{}{
				"excel_id":       testToken,
				"operation":      "freeze",
				"sheet_id":       testSheetID,
				"freeze_rows":    float64(1),
				"freeze_columns": float64(2),
			},
		},
		{
			// Stating the survivor is how you unfreeze one axis and keep the
			// other; a zero axis is simply omitted from the body.
			name:     "+dim-freeze --rows 0 --cols 2 → columns only",
			sc:       DimFreeze,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--rows", "0", "--cols", "2"},
			toolName: "modify_sheet_structure",
			wantInput: map[string]interface{}{
				"excel_id":       testToken,
				"operation":      "freeze",
				"sheet_id":       testSheetID,
				"freeze_columns": float64(2),
			},
		},
		{
			name:     "+dim-freeze --rows 0 --cols 0 → unfreeze",
			sc:       DimFreeze,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--rows", "0", "--cols", "0"},
			toolName: "modify_sheet_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "unfreeze",
				"sheet_id":  testSheetID,
			},
		},
		{
			name:     "+dim-group row 1:5 fold",
			sc:       DimGroup,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "1:5", "--group-state", "fold"},
			toolName: "modify_sheet_structure",
			wantInput: map[string]interface{}{
				"excel_id":    testToken,
				"operation":   "group",
				"sheet_id":    testSheetID,
				"range":       "1:5",
				"group_state": "fold",
			},
		},
		{
			name:     "+dim-ungroup row 1:5",
			sc:       DimUngroup,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "1:5"},
			toolName: "modify_sheet_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "ungroup",
				"sheet_id":  testSheetID,
				"range":     "1:5",
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

func TestDimInsertInheritStyleSideMapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		position     string
		inherit      string
		wantPosition string
		wantSide     string
		wantSideSet  bool
	}{
		{
			name:         "after copies the following style with a plain before-insert, position unchanged",
			position:     "D",
			inherit:      "after",
			wantPosition: "D",
			wantSide:     "before",
			wantSideSet:  true,
		},
		{
			name:         "before anchors one column earlier (side=after) to copy the preceding style",
			position:     "D",
			inherit:      "before",
			wantPosition: "C",
			wantSide:     "after",
			wantSideSet:  true,
		},
		{
			name:         "before on a row anchors one row earlier",
			position:     "6",
			inherit:      "before",
			wantPosition: "5",
			wantSide:     "after",
			wantSideSet:  true,
		},
		{
			name:         "before at the first column falls back to a plain before-insert",
			position:     "A",
			inherit:      "before",
			wantPosition: "A",
			wantSideSet:  false,
		},
		{
			name:         "after at the first column still works (before-insert anchors the following)",
			position:     "A",
			inherit:      "after",
			wantPosition: "A",
			wantSide:     "before",
			wantSideSet:  true,
		},
		{
			name:         "default (flag omitted) omits side, backend inherits the following row/column",
			position:     "D",
			wantPosition: "D",
			wantSideSet:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			args := []string{"--url", testURL, "--sheet-id", testSheetID, "--position", tc.position, "--count", "1"}
			if tc.inherit != "" {
				args = append(args, "--inherit-style", tc.inherit)
			}
			body := parseDryRunBody(t, DimInsert, args)
			got := decodeToolInput(t, body, "modify_sheet_structure")
			assertInputEquals(t, got, map[string]interface{}{
				"excel_id":  testToken,
				"operation": "insert",
				"sheet_id":  testSheetID,
				"position":  tc.wantPosition,
				"count":     float64(1),
			})

			gv, ok := got["side"]
			if ok != tc.wantSideSet {
				t.Fatalf("side presence = %v, want %v (input=%#v)", ok, tc.wantSideSet, got)
			}
			if ok && gv != tc.wantSide {
				t.Fatalf("side = %v, want %q", gv, tc.wantSide)
			}
		})
	}
}

// TestDimRange_Validation covers the A1 range parser's edge cases routed
// through +dim-hide (any --range shortcut works; we just need to exercise
// the validator).
func TestDimRange_Validation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "end before start",
			args: []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "5:3", "--dry-run"},
			want: "end position is before start",
		},
		{
			name: "mix row+column",
			args: []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "3:C", "--dry-run"},
			want: "cannot mix row",
		},
		{
			name: "invalid characters",
			args: []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:B2", "--dry-run"},
			want: "expected pure digits",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, DimHide, tt.args)
			requireValidation(t, err, tt.want)
		})
	}
}

// TestDimFreezeEquivalent pins the replacement spelling printed by the
// phase-1 deprecation note: it must be the exact --rows/--cols call the user
// should switch to, not a generic pointer. Each pairing is also asserted for
// body equality, which is what makes the legacy form strictly redundant.
func TestDimFreezeEquivalent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		dimension string
		count     int
		want      string
	}{
		{"row", 2, "--rows 2"},
		{"column", 3, "--cols 3"},
		{"row", 0, "--rows 0 --cols 0"},
		{"column", 0, "--rows 0 --cols 0"},
	}
	for _, tt := range cases {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			legacy := newMapFlagViewForCommand("+dim-freeze", map[string]interface{}{
				"dimension": tt.dimension, "count": tt.count,
			})
			if got := dimFreezeEquivalent(legacy); got != tt.want {
				t.Fatalf("dimFreezeEquivalent = %q, want %q", got, tt.want)
			}
			// The advertised replacement must produce the identical body.
			modern := map[string]interface{}{}
			if tt.count > 0 {
				if tt.dimension == "row" {
					modern["rows"] = tt.count
				} else {
					modern["cols"] = tt.count
				}
			} else {
				modern["rows"], modern["cols"] = 0, 0
			}
			legacyInput, err := dimFreezeInput(legacy, testToken, testSheetID, "")
			if err != nil {
				t.Fatalf("legacy form: %v", err)
			}
			modernInput, err := dimFreezeInput(newMapFlagViewForCommand("+dim-freeze", modern), testToken, testSheetID, "")
			if err != nil {
				t.Fatalf("modern form: %v", err)
			}
			if !reflect.DeepEqual(legacyInput, modernInput) {
				t.Fatalf("bodies diverge:\n legacy = %v\n modern = %v", legacyInput, modernInput)
			}
		})
	}
}

// TestDimFreeze_FormValidation pins the two request forms as mutually
// exclusive, and pins that neither-form is a prescriptive error rather than a
// silent no-op.
func TestDimFreeze_FormValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "forms cannot be mixed",
			args: []string{"--rows", "1", "--dimension", "row", "--count", "1"},
			want: "not both",
		},
		{
			name: "neither form given",
			args: []string{},
			want: "nothing to freeze",
		},
		{
			name: "negative rows",
			args: []string{"--rows", "-1"},
			want: "--rows must be >= 0",
		},
		{
			name: "count without dimension",
			args: []string{"--count", "2"},
			want: "--dimension is required alongside --count",
		},
		{
			name: "dimension without count",
			args: []string{"--dimension", "row"},
			want: "--count is required alongside --dimension",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			args := append([]string{"--url", testURL, "--sheet-id", testSheetID, "--dry-run"}, tt.args...)
			_, _, err := runShortcutCapturingErr(t, DimFreeze, args)
			requireValidation(t, err, tt.want)
		})
	}
}

// TestDimMove_DryRun verifies the native v3 move_dimension payload shape.
// CLI's --source-range "1:3" (1-based inclusive) is parsed into
// source.{start_index=0, end_index=2} (0-based inclusive), and sheet_id is
// carried in the path, not the body. --target "11" → destination_index=10.
func TestDimMove_DryRun(t *testing.T) {
	t.Parallel()
	calls := parseDryRunAPI(t, DimMove, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--source-range", "1:3", "--target", "11",
	})
	if len(calls) != 1 {
		t.Fatalf("api calls = %d, want 1", len(calls))
	}
	c := calls[0].(map[string]interface{})
	wantURL := "/sheets/v3/spreadsheets/" + testToken + "/sheets/" + testSheetID + "/move_dimension"
	if !strings.Contains(c["url"].(string), wantURL) {
		t.Errorf("url = %v, want suffix %v", c["url"], wantURL)
	}
	body, _ := c["body"].(map[string]interface{})
	src, _ := body["source"].(map[string]interface{})
	if src["major_dimension"] != "ROWS" {
		t.Errorf("source.major_dimension = %v, want ROWS", src["major_dimension"])
	}
	if src["start_index"].(float64) != 0 {
		t.Errorf("start_index = %v, want 0", src["start_index"])
	}
	if src["end_index"].(float64) != 2 {
		t.Errorf("end_index = %v, want 2 (0-based inclusive)", src["end_index"])
	}
	if body["destination_index"].(float64) != 10 {
		t.Errorf("destination_index = %v, want 10 (target \"11\" → 0-based 10)", body["destination_index"])
	}
}

// TestDimMove_Column exercises the column path: --source-range "C:F" →
// COLUMNS / start=2 / end=5; --target "H" → destination_index=7.
func TestDimMove_Column(t *testing.T) {
	t.Parallel()
	calls := parseDryRunAPI(t, DimMove, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--source-range", "C:F", "--target", "H",
	})
	c := calls[0].(map[string]interface{})
	body, _ := c["body"].(map[string]interface{})
	src, _ := body["source"].(map[string]interface{})
	if src["major_dimension"] != "COLUMNS" {
		t.Errorf("major_dimension = %v, want COLUMNS", src["major_dimension"])
	}
	if src["start_index"].(float64) != 2 || src["end_index"].(float64) != 5 {
		t.Errorf("source = %v, want start=2 end=5", src)
	}
	if body["destination_index"].(float64) != 7 {
		t.Errorf("destination_index = %v, want 7", body["destination_index"])
	}
}

// TestDimMove_MismatchedDimension verifies that mixing source row + target
// column (or vice versa) is rejected at Validate.
func TestDimMove_MismatchedDimension(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, DimMove, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--source-range", "1:3", "--target", "H", "--dry-run",
	})
	requireValidation(t, err, "must match --source-range")
}

// TestParseA1Range covers parser edge cases directly.
func TestParseA1Range(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		dim     string
		start   int
		end     int
		wantErr bool
	}{
		{"3:7", "row", 2, 6, false},
		{"5", "row", 4, 4, false},
		{"C:F", "column", 2, 5, false},
		{"C", "column", 2, 2, false},
		{"aa:ac", "column", 26, 28, false}, // lower-case letters accepted
		{"", "", 0, 0, true},
		{"3:C", "", 0, 0, true},
		{"7:3", "", 0, 0, true},
		{"A1", "", 0, 0, true}, // cell ref, not a row/col range
		{"3:5:7", "", 0, 0, true},
		{"0", "", 0, 0, true}, // rows are 1-based
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()
			dim, start, end, err := parseA1Range(c.in)
			if c.wantErr {
				if err == nil {
					t.Errorf("parseA1Range(%q) = (%q, %d, %d, nil), want error", c.in, dim, start, end)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseA1Range(%q) unexpected error: %v", c.in, err)
			}
			if dim != c.dim || start != c.start || end != c.end {
				t.Errorf("parseA1Range(%q) = (%q, %d, %d), want (%q, %d, %d)", c.in, dim, start, end, c.dim, c.start, c.end)
			}
		})
	}
}

// TestColumnIndexToLetter exercises the corner cases of the letter helper
// (still in use by lark_sheet_workbook.go for absolute column refs).
func TestColumnIndexToLetter(t *testing.T) {
	t.Parallel()
	cases := []struct {
		idx  int
		want string
	}{
		{0, "A"}, {25, "Z"}, {26, "AA"}, {27, "AB"}, {51, "AZ"},
		{52, "BA"}, {701, "ZZ"}, {702, "AAA"},
	}
	for _, c := range cases {
		if got := columnIndexToLetter(c.idx); got != c.want {
			t.Errorf("columnIndexToLetter(%d) = %q, want %q", c.idx, got, c.want)
		}
	}
}
