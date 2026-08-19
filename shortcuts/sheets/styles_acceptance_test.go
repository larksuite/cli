// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"fmt"
	"strings"
	"testing"
)

// ─── styles acceptance contract ───────────────────────────────────────
//
// Two closure properties that turn the --styles acceptance surface from
// "endless patching" into a locked contract (07-20 rerun lesson: the
// redesign moved traffic onto the payload path while the flag path's
// forgiveness layers stayed behind):
//
//  1. Vocabulary parity — every style the flag path (+cells-set-style)
//     can express must be accepted verbatim by the payload path.
//  2. Prior corpus — every model spelling observed in eval traces must
//     either normalize to the canonical form or produce a targeted
//     prescription. Silent ignoring and bare rejection are both bugs.
//     New eval finding → add a corpus row → fix → locked forever.

// acceptStyleItem runs one cell_styles item through the styles-put pipeline
// and returns the emitted cell prototype (cell_styles/border_styles) or the
// error.
func acceptStyleItem(t *testing.T, fields map[string]interface{}) (map[string]interface{}, error) {
	t.Helper()
	item := map[string]interface{}{"range": "A1:B2"}
	for k, v := range fields {
		item[k] = v
	}
	ops, err := stylesPutOperations(stylesPutView(map[string]interface{}{
		"styles": []interface{}{map[string]interface{}{
			"name":        "S1",
			"cell_styles": []interface{}{item},
		}},
	}), testToken)
	if err != nil {
		return nil, err
	}
	input := ops[0].(map[string]interface{})["input"].(map[string]interface{})
	cells := input["cells"].([][]interface{})
	return cells[0][0].(map[string]interface{}), nil
}

// TestStylesAcceptance_VocabularyParity locks property 1: iterate the
// +cells-set-style flag vocabulary from flag-defs and assert the payload
// path accepts each field with a valid value and emits it.
func TestStylesAcceptance_VocabularyParity(t *testing.T) {
	t.Parallel()
	defs, err := loadFlagDefs()
	if err != nil {
		t.Fatalf("loadFlagDefs: %v", err)
	}
	spec, ok := defs["+cells-set-style"]
	if !ok {
		t.Fatal("no +cells-set-style flag defs")
	}
	sample := func(df flagDef) interface{} {
		if len(df.Enum) > 0 {
			return df.Enum[0]
		}
		switch df.Type {
		case "float64", "int":
			return float64(12)
		}
		switch df.Name {
		case "font-family":
			return "Arial"
		case "number-format":
			return "0.00"
		default: // colors and any future string field
			return "#112233"
		}
	}
	for _, df := range spec.Flags {
		if df.Kind != "own" || df.Name == "range" {
			continue
		}
		t.Run(df.Name, func(t *testing.T) {
			t.Parallel()
			field := strings.ReplaceAll(df.Name, "-", "_")
			var value interface{}
			if df.Name == "border-styles" {
				value = map[string]interface{}{"all": map[string]interface{}{"style": "solid"}}
			} else {
				value = sample(df)
			}
			proto, err := acceptStyleItem(t, map[string]interface{}{field: value})
			if err != nil {
				t.Fatalf("payload path rejects flag-path field %s: %v", field, err)
			}
			if df.Name == "border-styles" {
				if _, ok := proto["border_styles"].(map[string]interface{}); !ok {
					t.Fatalf("border_styles not emitted: %v", proto)
				}
				return
			}
			cs, _ := proto["cell_styles"].(map[string]interface{})
			if cs == nil || cs[field] == nil {
				t.Fatalf("field %s silently dropped: %v", field, proto)
			}
		})
	}
}

