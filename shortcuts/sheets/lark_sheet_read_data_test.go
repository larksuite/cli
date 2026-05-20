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
			name:     "+cells-get multi-range + include=style,formula",
			sc:       CellsGet,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:B2", "--range", "D1:E5", "--include", "style,formula"},
			toolName: "get_cell_ranges",
			wantInput: map[string]interface{}{
				"excel_id":            testToken,
				"sheet_id":            testSheetID,
				"ranges":              []interface{}{"A1:B2", "D1:E5"},
				"include_styles":      true,
				"value_render_option": "formula",
			},
		},
		{
			name:     "+csv-get with value-render-option",
			sc:       CsvGet,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:C10", "--value-render-option", "formula"},
			toolName: "get_range_as_csv",
			wantInput: map[string]interface{}{
				"excel_id":            testToken,
				"sheet_id":            testSheetID,
				"range":               "A1:C10",
				"value_render_option": "formula",
			},
		},
		{
			name:     "+dropdown-get range with sheet prefix only",
			sc:       DropdownGet,
			args:     []string{"--url", testURL, "--range", "sheet1!A2:A100"},
			toolName: "get_cell_ranges",
			wantInput: map[string]interface{}{
				"excel_id":            testToken,
				"ranges":              []interface{}{"sheet1!A2:A100"},
				"include_styles":      false,
				"value_render_option": "formatted_value",
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

func TestDropdownGet_RequiresSheetPrefix(t *testing.T) {
	t.Parallel()
	stdout, stderr, err := runShortcutCapturingErr(t, DropdownGet, []string{
		"--url", testURL, "--range", "A2:A100", "--dry-run",
	})
	if err == nil {
		t.Fatalf("expected validation error; stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout+stderr+err.Error(), "must include a sheet prefix") {
		t.Errorf("expected sheet-prefix guard; got=%s|%s|%v", stdout, stderr, err)
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
