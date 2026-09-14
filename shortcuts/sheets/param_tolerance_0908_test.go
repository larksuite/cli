// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"
	"testing"
)

// A dropdown option is a label the caller wrote, so a comma inside one is as
// likely as a comma between two. Only a value with no comma at all is lifted;
// the rest gets the split spelled out to confirm rather than applied.
func TestDropdownOptions_BareValue(t *testing.T) {
	t.Parallel()

	t.Run("a lone bare option becomes the one-element list", func(t *testing.T) {
		t.Parallel()
		stdout, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+dropdown-set"), []string{
			"--url", testURL, "--sheet-name", "s", "--range", "A1:A5",
			"--options", "Approved", "--dry-run",
		})
		if err != nil {
			t.Fatalf("a bare option should be accepted, got: %v", err)
		}
		if !strings.Contains(stdout, `Approved`) {
			t.Errorf("the option should reach the request, got %q", stdout)
		}
	})

	t.Run("a comma list is not split, and the error carries the split", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+dropdown-set"), []string{
			"--url", testURL, "--sheet-name", "s", "--range", "A1:A5",
			"--options", "Yes,No", "--dry-run",
		})
		ve := requireValidation(t, err, "invalid JSON")
		for _, want := range []string{`["Yes","No"]`, "2 separate options"} {
			if !strings.Contains(ve.Hint, want) {
				t.Errorf("hint should carry %q, got %q", want, ve.Hint)
			}
		}
	})
}

// The bare comparative with "than" dropped. gt / ge / >= already resolved, so
// refusing the spelled-out form of the same comparison was arbitrary.
func TestCondFormat_BareComparativeSpellings(t *testing.T) {
	t.Parallel()
	for spelling, want := range map[string]string{
		"greater": "greaterThan",
		"less":    "lessThan",
		"above":   "greaterThan",
		"below":   "lessThan",
	} {
		t.Run(spelling, func(t *testing.T) {
			t.Parallel()
			stdout, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cond-format-create"), []string{
				"--url", testURL, "--sheet-name", "s", "--ranges", `["A1:B2"]`,
				"--properties", `{"rule_type":"cellIs","attrs":[{"compare_type":"` + spelling +
					`","value":"5"}],"style":{"font_color":"#ff0000"}}`,
				"--dry-run",
			})
			if err != nil {
				t.Fatalf("%q should resolve to %q, got: %v", spelling, want, err)
			}
			if !strings.Contains(stdout, want) {
				t.Errorf("expected compare_type %q in the request, got %q", want, stdout)
			}
		})
	}
}

// The multi-area guard reads --range through pflag, which normalizes; on a
// command where --range is an ALIAS the lookup lands on the flag it aliases.
// +chart-create-basic is exactly that, and its real --data-range takes the
// comma-separated multi-range the guard exists to refuse.
func TestChartDataRange_MultiAreaNotBlocked(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"--data-range", "--range"} {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			if _, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+chart-create-basic"), []string{
				"--url", testURL, "--sheet-name", "s", "--chart-type", "column",
				flag, "A1:B10,D1:E10", "--dry-run",
			}); err != nil {
				t.Fatalf("a chart data range takes several areas, got: %v", err)
			}
		})
	}
}

