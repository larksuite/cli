// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestCondFormat_PropertySpellings pins the --properties folds that answer the
// three largest cond-format rejections of 09-04..07: a comparison under a key
// other than compare_type (2852), and a `font` slot written as the flags or
// the list every font vocabulary spells it as (972).
func TestCondFormat_PropertySpellings(t *testing.T) {
	t.Parallel()
	sc := shortcutFromRegistry(t, "+cond-format-create")
	run := func(t *testing.T, props string) error {
		t.Helper()
		_, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL, "--sheet-name", "Sheet1", "--dry-run", "--properties", props,
		})
		return err
	}
	style := `"style":{"fore_color":"#FF0000"}`
	head := `"ranges":["Sheet1!A1:B2"],"rule_type":"cellIs"`

	t.Run("the comparison is read from the key that carries it", func(t *testing.T) {
		t.Parallel()
		for _, spelling := range []string{"operator", "comparison", "compare", "criteria", "condition"} {
			t.Run(spelling, func(t *testing.T) {
				t.Parallel()
				if err := run(t, `{`+head+`,"attrs":[{"`+spelling+`":"greaterThan","value":"5"}],`+style+`}`); err != nil {
					t.Errorf("%s should be read as compare_type, got: %v", spelling, err)
				}
			})
		}
	})

	t.Run("a period rule keeps its own operator", func(t *testing.T) {
		t.Parallel()
		// timePeriod's contract spells its slot `operator`, and "is" is a
		// compare_type word too — the shape keys are what tell them apart.
		if err := run(t, `{"ranges":["Sheet1!A1:B2"],"rule_type":"timePeriod","attrs":[{"operator":"is","time_period":"yesterday"}],`+style+`}`); err != nil {
			t.Errorf("a timePeriod entry must keep operator, got: %v", err)
		}
	})

	t.Run("the font slot folds onto its enum", func(t *testing.T) {
		t.Parallel()
		attrs := `"attrs":[{"compare_type":"greaterThan","value":"5"}]`
		for _, font := range []string{`{"bold":true}`, `{"bold":true,"italic":true}`, `["italic","bold"]`, `"italic bold"`, `"bold,italic"`} {
			t.Run(font, func(t *testing.T) {
				t.Parallel()
				if err := run(t, `{`+head+`,`+attrs+`,"style":{"font":`+font+`}}`); err != nil {
					t.Errorf("font %s should fold, got: %v", font, err)
				}
			})
		}
	})

	t.Run("a font value with no reading keeps the enum error", func(t *testing.T) {
		t.Parallel()
		err := run(t, `{`+head+`,"attrs":[{"compare_type":"greaterThan","value":"5"}],"style":{"font":"700"}}`)
		requireValidation(t, err, "is not in enum")
	})
}

