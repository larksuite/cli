// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/httpmock"
)

// TestReadOutputPath_UnsafePathIsTypedValidation pins the error contract of
// the --output-path save seam: an escaping path must come back as a
// validation error with the path-validation cause preserved, not as
// internal/unknown from the raw FileIO.Save error.
func TestReadOutputPath_UnsafePathIsTypedValidation(t *testing.T) {
	t.Parallel()

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheet_ai/v2/spreadsheets/" + testToken + "/tools/invoke_read",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"output": `{"values":[["x"]]}`},
		},
	}
	_, err := runShortcutWithStubs(t, CellsGet, []string{
		"--url", testURL, "--sheet-id", testSheetID, "--range", "A1",
		"--output-path", "../../outside.json", "--as", "user",
	}, stub)
	ve := requireValidation(t, err, "unsafe output path")
	if ve.Cause == nil || !errors.Is(ve.Cause, fileio.ErrPathValidation) {
		t.Errorf("Cause = %v, want the fileio.ErrPathValidation chain preserved", ve.Cause)
	}
}

// TestReadResultTruncated_AllLevels pins the completeness classifier: a
// truncation marker at ANY level must be seen, or the --output-path receipt
// claims complete:true over a clipped file and an agent analyzes half the
// data believing it has all of it.
func TestReadResultTruncated_AllLevels(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		out  interface{}
		want bool
	}{
		{"top-level truncated", map[string]interface{}{"truncated": true}, true},
		{"top-level has_more", map[string]interface{}{"has_more": true}, true},
		{"ranges entry", map[string]interface{}{"ranges": []interface{}{map[string]interface{}{"truncated": true}}}, true},
		{"sheets entry", map[string]interface{}{"sheets": []interface{}{map[string]interface{}{"truncated": true}}}, true},
		{"nested ranges inside a sheet", map[string]interface{}{
			"sheets": []interface{}{map[string]interface{}{
				"ranges": []interface{}{map[string]interface{}{"truncated": true}},
			}},
		}, true},
		{"clean payload", map[string]interface{}{"sheets": []interface{}{map[string]interface{}{"data": []interface{}{}}}}, false},
		{"non-map payload", []interface{}{1, 2}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := readResultTruncated(tc.out); got != tc.want {
				t.Errorf("readResultTruncated = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestMaxCharsInput_ExplicitZero pins that asking for "no cap of my own" does
// not land on the SMALLEST cap. Omitting max_chars makes the read tool apply
// its own ~50000 fallback, so passing the request straight through would give
// --max-chars 0 a tighter limit than leaving the flag alone — the opposite of
// what it reads like, and silently.
func TestMaxCharsInput_ExplicitZero(t *testing.T) {
	t.Parallel()

	t.Run("resolves to whatever omitting the flag resolves to", func(t *testing.T) {
		t.Parallel()
		// Compared against the omitted call rather than against maxCharsFallback:
		// asserting the constant equals itself would pass even after the flag's
		// declared default moved in flag-defs.json and left the two out of step.
		// The contract is "0 means no cap of my own", i.e. behave as if unset.
		zero := cellsGetToolInput(t, []string{"--max-chars", "0"})
		omitted := cellsGetToolInput(t, nil)
		got, ok := zero["max_chars"]
		if !ok {
			t.Fatalf("max_chars must be sent, or the tool's ~50000 fallback binds: %#v", zero)
		}
		if want := omitted["max_chars"]; got != want {
			t.Errorf("--max-chars 0 sent max_chars=%v, omitting it sent %v; they must agree", got, want)
		}
	})

	t.Run("--output-path still raises it to the offload limit", func(t *testing.T) {
		t.Parallel()
		input := cellsGetToolInput(t, []string{"--max-chars", "0", "--output-path", "./o.json"})
		if got := input["max_chars"]; got != float64(outputPathReadLimit) {
			t.Errorf("max_chars = %v, want %d", got, outputPathReadLimit)
		}
	})

	t.Run("a positive explicit cap still wins over --output-path", func(t *testing.T) {
		t.Parallel()
		input := cellsGetToolInput(t, []string{"--max-chars", "1234", "--output-path", "./o.json"})
		if got := input["max_chars"]; got != float64(1234) {
			t.Errorf("max_chars = %v, want 1234", got)
		}
	})
}

func cellsGetToolInput(t *testing.T, extra []string) map[string]interface{} {
	t.Helper()
	args := append([]string{"--url", testURL, "--sheet-name", "S1", "--range", "A1:B2"}, extra...)
	return decodeToolInput(t, parseDryRunBody(t, CellsGet, args), "get_cell_ranges")
}