// splitAreasRespectingQuotes exists because a quoted sheet name may itself
// contain a comma, which separates nothing.
func TestSplitAreasRespectingQuotes(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		raw  string
		want []string
	}{
		{"no comma", "A1:B2", []string{"A1:B2"}},
		{"two areas", "A1:B2,D1:E2", []string{"A1:B2", "D1:E2"}},
		{"comma inside a quoted sheet name", "'Q1,Sales'!A1:B2", []string{"'Q1,Sales'!A1:B2"}},
		{"quoted name beside a second area", "'Q1,Sales'!A1:B2,D1:E2", []string{"'Q1,Sales'!A1:B2", "D1:E2"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := splitAreasRespectingQuotes(tt.raw)
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// A multi-area range is one call against one sheet, so a qualifier repeated on
// every area is lifted once and stripped from all of them; two different
// sheets are left alone rather than silently retargeted.
func TestSplitRangeSheetPrefixAcrossAreas(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, raw, wantSheet, wantRest string
		wantOK                         bool
	}{
		{"same qualifier on both", "'Q1'!A1:A2,'Q1'!C1:D2", "Q1", "A1:A2,C1:D2", true},
		{"only the first qualified", "Sheet1!A1:B2,D1:E2", "Sheet1", "A1:B2,D1:E2", true},
		{"no qualifier anywhere", "A1:B2,D1:E2", "", "", false},
		{"two different sheets", "Sheet1!A1:B2,Sheet2!D1:E2", "", "", false},
		{"single area keeps prior behaviour", "Sheet1!A1:B2", "Sheet1", "A1:B2", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sheet, rest, ok := splitRangeSheetPrefixAcrossAreas(tt.raw)
			if ok != tt.wantOK || sheet != tt.wantSheet || rest != tt.wantRest {
				t.Errorf("got (%q, %q, %v), want (%q, %q, %v)",
					sheet, rest, ok, tt.wantSheet, tt.wantRest, tt.wantOK)
			}
		})
	}
}

// A border needs a range, and a styles ITEM has none of its own. When the item
// carries exactly one cell_styles entry, that entry's range is the only one in
// reach, so a border_styles hoisted to the item is unambiguous; every other
// shape keeps the item-level key and with it the prescription.
func TestStylesItem_BorderStylesLiftedIntoTheSingleEntry(t *testing.T) {
	t.Parallel()

	t.Run("one entry without a border takes it", func(t *testing.T) {
		t.Parallel()
		item := map[string]interface{}{
			"name": "S1",
			"cell_styles": []interface{}{
				map[string]interface{}{"range": "A1:B2", "font_weight": "bold"},
			},
			"border_styles": map[string]interface{}{"all": map[string]interface{}{"style": "solid"}},
		}
		foldStyleItemKeys(item)
		if _, still := item["border_styles"]; still {
			t.Fatalf("border_styles should have moved off the item, got %v", item)
		}
		entry := item["cell_styles"].([]interface{})[0].(map[string]interface{})
		if _, landed := entry["border_styles"]; !landed {
			t.Errorf("border_styles should be on the entry, got %v", entry)
		}
	})

	for _, tt := range []struct {
		name string
		item map[string]interface{}
	}{
		{"two entries leave two candidate ranges", map[string]interface{}{
			"name": "S1",
			"cell_styles": []interface{}{
				map[string]interface{}{"range": "A1:B2"},
				map[string]interface{}{"range": "C1:D2"},
			},
			"border_styles": map[string]interface{}{"all": map[string]interface{}{"style": "solid"}},
		}},
		{"an entry that already has a border", map[string]interface{}{
			"name": "S1",
			"cell_styles": []interface{}{
				map[string]interface{}{"range": "A1:B2", "border_styles": map[string]interface{}{"top": map[string]interface{}{"style": "solid"}}},
			},
			"border_styles": map[string]interface{}{"all": map[string]interface{}{"style": "dashed"}},
		}},
		{"no cell_styles to land on", map[string]interface{}{
			"name":          "S1",
			"row_sizes":     []interface{}{map[string]interface{}{"range": "1:1", "type": "pixel", "size": float64(30)}},
			"border_styles": map[string]interface{}{"all": map[string]interface{}{"style": "solid"}},
		}},
	} {
		t.Run(tt.name+" keeps the item-level key", func(t *testing.T) {
			t.Parallel()
			foldStyleItemKeys(tt.item)
			if _, still := tt.item["border_styles"]; !still {
				t.Errorf("border_styles should have been left for the unknown-key report, got %v", tt.item)
			}
		})
	}
}

// The wrap and freeze spellings used to get a prescription that spelled the
// rename and nothing else. For wrap the stated reason — "the values differ
// too" — no longer holds: --word-wrap's enum normalizer already reads
// true/false, the Google Sheets API words and the bare ones, so the name was
// all that was left. The freeze counts were always the same integer.
func TestFlagRenames_FromTheUnknownFlagTable(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, command string
		args          []string
		wantInRequest string
	}{
		{"wrap-text carries a boolean", "+cells-set-style",
			[]string{"--range", "A1", "--wrap-text", "true"}, `"word_wrap": "auto-wrap"`},
		{"wrap-strategy carries the API word", "+cells-set-style",
			[]string{"--range", "A1", "--wrap-strategy", "WRAP"}, `"word_wrap": "auto-wrap"`},
		{"bare wrap", "+cells-set-style",
			[]string{"--range", "A1", "--wrap", "false"}, `"word_wrap": "overflow"`},
		{"border-style joins border-type", "+cells-set-style",
			[]string{"--range", "A1", "--border-style", "solid"}, `"border_styles"`},
		{"frozen-rows is rows", "+dim-freeze",
			[]string{"--frozen-rows", "2"}, `"freeze_rows": 2`},
		{"frozen-row-count is rows", "+dim-freeze",
			[]string{"--frozen-row-count", "2"}, `"freeze_rows": 2`},
		// The parse error reports the flag as typed, so the underscore
		// spelling has to fold onto the same alias as the hyphenated one.
		{"the underscore spelling folds too", "+dim-freeze",
			[]string{"--frozen_rows", "2"}, `"freeze_rows": 2`},
		{"frozen-cols is cols", "+dim-freeze",
			[]string{"--frozen-cols", "1"}, `"freeze_columns": 1`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			args := append([]string{"--url", testURL, "--sheet-name", "s"}, tt.args...)
			stdout, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, tt.command), append(args, "--dry-run"))
			if err != nil {
				t.Fatalf("%s should be a rename, got: %v", tt.name, err)
			}
			if !strings.Contains(stdout, tt.wantInRequest) {
				t.Errorf("expected %s in the request, got %q", tt.wantInRequest, stdout)
			}
		})
	}
}

