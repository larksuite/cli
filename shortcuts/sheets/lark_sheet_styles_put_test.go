// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"
	"testing"
)

func stylesPutView(spec map[string]interface{}) mapFlagView {
	return newMapFlagViewForCommand("+styles-put", map[string]interface{}{"styles": spec})
}

// TestStylesPutOperations_ExpansionOrder pins the per-sheet expansion:
// cell_merges → cell_styles → row_sizes → col_sizes → freeze, all inside one
// batch_update operations array (server-side order dependence verified live).
func TestStylesPutOperations_ExpansionOrder(t *testing.T) {
	t.Parallel()
	ops, err := stylesPutOperations(stylesPutView(map[string]interface{}{
		"styles": []interface{}{map[string]interface{}{
			"name":        "Sheet1",
			"cell_merges": []interface{}{map[string]interface{}{"range": "A5:A8"}},
			"cell_styles": []interface{}{map[string]interface{}{"range": "A1:B1", "font_weight": "bold"}},
			"row_sizes":   []interface{}{map[string]interface{}{"range": "1:1", "type": "pixel", "size": float64(36)}},
			"col_sizes":   []interface{}{map[string]interface{}{"range": "A:B", "type": "pixel", "size": float64(120)}},
			"freeze":      map[string]interface{}{"rows": float64(1), "cols": float64(2)},
		}},
	}), testToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantTools := []string{"merge_cells", "set_cell_range", "resize_range", "resize_range", "modify_sheet_structure"}
	if len(ops) != len(wantTools) {
		t.Fatalf("got %d ops, want %d", len(ops), len(wantTools))
	}
	for i, want := range wantTools {
		op := ops[i].(map[string]interface{})
		if op["tool_name"] != want {
			t.Fatalf("ops[%d].tool_name = %v, want %s", i, op["tool_name"], want)
		}
		input := op["input"].(map[string]interface{})
		if input["sheet_name"] != "Sheet1" {
			t.Fatalf("ops[%d] missing sheet_name: %v", i, input)
		}
		if input["excel_id"] != testToken {
			t.Fatalf("ops[%d] missing excel_id", i)
		}
	}
	// The style stamp carries a cells matrix matching the range (1×2).
	stamp := ops[1].(map[string]interface{})["input"].(map[string]interface{})
	cells := stamp["cells"].([][]interface{})
	if len(cells) != 1 || len(cells[0]) != 2 {
		t.Fatalf("style stamp matrix = %dx%d, want 1x2", len(cells), len(cells[0]))
	}
	// Freeze rows and columns are combined into one operation because freeze is
	// full-state replacement server-side (verified 07-31 live): two per-axis
	// calls leave only the last axis frozen.
	freeze := ops[4].(map[string]interface{})["input"].(map[string]interface{})
	if freeze["operation"] != "freeze" || freeze["freeze_rows"] != 1 || freeze["freeze_columns"] != 2 {
		t.Fatalf("freeze op = %v", freeze)
	}
}

// TestStylesPutOperations_Validation pins the aggregate error shape and the
// section/name requirements.
func TestStylesPutOperations_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing name and empty item aggregate", func(t *testing.T) {
		t.Parallel()
		_, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{
				map[string]interface{}{"cell_styles": []interface{}{map[string]interface{}{"range": "A1", "font_weight": "bold"}}},
				map[string]interface{}{"name": "S2"},
			},
		}), testToken)
		ve := requireValidation(t, err, "name is required")
		if !strings.Contains(ve.Message, "at least one of cell_styles/row_sizes/col_sizes/cell_merges/freeze") {
			t.Fatalf("message %q missing empty-item issue", ve.Message)
		}
	})

	t.Run("duplicate sheet name rejected", func(t *testing.T) {
		t.Parallel()
		item := map[string]interface{}{"name": "S1", "freeze": map[string]interface{}{"rows": float64(1)}}
		item2 := map[string]interface{}{"name": "S1", "freeze": map[string]interface{}{"rows": float64(2)}}
		_, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{item, item2},
		}), testToken)
		requireValidation(t, err, "appears twice")
	})

	t.Run("freeze-only item is valid", func(t *testing.T) {
		t.Parallel()
		ops, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{"name": "S1", "freeze": map[string]interface{}{"rows": float64(1)}}},
		}), testToken)
		if err != nil || len(ops) != 1 {
			t.Fatalf("ops=%d err=%v", len(ops), err)
		}
	})

	t.Run("all-zero freeze means unfreeze on an existing sheet", func(t *testing.T) {
		// Freeze is full-state replacement, so {rows:0} (cols omitted → 0) on an
		// existing sheet states "nothing frozen" — the same request +dim-freeze
		// --rows 0 --cols 0 sends. Only the create path rejects all-zero.
		t.Parallel()
		ops, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{"name": "S1", "freeze": map[string]interface{}{"rows": float64(0)}}},
		}), testToken)
		if err != nil || len(ops) != 1 {
			t.Fatalf("ops=%d err=%v, want one unfreeze op", len(ops), err)
		}
		input := ops[0].(map[string]interface{})["input"].(map[string]interface{})
		if input["operation"] != "unfreeze" {
			t.Fatalf("operation = %v, want unfreeze", input["operation"])
		}
	})

	t.Run("empty freeze object is rejected", func(t *testing.T) {
		t.Parallel()
		_, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{"name": "S1", "freeze": map[string]interface{}{}}},
		}), testToken)
		requireValidation(t, err, "must specify rows or cols")
	})

	t.Run("range prefixed with another sheet rejected", func(t *testing.T) {
		// Silently stripping "Detail!" would retarget the styles onto Summary.
		t.Parallel()
		_, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{
				"name":        "Summary",
				"cell_styles": []interface{}{map[string]interface{}{"range": "Detail!A1:D1", "font_weight": "bold"}},
			}},
		}), testToken)
		ve := requireValidation(t, err, `names sheet "Detail" but the item targets "Summary"`)
		if !strings.Contains(ve.Message, "cell_styles") {
			t.Fatalf("message %q should locate the offending section", ve.Message)
		}
	})

	t.Run("range prefixed with the item's own sheet passes and strips for every visual op", func(t *testing.T) {
		t.Parallel()
		ops, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{
				"name":        "Summary",
				"cell_styles": []interface{}{map[string]interface{}{"range": "'Summary'!A1:D1", "font_weight": "bold"}},
				"cell_merges": []interface{}{map[string]interface{}{"range": "Summary!A2:B2"}, "'Summary'!C2:D2"},
				"row_sizes":   []interface{}{map[string]interface{}{"range": "Summary!2:3", "type": "pixel", "size": float64(32)}},
				"col_sizes":   []interface{}{map[string]interface{}{"range": "'Summary'!A:C", "type": "pixel", "size": float64(120)}},
			}},
		}), testToken)
		if err != nil {
			t.Fatalf("matching prefix must stay accepted: %v", err)
		}
		gotRanges := []string{}
		for _, raw := range ops {
			input := raw.(map[string]interface{})["input"].(map[string]interface{})
			if rng, _ := input["range"].(string); rng != "" {
				gotRanges = append(gotRanges, rng)
			}
		}
		want := []string{"A2:B2", "C2:D2", "A1:D1", "2:3", "A:C"}
		if len(gotRanges) != len(want) {
			t.Fatalf("ranges = %v, want %v", gotRanges, want)
		}
		for i := range want {
			if gotRanges[i] != want[i] {
				t.Fatalf("ranges = %v, want %v", gotRanges, want)
			}
		}
	})

	t.Run("unknown item key rejected with did-you-mean", func(t *testing.T) {
		t.Parallel()
		_, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{
				"name":        "S1",
				"cell_styles": []interface{}{map[string]interface{}{"range": "A1", "font_weight": "bold"}},
				"freezee":     map[string]interface{}{"rows": float64(1)},
			}},
		}), testToken)
		requireValidation(t, err, `unknown key "freezee" — did you mean "freeze"`)
	})
}