// stylesPriorCorpus is the observed-model-spelling corpus (source: eval
// batches 2026-07-08 → 07-20). Every row must either normalize (checked via
// wantCell) or produce a targeted prescription (wantErr). Add a row for every
// new spelling an eval surfaces — never let one be silently ignored.
var stylesPriorCorpus = []struct {
	name    string
	fields  map[string]interface{}
	wantErr string                                    // "" = must be accepted
	check   func(proto map[string]interface{}) string // "" = ok, else failure detail
}{
	// border family (07-20: largest cluster)
	{name: "borders attr-keyed means all sides",
		fields: map[string]interface{}{"borders": map[string]interface{}{"style": "solid", "color": "#DDDDDD"}},
		check:  wantBorder("top", "style", "solid")},
	{name: "border side-keyed",
		fields: map[string]interface{}{"border": map[string]interface{}{"top": map[string]interface{}{"style": "solid"}}},
		check:  wantBorder("top", "style", "solid")},
	{name: "border_bottom object",
		fields: map[string]interface{}{"border_bottom": map[string]interface{}{"style": "solid"}},
		check:  wantBorder("bottom", "style", "solid")},
	{name: "border_style weight-vocabulary means thin solid",
		fields: map[string]interface{}{"border_style": "thin"},
		check:  wantBorder("top", "weight", "thin")},
	{name: "border_style style-vocabulary",
		fields: map[string]interface{}{"border_style": "dashed"},
		check:  wantBorder("top", "style", "dashed")},
	{name: "border_color scalar",
		fields: map[string]interface{}{"border_color": "#FF0000"},
		check:  wantBorder("top", "color", "#FF0000")},
	{name: "border_top_color flattened",
		fields: map[string]interface{}{"border_top_color": "#FF0000"},
		check:  wantBorder("top", "color", "#FF0000")},
	{name: "border_left_weight flattened",
		fields: map[string]interface{}{"border_left_weight": "thin"},
		check:  wantBorder("left", "weight", "thin")},
	{name: "border_styles invalid side prescribed",
		fields:  map[string]interface{}{"border_styles": map[string]interface{}{"outer": map[string]interface{}{"style": "solid"}}},
		wantErr: "not a valid side"},
	// wrap family
	{name: "wrap_text boolean", fields: map[string]interface{}{"wrap_text": true}, check: wantStyle("word_wrap", "auto-wrap")},
	{name: "text_wrap string", fields: map[string]interface{}{"text_wrap": "auto-wrap"}, check: wantStyle("word_wrap", "auto-wrap")},
	{name: "word_wrap false", fields: map[string]interface{}{"word_wrap": false}, check: wantStyle("word_wrap", "overflow")},
	// alignment family
	{name: "horizontal_align shorthand", fields: map[string]interface{}{"horizontal_align": "center"}, check: wantStyle("horizontal_alignment", "center")},
	{name: "valign shorthand", fields: map[string]interface{}{"valign": "top"}, check: wantStyle("vertical_alignment", "top")},
	{name: "CSS center for vertical", fields: map[string]interface{}{"vertical_alignment": "center"}, check: wantStyle("vertical_alignment", "middle")},
	{name: "casing normalized", fields: map[string]interface{}{"font_weight": "BOLD"}, check: wantStyle("font_weight", "bold")},
	// weight vocabulary in the FULL nested form's style slot (07-21 rerun:
	// the dominant residual — 8 tasks wrote border_styles.<side>.style:"thin")
	{name: "full-form thin in style slot",
		fields: map[string]interface{}{"border_styles": map[string]interface{}{"top": map[string]interface{}{"style": "thin"}}},
		check:  wantBorder("top", "weight", "thin")},
	{name: "full-form all-shorthand medium in style slot",
		fields: map[string]interface{}{"border_styles": map[string]interface{}{"all": map[string]interface{}{"style": "medium"}}},
		check:  wantBorder("bottom", "weight", "medium")},
	// border VALUE vocabulary (08-11 trace tally over 596 traces): the words
	// that actually recur are hair (476 hits / 19 tasks in the weight slot,
	// 76 / 2 in the style slot) and a numeric width; openpyxl's line-style
	// list and every other library's spelling scored zero and stay rejected.
	{name: "openpyxl hair in the weight slot means thin",
		fields: map[string]interface{}{"border_styles": map[string]interface{}{"top": map[string]interface{}{"style": "solid", "weight": "hair"}}},
		check:  wantBorder("top", "weight", "thin")},
	{name: "openpyxl hair in the style slot means a thin solid line",
		fields: map[string]interface{}{"border_styles": map[string]interface{}{"top": map[string]interface{}{"style": "hair"}}},
		check: wantAll(wantBorder("top", "weight", "thin"),
			wantBorder("top", "style", "solid"))},
	{name: "flattened border_style hair",
		fields: map[string]interface{}{"border_style": "hair"},
		check:  wantBorder("top", "weight", "thin")},
	{name: "numeric weight reads as a line width",
		fields: map[string]interface{}{"border_styles": map[string]interface{}{"top": map[string]interface{}{"style": "solid", "weight": float64(1)}}},
		check:  wantBorder("top", "weight", "thin")},
	{name: "numeric weight as string",
		fields: map[string]interface{}{"border_styles": map[string]interface{}{"top": map[string]interface{}{"style": "solid", "weight": "2"}}},
		check:  wantBorder("top", "weight", "medium")},
	{name: "Google Sheets width key aliases to weight",
		fields: map[string]interface{}{"border_styles": map[string]interface{}{"top": map[string]interface{}{"style": "solid", "width": float64(3)}}},
		check:  wantBorder("top", "weight", "thick")},
	{name: "unobserved openpyxl line style stays rejected",
		fields:  map[string]interface{}{"border_styles": map[string]interface{}{"top": map[string]interface{}{"style": "mediumDashed"}}},
		wantErr: "is invalid"},
	// side-first word order + Google Sheets wrap word (07-21 evening batch)
	{name: "side-first bottom_border object",
		fields: map[string]interface{}{"bottom_border": map[string]interface{}{"style": "solid"}},
		check:  wantBorder("bottom", "style", "solid")},
	{name: "side-first bottom_border_style scalar",
		fields: map[string]interface{}{"bottom_border_style": "solid"},
		check:  wantBorder("bottom", "style", "solid")},
	{name: "wrap_strategy aliases to word_wrap",
		fields: map[string]interface{}{"wrap_strategy": "auto-wrap"},
		check:  wantStyle("word_wrap", "auto-wrap")},
	// prescriptions (ambiguous / unsupported / typo)
	{name: "fore_color prescribed", fields: map[string]interface{}{"fore_color": "#F00"}, wantErr: "ambiguous"},
	{name: "indent rejected not ignored", fields: map[string]interface{}{"indent": float64(2)}, wantErr: "not a supported style field"},
	{name: "unknown field carries did-you-mean and the field list",
		fields: map[string]interface{}{"fontcolor": "#000000"}, wantErr: `did you mean "font_color"`},
	{name: "enum typo gets did-you-mean", fields: map[string]interface{}{"vertical_alignment": "botom"}, wantErr: "did you mean"},
}

