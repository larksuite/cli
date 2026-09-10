// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"
	"testing"
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