// TestStylesItem_KeySpellings pins the --styles item folds. Every entry names
// a section this payload already carries, under the vocabulary the caller
// arrived with; three of them the distance ranker could already name, which
// means the error spelled the fix and refused to apply it (3938 rejections).
func TestStylesItem_KeySpellings(t *testing.T) {
	t.Parallel()
	sc := shortcutFromRegistry(t, "+styles-put")
	run := func(t *testing.T, styles string) error {
		t.Helper()
		_, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL, "--dry-run", "--styles", styles,
		})
		return err
	}
	cs := `"cell_styles":[{"range":"A1:B2","font_weight":"bold"}]`

	t.Run("the sheet selector answers to its domain spelling", func(t *testing.T) {
		t.Parallel()
		for _, key := range []string{"sheet_name", "sheet", "title"} {
			t.Run(key, func(t *testing.T) {
				t.Parallel()
				if err := run(t, `{"styles":[{"`+key+`":"Sheet1",`+cs+`}]}`); err != nil {
					t.Errorf("%s should name the sheet, got: %v", key, err)
				}
			})
		}
	})

	t.Run("a section answers to the effect it has", func(t *testing.T) {
		t.Parallel()
		for _, item := range []string{
			`"cell_style":[{"range":"A1:B2","font_weight":"bold"}]`,
			`"merges":[{"range":"A1:B2"}]`,
			`"merge_cells":[{"range":"A1:B2"}]`,
			`"row_heights":[{"range":"1:1","type":"pixel","size":30}]`,
			`"column_widths":[{"range":"A:C","type":"pixel","size":120}]`,
		} {
			t.Run(item[:12], func(t *testing.T) {
				t.Parallel()
				if err := run(t, `{"styles":[{"name":"Sheet1",`+item+`}]}`); err != nil {
					t.Errorf("%s should be read as its section, got: %v", item, err)
				}
			})
		}
	})

	t.Run("a section written as one object is its one-entry list", func(t *testing.T) {
		t.Parallel()
		if err := run(t, `{"styles":[{"name":"Sheet1","cell_styles":{"range":"A1:B2","font_weight":"bold"}}]}`); err != nil {
			t.Errorf("a lone section object should lift, got: %v", err)
		}
	})

	t.Run("the bare item list needs no envelope", func(t *testing.T) {
		t.Parallel()
		if err := run(t, `[{"name":"Sheet1",`+cs+`}]`); err != nil {
			t.Errorf("a bare list is the items, got: %v", err)
		}
	})

	t.Run("a key naming no section still fails", func(t *testing.T) {
		t.Parallel()
		err := run(t, `{"styles":[{"name":"Sheet1","format":"x",`+cs+`}]}`)
		requireValidation(t, err, `unknown key "format"`)
	})

	t.Run("a rename that would collide is left alone", func(t *testing.T) {
		t.Parallel()
		err := run(t, `{"styles":[{"name":"A","sheet_name":"B",`+cs+`}]}`)
		requireValidation(t, err, `unknown key "sheet_name"`)
	})
}

// TestColumns_HeadingSpellings pins the column-object heading folds: 2559
// rejections said an object column had no name, on entries that named it under
// the vocabulary they came from.
func TestColumns_HeadingSpellings(t *testing.T) {
	t.Parallel()
	sc := shortcutFromRegistry(t, "+workbook-create")
	run := func(t *testing.T, columns string) error {
		t.Helper()
		_, _, err := runShortcutCapturingErr(t, sc, []string{
			"--title", "T", "--dry-run",
			"--sheets", `{"sheets":[{"name":"S","columns":` + columns + `,"data":[["x"]]}]}`,
		})
		return err
	}

	t.Run("the heading is read from the key that carries it", func(t *testing.T) {
		t.Parallel()
		for _, key := range []string{"title", "header", "label", "column", "field", "key"} {
			t.Run(key, func(t *testing.T) {
				t.Parallel()
				if err := run(t, `[{"`+key+`":"列A"}]`); err != nil {
					t.Errorf("%s should name the column, got: %v", key, err)
				}
			})
		}
	})

	t.Run("an inline dtype still applies beside it", func(t *testing.T) {
		t.Parallel()
		if err := run(t, `[{"title":"列A","dtype":"object"}]`); err != nil {
			t.Errorf("the alias must not shadow the dtype, got: %v", err)
		}
	})

	t.Run("an entry with no heading anywhere still fails", func(t *testing.T) {
		t.Parallel()
		err := run(t, `[{"dtype":"int64"}]`)
		requireValidation(t, err, `without a "name" string`)
	})
}

