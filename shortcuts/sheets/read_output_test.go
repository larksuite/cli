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
