// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"
	"testing"
)

// subOp builds a raw +batch-update sub-op for translateBatchOp tests.
func subOp(shortcut string, input map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"shortcut": shortcut, "input": input}
}

// TestBatchOp_UnknownInputKeyRejected pins the key-vocabulary guard: an
// off-vocabulary sub-op input key must error with a did-you-mean instead of
// being silently ignored (silent ignore surfaced as misleading "missing
// required flag" errors — the top batch error cluster in eval traces).
func TestBatchOp_UnknownInputKeyRejected(t *testing.T) {
	t.Parallel()

	t.Run("invented key errors with did-you-mean", func(t *testing.T) {
		t.Parallel()
		_, err := translateBatchOp(subOp("+cells-set", map[string]interface{}{
			"sheet_name": "S1",
			"rangee":     "A1:B2",
			"cells":      []interface{}{[]interface{}{map[string]interface{}{"value": "x"}}},
		}), testToken, 0)
		ve := requireValidation(t, err, `unknown input key "rangee"`)
		if !strings.Contains(ve.Message, `did you mean "range"`) {
			t.Fatalf("message %q missing did-you-mean", ve.Message)
		}
		if !strings.Contains(ve.Hint, "input keys:") {
			t.Fatalf("hint %q missing key contract", ve.Hint)
		}
	})

	t.Run("system flag is not sub-op vocabulary", func(t *testing.T) {
		t.Parallel()
		_, err := translateBatchOp(subOp("+cells-clear", map[string]interface{}{
			"sheet_name": "S1",
			"range":      "A1:B2",
			"dry_run":    true,
		}), testToken, 0)
		requireValidation(t, err, `unknown input key "dry_run"`)
	})

	t.Run("reserved locator in hyphen form still rejected", func(t *testing.T) {
		t.Parallel()
		_, err := translateBatchOp(subOp("+cells-clear", map[string]interface{}{
			"sheet_name":        "S1",
			"range":             "A1:B2",
			"spreadsheet-token": "shtXXX",
		}), testToken, 0)
		requireValidation(t, err, "do not pass input.spreadsheet-token")
	})
}