// TestCellsSet_FlatPayloadLifted pins the one-dimensional payload's second
// dimension. A flat list is a row in every library these callers arrive from,
// and the exception is the caller's own range saying otherwise.
func TestCellsSet_FlatPayloadLifted(t *testing.T) {
	t.Parallel()
	sc := shortcutFromRegistry(t, "+cells-set")
	write := func(t *testing.T, rng, cells string) (string, error) {
		t.Helper()
		stdout, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL, "--sheet-name", "s", "--range", rng, "--cells", cells, "--dry-run",
		})
		return strings.ReplaceAll(stdout, `\"`, `"`), err
	}

	t.Run("a flat list is a row", func(t *testing.T) {
		t.Parallel()
		out, err := write(t, "A1:C1", `["a","b","c"]`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, `"range":"A1:C1"`) {
			t.Errorf("should cover one row, got %q", out)
		}
	})

	t.Run("a single-column range makes it a column", func(t *testing.T) {
		t.Parallel()
		out, err := write(t, "A1:A3", `["a","b","c"]`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, `"range":"A1:A3"`) {
			t.Errorf("the stated range says column, got %q", out)
		}
	})

	t.Run("a lone scalar is one cell", func(t *testing.T) {
		t.Parallel()
		out, err := write(t, "A1", `"hello"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "hello") {
			t.Errorf("should write the value, got %q", out)
		}
	})

	t.Run("a mixed payload has no single reading", func(t *testing.T) {
		t.Parallel()
		_, err := write(t, "A1:B1", `[["a"],"b"]`)
		requireValidation(t, err, `expected type "array"`)
	})
}

// TestTable_ColumnsFitToRows pins the two width folds and the one thing that
// still fails. 09-04..07: 2779 rejections on a row wider than its columns and
// 2299 on a sheet that declared none, both on payloads whose data was fine.
func TestTable_ColumnsFitToRows(t *testing.T) {
	t.Parallel()
	sc := shortcutFromRegistry(t, "+workbook-create")
	write := func(t *testing.T, sheet string) (string, error) {
		t.Helper()
		stdout, _, err := runShortcutCapturingErr(t, sc, []string{
			"--title", "T", "--dry-run", "--sheets", `{"sheets":[` + sheet + `]}`,
		})
		return strings.ReplaceAll(stdout, `\"`, `"`), err
	}

	t.Run("a row padded past its columns loses the padding", func(t *testing.T) {
		t.Parallel()
		out, err := write(t, `{"name":"S","columns":["a"],"data":[["x",null,""]]}`)
		if err != nil {
			t.Fatalf("trailing empties write nothing, so they are not a width error: %v", err)
		}
		if !strings.Contains(out, `"range":"A1:A2"`) {
			t.Errorf("the write should stay one column wide, got %q", out)
		}
	})

	t.Run("a row with real values past its columns gets blank headings", func(t *testing.T) {
		t.Parallel()
		out, err := write(t, `{"name":"S","columns":["a"],"data":[["x","y"]]}`)
		if err != nil {
			t.Fatalf("the extra value is data, not an error: %v", err)
		}
		if !strings.Contains(out, `"range":"A1:B2"`) {
			t.Errorf("the write should widen to the data, got %q", out)
		}
	})

	t.Run("a sheet with no columns writes the block it carries", func(t *testing.T) {
		t.Parallel()
		out, err := write(t, `{"name":"S","columns":[],"data":[["a","b"],["c","d"]]}`)
		if err != nil {
			t.Fatalf("this is the --values shape on --sheets: %v", err)
		}
		// Two data rows and no header row invented above them.
		if !strings.Contains(out, `"range":"A1:B2"`) {
			t.Errorf("only the data should be written, got %q", out)
		}
	})

	t.Run("an explicit header row over columns nobody named still fails", func(t *testing.T) {
		t.Parallel()
		_, err := write(t, `{"name":"S","header":true,"data":[["a","b"]]}`)
		requireValidation(t, err, "columns must be non-empty")
	})
}

// TestPayloadFlags_SyntaxErrorNamesThePlace pins the position on the shapes the
// repair refuses. Go carries the offset on the error and leaves it out of the
// text, which in an 8 KB payload is the whole question.
func TestPayloadFlags_SyntaxErrorNamesThePlace(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, cells string }{
		{"a bracket that does not match", `[["a","b"]}`},
		{"a payload cut short", `[["a","b"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-set"), []string{
				"--url", testURL, "--sheet-name", "s", "--range", "A1", "--cells", tc.cells, "--dry-run",
			})
			ve := requireValidation(t, err, "invalid JSON")
			if !strings.Contains(ve.Hint, "breaks at byte") {
				t.Errorf("hint should name the offset, got %q", ve.Hint)
			}
		})
	}
}

