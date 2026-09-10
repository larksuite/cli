// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

// toolStub answers the one tool the body names, so several calls to the same
// invoke_read URL can be told apart.
func toolStub(toolName, output string) *httpmock.Stub {
	return &httpmock.Stub{
		Method:     "POST",
		URL:        "/tools/invoke_",
		BodyFilter: func(b []byte) bool { return bytes.Contains(b, []byte(`"`+toolName+`"`)) },
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"output": output},
		},
	}
}

// structureStub answers get_workbook_structure with the named sub-sheets.
func structureStub(names ...string) *httpmock.Stub {
	sheets := make([]map[string]interface{}, 0, len(names))
	for i, n := range names {
		sheets = append(sheets, map[string]interface{}{"sheet_name": n, "sheet_id": "id" + n, "index": i})
	}
	out, _ := json.Marshal(map[string]interface{}{"sheets": sheets})
	return toolStub("get_workbook_structure", string(out))
}

// TestSheetSelector_ResolvedFromWorkbook pins where the requirement to name a
// sheet is settled. 09-04..07 attributed 11115 rejections to "specify at least
// one", every one of them recoverable on the next call: what the caller was
// missing was which name to pass, and the workbook knows.
func TestSheetSelector_ResolvedFromWorkbook(t *testing.T) {
	t.Parallel()

	t.Run("a workbook with one sheet needs no selector", func(t *testing.T) {
		t.Parallel()
		stdout, err := runShortcutWithStubs(t, CellsGet,
			[]string{"--url", testURL, "--range", "A1:B2"},
			structureStub("Sheet1"),
			toolStub("get_cell_ranges", `{"ranges":[{"actual_range":"A1:B2","cells":[]}]}`),
		)
		if err != nil {
			t.Fatalf("the only sheet is the one meant, got: %v", err)
		}
		if !strings.Contains(stdout, "actual_range") {
			t.Errorf("the read should have gone through, got %q", stdout)
		}
	})

	t.Run("the resolved name reaches the request", func(t *testing.T) {
		t.Parallel()
		var sent []byte
		captured := &httpmock.Stub{
			Method: "POST",
			URL:    "/tools/invoke_",
			BodyFilter: func(b []byte) bool {
				if !bytes.Contains(b, []byte(`"get_cell_ranges"`)) {
					return false
				}
				sent = append([]byte(nil), b...)
				return true
			},
			Body: map[string]interface{}{
				"code": 0, "msg": "ok",
				"data": map[string]interface{}{"output": `{"ranges":[]}`},
			},
		}
		if _, err := runShortcutWithStubs(t, CellsGet,
			[]string{"--url", testURL, "--range", "A1:B2"},
			structureStub("只有一个"), captured,
		); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Contains(sent, []byte("只有一个")) {
			t.Errorf("the resolved sheet should travel in the request, got %s", sent)
		}
	})

	t.Run("a workbook with several sheets names them", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErrWithStubs(t, CellsGet,
			[]string{"--url", testURL, "--range", "A1:B2"},
			structureStub("数据", "汇总"),
		)
		ve := requireValidation(t, err, missingSheetSelectorMessage)
		for _, want := range []string{"数据", "汇总"} {
			if !strings.Contains(ve.Hint, want) {
				t.Errorf("hint should list the sheets, got %q", ve.Hint)
			}
		}
	})

	t.Run("--dry-run keeps the requirement", func(t *testing.T) {
		t.Parallel()
		// A preview sends nothing, so it cannot resolve the sheet either, and
		// printing a request whose selector the CLI would have filled in would
		// show one it never sends.
		_, _, err := runShortcutCapturingErr(t, CellsGet, []string{
			"--url", testURL, "--range", "A1:B2", "--dry-run",
		})
		requireValidation(t, err, missingSheetSelectorMessage)
	})

	t.Run("a write resolves the same way", func(t *testing.T) {
		t.Parallel()
		if _, err := runShortcutWithStubs(t, CellsSet,
			[]string{"--url", testURL, "--range", "A1", "--cells", `[[{"value":"x"}]]`},
			structureStub("Sheet1"),
			toolStub("set_cell_range", `{"updated_cells_count":1}`),
		); err != nil {
			t.Fatalf("a write should resolve its sheet too, got: %v", err)
		}
	})
}