// TestStylesPayloadVocabularyForgiveness pins the 07-20 rerun fixes: the
// payload path (--styles cell_styles objects) accepts the same habitual
// vocabulary the flag path already normalized — border family folding, wrap
// aliases, and enum VALUE canonicalization (CSS center → Lark middle etc.).
func TestStylesPayloadVocabularyForgiveness(t *testing.T) {
	t.Parallel()

	stamp := func(styleFields map[string]interface{}) ([]interface{}, error) {
		item := map[string]interface{}{"range": "A1:B1"}
		for k, v := range styleFields {
			item[k] = v
		}
		return stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{
				"name":        "S1",
				"cell_styles": []interface{}{item},
			}},
		}), testToken)
	}
	cellProto := func(t *testing.T, ops []interface{}) map[string]interface{} {
		t.Helper()
		input := ops[0].(map[string]interface{})["input"].(map[string]interface{})
		cells := input["cells"].([][]interface{})
		return cells[0][0].(map[string]interface{})
	}

	t.Run("vertical_alignment center canonicalizes to middle", func(t *testing.T) {
		t.Parallel()
		ops, err := stamp(map[string]interface{}{"vertical_alignment": "center", "font_weight": "BOLD"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		cs := cellProto(t, ops)["cell_styles"].(map[string]interface{})
		if cs["vertical_alignment"] != "middle" || cs["font_weight"] != "bold" {
			t.Fatalf("cell_styles = %v, want middle/bold", cs)
		}
	})

	t.Run("off-enum value rejected client-side with did-you-mean", func(t *testing.T) {
		t.Parallel()
		_, err := stamp(map[string]interface{}{"vertical_alignment": "botom"})
		requireValidation(t, err, `did you mean "bottom"`)
	})

	t.Run("boolean wrap_text folds to word_wrap auto-wrap", func(t *testing.T) {
		t.Parallel()
		ops, err := stamp(map[string]interface{}{"wrap_text": true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		cs := cellProto(t, ops)["cell_styles"].(map[string]interface{})
		if cs["word_wrap"] != "auto-wrap" {
			t.Fatalf("word_wrap = %v, want auto-wrap", cs["word_wrap"])
		}
	})

	t.Run("borders object folds into border_styles", func(t *testing.T) {
		t.Parallel()
		ops, err := stamp(map[string]interface{}{
			"borders": map[string]interface{}{"style": "solid", "color": "#000000"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		bs := cellProto(t, ops)["border_styles"].(map[string]interface{})
		top, _ := bs["top"].(map[string]interface{})
		if top == nil || top["style"] != "solid" {
			t.Fatalf("border_styles = %v, want all-sides solid", bs)
		}
	})

	t.Run("flattened border_bottom and border_top_color fold per side", func(t *testing.T) {
		t.Parallel()
		ops, err := stamp(map[string]interface{}{
			"border_bottom":    map[string]interface{}{"style": "solid"},
			"border_top_color": "#FF0000",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		bs := cellProto(t, ops)["border_styles"].(map[string]interface{})
		bottom, _ := bs["bottom"].(map[string]interface{})
		topSide, _ := bs["top"].(map[string]interface{})
		if bottom["style"] != "solid" || topSide["color"] != "#FF0000" {
			t.Fatalf("border_styles = %v", bs)
		}
	})

	t.Run("border_style thin means thin solid line", func(t *testing.T) {
		t.Parallel()
		ops, err := stamp(map[string]interface{}{"border_style": "thin"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		bs := cellProto(t, ops)["border_styles"].(map[string]interface{})
		top, _ := bs["top"].(map[string]interface{})
		if top["weight"] != "thin" || top["style"] != "solid" {
			t.Fatalf("border_styles.top = %v, want thin solid", top)
		}
	})

	t.Run("fore_color prescribes instead of guessing", func(t *testing.T) {
		t.Parallel()
		_, err := stamp(map[string]interface{}{"fore_color": "#FF0000"})
		requireValidation(t, err, "fore_color is ambiguous")
	})

	t.Run("bare string cell_merges accepted", func(t *testing.T) {
		t.Parallel()
		ops, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{
				"name":        "S1",
				"cell_merges": []interface{}{"A5:B6"},
			}},
		}), testToken)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		input := ops[0].(map[string]interface{})["input"].(map[string]interface{})
		if input["range"] != "A5:B6" || input["merge_type"] != "all" {
			t.Fatalf("merge op = %v", input)
		}
	})
}

// TestStylesResizeSizeAliases pins the one-way Excel-vocabulary aliases on
// the shared styles resize parser: height in row_sizes / width in col_sizes
// resolve to size silently; the wrong dimension's word is a targeted error.
func TestStylesResizeSizeAliases(t *testing.T) {
	t.Parallel()

	t.Run("height aliases to size in row_sizes", func(t *testing.T) {
		t.Parallel()
		ops, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{
				"name":      "S1",
				"row_sizes": []interface{}{map[string]interface{}{"range": "1:1", "type": "pixel", "height": float64(36)}},
			}},
		}), testToken)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		input := ops[0].(map[string]interface{})["input"].(map[string]interface{})
		block := input["resize_height"].(map[string]interface{})
		if block["value"] != 36 {
			t.Fatalf("resize_height = %v, want value 36", block)
		}
	})

	t.Run("width aliases to size in col_sizes", func(t *testing.T) {
		t.Parallel()
		_, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{
				"name":      "S1",
				"col_sizes": []interface{}{map[string]interface{}{"range": "A:C", "type": "pixel", "width": float64(120)}},
			}},
		}), testToken)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("wrong-dimension word is a targeted error", func(t *testing.T) {
		t.Parallel()
		_, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{
				"name":      "S1",
				"row_sizes": []interface{}{map[string]interface{}{"range": "1:1", "type": "pixel", "width": float64(36)}},
			}},
		}), testToken)
		requireValidation(t, err, "does not apply to this array")
	})

	t.Run("size plus alias together rejected", func(t *testing.T) {
		t.Parallel()
		_, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{
				"name":      "S1",
				"row_sizes": []interface{}{map[string]interface{}{"range": "1:1", "type": "pixel", "size": float64(36), "height": float64(40)}},
			}},
		}), testToken)
		requireValidation(t, err, "either size or height")
	})
}