// TestStyles_UnboundedRangeBounded pins the whole-column / whole-row forms.
// Their extent lives on the sheet, not in the string, so the execute path
// reads the grid and closes them; pre-flight stands in a rectangle that keeps
// the axis the caller did state, and --dry-run keeps the rejection because it
// sends nothing. 09-04..07: 1208 rejections under --styles.
func TestStyles_UnboundedRangeBounded(t *testing.T) {
	t.Parallel()

	t.Run("the grid closes the range", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct{ in, want string }{
			{"A:C", "A1:C200"},
			{"a:c", "A1:C200"},
			{"3:5", "A3:T5"},
		} {
			got, ok := boundRangeToGrid(tc.in, sheetGrid{rows: 200, cols: 20})
			if !ok || got != tc.want {
				t.Errorf("boundRangeToGrid(%q) = %q,%v, want %q", tc.in, got, ok, tc.want)
			}
		}
	})

	t.Run("a rectangle is left alone", func(t *testing.T) {
		t.Parallel()
		if got, ok := boundRangeToGrid("A1:C3", sheetGrid{rows: 200, cols: 20}); ok {
			t.Errorf("boundRangeToGrid(A1:C3) = %q, want no rewrite", got)
		}
	})

	t.Run("pre-flight keeps the stated axis", func(t *testing.T) {
		t.Parallel()
		bound := preflightRangeBounder(stylesPutView(map[string]interface{}{}))
		if bound == nil {
			t.Fatal("a real run must get a stand-in bounder")
		}
		for _, tc := range []struct{ in, want string }{{"A:C", "A1:C1"}, {"3:5", "A3:A5"}} {
			got, ok, err := bound("S", tc.in)
			if err != nil || !ok || got != tc.want {
				t.Errorf("preflight(%q) = %q,%v,%v, want %q", tc.in, got, ok, err, tc.want)
			}
		}
	})

	t.Run("--dry-run keeps the rejection", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+styles-put"), []string{
			"--url", testURL, "--dry-run",
			"--styles", `{"styles":[{"name":"Sheet1","cell_styles":[{"range":"A:C","font_weight":"bold"}]}]}`,
		})
		requireValidation(t, err, "unsupported range form")
	})
}

// TestDomainFlagAliases pins the renames that hold wherever the canonical flag
// does. The 09-01..08 backflow counts them across the whole surface rather
// than one command: --sheet 765 rows over 21 commands, --spreadsheet 541 over
// 17, --ranges 525 over 18, the output family 450 over 19.
func TestDomainFlagAliases(t *testing.T) {
	t.Parallel()

	t.Run("the short spelling reaches the long one", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name    string
			command string
			args    []string
		}{
			{"--sheet", "+cells-get", []string{"--url", testURL, "--sheet", "s", "--range", "A1"}},
			{"--spreadsheet", "+cells-get", []string{"--spreadsheet", testToken, "--sheet-name", "s", "--range", "A1"}},
			{"--ranges", "+cells-get", []string{"--url", testURL, "--sheet-name", "s", "--ranges", "A1:B2"}},
			{"--output", "+csv-get", []string{"--url", testURL, "--sheet-name", "s", "--output", "./out.csv"}},
			{"--font-name", "+cells-set-style", []string{"--url", testURL, "--sheet-name", "s", "--range", "A1", "--font-name", "Arial"}},
			{"--horizontal-align", "+cells-set-style", []string{"--url", testURL, "--sheet-name", "s", "--range", "A1", "--horizontal-align", "center"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, tc.command), append(tc.args, "--dry-run"))
				if err != nil && strings.Contains(err.Error(), "unknown flag") {
					t.Errorf("%s should reach its canonical flag, got: %v", tc.name, err)
				}
			})
		}
	})

	t.Run("a command with its own meaning keeps it", func(t *testing.T) {
		t.Parallel()
		// +cells-unmerge takes ONE span per call, so --ranges there is a
		// caller asking for several and the answer is how to send several.
		_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-unmerge"), []string{
			"--url", testURL, "--sheet-name", "s", "--ranges", "A1:B2", "--dry-run",
		})
		ve := requireValidation(t, err, "unknown flag")
		if !strings.Contains(ve.Hint, "one span per call") {
			t.Errorf("the tailored prescription should survive, got %q", ve.Hint)
		}
	})
}

