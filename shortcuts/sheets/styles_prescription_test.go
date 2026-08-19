// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestTablePut_StylesErrorsAggregate pins the one-retry contract for
// --styles: every issue across sections and ops is reported in a single
// error (eval V2U032 burned three round trips fixing a border side, then
// row_sizes.type, then size — each surfaced only after the previous fix).
func TestTablePut_StylesErrorsAggregate(t *testing.T) {
	t.Parallel()
	sc := shortcutFromRegistry(t, "+table-put")
	_, _, err := runShortcutCapturingErr(t, sc, []string{
		"--url", testURL,
		"--sheets", `{"sheets":[{"name":"s","columns":["a"],"data":[["x"]]}]}`,
		"--styles", `{"styles":[{"name":"s",
			"cell_styles":[{"range":"A1:A1","border_styles":{"horizontal":{"style":"solid"}}}],
			"row_sizes":[{"range":"1:1","type":"custom"}],
			"col_sizes":[{"range":"A:A","type":"pixel"}]}]}`,
		"--dry-run",
	})
	ve := requireValidation(t, err, "--styles has 3 issues")
	for _, want := range []string{
		"border_styles.horizontal is not a valid side",
		`row_sizes[0].type "custom" is invalid`,
		"col_sizes[0].type pixel requires size",
	} {
		if !strings.Contains(ve.Message, want) {
			t.Errorf("aggregated message should contain %q, got %q", want, ve.Message)
		}
	}
	// D2: each type/size error inlines a complete valid op.
	if !strings.Contains(ve.Message, `{"range":"2:10","type":"pixel","size":32}`) {
		t.Errorf("row_sizes error should inline a full valid example, got %q", ve.Message)
	}
	if !strings.Contains(ve.Message, `{"range":"A:C","type":"pixel","size":120}`) {
		t.Errorf("col_sizes error should inline a full valid example, got %q", ve.Message)
	}
}

// TestTablePut_StylesFieldPrescriptions pins the curated fixes for the
// high-frequency unsupported cell_styles field names, and the near-typo
// guard on the did-you-mean fallback (07-28 root-cause report #14/#21/#27:
// font_bold used to draw "did you mean font_color?" and nested font drew
// "font_line" — concept-swap neighbors that mislead worse than silence).
func TestTablePut_StylesFieldPrescriptions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		field      string // JSON fragment inside the cell_styles item
		want       []string
		notSuggest []string // must NOT appear as a did-you-mean
	}{
		{"bold", `"bold":true`, []string{`font_weight:"bold"`}, nil},
		{"font_bold", `"font_bold":true`, []string{`font_weight:"bold"`}, []string{"font_color"}},
		{"text_align", `"text_align":"center"`, []string{"horizontal_alignment"}, nil},
		{"nested font", `"font":{"bold":true,"size":18}`, []string{"flat font_*", `font_weight:"bold"`}, []string{"font_line"}},
		{"near-typo still suggests", `"font_colour":"#FFF"`, []string{`did you mean "font_color"`}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sc := shortcutFromRegistry(t, "+table-put")
			_, _, err := runShortcutCapturingErr(t, sc, []string{
				"--url", testURL,
				"--sheets", `{"sheets":[{"name":"s","columns":["a"],"data":[["x"]]}]}`,
				"--styles", `{"styles":[{"name":"s","cell_styles":[{"range":"A1:A1",` + tc.field + `}]}]}`,
				"--dry-run",
			})
			ve := requireValidation(t, err, "is not a supported style field")
			for _, want := range tc.want {
				if !strings.Contains(ve.Message, want) {
					t.Errorf("message should contain %q, got %q", want, ve.Message)
				}
			}
			for _, bad := range tc.notSuggest {
				if strings.Contains(ve.Message, `did you mean "`+bad+`"`) {
					t.Errorf("message must not suggest %q, got %q", bad, ve.Message)
				}
			}
		})
	}
}

