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
			name:     "+cells-get include=formula without style pins include_styles=false",
			sc:       CellsGet,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:B2", "--include", "formula"},
			toolName: "get_cell_ranges",
			wantInput: map[string]interface{}{
				"excel_id":            testToken,
				"sheet_id":            testSheetID,
				"ranges":              []interface{}{"A1:B2"},
				"include_styles":      false,
				"value_render_option": "formula",
				"cell_limit":          float64(unboundedReadLimit),
			},
		},
		{
			// --include truncation toggles include_truncation_info so the tool
			// estimates and returns per-cell isRowTruncated / isColTruncated.
			name:     "+cells-get include=truncation",
			sc:       CellsGet,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:B2", "--include", "truncation"},
			toolName: "get_cell_ranges",
			wantInput: map[string]interface{}{
				"excel_id":                testToken,
				"sheet_id":                testSheetID,
				"ranges":                  []interface{}{"A1:B2"},
				"include_styles":          false,
				"include_truncation_info": true,
				"cell_limit":              float64(unboundedReadLimit),
			},
		},
		{
			// --output-path alone raises the cap to the bounded file-offload
			// default — NOT the unbounded sentinel; the read path is not
			// streaming, so the cap is the OOM guard.
			name:     "+cells-get output-path uses bounded offload cap",
			sc:       CellsGet,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:B2", "--output-path", "out.json"},
			toolName: "get_cell_ranges",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"sheet_id":  testSheetID,
				"ranges":    []interface{}{"A1:B2"},
				"max_chars": float64(outputPathReadLimit),
			},
		},
		{
			// An explicit --max-chars survives --output-path instead of being
			// silently replaced by the unbounded sentinel.
			name:     "+cells-get explicit max-chars survives output-path",
			sc:       CellsGet,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:B2", "--output-path", "out.json", "--max-chars", "12345"},
			toolName: "get_cell_ranges",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"sheet_id":  testSheetID,
				"ranges":    []interface{}{"A1:B2"},
				"max_chars": float64(12345),
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
			name:     "+cond-format-result-get hardcodes style outputs",
			sc:       CondFormatResultGet,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:B2"},
			toolName: "get_cell_ranges",
			wantInput: map[string]interface{}{
				"excel_id":                         testToken,
				"sheet_id":                         testSheetID,
				"ranges":                           []interface{}{"A1:B2"},
				"include_styles":                   true,
				"include_conditional_format_style": true,
				"cell_limit":                       float64(unboundedReadLimit),
			},
		},
		{
			name:     "+cells-get --include conditional_format",
			sc:       CellsGet,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:B2", "--include", "conditional_format"},
			toolName: "get_cell_ranges",
			wantInput: map[string]interface{}{
				"excel_id":                         testToken,
				"sheet_id":                         testSheetID,
				"ranges":                           []interface{}{"A1:B2"},
				"include_conditional_format_style": true,
				"include_styles":                   true,
				"cell_limit":                       float64(unboundedReadLimit),
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
// must still be rejected by Validate). +csv-get is deliberately absent:
// its --range is optional — omitted/blank means a whole-sheet read (see
// TestCsvGet_RangeOptionalDefaultsToFullSheet).
func TestReadData_RequiresRange(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		sc   common.Shortcut
	}{
		{"+cells-get", CellsGet},
		{"+dropdown-get", DropdownGet},
		{"+cond-format-result-get", CondFormatResultGet},
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

// TestCsvGet_RangeOptionalDefaultsToFullSheet pins the whole-sheet default:
// with --range omitted the request carries the over-wide clip range, so a
// full read needs no workbook-info pre-flight (eval: --range was the most
// missed required flag on +csv-get once the rest of the surface settled).
func TestCsvGet_RangeOptionalDefaultsToFullSheet(t *testing.T) {
	t.Parallel()
	stdout, _, err := runShortcutCapturingErr(t, CsvGet, []string{
		"--url", testURL, "--sheet-id", testSheetID, "--dry-run",
	})
	if err != nil {
		t.Fatalf("rangeless +csv-get must pass validation, got: %v", err)
	}
	if !strings.Contains(stdout, csvGetFullSheetRange) {
		t.Fatalf("dry-run body should carry the full-sheet range %q, got %q", csvGetFullSheetRange, stdout)
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

// TestCellsGet_MultiAreaRange pins the split: get_cell_ranges already takes a
// LIST of ranges, so the areas a caller joined with commas go out as the
// several ranges they name rather than being refused. The prescription itself
// still exists for commands whose input is one range — see
// TestMultiAreaRangeStillPrescribedOnWrites.
func TestCellsGet_MultiAreaRange(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		rng   string
		sheet string
		want  []string
	}{
		{"single cells joined by commas", "A3,G3,H3,J3", "s", []string{"A3", "G3", "H3", "J3"}},
		{"ranges joined by commas", "A1:B2,D1:E2", "s", []string{"A1:B2", "D1:E2"}},
		{"a single continuous range is unchanged", "A3:L3", "s", []string{"A3:L3"}},
		// The qualifier is lifted into the selector once and every area loses
		// it; stripping only the first would ship a prefixed range beside a
		// sheet_name that already says the same thing.
		{"every area carries the same qualifier", "'Q1'!A1:A21,'Q1'!C1:D21", "", []string{"A1:A21", "C1:D21"}},
		{"only the first area is qualified", "Sheet1!A1:B2,D1:E2", "", []string{"A1:B2", "D1:E2"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			args := []string{"--url", testURL, "--range", tt.rng, "--dry-run"}
			if tt.sheet != "" {
				args = append(args, "--sheet-name", tt.sheet)
			}
			stdout, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-get"), args)
			if err != nil {
				t.Fatalf("a multi-area range should be forwarded, got: %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(stdout, `"`+want+`"`) {
					t.Errorf("range %q missing from the request, got %q", want, stdout)
				}
			}
		})
	}
}

// A command whose tool input is ONE range keeps the prescription: fanning a
// single call into several would change what a partial failure leaves behind.
// The enclosing-rectangle hint is the part worth pinning, since it has to
// cover every area rather than the first and last.
func TestMultiAreaRangeStillPrescribedOnWrites(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ name, rng, wantCount, wantHint string }{
		{"cells prescribe the enclosing rectangle", "A3,G3,H3,J3", "lists 4 separate areas", `--range "A3:J3"`},
		// The widest column is in the middle here; first-and-last would
		// prescribe "A3:G3" and silently drop the J3 the caller asked for.
		{"unordered areas still land inside the rectangle", "A3,J3,G3", "lists 3 separate areas", `--range "A3:J3"`},
		{"the rectangle spans both axes", "C5,A2,B9", "lists 3 separate areas", `--range "A2:C9"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-set-style"), []string{
				"--url", testURL, "--sheet-name", "s", "--range", tt.rng,
				"--font-weight", "bold", "--dry-run",
			})
			ve := requireValidation(t, err, tt.wantCount)
			if !strings.Contains(ve.Hint, tt.wantHint) {
				t.Errorf("hint should carry %q, got %q", tt.wantHint, ve.Hint)
			}
		})
	}

	t.Run("two ranges get no invented rectangle", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-set-style"), []string{
			"--url", testURL, "--sheet-name", "s", "--range", "A1:B2,D1:E2",
			"--font-weight", "bold", "--dry-run",
		})
		ve := requireValidation(t, err, "lists 2 separate areas")
		if strings.Contains(ve.Hint, `--range "A1:B2:D1:E2"`) {
			t.Errorf("hint must not invent a rectangle from two ranges, got %q", ve.Hint)
		}
	})
}