// TestBatchOp_HabitualKeysRewritten pins the silent rewrites: camelCase onto
// the declared flag, and the commandFlagAliases table (size → width/height on
// the resize pair — the pre-2026-07 vocabulary and the styles-protocol
// spelling, the single largest sub-op error cluster).
func TestBatchOp_HabitualKeysRewritten(t *testing.T) {
	t.Parallel()

	t.Run("camelCase sheetName resolves", func(t *testing.T) {
		t.Parallel()
		translated, err := translateBatchOp(subOp("+cells-clear", map[string]interface{}{
			"sheetName": "S1",
			"range":     "A1:B2",
		}), testToken, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		input := translated["input"].(map[string]interface{})
		if input["sheet_name"] != "S1" {
			t.Fatalf("sheet_name = %v, want S1", input["sheet_name"])
		}
	})

	t.Run("size aliases to width on +cols-resize", func(t *testing.T) {
		t.Parallel()
		translated, err := translateBatchOp(subOp("+cols-resize", map[string]interface{}{
			"sheet_name": "S1",
			"range":      "A:C",
			"type":       "pixel",
			"size":       float64(120),
		}), testToken, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		input := translated["input"].(map[string]interface{})
		width, _ := input["resize_width"].(map[string]interface{})
		if width["value"] != 120 {
			t.Fatalf("resize_width = %v, want value 120", input["resize_width"])
		}
	})

	t.Run("size aliases to height on +rows-resize", func(t *testing.T) {
		t.Parallel()
		_, err := translateBatchOp(subOp("+rows-resize", map[string]interface{}{
			"sheet_name": "S1",
			"range":      "1:3",
			"type":       "pixel",
			"size":       float64(36),
		}), testToken, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("single-entry ranges unwraps onto range", func(t *testing.T) {
		t.Parallel()
		translated, err := translateBatchOp(subOp("+cells-clear", map[string]interface{}{
			"sheet_name": "S1",
			"ranges":     []interface{}{"A1:B2"},
		}), testToken, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		input := translated["input"].(map[string]interface{})
		if input["range"] != "A1:B2" {
			t.Fatalf("range = %v, want A1:B2", input["range"])
		}
	})

	t.Run("multi-entry ranges prescribes a split", func(t *testing.T) {
		t.Parallel()
		_, err := translateBatchOp(subOp("+cells-clear", map[string]interface{}{
			"sheet_name": "S1",
			"ranges":     []interface{}{"A1:B2", "C1:D2"},
		}), testToken, 0)
		requireValidation(t, err, "split them into 2 sub-ops")
	})

	// A variant next to its canonical key must reject, not silently overwrite:
	// keys iterate in sorted order, so the variant's rewrite would land after
	// the canonical value was already accepted and clobber it.
	t.Run("camelCase variant alongside canonical rejects", func(t *testing.T) {
		t.Parallel()
		_, err := translateBatchOp(subOp("+cells-clear", map[string]interface{}{
			"sheetName":  "shadow",
			"sheet_name": "S1",
			"range":      "A1:B2",
		}), testToken, 0)
		requireValidation(t, err, "got both")
	})

	t.Run("ranges alongside range rejects", func(t *testing.T) {
		t.Parallel()
		_, err := translateBatchOp(subOp("+cells-clear", map[string]interface{}{
			"sheet_name": "S1",
			"range":      "A1:B2",
			"ranges":     []interface{}{"C1:D2"},
		}), testToken, 0)
		requireValidation(t, err, "got both")
	})
}

// TestBatchOperations_AggregatesValidationErrors pins the one-pass contract:
// several invalid ops come back in a single error (each with its own
// operations[i] context) instead of the first only — eval traces show
// fix-one-resend loops of up to 7 round trips under first-error-only.
func TestBatchOperations_AggregatesValidationErrors(t *testing.T) {
	t.Parallel()

	t.Run("two bad ops both reported", func(t *testing.T) {
		t.Parallel()
		_, err := translateBatchOperations([]interface{}{
			subOp("+cells-clear", map[string]interface{}{"range": "A1:B2"}),                     // missing sheet selector
			subOp("+cells-set", map[string]interface{}{"sheet_name": "S1", "range": "A1"}),      // missing cells
			subOp("+cells-clear", map[string]interface{}{"sheet_name": "S1", "range": "A1:B2"}), // valid
		}, testToken)
		ve := requireValidation(t, err, "2 of 3 operations failed validation")
		for _, want := range []string{"operations[0] (+cells-clear)", "operations[1] (+cells-set)", "--cells is required"} {
			if !strings.Contains(ve.Message, want) {
				t.Fatalf("message %q missing %q", ve.Message, want)
			}
		}
	})

	t.Run("single bad op keeps the standalone-shaped error", func(t *testing.T) {
		t.Parallel()
		_, err := translateBatchOperations([]interface{}{
			subOp("+cells-set", map[string]interface{}{"sheet_name": "S1", "range": "A1"}),
		}, testToken)
		ve := requireValidation(t, err, "--cells is required")
		if strings.Contains(ve.Message, "failed validation") {
			t.Fatalf("single-error message must not use the aggregate wrapper: %q", ve.Message)
		}
	})
}

// TestCellsSetInput_MatrixPrecheck pins the local cells-vs-range guard that
// front-runs the server's mid-batch "does not match range" failures.
func TestCellsSetInput_MatrixPrecheck(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		input        map[string]interface{}
		wantContains string // "" = expect success
	}{
		{
			"empty cells prescribes +cells-clear",
			map[string]interface{}{"sheet_name": "S1", "range": "A1:B2", "cells": []interface{}{}},
			"+cells-clear",
		},
		{
			"row count mismatch",
			map[string]interface{}{"sheet_name": "S1", "range": "A1:B3",
				"cells": []interface{}{
					[]interface{}{map[string]interface{}{"value": "a"}, map[string]interface{}{"value": "b"}},
				}},
			"has 1 rows but --range \"A1:B3\" spans 3 rows",
		},
		{
			"column count mismatch",
			map[string]interface{}{"sheet_name": "S1", "range": "A1:B1",
				"cells": []interface{}{
					[]interface{}{map[string]interface{}{"value": "a"}},
				}},
			"has 1 columns but --range \"A1:B1\" spans 2 columns",
		},
		{
			"matching matrix passes",
			map[string]interface{}{"sheet_name": "S1", "range": "A1:B2",
				"cells": []interface{}{
					[]interface{}{map[string]interface{}{"value": "a"}, map[string]interface{}{"value": "b"}},
					[]interface{}{map[string]interface{}{"value": "c"}, map[string]interface{}{"value": "d"}},
				}},
			"",
		},
		{
			"bare single-cell range enforces the 1x1 match (07-21: server rejects anchors too)",
			map[string]interface{}{"sheet_name": "S1", "range": "A1",
				"cells": []interface{}{
					[]interface{}{map[string]interface{}{"value": "a"}, map[string]interface{}{"value": "b"}},
				}},
			"has 2 columns but --range \"A1\" spans 1 columns",
		},
		{
			"single-cell range with a single cell passes",
			map[string]interface{}{"sheet_name": "S1", "range": "B3",
				"cells": []interface{}{
					[]interface{}{map[string]interface{}{"value": "a"}},
				}},
			"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := translateBatchOp(subOp("+cells-set", tc.input), testToken, 0)
			if tc.wantContains == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			requireValidation(t, err, tc.wantContains)
		})
	}
}

// TestFlattenToolErrorMsg_PartialFailureRecovery pins the no-rollback recovery
// prescription appended to server-side "N succeeded, M failed" errors.
func TestFlattenToolErrorMsg_PartialFailureRecovery(t *testing.T) {
	t.Parallel()

	wrap := func(inner string) string {
		return `{"error":` + jsonQuote(inner) + `}`
	}

	t.Run("single failure prescribes resend-from-index", func(t *testing.T) {
		t.Parallel()
		msg := flattenToolErrorMsg(wrap(`{"message":"batch_update: 4 succeeded, 1 failed","failures":[{"index":4,"tool_name":"set_cell_range","error":"cells is required"}]}`))
		for _, want := range []string{"operations[4] (set_cell_range)", "no rollback", "resend only operations[4:]"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("msg %q missing %q", msg, want)
			}
		}
	})

	t.Run("multiple failures prescribe failed-only resend", func(t *testing.T) {
		t.Parallel()
		msg := flattenToolErrorMsg(wrap(`{"message":"batch_update: 3 succeeded, 2 failed","failures":[{"index":1,"tool_name":"set_cell_range","error":"e1"},{"index":3,"tool_name":"resize_range","error":"e2"}]}`))
		if !strings.Contains(msg, "resend only the failed operations") {
			t.Fatalf("msg %q missing failed-only prescription", msg)
		}
	})

	t.Run("zero succeeded gets no note", func(t *testing.T) {
		t.Parallel()
		msg := flattenToolErrorMsg(wrap(`{"message":"batch_update: 0 succeeded, 1 failed","failures":[{"index":0,"tool_name":"set_cell_range","error":"e"}]}`))
		if strings.Contains(msg, "no rollback") {
			t.Fatalf("msg %q must not carry the note when nothing was applied", msg)
		}
	})
}

// jsonQuote wraps s as a JSON string literal (escaping quotes), mirroring how
// the server double-encodes the inner error payload.
func jsonQuote(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}