// TestTablePut_StylesBorderAllExpands verifies the "all" shorthand is
// rewritten to four explicit sides instead of being rejected (or worse,
// passed through for the server to reject, as happened on the typed-cells
// path in eval V2U013/V2U021).
func TestTablePut_StylesBorderAllExpands(t *testing.T) {
	t.Parallel()
	sc := shortcutFromRegistry(t, "+table-put")
	stdout, _, err := runShortcutCapturingErr(t, sc, []string{
		"--url", testURL,
		"--sheets", `{"sheets":[{"name":"s","columns":["a"],"data":[["x"]]}]}`,
		"--styles", `{"styles":[{"name":"s","cell_styles":[{"range":"A1:A1","border_styles":{"all":{"style":"solid","weight":"thin"}}}]}]}`,
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("border all should expand to four sides and pass, got: %v", err)
	}
	// table-put's dry-run body carries the tool input as an escaped JSON
	// string, so match the escaped key form.
	for _, side := range []string{`\"top\"`, `\"bottom\"`, `\"left\"`, `\"right\"`} {
		if !strings.Contains(stdout, side) {
			t.Errorf("dry-run body should carry expanded side %s, got %q", side, stdout)
		}
	}
	if strings.Contains(stdout, `\"all\"`) {
		t.Errorf("dry-run body must not carry the raw all shorthand, got %q", stdout)
	}
}

// TestCellsSet_BorderAllAndMisNestedBorder covers the typed --cells path:
// the "all" shorthand expands CLI-side, and border_styles mis-nested inside
// cell_styles is intercepted with a move-it prescription instead of a
// server-side 900015206.
func TestCellsSet_BorderAllAndMisNestedBorder(t *testing.T) {
	t.Parallel()

	t.Run("border all expands", func(t *testing.T) {
		t.Parallel()
		sc := shortcutFromRegistry(t, "+cells-set")
		stdout, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--sheet-name", "s",
			"--range", "A1",
			"--cells", `[[{"value":"x","border_styles":{"all":{"style":"solid"}}}]]`,
			"--dry-run",
		})
		if err != nil {
			t.Fatalf("border all should expand and pass, got: %v", err)
		}
		if strings.Contains(stdout, `"all"`) || !strings.Contains(stdout, `"top"`) {
			t.Errorf("dry-run body should carry expanded sides, got %q", stdout)
		}
	})

	t.Run("mis-nested border_styles intercepted", func(t *testing.T) {
		t.Parallel()
		sc := shortcutFromRegistry(t, "+cells-set")
		_, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--sheet-name", "s",
			"--range", "A1",
			"--cells", `[[{"value":"x","cell_styles":{"font_weight":"bold","border_styles":{"top":{"style":"solid"}}}}]]`,
			"--dry-run",
		})
		ve := requireValidation(t, err, "cell_styles.border_styles is not valid")
		if !strings.Contains(ve.Message, "sibling of cell_styles") {
			t.Errorf("message should prescribe moving it up one level, got %q", ve.Message)
		}
	})
}