func wantStyle(field, want string) func(map[string]interface{}) string {
	return func(proto map[string]interface{}) string {
		cs, _ := proto["cell_styles"].(map[string]interface{})
		if cs == nil || cs[field] != want {
			return fmt.Sprintf("cell_styles.%s = %v, want %q", field, cs[field], want)
		}
		return ""
	}
}

func wantBorder(side, attr, want string) func(map[string]interface{}) string {
	return func(proto map[string]interface{}) string {
		bs, _ := proto["border_styles"].(map[string]interface{})
		sideObj, _ := bs[side].(map[string]interface{})
		if sideObj == nil || sideObj[attr] != want {
			return fmt.Sprintf("border_styles.%s.%s = %v, want %q", side, attr, sideObj[attr], want)
		}
		return ""
	}
}

// wantAll reports the first failing check, so one corpus row can pin every
// field a rewrite touches (a thickness word in the style slot moves the word
// to weight AND defaults the line type — asserting one of the two leaves the
// other free to regress).
func wantAll(checks ...func(map[string]interface{}) string) func(map[string]interface{}) string {
	return func(proto map[string]interface{}) string {
		for _, check := range checks {
			if detail := check(proto); detail != "" {
				return detail
			}
		}
		return ""
	}
}

func TestStylesAcceptance_PriorCorpus(t *testing.T) {
	t.Parallel()
	for _, tc := range stylesPriorCorpus {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			proto, err := acceptStyleItem(t, tc.fields)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want prescription containing %q, got err=%v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("corpus spelling rejected: %v", err)
			}
			if detail := tc.check(proto); detail != "" {
				t.Fatal(detail)
			}
		})
	}
}

