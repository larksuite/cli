// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"fmt"
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

// TestUnwrapCellsEnvelope pins the {"cells": …} envelope contract: a lone
// "cells" key is the flag name mistaken for a JSON key and unwraps; an
// object carrying siblings is the whole tool input and must not silently
// lose them.
func TestUnwrapCellsEnvelope(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		in        string
		unwrapped bool
	}{
		{"envelope around a 2D array", `{"cells":[[{"value":"a"}]]}`, true},
		{"envelope around a lone cell object", `{"cells":{"value":"a"}}`, true},
		{"envelope around an empty array", `{"cells":[]}`, true},
		{"sibling keys stay (dropping them would write elsewhere)", `{"cells":[[{"value":"a"}]],"range":"A1"}`, false},
		{"scalar under the key stays", `{"cells":"A1"}`, false},
		{"unrelated object stays", `{"value":"a"}`, false},
		{"proper 2D array stays", `[[{"value":"a"}]]`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var v interface{}
			if err := json.Unmarshal([]byte(tc.in), &v); err != nil {
				t.Fatalf("bad fixture: %v", err)
			}
			out := unwrapCellsEnvelope(v)
			changed := fmt.Sprintf("%#v", out) != fmt.Sprintf("%#v", v)
			if changed != tc.unwrapped {
				t.Errorf("unwrapped=%v, want %v (got %#v)", changed, tc.unwrapped, out)
			}
		})
	}
}

// TestScalarCellValue pins which bare cell-slot values lift into
// {"value": …}. null stays nil on purpose: {} and {"value":""} are both
// plausible readings, so it belongs to the validator, not the normalizer.
func TestScalarCellValue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		in     interface{}
		lifted bool
	}{
		{"string", "hello", true},
		{"number", float64(42), true},
		{"bool", true, true},
		{"json.Number", json.Number("42"), true},
		{"null stays for the validator", nil, false},
		{"cell object stays", map[string]interface{}{"value": "a"}, false},
		{"array stays", []interface{}{"a"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := scalarCellValue(tc.in)
			if (got != nil) != tc.lifted {
				t.Fatalf("lifted=%v, want %v", got != nil, tc.lifted)
			}
			if tc.lifted && got["value"] != tc.in {
				t.Errorf("want value %#v, got %#v", tc.in, got["value"])
			}
		})
	}
}

// TestCellsSet_EnvelopeAndScalarCellsAccepted runs both new rewrites through
// the mounted path with the exact eval-trace shapes: a payload script's
// json.dump({"cells": cells}) envelope, and the openpyxl / gspread habit of
// a plain values matrix — which real rows MIX with cell objects as soon as a
// formula appears.
func TestCellsSet_EnvelopeAndScalarCellsAccepted(t *testing.T) {
	t.Parallel()
	sc := shortcutFromRegistry(t, "+cells-set")

	t.Run("envelope with a mixed scalar / object row", func(t *testing.T) {
		t.Parallel()
		stdout, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--sheet-name", "s",
			"--range", "A1:C1",
			"--cells", `{"cells":[["电动大门",10331.00,{"formula":"=A1*2"}]]}`,
			"--dry-run",
		})
		if err != nil {
			t.Fatalf("envelope + scalar cells should normalize, got: %v", err)
		}
		for _, want := range []string{"电动大门", "10331", "=A1*2"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("dry-run body should carry %s, got %q", want, stdout)
			}
		}
	})

	t.Run("null cell keeps the validation error", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--sheet-name", "s",
			"--range", "A1",
			"--cells", `[[null]]`,
			"--dry-run",
		})
		requireValidation(t, err, `got "null"`)
	})

	t.Run("envelope with sibling keys keeps the validation error", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--sheet-name", "s",
			"--range", "A1",
			"--cells", `{"cells":[[{"value":"a"}]],"range":"A1"}`,
			"--dry-run",
		})
		requireValidation(t, err, `expected type "array"`)
	})
}

// TestCellsSetStyle_BorderWeightWordInStyleNormalizes pins the reachability
// fix for the border acceptance layer on the --border-styles flag path: the
// eval-trace failure shape ({"style":"thin"} — 07-28 root-cause report #2,
// 173 occurrences) must normalize to style:solid + weight:thin BEFORE the
// schema enum check, instead of dying on `value "thin" is not in enum`.
func TestCellsSetStyle_BorderWeightWordInStyleNormalizes(t *testing.T) {
	t.Parallel()
	sc := shortcutFromRegistry(t, "+cells-set-style")

	t.Run("full nested form with weight word in style", func(t *testing.T) {
		t.Parallel()
		stdout, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--sheet-name", "s",
			"--range", "A1:B2",
			"--border-styles", `{"top":{"style":"thin","color":"#B4B4B4"},"bottom":{"style":"thin","color":"#B4B4B4"}}`,
			"--dry-run",
		})
		if err != nil {
			t.Fatalf("weight word in style slot should normalize, got: %v", err)
		}
		for _, want := range []string{`"style": "solid"`, `"weight": "thin"`} {
			if !strings.Contains(stdout, want) {
				t.Errorf("dry-run body should carry %s, got %q", want, stdout)
			}
		}
	})

	t.Run("all shorthand with weight word in style", func(t *testing.T) {
		t.Parallel()
		stdout, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--sheet-name", "s",
			"--range", "A1",
			"--border-styles", `{"all":{"style":"medium","color":"#000000"}}`,
			"--dry-run",
		})
		if err != nil {
			t.Fatalf("all shorthand + weight word should normalize, got: %v", err)
		}
		for _, want := range []string{`"top"`, `"bottom"`, `"weight": "medium"`, `"style": "solid"`} {
			if !strings.Contains(stdout, want) {
				t.Errorf("dry-run body should carry %s, got %q", want, stdout)
			}
		}
	})

	t.Run("explicit conflicting weight keeps the enum error", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--sheet-name", "s",
			"--range", "A1",
			"--border-styles", `{"top":{"style":"thin","weight":"thick"}}`,
			"--dry-run",
		})
		requireValidation(t, err, "not in enum")
	})
}