// TestCellsSetStyle_BorderAllExpands covers the --border-styles flag path
// (+cells-set-style / +cells-batch-set-style go through borderStylesFromFlag,
// not the typed --cells or --styles walkers): the "all" shorthand must expand
// CLI-side here too, or the backend rejects {"all":…}.
func TestCellsSetStyle_BorderAllExpands(t *testing.T) {
	t.Parallel()
	sc := shortcutFromRegistry(t, "+cells-set-style")
	stdout, _, err := runShortcutCapturingErr(t, sc, []string{
		"--url", testURL,
		"--sheet-name", "s",
		"--range", "A1:A1",
		"--border-styles", `{"all":{"style":"solid","weight":"thin"}}`,
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("border all should expand to four sides and pass, got: %v", err)
	}
	for _, side := range []string{`"top"`, `"bottom"`, `"left"`, `"right"`} {
		if !strings.Contains(stdout, side) {
			t.Errorf("dry-run body should carry expanded side %s, got %q", side, stdout)
		}
	}
	if strings.Contains(stdout, `"all"`) {
		t.Errorf("dry-run body must not carry the raw all shorthand, got %q", stdout)
	}
}

// TestCellsMerge_RawAPIVocabularyNormalizes pins MERGE_ALL → all (the raw
// OpenAPI enum agents copy from Lark API docs) via the enum alias table.
func TestCellsMerge_RawAPIVocabularyNormalizes(t *testing.T) {
	t.Parallel()
	sc := shortcutFromRegistry(t, "+cells-merge")
	stdout, _, err := runShortcutCapturingErr(t, sc, []string{
		"--url", testURL,
		"--sheet-name", "s",
		"--range", "A1:B2",
		"--merge-type", "MERGE_ALL",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("MERGE_ALL should normalize to all and pass, got: %v", err)
	}
	if !strings.Contains(stdout, `"all"`) {
		t.Errorf("dry-run body should carry the normalized merge type, got %q", stdout)
	}
}

// TestCellsSetStyle_WordWrapBooleanNormalizes pins --word-wrap true →
// auto-wrap (eval V2U029).
func TestCellsSetStyle_WordWrapBooleanNormalizes(t *testing.T) {
	t.Parallel()
	sc := shortcutFromRegistry(t, "+cells-set-style")
	stdout, _, err := runShortcutCapturingErr(t, sc, []string{
		"--url", testURL,
		"--sheet-name", "s",
		"--range", "A1:A1",
		"--word-wrap", "true",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("--word-wrap true should normalize to auto-wrap, got: %v", err)
	}
	if !strings.Contains(stdout, "auto-wrap") {
		t.Errorf("dry-run body should carry auto-wrap, got %q", stdout)
	}
}

// TestCellsSetStyle_WordWrapGoogleVocabularyNormalizes pins the Google
// Sheets wrapStrategy words (WRAP / CLIP) onto the Lark enum — the flag
// name --wrap-strategy already prescribes --word-wrap, so the value
// vocabulary has to land too or the retry fails a second time.
func TestCellsSetStyle_WordWrapGoogleVocabularyNormalizes(t *testing.T) {
	t.Parallel()
	for word, want := range map[string]string{"wrap": "auto-wrap", "WRAP": "auto-wrap", "clip": "word-clip"} {
		t.Run(word, func(t *testing.T) {
			t.Parallel()
			sc := shortcutFromRegistry(t, "+cells-set-style")
			stdout, _, err := runShortcutCapturingErr(t, sc, []string{
				"--url", testURL,
				"--sheet-name", "s",
				"--range", "A1:A1",
				"--word-wrap", word,
				"--dry-run",
			})
			if err != nil {
				t.Fatalf("--word-wrap %s should normalize to %s, got: %v", word, want, err)
			}
			if !strings.Contains(stdout, want) {
				t.Errorf("dry-run body should carry %s, got %q", want, stdout)
			}
		})
	}
}

// TestCellsSetStyle_BorderWeightNumberNamesEnum pins the enum-over-skeleton
// rule: a type mismatch on an enum-bearing field answers with the allowed
// values, not a whole-payload skeleton ({"bottom": {…}, "left": {…}, …}
// told the caller nothing about thin/medium/thick — 07-28 root-cause
// report #5, 75 occurrences).
//
// The probe is a boolean because a NUMBER no longer reaches this path: a
// numeric weight reads as a line width and normalizes to a bucket
// (a number reads as a line width). Only a value with no width reading
// has to answer with the enum.
func TestCellsSetStyle_BorderWeightNumberNamesEnum(t *testing.T) {
	t.Parallel()
	sc := shortcutFromRegistry(t, "+cells-set-style")
	_, _, err := runShortcutCapturingErr(t, sc, []string{
		"--url", testURL,
		"--sheet-name", "s",
		"--range", "A1",
		"--border-styles", `{"top":{"style":"solid","weight":true}}`,
		"--dry-run",
	})
	ve := requireValidation(t, err, `expected type "string", got "boolean"`)
	for _, want := range []string{`"thin"`, `"medium"`, `"thick"`} {
		if !strings.Contains(ve.Message, want) {
			t.Errorf("message should name the weight enum %s, got %q", want, ve.Message)
		}
	}
	if strings.Contains(ve.Message, "expected shape:") {
		t.Errorf("enum-bearing mismatch should not fall back to the shape skeleton, got %q", ve.Message)
	}
}

// TestUnderscoreFlagFormsParse pins the wire-vocabulary underscore rewrite:
// --sheet_name / --border_styles parse as their hyphen forms (agents copy
// field names out of JSON payloads where underscores are canonical).
func TestUnderscoreFlagFormsParse(t *testing.T) {
	t.Parallel()
	sc := shortcutFromRegistry(t, "+cells-set-style")
	stdout, _, err := runShortcutCapturingErr(t, sc, []string{
		"--url", testURL,
		"--sheet_name", "s",
		"--range", "A1:A1",
		"--font_weight", "bold",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("underscore flag forms should parse as hyphen forms, got: %v", err)
	}
	if !strings.Contains(stdout, "bold") {
		t.Errorf("dry-run body should carry the style, got %q", stdout)
	}
}

// TestPrintFlagSchema_UnderscoreFlagName pins --flag-name border_styles
// resolving the border-styles schema (eval V2U013 burned a retry on this).
func TestPrintFlagSchema_UnderscoreFlagName(t *testing.T) {
	t.Parallel()
	print := printFlagSchemaFor("+cells-set-style")
	out, err := print("border_styles")
	if err != nil {
		t.Fatalf("underscore flag-name should resolve the hyphen schema, got: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected schema output")
	}
}

// TestPrintFlagSchema_DottedPathSlices pins the schema sub-path slicing
// contract on the real embedded chart schema: a dotted --flag-name returns
// just that subtree, and a path miss lists the keys actually available.
func TestPrintFlagSchema_DottedPathSlices(t *testing.T) {
	t.Parallel()
	print := printFlagSchemaFor("+chart-create")

	t.Run("slices a nested subtree", func(t *testing.T) {
		t.Parallel()
		out, err := print("properties.snapshot.plotArea.axes")
		if err != nil {
			t.Fatalf("dotted path should slice, got: %v", err)
		}
		full, err2 := print("properties")
		if err2 != nil {
			t.Fatalf("full dump: %v", err2)
		}
		if len(out) == 0 || len(out) >= len(full) {
			t.Errorf("slice should be non-empty and smaller than the full schema (%d vs %d bytes)", len(out), len(full))
		}
	})

	t.Run("path miss lists available keys", func(t *testing.T) {
		t.Parallel()
		_, err := print("properties.snapshot.nosuchkey")
		if err == nil {
			t.Fatal("expected error for unknown path segment")
		}
		if !strings.Contains(err.Error(), "available keys:") {
			t.Errorf("error should list available keys, got %v", err)
		}
	})
}

// TestStylesFieldTypesValidated pins the type half of style validation: the
// --styles payloads skip the generic JSON-schema pass (their schema describes
// the outer envelope), so scalar fields are type-checked against flag-defs
// here. Without it, {"font_weight": true} reached the server as a boolean.
func TestStylesFieldTypesValidated(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		field string
		want  string
	}{
		{"boolean font_weight", `"font_weight":true`, "font_weight must be a string, got boolean"},
		{"numeric background_color", `"background_color":123`, "background_color must be a string, got number"},
		{"string font_size", `"font_size":"12"`, "font_size must be a number, got string"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := stylesPutOperations(stylesPutView(map[string]interface{}{
				"styles": []interface{}{map[string]interface{}{
					"name":        "s",
					"cell_styles": []interface{}{mustJSONMap(t, `{"range":"A1",`+tc.field+`}`)},
				}},
			}), testToken)
			requireValidation(t, err, tc.want)
		})
	}

	t.Run("well-typed fields still pass", func(t *testing.T) {
		t.Parallel()
		_, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{
				"name":        "s",
				"cell_styles": []interface{}{mustJSONMap(t, `{"range":"A1","font_weight":"bold","font_size":12,"background_color":"#FFFFFF"}`)},
			}},
		}), testToken)
		if err != nil {
			t.Fatalf("unexpected error for well-typed styles: %v", err)
		}
	})
}