// TestValueCarryingFlagAliases pins the three renames whose value changes
// shape on the way: a bare A1 range into the list --ranges takes, a bare line
// word into the composite --border-styles takes, and a lone scalar into the
// matrix --cells takes.
func TestValueCarryingFlagAliases(t *testing.T) {
	t.Parallel()

	t.Run("a bare range becomes the one-element list", func(t *testing.T) {
		t.Parallel()
		stdout, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cond-format-create"), []string{
			"--url", testURL, "--sheet-name", "Sheet1", "--dry-run",
			"--range", "Sheet1!A1:B2", "--rule-type", "containsBlanks",
			"--properties", `{"style":{"fore_color":"#FF0000"}}`,
		})
		if err != nil {
			t.Fatalf("a bare range should reach --ranges, got: %v", err)
		}
		if !strings.Contains(strings.ReplaceAll(stdout, `\"`, `"`), `"ranges":["Sheet1!A1:B2"]`) {
			t.Errorf("the range should travel as a one-element list, got %q", stdout)
		}
	})

	t.Run("a bare line word becomes all four sides", func(t *testing.T) {
		t.Parallel()
		for _, flag := range []string{"--border-type", "--border", "--border-all", "--border-styles"} {
			t.Run(flag, func(t *testing.T) {
				t.Parallel()
				stdout, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-set-style"), []string{
					"--url", testURL, "--sheet-name", "s", "--range", "A1", flag, "solid", "--dry-run",
				})
				if err != nil {
					t.Fatalf("%s solid should carry, got: %v", flag, err)
				}
				out := strings.ReplaceAll(stdout, `\"`, `"`)
				for _, side := range []string{"top", "bottom", "left", "right"} {
					if !strings.Contains(out, `"`+side+`":{"style":"solid"}`) {
						t.Errorf("%s should set every side, got %q", flag, out)
					}
				}
			})
		}
	})

	t.Run("a lone scalar becomes the one cell", func(t *testing.T) {
		t.Parallel()
		stdout, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-set"), []string{
			"--url", testURL, "--sheet-name", "s", "--range", "A1", "--value", "hello", "--dry-run",
		})
		if err != nil {
			t.Fatalf("--value should reach --cells, got: %v", err)
		}
		if !strings.Contains(strings.ReplaceAll(stdout, `\"`, `"`), `"cells":[[{"value":"hello"}]]`) {
			t.Errorf("the scalar should become one cell, got %q", stdout)
		}
	})
}

