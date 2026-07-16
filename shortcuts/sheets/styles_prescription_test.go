// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
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
