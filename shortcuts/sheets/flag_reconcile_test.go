// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"fmt"
	"strings"
	"testing"
)

// A --sheet-name naming the sub-sheet the payload already names says nothing
// the payload does not; refusing it spends a turn on a disagreement that does
// not exist. 6,697 calls on +table-put, 925 on +styles-put.
func TestReconcile_SheetSelectorEchoingThePayload(t *testing.T) {
	t.Parallel()
	t.Run("agreeing selector is accepted", func(t *testing.T) {
		t.Parallel()
		calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+table-put"), []string{
			"--url", testURL, "--sheet-name", "S",
			"--sheets", `{"sheets":[{"name":"S","columns":["a"],"data":[["x"]]}]}`,
		})
		if len(calls) == 0 {
			t.Fatal("a --sheet-name matching the payload must not fail the call")
		}
	})

	t.Run("disagreeing selector still fails, naming both sides", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+table-put"), []string{
			"--url", testURL, "--sheet-name", "OTHER", "--dry-run",
			"--sheets", `{"sheets":[{"name":"S","columns":["a"],"data":[["x"]]}]}`,
		})
		ve := requireValidation(t, err, "does not match any sub-sheet")
		if !strings.Contains(ve.Message, `"S"`) {
			t.Errorf("the message must name what the payload DOES carry, got %q", ve.Message)
		}
	})
}

// --dimension restates the axis the span already spells. Agreement passes;
// a real disagreement fails rather than picking a side.
func TestReconcile_DimensionAgreesWithTheSpan(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, dim, span string
		wantErr         bool
	}{
		{"rows agree", "row", "3:5", false},
		{"columns agree", "column", "C:E", false},
		{"plural spelling agrees", "rows", "3:5", false},
		{"column against a row span", "column", "3:5", true},
		{"row against a column span", "row", "C:E", true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+dim-delete"), []string{
				"--url", testURL, "--sheet-name", "S",
				"--dimension", tt.dim, "--range", tt.span, "--dry-run",
			})
			if tt.wantErr {
				requireValidation(t, err, "disagrees with")
				return
			}
			if err != nil {
				t.Fatalf("agreeing --dimension was refused: %v", err)
			}
		})
	}
}

func TestDimensionOfSpan(t *testing.T) {
	t.Parallel()
	for span, want := range map[string]string{
		"3": "row", "3:5": "row", "C": "column", "C:E": "column",
		"AA:AC": "column", "": "", "A1:B2": "", "!!": "",
	} {
		if got := dimensionOfSpan(span); got != want {
			t.Errorf("dimensionOfSpan(%q) = %q, want %q", span, got, want)
		}
	}
}

// +cells-unmerge takes one span per call. A --ranges holding exactly one is
// that span; holding several is the prescription it always was.
func TestReconcile_SingletonRanges(t *testing.T) {
	t.Parallel()
	t.Run("one span is the span", func(t *testing.T) {
		t.Parallel()
		calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+cells-unmerge"), []string{
			"--url", testURL, "--sheet-id", testSheetID, "--ranges", `["A1:B2"]`,
		})
		if len(calls) == 0 {
			t.Fatal("a single-span --ranges must resolve to --range")
		}
	})
	t.Run("several spans keep the prescription", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-unmerge"), []string{
			"--url", testURL, "--sheet-id", testSheetID, "--ranges", `["A1:B2","D1:E2"]`, "--dry-run",
		})
		requireValidation(t, err, "takes one span per call")
	})
}

// The OpenAPI's {style:{…}} envelope against a command of flat flags.
func TestReconcile_StyleEnvelopeUnpacks(t *testing.T) {
	t.Parallel()
	t.Run("fields land on their own flags", func(t *testing.T) {
		t.Parallel()
		calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+cells-set-style"), []string{
			"--url", testURL, "--sheet-id", testSheetID, "--range", "A1",
			"--style", `{"font_weight":"bold","font_size":14}`,
		})
		if len(calls) == 0 {
			t.Fatal("the style envelope was refused")
		}
		body := fmt.Sprint(calls...)
		for _, want := range []string{"font_weight", "bold", "font_size"} {
			if !strings.Contains(body, want) {
				t.Errorf("%q missing from the plan: %s", want, body)
			}
		}
	})

	t.Run("a field this command does not have keeps the prescription", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-set-style"), []string{
			"--url", testURL, "--sheet-id", testSheetID, "--range", "A1",
			"--style", `{"nonsense":"x"}`, "--dry-run",
		})
		requireValidation(t, err, "is not a field of this command")
	})

	t.Run("an explicit flag outranks the envelope", func(t *testing.T) {
		t.Parallel()
		calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+cells-set-style"), []string{
			"--url", testURL, "--sheet-id", testSheetID, "--range", "A1",
			"--font-weight", "normal", "--style", `{"font_weight":"bold"}`,
		})
		body := fmt.Sprint(calls...)
		if strings.Contains(body, "bold") {
			t.Errorf("the envelope overwrote an explicit flag: %s", body)
		}
	})
}

// Lark's own borderType vocabulary names which EDGES, not which line.
func TestBorderSidesFromTypeWord(t *testing.T) {
	t.Parallel()
	all, ok := borderSidesFromTypeWord("FULL_BORDER")
	if !ok || len(all) != 4 {
		t.Fatalf("FULL_BORDER = %v ok=%v, want four sides", all, ok)
	}
	top, ok := borderSidesFromTypeWord("top-border")
	if !ok || len(top) != 1 || top["top"] == nil {
		t.Errorf("top-border = %v ok=%v", top, ok)
	}
	// A line style is not a borderType word and must reach the scalar path.
	if _, ok := borderSidesFromTypeWord("solid"); ok {
		t.Error("a line style was read as a borderType word")
	}
	if _, ok := borderSidesFromTypeWord(2); ok {
		t.Error("a width was read as a borderType word")
	}
}
