// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"strings"
	"testing"
)

// firstWriteOpInput decodes a --writes dry-run down to the tool input of its
// first fanned-out set_cell_range operation, so tests assert the fields that
// reach the wire instead of substrings of the rendered envelope.
func firstWriteOpInput(t *testing.T, stdout string) map[string]interface{} {
	t.Helper()
	ops, _ := decodeToolInput(t, decodeDryRunFirstCall(t, stdout), "batch_update")["operations"].([]interface{})
	if len(ops) == 0 {
		t.Fatalf("batch_update carried no operations:\n%s", stdout)
	}
	op, _ := ops[0].(map[string]interface{})
	if got := op["tool_name"]; got != "set_cell_range" {
		t.Fatalf("operations[0].tool_name = %v, want set_cell_range", got)
	}
	in, _ := op["input"].(map[string]interface{})
	if in == nil {
		t.Fatalf("operations[0] has no input: %#v", op)
	}
	return in
}

// TestCellsSetWrites pins the --writes plural form: scattered (cross-sheet)
// regions fan into ONE atomic batch_update, each item self-carrying its
// sheet selector (no top-level fallback — same convention as +batch-update
// sub-ops and +styles-put items), with per-item errors aggregated.
func TestCellsSetWrites(t *testing.T) {
	t.Parallel()

	writes := func(items string, extra ...string) (string, string, error) {
		args := append([]string{
			"--url", testURL, "--dry-run", "--writes", items,
		}, extra...)
		return runShortcutCapturingErr(t, CellsSet, args)
	}

	t.Run("cross-sheet items expand into one batch", func(t *testing.T) {
		t.Parallel()
		stdout, _, err := writes(`[
			{"sheet_name":"明细","range":"D5","cells":[[{"formula":"=IFERROR(C5/B5,0)"}]]},
			{"sheet_name":"汇总","range":"B3","cells":[[{"formula":"=SUM(C:C)"}]]}
		]`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, want := range []string{"batch_update", "明细", "汇总", "IFERROR"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("dry-run body missing %q: %s", want, stdout[:min(len(stdout), 400)])
			}
		}
	})

	t.Run("item without sheet selector errors", func(t *testing.T) {
		t.Parallel()
		_, _, err := writes(`[{"range":"A1","cells":[[{"value":"x"}]]}]`)
		requireValidation(t, err, "sheet-id or --sheet-name")
	})

	t.Run("item names its sheet only in range", func(t *testing.T) {
		t.Parallel()
		// The third entry point for a sheet-prefixed range: --writes builds its
		// own per-item flag view, so it needs the rewrite the standalone command
		// (cobra PreRunE) and the +batch-update sub-op each carry — otherwise the
		// same item that translates as a sub-op dies on the selector check here.
		stdout, _, err := writes(`[{"range":"Sheet1!A1","cells":[[{"value":"x"}]]}]`)
		if err != nil {
			t.Fatalf("item with a sheet-prefixed range must translate, got: %v", err)
		}
		in := firstWriteOpInput(t, stdout)
		if got := in["sheet_name"]; got != "Sheet1" {
			t.Errorf("sheet_name = %v, want Sheet1", got)
		}
		if got := in["range"]; got != "A1" {
			t.Errorf("range = %v, want A1 (the prefix belongs in the selector)", got)
		}
	})

	// The two shapes +batch-update sub-ops accept: an item may spell the
	// payload "values", or wrap it in the {"cells": …} envelope a payload
	// generator produces. Both are rewritten before the writes array is
	// schema-validated, so the same item translates on either path.
	t.Run("item payload in a habitual shape", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct{ name, items string }{
			{"values alias", `[{"sheet_name":"S1","range":"A1","values":[["x"]]}]`},
			{"cells envelope", `[{"sheet_name":"S1","range":"A1","cells":{"cells":[[{"value":"x"}]]}}]`},
			{"bare scalar matrix", `[{"sheet_name":"S1","range":"A1","cells":[["x"]]}]`},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				stdout, _, err := writes(tc.items)
				if err != nil {
					t.Fatalf("item must translate the same as a +batch-update sub-op, got: %v", err)
				}
				in := firstWriteOpInput(t, stdout)
				cells, _ := json.Marshal(in["cells"])
				if string(cells) != `[[{"value":"x"}]]` {
					t.Errorf("cells = %s, want [[{\"value\":\"x\"}]]", cells)
				}
			})
		}
	})

	t.Run("item spelling the payload twice is a conflict", func(t *testing.T) {
		t.Parallel()
		_, _, err := writes(`[{"sheet_name":"S1","range":"A1","values":[["v"]],"cells":[[{"value":"c"}]]}]`)
		requireValidation(t, err, "conflicting values")
	})

	t.Run("top-level sheet selector rejected with prescription", func(t *testing.T) {
		t.Parallel()
		_, _, err := writes(`[{"sheet_name":"S1","range":"A1","cells":[[{"value":"x"}]]}]`,
			"--sheet-name", "S1")
		requireValidation(t, err, "put sheet_name (or sheet_id) inside each writes item")
	})

	t.Run("writes and range are mutually exclusive", func(t *testing.T) {
		t.Parallel()
		_, _, err := writes(`[{"sheet_name":"S1","range":"A1","cells":[[{"value":"x"}]]}]`,
			"--range", "A1")
		requireValidation(t, err, "mutually exclusive")
	})

	t.Run("per-item errors aggregate", func(t *testing.T) {
		t.Parallel()
		// Both items pass the --writes schema (range+cells present) but fail
		// deeper: item 0 a matrix mismatch, item 1 a missing sheet selector.
		_, _, err := writes(`[
			{"sheet_name":"S1","range":"A1:B2","cells":[[{"value":"x"}]]},
			{"range":"C1","cells":[[{"value":"y"}]]}
		]`)
		ve := requireValidation(t, err, "--writes has 2 issues")
		for _, want := range []string{"--writes[0]", "--writes[1]", "sheet-name"} {
			if !strings.Contains(ve.Message, want) {
				t.Fatalf("message %q missing %q", ve.Message, want)
			}
		}
	})

	t.Run("item keys go through the vocabulary layer", func(t *testing.T) {
		t.Parallel()
		stdout, _, err := writes(`[{"sheetName":"S1","range":"A1","cells":[[{"value":"x"}]]}]`)
		if err != nil {
			t.Fatalf("camelCase sheetName must normalize: %v", err)
		}
		if !strings.Contains(stdout, "S1") {
			t.Fatalf("normalized item missing sheet: %s", stdout[:min(len(stdout), 300)])
		}
	})

	t.Run("explicit --allow-overwrite=false reaches every item", func(t *testing.T) {
		t.Parallel()
		// The per-item flag view falls back to the declared default (true), so
		// an explicit top-level false must be written into items that don't
		// carry their own — otherwise the batch silently overwrites cells the
		// caller asked to protect.
		stdout, _, err := writes(`[{"sheet_name":"S1","range":"A1","cells":[[{"value":"x"}]]}]`,
			"--allow-overwrite=false")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(stdout, "allow_overwrite") {
			t.Fatalf("dry-run body missing allow_overwrite:false: %s", stdout[:min(len(stdout), 400)])
		}
	})

	t.Run("item-level allow_overwrite wins over the top-level flag", func(t *testing.T) {
		t.Parallel()
		stdout, _, err := writes(`[
			{"sheet_name":"S1","range":"A1","cells":[[{"value":"x"}]],"allow_overwrite":true},
			{"sheet_name":"S1","range":"B1","cells":[[{"value":"y"}]]}
		]`, "--allow-overwrite=false")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Item 0 opts back in (true → key omitted on the wire); item 1 inherits
		// the explicit false. Any allow_overwrite in the output must be false.
		if strings.Contains(stdout, `allow_overwrite\":true`) || strings.Contains(stdout, `"allow_overwrite": true`) {
			t.Fatalf("item-level true must be omitted on the wire, not serialized: %s", stdout[:min(len(stdout), 600)])
		}
		if !strings.Contains(stdout, "allow_overwrite") {
			t.Fatalf("item without override must inherit the explicit false: %s", stdout[:min(len(stdout), 600)])
		}
	})

	t.Run("default overwrite omits the key entirely", func(t *testing.T) {
		t.Parallel()
		stdout, _, err := writes(`[{"sheet_name":"S1","range":"A1","cells":[[{"value":"x"}]]}]`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(stdout, "allow_overwrite") {
			t.Fatalf("default run must not emit allow_overwrite: %s", stdout[:min(len(stdout), 400)])
		}
	})

	t.Run("cannot nest inside batch-update", func(t *testing.T) {
		t.Parallel()
		_, err := translateBatchOp(subOp("+cells-set", map[string]interface{}{
			"writes": []interface{}{map[string]interface{}{
				"sheet_name": "S1", "range": "A1", "cells": []interface{}{[]interface{}{map[string]interface{}{"value": "x"}}},
			}},
		}), testToken, 0)
		requireValidation(t, err, "not supported inside +batch-update")
	})

	t.Run("styles flag gets the layering prescription", func(t *testing.T) {
		t.Parallel()
		// Ergonomics (FlagErrorFunc hints) mount via the registry, not the
		// bare shortcut var — mirror the real CLI wiring.
		sc := shortcutFromRegistry(t, "+cells-set")
		_, _, err := runShortcutCapturingErr(t, sc, []string{
			"--url", testURL, "--dry-run",
			"--writes", `[{"sheet_name":"S1","range":"A1","cells":[[{"value":"x"}]]}]`,
			"--styles", `{"styles":[]}`,
		})
		ve := requireValidation(t, err, "unknown flag")
		if !strings.Contains(ve.Hint, "+styles-put") || !strings.Contains(ve.Hint, "cell_styles") {
			t.Fatalf("want the styles-put layering hint, got hint=%q", ve.Hint)
		}
	})
}
