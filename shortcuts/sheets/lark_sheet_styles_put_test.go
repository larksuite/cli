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
	wantTools := []string{"merge_cells", "set_cell_range", "resize_range", "resize_range", "modify_sheet_structure", "modify_sheet_structure"}
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
	// Freeze ops carry the freeze counts.
	fr := ops[4].(map[string]interface{})["input"].(map[string]interface{})
	if fr["operation"] != "freeze" || fr["freeze_rows"] != 1 {
		t.Fatalf("freeze rows op = %v", fr)
	}
	fc := ops[5].(map[string]interface{})["input"].(map[string]interface{})
	if fc["freeze_columns"] != 2 {
		t.Fatalf("freeze cols op = %v", fc)
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

	t.Run("all-zero freeze rejected", func(t *testing.T) {
		t.Parallel()
		_, err := stylesPutOperations(stylesPutView(map[string]interface{}{
			"styles": []interface{}{map[string]interface{}{"name": "S1", "freeze": map[string]interface{}{"rows": float64(0)}}},
		}), testToken)
		requireValidation(t, err, "at least one dimension")
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