// TestAggregatedStyleErrorsCarryTypedParam pins the error contract for the
// aggregate path: an agent must be able to read which flag to fix from the
// typed envelope, not by parsing the prose message.
func TestAggregatedStyleErrorsCarryTypedParam(t *testing.T) {
	t.Parallel()
	_, err := stylesPutOperations(stylesPutView(map[string]interface{}{
		"styles": []interface{}{map[string]interface{}{
			"name": "s",
			"cell_styles": []interface{}{
				mustJSONMap(t, `{"range":"A1","font_weight":"heavy"}`),
				mustJSONMap(t, `{"range":"B1"}`),
			},
		}},
	}), testToken)
	ve := requireValidation(t, err, "has 2 issues")
	if ve.Param != "--styles" {
		t.Errorf("Param = %q, want --styles", ve.Param)
	}
	if ve.Cause == nil {
		t.Error("aggregate error should keep the first underlying error as Cause")
	}
}

func mustJSONMap(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("bad test JSON %q: %v", raw, err)
	}
	return m
}

// TestFreezeAliasConflictRejected pins determinism: "cols" and "columns" are
// aliases for one field, so accepting both would make the frozen column count
// depend on Go's randomized map iteration order — the same payload could
// freeze 1 column on one run and 2 on the next.
func TestFreezeAliasConflictRejected(t *testing.T) {
	t.Parallel()

	t.Run("conflicting alias values reject every time", func(t *testing.T) {
		t.Parallel()
		for i := 0; i < 200; i++ {
			_, err := parseWorkbookCreateFreezeOp(map[string]interface{}{
				"cols": float64(1), "columns": float64(2),
			}, "--styles.styles[0].freeze", false)
			if err == nil {
				t.Fatalf("iteration %d: conflicting cols/columns must be rejected", i)
			}
		}
	})

	t.Run("identical alias values pass", func(t *testing.T) {
		t.Parallel()
		got, err := parseWorkbookCreateFreezeOp(map[string]interface{}{
			"cols": float64(2), "columns": float64(2),
		}, "p", false)
		if err != nil || got.Cols != 2 {
			t.Fatalf("got=%+v err=%v, want Cols=2", got, err)
		}
	})

	t.Run("either alias alone still works", func(t *testing.T) {
		t.Parallel()
		for _, key := range []string{"cols", "columns"} {
			got, err := parseWorkbookCreateFreezeOp(map[string]interface{}{key: float64(3)}, "p", false)
			if err != nil || got.Cols != 3 {
				t.Fatalf("%s: got=%+v err=%v, want Cols=3", key, got, err)
			}
		}
	})
}

