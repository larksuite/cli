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
	_, _, err := runShortcutCapturingErr(t, DropdownGet, []string{
		"--url", testURL, "--range", "A2:A100", "--dry-run",
	})
	ve := requireValidation(t, err, "")
	if !strings.Contains(ve.Message, "sheet-id") && !strings.Contains(ve.Message, "sheet-name") {
		t.Errorf("expected --sheet-id/--sheet-name guard; got message=%q", ve.Message)
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
			_, _, err := runShortcutCapturingErr(t, c.sc, []string{
				"--url", testURL, "--sheet-id", testSheetID, "--range", "  ", "--dry-run",
			})
			requireValidation(t, err, "--range is required")
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