// TestCellsSetStyle_BorderValueVocabularyNormalizes pins the two border
// value habits the 08-11 trace tally (596 traces) actually shows, beyond
// the weight-word-in-style half fixed on 07-28: openpyxl's "hair" (476
// hits / 19 tasks in the weight slot) and a thickness that arrives as a
// number (the 07-28 report's second failing shape). Everything else in
// openpyxl's line-style list scored zero and must STAY rejected — the
// last row pins that, so the table cannot quietly grow past its evidence.
func TestCellsSetStyle_BorderValueVocabularyNormalizes(t *testing.T) {
	t.Parallel()
	sc := shortcutFromRegistry(t, "+cells-set-style")
	cases := []struct {
		name    string
		border  string
		want    []string
		wantErr string
	}{
		{name: "numeric width as string", border: `{"top":{"style":"solid","weight":"1"}}`,
			want: []string{`"style": "solid"`, `"weight": "thin"`}},
		{name: "numeric width as number", border: `{"top":{"style":"solid","weight":2}}`,
			want: []string{`"style": "solid"`, `"weight": "medium"`}},
		{name: "hair in the weight slot", border: `{"top":{"style":"solid","weight":"hair"}}`,
			want: []string{`"weight": "thin"`}},
		{name: "hair in the style slot", border: `{"all":{"style":"hair"}}`,
			want: []string{`"style": "solid"`, `"weight": "thin"`}},
		{name: "Google Sheets width key", border: `{"top":{"style":"solid","width":1}}`,
			want: []string{`"weight": "thin"`}},
		// The two ends of the width mapping: 3 is where thick starts, and a
		// width the mapping deliberately does not guess at ("no width" is a
		// border the caller should spell style:"none") keeps its own error.
		{name: "numeric width at the thick boundary", border: `{"top":{"style":"solid","weight":3}}`,
			want: []string{`"weight": "thick"`}},
		{name: "zero width is not guessed at", border: `{"top":{"style":"solid","weight":0}}`,
			wantErr: `expected type "string", got "number"`},
		// strconv.ParseFloat answers yes to these; an infinite line width is
		// not a width the caller meant, so it must not fold onto thick.
		{name: "infinite width stays rejected", border: `{"top":{"style":"solid","weight":"Inf"}}`,
			wantErr: "not in enum"},
		{name: "unobserved line style stays rejected", border: `{"top":{"style":"dashDot"}}`,
			wantErr: "not in enum"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stdout, _, err := runShortcutCapturingErr(t, sc, []string{
				"--url", testURL,
				"--sheet-name", "s",
				"--range", "A1",
				"--border-styles", tc.border,
				"--dry-run",
			})
			if tc.wantErr != "" {
				requireValidation(t, err, tc.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("%s should normalize, got: %v", tc.border, err)
			}
			for _, want := range tc.want {
				if !strings.Contains(stdout, want) {
					t.Errorf("dry-run body should carry %s, got %q", want, stdout)
				}
			}
		})
	}
}

// TestCellsSet_BorderWeightWordInStyleNormalizes pins the same reachability
// fix on the typed --cells carrier (07-28 root-cause report #10, 58
// occurrences): border_styles inside a cell object normalizes before the
// enum check.
func TestCellsSet_BorderWeightWordInStyleNormalizes(t *testing.T) {
	t.Parallel()
	sc := shortcutFromRegistry(t, "+cells-set")
	stdout, _, err := runShortcutCapturingErr(t, sc, []string{
		"--url", testURL,
		"--sheet-name", "s",
		"--range", "A1",
		"--cells", `[[{"value":"x","border_styles":{"top":{"style":"thin","color":"#000000"}}}]]`,
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("weight word in style slot should normalize on --cells, got: %v", err)
	}
	for _, want := range []string{`"style": "solid"`, `"weight": "thin"`} {
		if !strings.Contains(stdout, want) {
			t.Errorf("dry-run body should carry %s, got %q", want, stdout)
		}
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

	t.Run("bare array names the missing envelope", func(t *testing.T) {
		t.Parallel()
		sc := shortcutFromRegistry(t, "+table-put")
		_, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL,
			"--sheets", `[{"name":"s","columns":["a"],"data":[["x"]]}]`,
			"--dry-run",
		})
		// The Go unmarshal text names the internal struct, not the fix
		// (07-28 root-cause report #4, 84 occurrences).
		ve := requireValidation(t, err, `top level must be the object {"sheets":[…]}`)
		if strings.Contains(ve.Message, "cannot unmarshal") {
			t.Errorf("message should not leak the Go unmarshal wording, got %q", ve.Message)
		}
		if !strings.Contains(ve.Hint, "expected shape:") {
			t.Errorf("hint should still inline the skeleton, got %q", ve.Hint)
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