// The --ranges commands keep refusing a top-level sheet selector -- the sheet
// belongs in each entry's prefix, and a reference_id has no expression there
// at all. What they used to answer with was the generic locator, which names
// the OTHER commands --sheet-name is valid on and never says where the sheet
// goes here. These pin the answer rather than the refusal.
func TestRangesCommands_PrescribeTheSheetPrefix(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		command string
		extra   []string
	}{
		{"+cells-batch-clear", []string{"--ranges", `["A1:B2"]`, "--scope", "content", "--yes"}},
		{"+dropdown-delete", []string{"--ranges", `["A1:B2"]`, "--yes"}},
		{"+dropdown-update", []string{"--ranges", `["A1:B2"]`, "--options", `["a"]`}},
	} {
		t.Run(tt.command+" --sheet-name", func(t *testing.T) {
			t.Parallel()
			args := append([]string{"--url", testURL, "--sheet-name", "S1"}, tt.extra...)
			_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, tt.command), append(args, "--dry-run"))
			ve := requireValidation(t, err, "unknown flag")
			for _, want := range []string{"--ranges", "prefix", "Sheet1!"} {
				if !strings.Contains(ve.Hint, want) {
					t.Errorf("hint should carry %q, got %q", want, ve.Hint)
				}
			}
		})

		// A reference_id cannot be a prefix, so this one has to send the
		// caller to look the display name up rather than to a form that does
		// not exist.
		t.Run(tt.command+" --sheet-id", func(t *testing.T) {
			t.Parallel()
			args := append([]string{"--url", testURL, "--sheet-id", "shtabc"}, tt.extra...)
			_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, tt.command), append(args, "--dry-run"))
			ve := requireValidation(t, err, "unknown flag")
			for _, want := range []string{"reference_id cannot appear", "+sheet-list"} {
				if !strings.Contains(ve.Hint, want) {
					t.Errorf("hint should carry %q, got %q", want, ve.Hint)
				}
			}
		})
	}
}

