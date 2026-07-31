// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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

// TestEmitReadResult_ReceiptStatesCompleteness drives a real --output-path read
// end to end and checks the stdout receipt against the payload written to disk.
//
// The receipt is the ONLY completeness signal a caller gets on this path — the
// data went to a file, stdout carries just the summary — and the skill docs
// instruct agents to read `complete` before using the file. Nothing was pinning
// it: hard-coding complete:true passed the whole suite, which is exactly the
// failure that makes an agent analyze half a sheet believing it has all of it.
//
// Not parallel: t.Chdir scopes the relative --output-path to a temp dir.
func TestEmitReadResult_ReceiptStatesCompleteness(t *testing.T) {
	cases := []struct {
		name         string
		output       string
		wantComplete bool
	}{
		{
			name:         "clean read reports complete",
			output:       `{"ranges":[{"range":"A1:B2","values":[["x","y"]]}]}`,
			wantComplete: true,
		},
		{
			name:         "per-range truncation flag reports incomplete",
			output:       `{"ranges":[{"range":"A1:B2","truncated":true,"values":[["x","y"]]}]}`,
			wantComplete: false,
		},
		{
			name:         "top-level has_more reports incomplete",
			output:       `{"has_more":true,"ranges":[{"range":"A1:B2","values":[["x","y"]]}]}`,
			wantComplete: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			// os.Chdir + Cleanup rather than t.Chdir: the latter is Go 1.24,
			// and go.mod declares 1.23.0 (CI resolves its toolchain from it).
			orig, err := os.Getwd()
			if err != nil {
				t.Fatalf("getwd: %v", err)
			}
			if err := os.Chdir(dir); err != nil {
				t.Fatalf("chdir: %v", err)
			}
			t.Cleanup(func() { _ = os.Chdir(orig) })
			stub := &httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/sheet_ai/v2/spreadsheets/" + testToken + "/tools/invoke_read",
				Body: map[string]interface{}{
					"code": 0, "msg": "ok",
					"data": map[string]interface{}{"output": tc.output},
				},
			}
			stdout, err := runShortcutWithStubs(t, CellsGet, []string{
				"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:B2",
				"--output-path", "out.json", "--as", "user",
			}, stub)
			if err != nil {
				t.Fatalf("read failed: %v", err)
			}
			receipt := decodeEnvelopeData(t, stdout)
			if got := receipt["complete"]; got != tc.wantComplete {
				t.Errorf("complete = %v, want %v (receipt=%v)", got, tc.wantComplete, receipt)
			}
			if !tc.wantComplete {
				if receipt["truncated"] != true {
					t.Errorf("an incomplete receipt must also set truncated:true, got %v", receipt)
				}
				if w, _ := receipt["truncation_warning"].(string); w == "" {
					t.Error("an incomplete receipt must carry a truncation_warning telling the caller what to do")
				}
			} else if _, has := receipt["truncated"]; has {
				t.Errorf("a complete receipt must not carry a truncation marker, got %v", receipt)
			}

			// The file must actually hold the payload, not the receipt.
			written, readErr := os.ReadFile(filepath.Join(dir, "out.json"))
			if readErr != nil {
				t.Fatalf("output file not written: %v", readErr)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(written, &payload); err != nil {
				t.Fatalf("output file is not JSON: %v", err)
			}
			if _, has := payload["ranges"]; !has {
				t.Errorf("file should hold the data payload, got %s", written)
			}
			if n, _ := receipt["bytes_written"].(float64); int(n) != len(written) {
				t.Errorf("bytes_written = %v, file is %d bytes", receipt["bytes_written"], len(written))
			}
		})
	}
}