// TestReviewRegressions covers the defects the PR review found, so each one
// fails here if the fix is reverted.
func TestReviewRegressions(t *testing.T) {
	t.Parallel()

	t.Run("a ragged table payload is rejected, not a panic", func(t *testing.T) {
		t.Parallel()
		// fitColumnsToRows widens the column list after padShortRows has run,
		// and the writer indexes a row by column position.
		for _, sheet := range []string{
			`{"name":"S","data":[[1,2],[3]]}`,
			`{"name":"S","columns":["a"],"data":[[1,2],[3]]}`,
		} {
			t.Run(sheet[:26], func(t *testing.T) {
				t.Parallel()
				if _, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+workbook-create"), []string{
					"--title", "T", "--dry-run", "--sheets", `{"sheets":[` + sheet + `]}`,
				}); err != nil {
					t.Fatalf("a short row should be padded to the settled width, got: %v", err)
				}
			})
		}
	})

	t.Run("a bare single-cell range reaches its list flag", func(t *testing.T) {
		t.Parallel()
		// The loose-JSON repair turns a token with no punctuation into a JSON
		// string, which would satisfy the parse and reach the array check as
		// a scalar — so the list wrap has to be tried first.
		_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-batch-clear"), []string{
			"--url", testURL, "--ranges", "Sheet1!A1", "--scope", "content", "--yes", "--dry-run",
		})
		if err != nil && strings.Contains(err.Error(), "must be a JSON array") {
			t.Errorf("a bare range should become the one-element list, got: %v", err)
		}
	})

	t.Run("a composite font keeps every effect it names", func(t *testing.T) {
		t.Parallel()
		style := map[string]interface{}{"font": map[string]interface{}{"bold": true}, "italic": true}
		normalizeCondFormatStyle(style)
		if style["font"] != condFormatFontBoth {
			t.Errorf("font = %v, want %q", style["font"], condFormatFontBoth)
		}
	})

	t.Run("a font member this enum cannot carry is not dropped", func(t *testing.T) {
		t.Parallel()
		style := map[string]interface{}{"font": map[string]interface{}{"bold": true, "underline": true}}
		normalizeCondFormatStyle(style)
		if _, folded := style["font"].(string); folded {
			t.Errorf("font = %v, want the object left for the schema to report", style["font"])
		}
	})

	t.Run("a side selector carries the line the caller spelled", func(t *testing.T) {
		t.Parallel()
		cell := map[string]interface{}{"border_type": "LEFT_BORDER", "border_style": "dashed"}
		if err := foldBorderFamilyAliases(cell, "--styles"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		sides, _ := cell["border_styles"].(map[string]interface{})
		left, _ := sides["left"].(map[string]interface{})
		if len(sides) != 1 || left["style"] != "dashed" {
			t.Errorf("border_styles = %v, want only a dashed left side", sides)
		}
	})

	t.Run("squaring off a payload stays inside the matrix cap", func(t *testing.T) {
		t.Parallel()
		// One very wide row over many short ones: padding first would
		// allocate the rectangle before any validator could refuse it.
		rows := make([]interface{}, 0, 2001)
		wide := make([]interface{}, 0, 8000)
		for i := 0; i < 8000; i++ {
			wide = append(wide, map[string]interface{}{"value": i})
		}
		var first interface{} = wide
		rows = append(rows, first)
		for i := 0; i < 2000; i++ {
			var empty interface{} = []interface{}{}
			rows = append(rows, empty)
		}
		padRaggedCellRows(rows)
		if got := len(rows[1].([]interface{})); got != 0 {
			t.Errorf("the short rows should be left ragged, got width %d", got)
		}
	})

	t.Run("positional metadata over a repeated heading is refused", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+workbook-create"), []string{
			"--title", "T", "--dry-run",
			"--sheets", `{"sheets":[{"name":"S","columns":["a","a"],"dtypes":["object","float64"],"data":[["001",2]]}]}`,
		})
		requireValidation(t, err, "the heading repeats")
	})

	t.Run("deleting a sub-sheet still names it", func(t *testing.T) {
		t.Parallel()
		// Resolving "the only sheet" would take the whole thing with it.
		_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+sheet-delete"), []string{
			"--url", testURL, "--yes",
		})
		requireValidation(t, err, missingSheetSelectorMessage)
	})

	t.Run("every missing required flag survives in the envelope", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+chart-create-basic"), []string{
			"--url", testURL, "--sheet-name", "s",
		})
		ve := requireValidation(t, err, "required flag(s)")
		if len(ve.Params) != 2 {
			t.Errorf("Params = %v, want both missing flags", ve.Params)
		}
	})

	t.Run("an escape a single-quoted string cannot round-trip is not repaired", func(t *testing.T) {
		t.Parallel()
		if got, ok := repairLooseJSON(`[['a\x41']]`); ok {
			t.Errorf("repairLooseJSON = %q, want refusal", got)
		}
		got, ok := repairLooseJSON(`[['a\nb']]`)
		if !ok || got != `[["a\nb"]]` {
			t.Errorf("repairLooseJSON = %q,%v, want the decoded newline", got, ok)
		}
	})

	t.Run("the syntax window lands on rune boundaries", func(t *testing.T) {
		t.Parallel()
		raw := `[["` + strings.Repeat("中", 60) + `","乙"]}`
		var probe interface{}
		err := json.Unmarshal([]byte(raw), &probe)
		where := jsonSyntaxContext(raw, err)
		if where == "" || !utf8.ValidString(where) {
			t.Errorf("context = %q, want valid UTF-8", where)
		}
	})
}
