// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
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
//
// "above" / "below" stay refused: aboveAverage is a rule_type in this same
// schema, so they name a plausible OTHER rule rather than a comparison.
func TestCondFormat_BareComparativeSpellings(t *testing.T) {
	t.Parallel()
	for spelling, want := range map[string]string{
		"greater": "greaterThan",
		"less":    "lessThan",
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

// The wrap spellings used to get a prescription that spelled the rename and
// nothing else. The stated reason — "the values differ too" — no longer
// holds: --word-wrap's enum normalizer already reads true/false, the Google
// Sheets API words and the bare ones, so the name was all that was left.
// The freeze spellings are NOT here; see TestDimFreeze_KeepsItsPrescription.
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

// +dim-freeze sends the WHOLE freeze state every call, so --frozen-rows 2 as
// a silent rename would ship freeze_rows:2 with no freeze_columns and unfreeze
// an existing column freeze the caller never mentioned. The names stay on the
// prescription tier, where the answer can name the other axis.
func TestDimFreeze_KeepsItsPrescription(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{
		"--frozen-rows", "--frozen-row-count", "--row-count",
		// The parse error reports the flag as typed, so the underscore
		// spelling has to reach the same entry as the hyphenated one.
		"--frozen_rows",
		"--frozen-cols", "--frozen-columns", "--frozen-col-count", "--column-count",
	} {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+dim-freeze"),
				[]string{"--url", testURL, "--sheet-name", "s", flag, "2", "--dry-run"})
			ve := requireValidation(t, err, "unknown flag")
			for _, want := range []string{"one call states the whole freeze state", "UNFROZEN"} {
				if !strings.Contains(ve.Hint, want) {
					t.Errorf("hint should carry %q, got %q", want, ve.Hint)
				}
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

	// The bare-value lift splits too: wrapBareListValue turns an unbracketed
	// --ranges into its JSON array, and a naive split halved the name there
	// the same way the read expansion used to.
	t.Run("the bare --ranges lift keeps the name whole", func(t *testing.T) {
		t.Parallel()
		stdout, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-batch-clear"), []string{
			"--url", testURL, "--ranges", `'Q1,Sales'!A1:B2`,
			"--scope", "content", "--yes", "--dry-run",
		})
		if err != nil {
			t.Fatalf("a bare range carrying a quoted comma should survive, got: %v", err)
		}
		// One operation, its prefix parsed back into the whole sheet name --
		// the naive split sent "'Q1" and "Sales'!A1:B2" as two.
		if n := strings.Count(stdout, `"tool_name": "clear_cell_range"`); n != 1 {
			t.Errorf("expected 1 cleared area, got %d in %q", n, stdout)
		}
		if !strings.Contains(stdout, `"sheet_name": "Q1,Sales"`) {
			t.Errorf("expected the sheet name intact in the request, got %q", stdout)
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

// A ragged payload states far fewer cells than the rectangle it occupies: one
// 8,000-cell row followed by 2,000 empty ones is 54KB of JSON and a 16M-slot
// matrix. padShortRows used to build that matrix before checkCellBudget ran at
// the end of validate, so the cap could only reject a payload the process had
// already allocated (measured: 575MB peak for this input).
//
// Both shapes reach a padding path -- one with no columns declared, one whose
// column list gets widened -- so both are pinned here. The budget is checked
// while only the slice headers have moved, which is why these return fast
// instead of allocating first.
func TestTablePut_RaggedPayloadBudgetedBeforePadding(t *testing.T) {
	t.Parallel()

	ragged := func(columns []tableColumnSpec, width, empties int) *tableSheetSpec {
		rows := make([][]interface{}, 0, empties+1)
		first := make([]interface{}, width)
		for i := range first {
			first[i] = float64(i)
		}
		rows = append(rows, first)
		for i := 0; i < empties; i++ {
			rows = append(rows, []interface{}{})
		}
		return &tableSheetSpec{Name: "S", Columns: columns, Rows: rows}
	}

	for _, tt := range []struct {
		name    string
		columns []tableColumnSpec
	}{
		{"no columns declared", nil},
		{"declared columns get widened", []tableColumnSpec{{Name: "a"}, {Name: "b"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var projected int64
			s := ragged(tt.columns, 8000, 2000)
			err := fitColumnsToRows(s, &projected)
			requireValidation(t, err, "over the 1000000-cell safety cap")
			// The rectangle must not have been built on the way to that error.
			for r := range s.Rows {
				if r > 0 && len(s.Rows[r]) != 0 {
					t.Fatalf("row %d was padded to %d cells before the budget rejected it", r, len(s.Rows[r]))
				}
			}
		})
	}

	t.Run("a payload inside the cap is still squared off", func(t *testing.T) {
		t.Parallel()
		var projected int64
		s := ragged(nil, 4, 2)
		if err := fitColumnsToRows(s, &projected); err != nil {
			t.Fatalf("a small payload should pad, got: %v", err)
		}
		for r := range s.Rows {
			if len(s.Rows[r]) != 4 {
				t.Errorf("row %d = %d cells, want 4", r, len(s.Rows[r]))
			}
		}
	})

	// The cap bounds the whole payload, so two sheets that each fit but
	// together do not are refused at the one that crosses it.
	t.Run("the total is accumulated across sheets", func(t *testing.T) {
		t.Parallel()
		var projected int64
		big := func() *tableSheetSpec { return ragged(nil, 1000, 699) }
		if err := fitColumnsToRows(big(), &projected); err != nil {
			t.Fatalf("the first sheet fits on its own, got: %v", err)
		}
		requireValidation(t, fitColumnsToRows(big(), &projected), "over the 1000000-cell safety cap")
	})
}

// Omitting the pivot's target sheet is not a missing selector -- it is the
// contract. The backend then creates a fresh sub-sheet for the result, which
// is the zero-overwrite path +pivot-create's own tips recommend. The
// omitted-selector resolver must not answer it: in a single-sheet workbook it
// would name the sheet holding the SOURCE data and land the pivot on top of
// it, and in a multi-sheet one it would demand a selector the command does not
// require. Execute is where this broke; the dry-run and input-builder tests
// never reached the resolver.
func TestPivotCreate_OmittedTargetIsLeftToTheBackend(t *testing.T) {
	t.Parallel()
	const props = `{"rows":[{"field":"a"}],"values":[{"field":"b","statistic_type":"sum"}]}`

	// The stub takes every pivot call and keeps the body, so the assertion is
	// on what was actually sent rather than on which stub happened to match.
	capturing := func(seen *[]byte) *httpmock.Stub {
		return &httpmock.Stub{
			Method: "POST",
			URL:    "/tools/invoke_",
			BodyFilter: func(b []byte) bool {
				if bytes.Contains(b, []byte("manage_pivot_table_object")) {
					*seen = append([]byte(nil), b...)
					return true
				}
				return false
			},
			Body: map[string]interface{}{"code": 0, "msg": "ok",
				"data": map[string]interface{}{"output": `{"ok":true}`}},
		}
	}
	carriesSelector := func(b []byte) bool {
		return bytes.Contains(b, []byte("sheet_name")) || bytes.Contains(b, []byte("sheet_id"))
	}

	for _, tt := range []struct {
		name   string
		sheets []string
	}{
		{"a single-sheet workbook is not used as the target", []string{"Data"}},
		{"a multi-sheet workbook does not demand one either", []string{"Data", "Other"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var seen []byte
			if _, err := runShortcutWithStubs(t, PivotCreate,
				[]string{"--url", testURL, "--source", "'Data'!A1:C10", "--properties", props},
				capturing(&seen)); err != nil {
				t.Fatalf("an omitted target is the documented path, got: %v", err)
			}
			if carriesSelector(seen) {
				t.Errorf("the request carried a placement selector the caller never gave: %s", seen)
			}
		})
	}

	// An explicit target is still honoured and still reaches the request.
	t.Run("an explicit target is forwarded", func(t *testing.T) {
		t.Parallel()
		var seen []byte
		if _, err := runShortcutWithStubs(t, PivotCreate,
			[]string{"--url", testURL, "--source", "'Data'!A1:C10",
				"--target-sheet-name", "Report", "--properties", props},
			capturing(&seen)); err != nil {
			t.Fatalf("an explicit target should be forwarded, got: %v", err)
		}
		if !carriesSelector(seen) {
			t.Errorf("the explicit target should have reached the request: %s", seen)
		}
	})
}

// The other shape that reaches a padding path before any budget check: rows
// are padded out to the DECLARED column count in normalize, long before
// validate's checkCellBudget. 8,000 declared columns against 2,000 one-cell
// rows is 80KB of JSON and a 16M-cell rectangle (measured: 416MB peak, against
// 34MB for a trivial payload). fitColumnsToRows covers the ragged shape; this
// covers the wide-declaration one.
func TestTablePut_DeclaredColumnsBudgetedBeforePadding(t *testing.T) {
	t.Parallel()

	sheetWith := func(columns, rows int) map[string]interface{} {
		cols := make([]interface{}, columns)
		for i := range cols {
			cols[i] = fmt.Sprintf("c%d", i)
		}
		data := make([]interface{}, rows)
		for i := range data {
			data[i] = []interface{}{float64(1)}
		}
		return map[string]interface{}{"name": "S", "columns": cols, "data": data}
	}

	t.Run("a wide declaration over the cap is refused before padding", func(t *testing.T) {
		t.Parallel()
		in := &tableSheetIn{}
		raw, err := json.Marshal(sheetWith(8000, 2000))
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if err := json.Unmarshal(raw, in); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		spec, err := in.normalize(0)
		requireValidation(t, err, "over the 1000000-cell safety cap")
		if len(spec.Rows) != 0 {
			t.Errorf("the spec should come back empty, got %d rows", len(spec.Rows))
		}
	})

	t.Run("a declaration inside the cap still pads", func(t *testing.T) {
		t.Parallel()
		in := &tableSheetIn{}
		raw, err := json.Marshal(sheetWith(4, 3))
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if err := json.Unmarshal(raw, in); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		spec, err := in.normalize(0)
		if err != nil {
			t.Fatalf("a small payload should normalize, got: %v", err)
		}
		for r := range spec.Rows {
			if len(spec.Rows[r]) != 4 {
				t.Errorf("row %d = %d cells, want 4", r, len(spec.Rows[r]))
			}
		}
	})
}

// +styles-put closes a whole-column cell_styles range against the sheet's real
// grid, which is a structure READ from a command declaring only the write
// scope. It carries no sheet selector, so the flag-derived declaration in
// Shortcuts() does not reach it and it has to say so itself.
func TestStylesPut_DeclaresTheGridReadScope(t *testing.T) {
	t.Parallel()
	for _, sc := range Shortcuts() {
		if sc.Command != "+styles-put" {
			continue
		}
		for _, s := range sc.DeclaredScopesForIdentity("user") {
			if s == sheetsStructureReadScope {
				return
			}
		}
		t.Fatalf("+styles-put reads the workbook grid but declares %v", sc.DeclaredScopesForIdentity("user"))
	}
	t.Fatal("+styles-put not found in the registry")
}

// Lifting border_styles out of cell_styles must leave the payload exactly as
// the correctly spelled one, including NOT leaving an empty cell_styles behind
// when the border was its only key.
func TestCellsSet_LiftedBorderMatchesTheCorrectSpelling(t *testing.T) {
	t.Parallel()
	run := func(cells string) string {
		t.Helper()
		stdout, _, err := runShortcutCapturingErr(t, CellsSet, []string{
			"--url", testURL, "--sheet-name", "s", "--range", "A1",
			"--cells", cells, "--dry-run",
		})
		if err != nil {
			t.Fatalf("payload should be accepted: %v", err)
		}
		return stdout
	}
	lifted := run(`[[{"value":"x","cell_styles":{"border_styles":{"all":{"style":"solid"}}}}]]`)
	direct := run(`[[{"value":"x","border_styles":{"all":{"style":"solid"}}}]]`)
	if strings.Contains(lifted, `"cell_styles"`) {
		t.Errorf("an emptied cell_styles should not reach the request: %s", lifted)
	}
	if lifted != direct {
		t.Errorf("lifted payload differs from the correctly spelled one:\n lifted: %s\n direct: %s", lifted, direct)
	}
}

// aboveAverage is a rule_type in the same schema, so "above" names a plausible
// other rule rather than a comparison operator. It keeps the did-you-mean.
func TestCondFormat_AverageWordsStayRefused(t *testing.T) {
	t.Parallel()
	for _, spelling := range []string{"above", "below"} {
		t.Run(spelling, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cond-format-create"), []string{
				"--url", testURL, "--sheet-name", "s", "--ranges", `["A1:B2"]`,
				"--properties", `{"rule_type":"cellIs","attrs":[{"compare_type":"` + spelling +
					`","value":"5"}],"style":{"font_color":"#ff0000"}}`,
				"--dry-run",
			})
			if err == nil {
				t.Fatalf("%q is ambiguous against the aboveAverage rule and must not be rewritten", spelling)
			}
		})
	}
}

// 200 columns and 50000 rows are what the CREATE call accepts, not a limit on
// what a sheet may hold: the backend expands the grid on write. Live-verified
// 2026-09-14 -- GS1 (column 201) and IZ1 (column 260) written into a
// 200-column sheet grew it to A1:IZ200 with both values readable, and A60000
// on a 20-column sheet grew the rows. These payloads used to be refused
// offline; the only cap that is real counts CELLS and is checkCellBudget's.
func TestTablePut_WideAndTallPayloadsAreNotRefusedLocally(t *testing.T) {
	t.Parallel()

	wide := func(columns int) string {
		cols := make([]string, columns)
		for i := range cols {
			cols[i] = fmt.Sprintf("c%d", i)
		}
		sheet := map[string]interface{}{"name": "S", "columns": cols, "data": []interface{}{}}
		out, err := json.Marshal(map[string]interface{}{"sheets": []interface{}{sheet}})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return string(out)
	}

	for _, tt := range []struct{ name, sheets, styles string }{
		{"260 columns", wide(260), ""},
		// The styles pass grows the matrix after the data does, and reached
		// past the same phantom ceiling.
		{"cell_styles at column 201", `{"sheets":[{"name":"S","columns":["a"],"data":[[1]]}]}`,
			`{"styles":[{"name":"S","cell_styles":[{"range":"GS1","font_weight":"bold"}]}]}`},
		{"cell_merges past column 200", `{"sheets":[{"name":"S","columns":["a"],"data":[[1]]}]}`,
			`{"styles":[{"name":"S","cell_merges":[{"range":"A1:GS1"}]}]}`},
		{"col_sizes past column 200", `{"sheets":[{"name":"S","columns":["a"],"data":[[1]]}]}`,
			`{"styles":[{"name":"S","col_sizes":[{"range":"A:GS","type":"pixel","size":80}]}]}`},
		{"row_sizes past row 50000", `{"sheets":[{"name":"S","columns":["a"],"data":[[1]]}]}`,
			`{"styles":[{"name":"S","row_sizes":[{"range":"1:50001","type":"pixel","size":20}]}]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			args := []string{"--url", testURL, "--sheets", tt.sheets}
			if tt.styles != "" {
				args = append(args, "--styles", tt.styles)
			}
			if _, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+table-put"), append(args, "--dry-run")); err != nil {
				t.Fatalf("the backend expands the grid for this; it should not be refused locally, got: %v", err)
			}
		})
	}

	// What the backend does refuse is the cell count, and that cap is already
	// checkCellBudget's -- 50001 rows x 260 columns is the shape that failed
	// live, and it is over maxTablePutCells here too.
	t.Run("the cell budget still binds", func(t *testing.T) {
		t.Parallel()
		no := false
		p := &tablePayload{Sheets: []tableSheetSpec{{Name: "S", Header: &no,
			Columns: make([]tableColumnSpec, 260), Rows: make([][]interface{}, 50001)}}}
		requireValidation(t, p.checkCellBudget(), "cell safety cap")
	})
}

// headerOn cannot see that an appended-to sheet is empty, but writeSheetData
// forces a header there anyway. Every consumer of "will a header be written"
// has to use the same predicate, or the budget undercounts by a row.
func TestBudgetRows_CountsTheForcedAppendHeader(t *testing.T) {
	t.Parallel()
	appendNoChoice := &tableSheetSpec{Name: "S", Mode: "append",
		Columns: make([]tableColumnSpec, 200), Rows: make([][]interface{}, 5000)}
	if got := headerRowCount(appendNoChoice); got != 1 {
		t.Errorf("append with no header choice may still write one, got %d", got)
	}
	p := &tablePayload{Sheets: []tableSheetSpec{*appendNoChoice}}
	requireValidation(t, p.checkCellBudget(), "1000200 cells")

	// An explicit header:false on append writes none, and is budgeted as none.
	no := false
	explicit := &tableSheetSpec{Name: "S", Mode: "append", Header: &no,
		Columns: make([]tableColumnSpec, 200), Rows: make([][]interface{}, 5000)}
	if got := headerRowCount(explicit); got != 0 {
		t.Errorf("an explicit header:false writes no header, got %d", got)
	}
	if err := (&tablePayload{Sheets: []tableSheetSpec{*explicit}}).checkCellBudget(); err != nil {
		t.Errorf("1,000,000 cells is at the cap, got: %v", err)
	}
}