// TestDimDeleteRangesOps pins the descending-order expansion and the
// same-dimension / non-overlap guards.
func TestDimDeleteRangesOps(t *testing.T) {
	t.Parallel()

	view := func(ranges ...interface{}) mapFlagView {
		return newMapFlagViewForCommand("+dim-delete", map[string]interface{}{"ranges": ranges})
	}

	t.Run("rows execute descending", func(t *testing.T) {
		t.Parallel()
		ops, err := dimDeleteRangesOps(view("5:5", "11:13", "8:8"), testToken, "", "S1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var got []string
		for _, op := range ops {
			got = append(got, op.(map[string]interface{})["input"].(map[string]interface{})["range"].(string))
		}
		want := []string{"11:13", "8:8", "5:5"}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("order = %v, want %v", got, want)
			}
		}
	})

	t.Run("mixed dimensions rejected", func(t *testing.T) {
		t.Parallel()
		_, err := dimDeleteRangesOps(view("5:5", "C:C"), testToken, "", "S1")
		requireValidation(t, err, "rows OR columns")
	})

	t.Run("overlap rejected", func(t *testing.T) {
		t.Parallel()
		_, err := dimDeleteRangesOps(view("5:8", "7:9"), testToken, "", "S1")
		requireValidation(t, err, "overlap")
	})

	t.Run("ranges cannot nest inside batch", func(t *testing.T) {
		t.Parallel()
		_, err := translateBatchOp(subOp("+dim-delete", map[string]interface{}{
			"sheet_name": "S1",
			"ranges":     []interface{}{"5:5", "8:8"},
		}), testToken, 0)
		requireValidation(t, err, "not supported inside +batch-update")
	})
}

