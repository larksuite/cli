// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestWrapLoneCellObject pins the auto-wrap contract: a bare cell object —
// the classic missing-[[…]] shape agents produce for a 1×1 write — is
// rewritten to [[cell]]; anything whose meaning is not beyond doubt stays
// untouched for the schema validator to prescribe.
func TestWrapLoneCellObject(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		in      string
		wrapped bool
	}{
		{"lone value cell", `{"value":"hi"}`, true},
		{"lone formula cell with styles", `{"formula":"=SUM(A1:A3)","cell_styles":{"font_weight":"bold"}}`, true},
		{"unknown key stays", `{"value":"hi","range":"A1"}`, false},
		{"array of cells stays (row vs column ambiguous)", `[{"value":"a"},{"value":"b"}]`, false},
		{"proper 2D array stays", `[[{"value":"a"}]]`, false},
		{"empty object stays", `{}`, false},
		{"scalar stays", `"hi"`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var v interface{}
			if err := json.Unmarshal([]byte(tc.in), &v); err != nil {
				t.Fatalf("bad fixture: %v", err)
			}
			out := wrapLoneCellObject(v)
			_, isWrapped := out.([]interface{})
			_, wasArray := v.([]interface{})
			if tc.wrapped && (!isWrapped || wasArray) {
				t.Errorf("expected wrap to [[cell]], got %#v", out)
			}
			if !tc.wrapped && !wasArray && isWrapped {
				t.Errorf("expected no wrap, got %#v", out)
			}
			if tc.wrapped {
				rows, _ := out.([]interface{})
				if len(rows) != 1 {
					t.Fatalf("want 1 row, got %d", len(rows))
				}
				cells, _ := rows[0].([]interface{})
				if len(cells) != 1 {
					t.Fatalf("want 1 cell, got %d", len(cells))
				}
			}
		})
	}
}

// TestCellObjectKeys_MatchEmbeddedSchema drift-guards the hardcoded cell
// vocabulary against the embedded +cells-set --cells schema: if the spec
// repo adds or removes a cell property, this fails and cellObjectKeys must
// be updated (an outdated set only narrows the auto-wrap, but silently
// narrowing is still drift).
func TestCellObjectKeys_MatchEmbeddedSchema(t *testing.T) {
	t.Parallel()
	idx, err := loadFlagSchemas()
	if err != nil {
		t.Fatalf("loadFlagSchemas: %v", err)
	}
	raw, ok := idx.Flags["+cells-set"]["cells"]
	if !ok {
		t.Fatal("embedded schema for +cells-set --cells missing")
	}
	var schema schemaProperty
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	cell := schema.Items
	if cell != nil && cell.Items != nil {
		cell = cell.Items
	}
	if cell == nil || len(cell.Properties) == 0 {
		t.Fatal("schema shape changed: expected array→array→object with properties")
	}
	for k := range cell.Properties {
		if _, ok := cellObjectKeys[k]; !ok {
			t.Errorf("schema property %q missing from cellObjectKeys", k)
		}
	}
	for k := range cellObjectKeys {
		if _, ok := cell.Properties[k]; !ok {
			t.Errorf("cellObjectKeys has %q which the schema no longer declares", k)
		}
	}
}

