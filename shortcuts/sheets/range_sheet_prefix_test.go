// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"testing"
)

func TestSplitRangeSheetPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		in        string
		wantSheet string
		wantRest  string
		wantOK    bool
	}{
		{"plain prefix", "Sheet1!A1:D20", "Sheet1", "A1:D20", true},
		{"single cell", "Sheet1!A1", "Sheet1", "A1", true},
		{"row range", "Sheet1!3:7", "Sheet1", "3:7", true},
		{"cjk name", "销售数据!A1:C9", "销售数据", "A1:C9", true},
		{"surrounding spaces", "  Sheet1 ! A1:B2 ", "Sheet1", "A1:B2", true},
		{"quoted name with space", "'My Sheet'!A1:B2", "My Sheet", "A1:B2", true},
		{"quoted name with escaped quote", "'It''s'!A1", "It's", "A1", true},
		// The lexer's unquoted name production excludes whitespace so a formula
		// can tokenize; a --range flag has no such ambiguity.
		{"unquoted name with space", "My Sheet!A1", "My Sheet", "A1", true},
		{"no prefix", "A1:D20", "", "", false},
		{"empty sheet", "!A1", "", "", false},
		{"empty range", "Sheet1!", "", "", false},
		{"unterminated quote", "'My Sheet!A1", "", "", false},
		{"quoted name without bang", "'My Sheet'A1", "", "", false},
		{"empty input", "", "", "", false},

		// Separator spellings the front-end lexer treats as one and the same
		// (ExclamationMark accepts the full-width form; the escaped variants
		// survive shell history expansion).
		{"full-width separator", "销售数据！A1:C9", "销售数据", "A1:C9", true},
		{"escaped separator", `Sheet1\!A1:D20`, "Sheet1", "A1:D20", true},
		{"escaped full-width separator", `Sheet1\！A1`, "Sheet1", "A1", true},
		{"full-width separator after quotes", "'My Sheet'！A1", "My Sheet", "A1", true},
		{"escaped separator after quotes", `'My Sheet'\!A1`, "My Sheet", "A1", true},

		// Quotes delimit the name, so one may legally contain a separator —
		// splitting on the first "!" would name the wrong sheet.
		{"bang inside quoted name", "'Q!A'!A1:B2", "Q!A", "A1:B2", true},
		{"full-width bang inside quoted name", "'销量！'!A1", "销量！", "A1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sheet, rest, ok := splitRangeSheetPrefix(tt.in)
			if ok != tt.wantOK || sheet != tt.wantSheet || rest != tt.wantRest {
				t.Errorf("splitRangeSheetPrefix(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.in, sheet, rest, ok, tt.wantSheet, tt.wantRest, tt.wantOK)
			}
		})
	}
}

// TestRangeSheetPrefix_StandaloneFillsSelector is the eval case verbatim: a
// read that names its sheet only inside --range used to die on "specify at
// least one of --sheet-id or --sheet-name" (707 occurrences, 53% of them with
// the prefix already present). It now resolves, with the prefix moved into
// sheet_name and the bare A1 range sent to the tool.
func TestRangeSheetPrefix_StandaloneFillsSelector(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		command   string
		args      []string
		toolName  string
		wantInput map[string]interface{}
	}{
		{
			name:     "+cells-get",
			command:  "+cells-get",
			args:     []string{"--url", testURL, "--range", "Sheet1!A1:D20"},
			toolName: "get_cell_ranges",
			wantInput: map[string]interface{}{
				"excel_id":   testToken,
				"sheet_name": "Sheet1",
				"ranges":     []interface{}{"A1:D20"},
				"cell_limit": float64(unboundedReadLimit),
			},
		},
		{
			name:     "+csv-get",
			command:  "+csv-get",
			args:     []string{"--url", testURL, "--range", "Sheet1!A1:D20"},
			toolName: "get_range_as_csv",
			wantInput: map[string]interface{}{
				"excel_id":   testToken,
				"sheet_name": "Sheet1",
				"range":      "A1:D20",
				"max_rows":   float64(unboundedReadLimit),
			},
		},
		{
			name:     "+cells-clear",
			command:  "+cells-clear",
			args:     []string{"--url", testURL, "--range", "'My Sheet'!A1:B2"},
			toolName: "clear_cell_range",
			wantInput: map[string]interface{}{
				"excel_id":   testToken,
				"sheet_name": "My Sheet",
				"range":      "A1:B2",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sc := shortcutFromRegistry(t, tt.command)
			body := parseDryRunBody(t, sc, append(append([]string{}, tt.args...), "--dry-run"))
			got := decodeToolInput(t, body, tt.toolName)
			assertInputEquals(t, got, tt.wantInput)
		})
	}
}