// TestFreezeAllZeroUnfreeze pins the carrier split for {rows:0, cols:0}:
// freeze is full-state replacement server-side, so on an existing sheet the
// all-zero state means "unfreeze both axes" (the same request +dim-freeze
// --rows 0 --cols 0 sends), while on the create path a new sheet starts
// unfrozen and all-zero stays a rejected no-op.
func TestFreezeAllZeroUnfreeze(t *testing.T) {
	t.Parallel()

	t.Run("create path still rejects all-zero", func(t *testing.T) {
		t.Parallel()
		_, err := parseWorkbookCreateFreezeOp(map[string]interface{}{
			"rows": float64(0), "cols": float64(0),
		}, "p", false)
		if err == nil || !strings.Contains(err.Error(), "must freeze at least one dimension") {
			t.Fatalf("create-path all-zero must be rejected, got err=%v", err)
		}
	})

	t.Run("existing-sheet path accepts all-zero", func(t *testing.T) {
		t.Parallel()
		got, err := parseWorkbookCreateFreezeOp(map[string]interface{}{
			"rows": float64(0), "cols": float64(0),
		}, "p", true)
		if err != nil || got == nil || got.Rows != 0 || got.Cols != 0 {
			t.Fatalf("got=%+v err=%v, want an accepted {0,0} op", got, err)
		}
	})

	t.Run("styles-put all-zero freeze emits the bare unfreeze operation", func(t *testing.T) {
		t.Parallel()
		ops, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{
				"name":   "s",
				"freeze": map[string]interface{}{"rows": float64(0), "cols": float64(0)},
			}},
		}), testToken)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ops) != 1 {
			t.Fatalf("ops = %d, want exactly the unfreeze: %v", len(ops), ops)
		}
		op := ops[0].(map[string]interface{})
		if op["tool_name"] != "modify_sheet_structure" {
			t.Fatalf("tool_name = %v, want modify_sheet_structure", op["tool_name"])
		}
		input := op["input"].(map[string]interface{})
		if input["operation"] != "unfreeze" {
			t.Fatalf("operation = %v, want unfreeze", input["operation"])
		}
		// The bare unfreeze carries no counts — same shape as +dim-freeze's.
		for _, k := range []string{"freeze_rows", "freeze_columns"} {
			if _, has := input[k]; has {
				t.Fatalf("unfreeze must not carry %s: %v", k, input)
			}
		}
	})

	t.Run("all-zero describes as unfreeze", func(t *testing.T) {
		t.Parallel()
		if got := (workbookCreateStyleOp{Kind: "freeze"}).describe(); got != "unfreeze" {
			t.Fatalf("describe = %q, want %q", got, "unfreeze")
		}
	})
}

