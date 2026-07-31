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