// TestCellsSet_LoneCellObjectAutoWraps runs the mounted path end-to-end: the
// eval-trace failure shape (--cells with a bare object) now dry-runs clean
// instead of failing "expected type array, got object".
func TestCellsSet_LoneCellObjectAutoWraps(t *testing.T) {
	t.Parallel()
	sc := shortcutFromRegistry(t, "+cells-set")
	stdout, _, err := runShortcutCapturingErr(t, sc, []string{
		"--url", testURL,
		"--sheet-name", "s",
		"--range", "A1",
		"--cells", `{"value":"hello"}`,
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("lone cell object should auto-wrap to [[cell]], got: %v", err)
	}
	if !strings.Contains(stdout, "hello") {
		t.Errorf("dry-run body should carry the cell value, got %q", stdout)
	}
}

// TestTablePut_SheetsDecodeHints pins the two decode-failure prescriptions:
// wrong JSON kind inlines the expected shape; mangled JSON steers to
// stdin/@file.
func TestTablePut_SheetsDecodeHints(t *testing.T) {
	t.Parallel()

	t.Run("type mismatch inlines skeleton", func(t *testing.T) {
		t.Parallel()
		sc := shortcutFromRegistry(t, "+table-put")
		_, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--sheets", `{"sheets":[{"name":"s","columns":[{"name":"a"}],"data":[]}]}`,
			"--dry-run",
		})
		ve := requireValidation(t, err, "--sheets: invalid JSON")
		for _, want := range []string{"expected shape:", `"columns":["City","Revenue"]`, `"dtypes":{"Revenue":"float64"}`} {
			if !strings.Contains(ve.Hint, want) {
				t.Errorf("hint should contain %q, got %q", want, ve.Hint)
			}
		}
	})

	t.Run("syntax error steers to stdin or @file", func(t *testing.T) {
		t.Parallel()
		sc := shortcutFromRegistry(t, "+table-put")
		_, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--sheets", `{"sheets":[)`,
			"--dry-run",
		})
		ve := requireValidation(t, err, "--sheets: invalid JSON")
		for _, want := range []string{"stdin", "@./payload.json"} {
			if !strings.Contains(ve.Hint, want) {
				t.Errorf("hint should contain %q, got %q", want, ve.Hint)
			}
		}
	})
}

// TestNormalizeChartHexColors pins the '#' prefixing on bare hex color
// values (eval V2U024: bars.color "4472C4" rejected server-side) and the
// pass-through of everything else, including the parseJSONFlag wiring for
// the batch sub-op path.
func TestNormalizeChartHexColors(t *testing.T) {
	t.Parallel()
	props := map[string]interface{}{
		"plotArea": map[string]interface{}{
			"plot": map[string]interface{}{
				"series": []interface{}{
					map[string]interface{}{"bars": map[string]interface{}{"color": "4472C4"}},
					map[string]interface{}{"line": map[string]interface{}{"color": "#ED7D31"}},
					map[string]interface{}{"area": map[string]interface{}{"color": "rgba(1,2,3,0.5)"}},
					map[string]interface{}{"font_color": "ED7D31AA", "label": "not a color 4472C4"},
				},
			},
		},
	}
	normalizeChartHexColors(props)
	series := props["plotArea"].(map[string]interface{})["plot"].(map[string]interface{})["series"].([]interface{})
	if got := series[0].(map[string]interface{})["bars"].(map[string]interface{})["color"]; got != "#4472C4" {
		t.Errorf("bare hex should gain #, got %v", got)
	}
	if got := series[1].(map[string]interface{})["line"].(map[string]interface{})["color"]; got != "#ED7D31" {
		t.Errorf("already-prefixed color must not change, got %v", got)
	}
	if got := series[2].(map[string]interface{})["area"].(map[string]interface{})["color"]; got != "rgba(1,2,3,0.5)" {
		t.Errorf("rgba color must not change, got %v", got)
	}
	last := series[3].(map[string]interface{})
	if got := last["font_color"]; got != "#ED7D31AA" {
		t.Errorf("8-digit hex on a *_color key should gain #, got %v", got)
	}
	if got := last["label"]; got != "not a color 4472C4" {
		t.Errorf("non-color key must not change, got %v", got)
	}

	// Wiring: a +chart-create sub-op style view routes through parseJSONFlag
	// and picks up the normalizer.
	fv := newMapFlagViewForCommand("+chart-create", map[string]interface{}{
		"properties": map[string]interface{}{"title": map[string]interface{}{"font_color": "112233"}},
	})
	out, err := parseJSONFlag(fv, "properties")
	if err != nil {
		t.Fatalf("parseJSONFlag: %v", err)
	}
	title := out.(map[string]interface{})["title"].(map[string]interface{})
	if title["font_color"] != "#112233" {
		t.Errorf("parseJSONFlag should apply the chart color normalizer, got %v", title["font_color"])
	}
}