// TestSingleIssueStillAttributesFlag pins that the aggregate entry points
// attribute the outer flag even for ONE issue — an agent must read the flag
// to fix from the typed envelope, not by parsing a nested path out of prose.
func TestSingleIssueStillAttributesFlag(t *testing.T) {
	t.Parallel()
	_, err := stylesPutOperations(stylesPutView(map[string]interface{}{
		"styles": []interface{}{map[string]interface{}{
			"name":        "s",
			"cell_styles": []interface{}{mustJSONMap(t, `{"range":"A1","font_weight":true}`)},
		}},
	}), testToken)
	ve := requireValidation(t, err, "font_weight must be a string")
	if ve.Param != "--styles" {
		t.Errorf("Param = %q, want --styles even for a single issue", ve.Param)
	}
	if ve.Cause == nil {
		t.Error("single-issue aggregate should keep the underlying error as Cause")
	}
}

// TestAggregatedIssuesKeepPrescriptions pins that folding per-item failures
// into one flag error does not drop the Hint the inner error carried. The
// prescriptions this domain adds (requireSheetSelector's "+workbook-info"
// pointer, for one) live in Hint, and a Problem has exactly one Hint slot —
// so a lone issue must inherit it and a folded list must inline each one.
func TestAggregatedIssuesKeepPrescriptions(t *testing.T) {
	t.Parallel()

	t.Run("single --writes issue inherits the inner hint", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, CellsSet, []string{
			"--url", testURL,
			"--writes", `[{"range":"A1","cells":[[{"value":1}]]}]`,
		})
		ve := requireValidation(t, err, "specify at least one of --sheet-id or --sheet-name")
		if !strings.Contains(ve.Hint, "+workbook-info") {
			t.Errorf("the inner prescription must survive the fold, got hint %q", ve.Hint)
		}
	})

	t.Run("folded --writes issues inline each hint", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, CellsSet, []string{
			"--url", testURL,
			"--writes", `[{"range":"A1","cells":[[{"value":1}]]},{"range":"B1","cells":[[{"value":2}]]}]`,
		})
		ve := requireValidation(t, err, "--writes has 2 issues")
		if strings.Count(ve.Message, "+workbook-info") != 2 {
			t.Errorf("each issue should carry its own prescription inline, got %q", ve.Message)
		}
	})
}