// TestStylesPut_CoalescesSameStyleRanges pins the declarative-spec
// optimization: per-row entries with the identical style fuse into one
// rectangle, so row-by-row specs (07-21 rerun: 184/203/861-op expansions
// against the 100-op cap) no longer hit the cap.
func TestStylesPut_CoalescesSameStyleRanges(t *testing.T) {
	t.Parallel()

	t.Run("150 same-style rows fuse into one stamp", func(t *testing.T) {
		t.Parallel()
		entries := make([]interface{}, 0, 150)
		for r := 1; r <= 150; r++ {
			entries = append(entries, map[string]interface{}{
				"range": fmt.Sprintf("A%d:F%d", r, r), "font_weight": "bold",
			})
		}
		ops, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{"name": "S1", "cell_styles": entries}},
		}), testToken)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ops) != 1 {
			t.Fatalf("got %d ops, want 1 fused stamp", len(ops))
		}
		input := ops[0].(map[string]interface{})["input"].(map[string]interface{})
		if input["range"] != "A1:F150" {
			t.Fatalf("range = %v, want A1:F150", input["range"])
		}
	})

	t.Run("different styles stay separate", func(t *testing.T) {
		t.Parallel()
		ops, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{"name": "S1", "cell_styles": []interface{}{
				map[string]interface{}{"range": "A1:F1", "font_weight": "bold"},
				map[string]interface{}{"range": "A2:F2", "background_color": "#EEEEEE"},
			}}},
		}), testToken)
		if err != nil || len(ops) != 2 {
			t.Fatalf("ops=%d err=%v, want 2", len(ops), err)
		}
	})

	t.Run("horizontal fuse with same rows", func(t *testing.T) {
		t.Parallel()
		ops, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{"name": "S1", "cell_styles": []interface{}{
				map[string]interface{}{"range": "A1:C5", "font_weight": "bold"},
				map[string]interface{}{"range": "D1:F5", "font_weight": "bold"},
			}}},
		}), testToken)
		if err != nil || len(ops) != 1 {
			t.Fatalf("ops=%d err=%v, want 1", len(ops), err)
		}
		input := ops[0].(map[string]interface{})["input"].(map[string]interface{})
		if input["range"] != "A1:F5" {
			t.Fatalf("range = %v, want A1:F5", input["range"])
		}
	})

	// The cases above all pin that adjacent ranges DO fuse. The dangerous
	// direction is the other one: coalescing rewrites a declarative spec into
	// bigger rectangles, so a too-generous adjacency rule would paint cells the
	// caller never named — silently, and only visible in the finished sheet.
	// Widening the `+1` touch test in union() to `+2` passes every test above.
	t.Run("a one-row gap is not fused across", func(t *testing.T) {
		t.Parallel()
		ops, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{"name": "S1", "cell_styles": []interface{}{
				map[string]interface{}{"range": "A1:C1", "font_weight": "bold"},
				map[string]interface{}{"range": "A3:C3", "font_weight": "bold"},
			}}},
		}), testToken)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ops) != 2 {
			t.Fatalf("ops=%d, want 2 — row 2 was never named and must not be styled", len(ops))
		}
	})

	t.Run("a one-column gap is not fused across", func(t *testing.T) {
		t.Parallel()
		ops, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{"name": "S1", "cell_styles": []interface{}{
				map[string]interface{}{"range": "A1:B5", "font_weight": "bold"},
				map[string]interface{}{"range": "D1:E5", "font_weight": "bold"},
			}}},
		}), testToken)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ops) != 2 {
			t.Fatalf("ops=%d, want 2 — column C was never named and must not be styled", len(ops))
		}
	})

	// The general property behind both: whatever coalescing does to the shape
	// of the stamps, the SET of cells it covers must be exactly the set the
	// caller named. Checked over a mix of touching, overlapping and separated
	// rectangles so it constrains the merge rule rather than one example.
	t.Run("coverage is preserved exactly", func(t *testing.T) {
		t.Parallel()
		inputs := []string{
			"A1:C1", "A2:C2", // touching vertically -> may fuse
			"E1:F2", "E3:F4", // touching vertically, different block
			"A5:C5", // separated from A2:C2 by row 3-4 in columns A-C
			"B2:D3", // overlaps the first block
			"H10:H10",
		}
		entries := make([]interface{}, 0, len(inputs))
		for _, r := range inputs {
			entries = append(entries, map[string]interface{}{"range": r, "font_weight": "bold"})
		}
		ops, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{"name": "S1", "cell_styles": entries}},
		}), testToken)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[[2]int]bool{}
		for _, r := range inputs {
			addRangeCells(t, want, r)
		}
		got := map[[2]int]bool{}
		for _, op := range ops {
			input := op.(map[string]interface{})["input"].(map[string]interface{})
			addRangeCells(t, got, input["range"].(string))
		}
		for cell := range want {
			if !got[cell] {
				t.Errorf("cell %v was named but no stamp covers it", cell)
			}
		}
		for cell := range got {
			if !want[cell] {
				t.Errorf("cell %v is stamped but was never named by the caller", cell)
			}
		}
	})
}

