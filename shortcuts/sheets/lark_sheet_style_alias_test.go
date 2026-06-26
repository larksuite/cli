// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestNormalizeCellStyleAliases pins the shorthand → canonical renaming for a
// single cell_styles map: the alignment shorthands models commonly hallucinate
// are rewritten in place, values are preserved, and a shorthand colliding with
// its canonical key is a hard error rather than a silent pick.
func TestNormalizeCellStyleAliases(t *testing.T) {
	t.Parallel()

	t.Run("renames *_align shorthands, keeps values and other fields", func(t *testing.T) {
		t.Parallel()
		style := map[string]interface{}{
			"horizontal_align": "center",
			"vertical_align":   "middle",
			"font_weight":      "bold",
		}
		if err := normalizeCellStyleAliases(style, "x"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if style["horizontal_alignment"] != "center" || style["vertical_alignment"] != "middle" {
			t.Errorf("alignment not renamed: %#v", style)
		}
		if _, ok := style["horizontal_align"]; ok {
			t.Errorf("shorthand horizontal_align should be removed: %#v", style)
		}
		if _, ok := style["vertical_align"]; ok {
			t.Errorf("shorthand vertical_align should be removed: %#v", style)
		}
		if style["font_weight"] != "bold" {
			t.Errorf("unrelated field font_weight dropped: %#v", style)
		}
	})

	t.Run("renames halign/valign shorthands", func(t *testing.T) {
		t.Parallel()
		style := map[string]interface{}{"halign": "left", "valign": "top"}
		if err := normalizeCellStyleAliases(style, "x"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if style["horizontal_alignment"] != "left" || style["vertical_alignment"] != "top" {
			t.Errorf("halign/valign not renamed: %#v", style)
		}
	})

	t.Run("shorthand colliding with canonical is an error", func(t *testing.T) {
		t.Parallel()
		style := map[string]interface{}{
			"horizontal_align":     "center",
			"horizontal_alignment": "left",
		}
		err := normalizeCellStyleAliases(style, "cell_styles[0]")
		requireValidation(t, err, "conflicts with horizontal_alignment")
	})

	t.Run("no shorthand leaves the map untouched", func(t *testing.T) {
		t.Parallel()
		style := map[string]interface{}{"font_weight": "bold", "horizontal_alignment": "center"}
		if err := normalizeCellStyleAliases(style, "x"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(style) != 2 || style["font_weight"] != "bold" || style["horizontal_alignment"] != "center" {
			t.Errorf("map should be unchanged: %#v", style)
		}
	})

	t.Run("empty map is a no-op", func(t *testing.T) {
		t.Parallel()
		if err := normalizeCellStyleAliases(map[string]interface{}{}, "x"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// TestNormalizeTypedCellsStyleAliases pins the 2D --cells walk: every cell's
// inline cell_styles is normalized, malformed shapes are skipped (matching the
// pass-through contract) rather than rejected, and a conflict propagates.
func TestNormalizeTypedCellsStyleAliases(t *testing.T) {
	t.Parallel()

	t.Run("normalizes inline cell_styles across the grid", func(t *testing.T) {
		t.Parallel()
		cells := []interface{}{
			[]interface{}{
				map[string]interface{}{
					"value":       "x",
					"cell_styles": map[string]interface{}{"horizontal_align": "center"},
				},
				map[string]interface{}{"value": "y"}, // no cell_styles → untouched
			},
		}
		if err := normalizeTypedCellsStyleAliases(cells, "--cells"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		row := cells[0].([]interface{})
		st := row[0].(map[string]interface{})["cell_styles"].(map[string]interface{})
		if st["horizontal_alignment"] != "center" {
			t.Errorf("cell_styles not normalized: %#v", st)
		}
		if _, ok := st["horizontal_align"]; ok {
			t.Errorf("shorthand should be removed: %#v", st)
		}
	})

	t.Run("malformed shapes are skipped, not rejected", func(t *testing.T) {
		t.Parallel()
		cells := []interface{}{
			"not-a-row",
			[]interface{}{
				"not-a-cell",
				map[string]interface{}{"cell_styles": "not-a-map"},
			},
		}
		if err := normalizeTypedCellsStyleAliases(cells, "--cells"); err != nil {
			t.Fatalf("lenient walk should not error on odd shapes: %v", err)
		}
	})

	t.Run("conflict inside a cell propagates", func(t *testing.T) {
		t.Parallel()
		cells := []interface{}{
			[]interface{}{
				map[string]interface{}{
					"cell_styles": map[string]interface{}{
						"valign":             "top",
						"vertical_alignment": "middle",
					},
				},
			},
		}
		err := normalizeTypedCellsStyleAliases(cells, "--cells")
		requireValidation(t, err, "--cells[0][0].cell_styles")
	})
}

// TestCellsSet_StyleAliasesNormalized is the end-to-end guard for +cells-set:
// a typed --cells payload using alignment shorthands reaches set_cell_range
// with canonical field names so the backend doesn't silently drop them.
func TestCellsSet_StyleAliasesNormalized(t *testing.T) {
	t.Parallel()
	cells := `[[{"value":"Header","cell_styles":{"horizontal_align":"center","vertical_align":"middle","font_weight":"bold"}}]]`
	body := parseDryRunBody(t, CellsSet, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A1", "--cells", cells,
	})
	input := decodeToolInput(t, body, "set_cell_range")
	raw, _ := json.Marshal(input["cells"])
	s := string(raw)
	if !strings.Contains(s, `"horizontal_alignment":"center"`) || !strings.Contains(s, `"vertical_alignment":"middle"`) {
		t.Errorf("alignment shorthands not normalized in cells: %s", s)
	}
	if strings.Contains(s, `"horizontal_align":`) || strings.Contains(s, `"vertical_align":`) {
		t.Errorf("shorthand keys leaked through to backend payload: %s", s)
	}
}

// TestWorkbookCreate_StyleAliasesNormalized is the end-to-end guard for
// +workbook-create --styles: alignment shorthands in a cell_styles op are
// accepted (no "unsupported style field" error) and emitted as canonical
// field names merged into the fill cells.
func TestWorkbookCreate_StyleAliasesNormalized(t *testing.T) {
	t.Parallel()
	calls := parseDryRunAPI(t, WorkbookCreate, []string{
		"--title", "Sales",
		"--values", `[["Name","Score"],["alice",95]]`,
		"--styles", `{"styles":[{"name":"Sheet1","cell_styles":[{"range":"A1:B2","horizontal_align":"center","vertical_align":"middle"}]}]}`,
	})
	if len(calls) != 2 {
		t.Fatalf("api calls = %d, want 2 (create + fill)", len(calls))
	}
	body, _ := calls[1].(map[string]interface{})["body"].(map[string]interface{})
	input := decodeToolInput(t, body, "set_cell_range")
	raw, _ := json.Marshal(input["cells"])
	s := string(raw)
	if c := strings.Count(s, `"horizontal_alignment":"center"`); c != 4 {
		t.Errorf("horizontal_alignment occurrences = %d, want 4 in 2x2 range; cells=%s", c, s)
	}
	if strings.Contains(s, `"horizontal_align":`) || strings.Contains(s, `"vertical_align":`) {
		t.Errorf("shorthand keys leaked through after normalization: %s", s)
	}
}