// A quoted sheet name may contain a comma, and the multi-area splitter has to
// be the quote-aware one everywhere -- including the read expansion, which
// used strings.Split and forwarded two halves of one name as two ranges.
func TestMultiArea_QuotedSheetNameWithAComma(t *testing.T) {
	t.Parallel()

	t.Run("read expansion keeps the name whole", func(t *testing.T) {
		t.Parallel()
		stdout, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-get"), []string{
			"--url", testURL, "--sheet-name", "Q1,Sales",
			"--range", `'Q1,Sales'!A1,'Q1,Sales'!B2`, "--dry-run",
		})
		if err != nil {
			t.Fatalf("a quoted name carrying a comma should survive, got: %v", err)
		}
		for _, want := range []string{`'Q1,Sales'!A1`, `'Q1,Sales'!B2`} {
			if !strings.Contains(stdout, want) {
				t.Errorf("expected %q intact in the request, got %q", want, stdout)
			}
		}
	})

	// The rejection path counts areas too, and counting on a naive split would
	// report four areas for two.
	t.Run("the refusal counts two areas, not four", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-set-style"), []string{
			"--url", testURL, "--sheet-name", "Q1,Sales",
			"--range", `'Q1,Sales'!A1,'Q1,Sales'!B2`, "--font-weight", "bold", "--dry-run",
		})
		requireValidation(t, err, "lists 2 separate areas")
	})
}

// +filter-update / +filter-delete need the sub-sheet's id (filter_id ==
// sheet_id), which neither an omitted selector nor an explicit --sheet-name
// supplies. The old error told the caller to run +workbook-info and pass the
// id by hand; the execute path can make that trade itself.
func TestFilter_SheetNameTradedForTheID(t *testing.T) {
	t.Parallel()

	t.Run("the sole sheet needs no selector at all", func(t *testing.T) {
		t.Parallel()
		if _, err := runShortcutWithStubs(t, FilterDelete,
			[]string{"--url", testURL, "--yes"},
			structureStub("OnlySheet"), structureStub("OnlySheet"),
			toolStub("manage_filter_object", `{"ok":true}`)); err != nil {
			t.Fatalf("the only sheet is the one meant, got: %v", err)
		}
	})

	t.Run("an explicit name is resolved to its id", func(t *testing.T) {
		t.Parallel()
		if _, err := runShortcutWithStubs(t, FilterDelete,
			[]string{"--url", testURL, "--sheet-name", "Two", "--yes"},
			structureStub("One", "Two"),
			toolStub("manage_filter_object", `{"ok":true}`)); err != nil {
			t.Fatalf("--sheet-name should be traded for the id, got: %v", err)
		}
	})

	// A preview performs no lookup, so it still has to be given the id -- and
	// says so, instead of claiming no lookup exists anywhere.
	t.Run("a preview still requires the id", func(t *testing.T) {
		t.Parallel()
		_, err := runShortcutWithStubs(t, FilterDelete,
			[]string{"--url", testURL, "--sheet-name", "Two", "--yes", "--dry-run"})
		requireValidation(t, err, "a preview and a +batch-update sub-op both perform no lookup")
	})
}

// The omitted-selector lookup is a READ issued by commands that mostly declare
// only the write scope. Every command that can omit a selector has to declare
// it conditionally, or a least-privilege token passes pre-flight and then 403s
// inside the lookup. Derived from the flags, so this cannot drift.
func TestOmittedSelectorReadScopeIsDeclared(t *testing.T) {
	t.Parallel()
	for _, sc := range Shortcuts() {
		if selectorMustBeExplicit[sc.Command] || !hasSheetSelectorFlag(sc.Flags) {
			continue
		}
		declared := false
		for _, s := range sc.DeclaredScopesForIdentity("user") {
			if s == sheetsStructureReadScope {
				declared = true
			}
		}
		if !declared {
			t.Errorf("%s can omit its sheet selector but never declares %s", sc.Command, sheetsStructureReadScope)
		}
	}
}