// addRangeCells records every (col,row) an A1 rectangle covers, so a test can
// compare what a spec named against what the expanded stamps actually touch.
func addRangeCells(t *testing.T, set map[[2]int]bool, rangeStr string) {
	t.Helper()
	c1, r1, c2, r2, err := workbookCreateStyleRangeBounds(rangeStr)
	if err != nil {
		t.Fatalf("bad range %q in test data: %v", rangeStr, err)
	}
	for c := c1; c <= c2; c++ {
		for r := r1; r <= r2; r++ {
			set[[2]int{c, r}] = true
		}
	}
}

// TestTypedCellsHabitualKeys pins the typed --cells cell-object fixes
// (recurring server-side 900015206 across 07-20/07-21 reruns).
func TestTypedCellsHabitualKeys(t *testing.T) {
	t.Parallel()

	t.Run("style object rewrites to cell_styles through batch", func(t *testing.T) {
		t.Parallel()
		translated, err := translateBatchOp(subOp("+cells-set", map[string]interface{}{
			"sheet_name": "S1", "range": "A1",
			"cells": []interface{}{[]interface{}{map[string]interface{}{
				"value": "x", "style": map[string]interface{}{"font_weight": "bold"},
			}}},
		}), testToken, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		input := translated["input"].(map[string]interface{})
		cell := input["cells"].([]interface{})[0].([]interface{})[0].(map[string]interface{})
		cs, _ := cell["cell_styles"].(map[string]interface{})
		if cs == nil || cs["font_weight"] != "bold" {
			t.Fatalf("cell = %v, want cell_styles.font_weight bold", cell)
		}
		if _, has := cell["style"]; has {
			t.Fatalf("style key must be renamed, got %v", cell)
		}
	})

	t.Run("type key gets a prescription", func(t *testing.T) {
		t.Parallel()
		_, err := translateBatchOp(subOp("+cells-set", map[string]interface{}{
			"sheet_name": "S1", "range": "A1",
			"cells": []interface{}{[]interface{}{map[string]interface{}{
				"value": "x", "type": "text",
			}}},
		}), testToken, 0)
		requireValidation(t, err, "not a cell field")
	})
}

// TestStylesAcceptance_ResizeAndMergeCorpus extends the corpus to the
// row/col_sizes and cell_merges sections.
func TestStylesAcceptance_ResizeAndMergeCorpus(t *testing.T) {
	t.Parallel()

	runSection := func(section string, entry interface{}) ([]interface{}, error) {
		return stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{
				"name":  "S1",
				section: []interface{}{entry},
			}},
		}), testToken)
	}
	pixelValue := func(t *testing.T, ops []interface{}, key string) interface{} {
		t.Helper()
		input := ops[0].(map[string]interface{})["input"].(map[string]interface{})
		block, _ := input[key].(map[string]interface{})
		if block == nil || block["type"] != "pixel" {
			t.Fatalf("%s = %v, want pixel block", key, input[key])
		}
		return block["value"]
	}

	t.Run("size alone implies pixel", func(t *testing.T) {
		t.Parallel()
		ops, err := runSection("row_sizes", map[string]interface{}{"range": "1:1", "size": float64(36)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v := pixelValue(t, ops, "resize_height"); v != 36 {
			t.Fatalf("value = %v, want 36", v)
		}
	})

	t.Run("width alone implies pixel on col_sizes", func(t *testing.T) {
		t.Parallel()
		ops, err := runSection("col_sizes", map[string]interface{}{"range": "A:C", "width": float64(120)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v := pixelValue(t, ops, "resize_width"); v != 120 {
			t.Fatalf("value = %v, want 120", v)
		}
	})

	t.Run("type auto still works on rows", func(t *testing.T) {
		t.Parallel()
		if _, err := runSection("row_sizes", map[string]interface{}{"range": "1:1", "type": "auto"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("neither size nor type prescribed", func(t *testing.T) {
		t.Parallel()
		_, err := runSection("row_sizes", map[string]interface{}{"range": "1:1"})
		requireValidation(t, err, "needs size (px) or type")
	})

	t.Run("wrong-dimension word prescribed", func(t *testing.T) {
		t.Parallel()
		_, err := runSection("col_sizes", map[string]interface{}{"range": "A:C", "height": float64(36)})
		requireValidation(t, err, "does not apply")
	})

	t.Run("raw OpenAPI merge_type accepted", func(t *testing.T) {
		t.Parallel()
		ops, err := runSection("cell_merges", map[string]interface{}{"range": "A1:B2", "merge_type": "MERGE_ALL"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		input := ops[0].(map[string]interface{})["input"].(map[string]interface{})
		if input["merge_type"] != "all" {
			t.Fatalf("merge_type = %v, want all", input["merge_type"])
		}
	})

	t.Run("bare string merge accepted", func(t *testing.T) {
		t.Parallel()
		if _, err := runSection("cell_merges", "A1:B2"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// TestStylesAcceptance_FlagPathParity is property 1's missing half.
//
// TestStylesAcceptance_VocabularyParity walks the same flag-defs vocabulary but
// exercises the PAYLOAD path (--styles items). The FLAG path — the flat
// --font-color / --number-format / … flags on +cells-set-style — is a separate
// hand-written mapping in buildCellStyleFromFlags, and a coverage run showed
// six of its eleven branches never executed by any test. A typo there (writing
// the wrong wire key, or reading the wrong flag) silently drops a style the
// caller explicitly asked for: the request still succeeds, the sheet just does
// not change. Derived from flag-defs so a new style flag is covered the moment
// it is declared.
func TestStylesAcceptance_FlagPathParity(t *testing.T) {
	t.Parallel()
	defs, err := loadFlagDefs()
	if err != nil {
		t.Fatalf("loadFlagDefs: %v", err)
	}
	spec, ok := defs["+cells-set-style"]
	if !ok {
		t.Fatal("no +cells-set-style flag defs")
	}

	args := []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:B2"}
	want := map[string]interface{}{}
	for _, df := range spec.Flags {
		if df.Kind != "own" || df.Name == "range" || df.Name == "border-styles" {
			continue // border-styles is a composite with its own structural tests
		}
		field := strings.ReplaceAll(df.Name, "-", "_")
		var value string
		switch {
		case len(df.Enum) > 0:
			value = df.Enum[0]
			want[field] = value
		case df.Type == "float64" || df.Type == "int":
			value = "14"
			want[field] = float64(14)
		case df.Name == "font-family":
			value, want[field] = "Arial", "Arial"
		case df.Name == "number-format":
			value, want[field] = "0.00", "0.00"
		default:
			value, want[field] = "#112233", "#112233"
		}
		args = append(args, "--"+df.Name, value)
	}
	if len(want) < 5 {
		t.Fatalf("expected the flat style vocabulary, only built %d fields", len(want))
	}

	input := decodeToolInput(t, parseDryRunBody(t, CellsSetStyle, args), "set_cell_range")
	cells, _ := input["cells"].([]interface{})
	if len(cells) == 0 {
		t.Fatalf("no cells in %v", input)
	}
	row, _ := cells[0].([]interface{})
	cell, _ := row[0].(map[string]interface{})
	got, _ := cell["cell_styles"].(map[string]interface{})
	if got == nil {
		t.Fatalf("no cell_styles emitted: %v", cell)
	}
	for field, expected := range want {
		actual, present := got[field]
		if !present {
			t.Errorf("flag --%s produced no %q on the wire — the style is silently dropped",
				strings.ReplaceAll(field, "_", "-"), field)
			continue
		}
		if actual != expected {
			t.Errorf("%s = %#v, want %#v", field, actual, expected)
		}
	}
	for field := range got {
		if _, expected := want[field]; !expected {
			t.Errorf("unexpected wire field %q emitted by the flag path", field)
		}
	}
}