// TestCoalesceStyleStamps_PreservesLastWriteWins pins the ordering contract of
// the stamp optimizer: style writes are field-wise last-write-wins, so two
// same-style stamps may only be merged when nothing between them touches the
// cells whose write would move earlier. Grouping globally by style content
// (the original implementation) turned red → blue → red into red → blue and
// silently changed the final color.
func TestCoalesceStyleStamps_PreservesLastWriteWins(t *testing.T) {
	t.Parallel()

	red := map[string]interface{}{"background_color": "#FF0000"}
	blue := map[string]interface{}{"background_color": "#0000FF"}
	stamp := func(rng string, style map[string]interface{}) workbookCreateCellStyleOp {
		return workbookCreateCellStyleOp{Range: rng, Style: style}
	}
	lastStyleFor := func(ops []workbookCreateCellStyleOp, rng string) map[string]interface{} {
		var out map[string]interface{}
		for _, op := range ops {
			if op.Range == rng {
				out = op.Style
			}
		}
		return out
	}

	t.Run("same cell red blue red keeps red last", func(t *testing.T) {
		t.Parallel()
		got := coalesceStyleStamps([]workbookCreateCellStyleOp{
			stamp("A1:A1", red), stamp("A1:A1", blue), stamp("A1:A1", red),
		})
		if last := lastStyleFor(got, "A1:A1"); last == nil || last["background_color"] != "#FF0000" {
			t.Fatalf("final style for A1 = %v, want the trailing red; ops=%+v", last, got)
		}
	})

	t.Run("intervening overlapping stamp blocks the merge", func(t *testing.T) {
		t.Parallel()
		// bold A1:B1, italic on B1, bold B1 again: merging the two bolds would
		// hoist B1's bold ahead of the italic and lose the italic.
		bold := map[string]interface{}{"font_weight": "bold"}
		italic := map[string]interface{}{"font_style": "italic"}
		got := coalesceStyleStamps([]workbookCreateCellStyleOp{
			stamp("A1:A1", bold), stamp("A1:A1", italic), stamp("A1:A1", bold),
		})
		if len(got) != 3 {
			t.Fatalf("overlapping intermediate stamp must prevent merging, got %d ops: %+v", len(got), got)
		}
	})

	t.Run("adjacent same-style runs still coalesce", func(t *testing.T) {
		t.Parallel()
		bold := map[string]interface{}{"font_weight": "bold"}
		got := coalesceStyleStamps([]workbookCreateCellStyleOp{
			stamp("A1:A1", bold), stamp("A2:A2", bold), stamp("A3:A3", bold),
		})
		if len(got) != 1 || got[0].Range != "A1:A3" {
			t.Fatalf("adjacent same-style stamps should merge into A1:A3, got %+v", got)
		}
	})

	t.Run("disjoint intermediate stamp does not block the merge", func(t *testing.T) {
		t.Parallel()
		bold := map[string]interface{}{"font_weight": "bold"}
		italic := map[string]interface{}{"font_style": "italic"}
		got := coalesceStyleStamps([]workbookCreateCellStyleOp{
			stamp("A1:A1", bold), stamp("Z9:Z9", italic), stamp("A2:A2", bold),
		})
		if len(got) != 2 {
			t.Fatalf("disjoint intermediate stamp should still allow merging, got %+v", got)
		}
		if got[0].Range != "A1:A2" {
			t.Fatalf("bold stamps should merge to A1:A2, got %+v", got)
		}
	})
}