// An explicit selector stays authoritative: --range is passed through
// untouched, so a prefix that disagrees with --sheet-name can never silently
// retarget the call.
func TestRangeSheetPrefix_ExplicitSelectorWins(t *testing.T) {
	t.Parallel()

	sc := shortcutFromRegistry(t, "+cells-get")
	body := parseDryRunBody(t, sc, []string{
		"--url", testURL, "--sheet-name", "Other", "--range", "Sheet1!A1:D20", "--dry-run",
	})
	got := decodeToolInput(t, body, "get_cell_ranges")
	assertInputEquals(t, got, map[string]interface{}{
		"excel_id":   testToken,
		"sheet_name": "Other",
		"ranges":     []interface{}{"Sheet1!A1:D20"},
		"cell_limit": float64(unboundedReadLimit),
	})
}

// +csv-put takes --range as an alias for --start-cell, so the prefix has to be
// consumed before the range collapses to its anchor cell.
func TestRangeSheetPrefix_CsvPutRangeAlias(t *testing.T) {
	t.Parallel()

	sc := shortcutFromRegistry(t, "+csv-put")
	body := parseDryRunBody(t, sc, []string{
		"--url", testURL, "--range", "Sheet1!B2:D9", "--csv", "a,b", "--dry-run",
	})
	got := decodeToolInput(t, body, "set_range_from_csv")
	assertInputEquals(t, got, map[string]interface{}{
		"excel_id":   testToken,
		"sheet_name": "Sheet1",
		"start_cell": "B2",
		"csv":        "a,b",
	})
}

// The +batch-update path never runs cobra, so the sub-op translator carries
// the same rewrite — otherwise a batched write would still fail on the
// selector check its standalone twin no longer raises.
func TestRangeSheetPrefix_BatchSubOp(t *testing.T) {
	t.Parallel()

	var subInput map[string]interface{}
	if err := json.Unmarshal([]byte(`{"range":"Sheet1!A1","cells":[[{"value":"x"}]]}`), &subInput); err != nil {
		t.Fatalf("bad subInput JSON: %v", err)
	}
	op, err := translateBatchOp(map[string]interface{}{
		"shortcut": "+cells-set",
		"input":    subInput,
	}, testToken, 0)
	if err != nil {
		t.Fatalf("sub-op with a sheet-prefixed range must translate, got: %v", err)
	}
	input, ok := op["input"].(map[string]interface{})
	if !ok {
		t.Fatalf("translated op carries no input map: %#v", op)
	}
	if got := input["sheet_name"]; got != "Sheet1" {
		t.Errorf("sheet_name = %v, want %q", got, "Sheet1")
	}
	if got := input["range"]; got != "A1" {
		t.Errorf("range = %v, want %q", got, "A1")
	}
}

// An input may reach the flag view carrying both spellings of the selector:
// normalizeSubOpInputKeys keeps a duplicate whose two values agree instead of
// erroring, and two empty strings agree. lookupRaw answers with the first
// spelling it finds, so the derived selector has to end up as the only one —
// an empty "sheet-name" left beside it would shadow it back to "no selector".
func TestRangeSheetPrefix_BothSelectorSpellingsEmpty(t *testing.T) {
	t.Parallel()

	var subInput map[string]interface{}
	if err := json.Unmarshal([]byte(
		`{"range":"Sheet1!A1","cells":[[{"value":"x"}]],"sheet-name":"","sheet_name":""}`,
	), &subInput); err != nil {
		t.Fatalf("bad subInput JSON: %v", err)
	}
	op, err := translateBatchOp(map[string]interface{}{
		"shortcut": "+cells-set",
		"input":    subInput,
	}, testToken, 0)
	if err != nil {
		t.Fatalf("empty selector keys must not shadow the derived one, got: %v", err)
	}
	input, ok := op["input"].(map[string]interface{})
	if !ok {
		t.Fatalf("translated op carries no input map: %#v", op)
	}
	if got := input["sheet_name"]; got != "Sheet1" {
		t.Errorf("sheet_name = %v, want %q", got, "Sheet1")
	}
	if got := input["range"]; got != "A1" {
		t.Errorf("range = %v, want %q", got, "A1")
	}
}
