// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"testing"

	"github.com/larksuite/cli/errs"
)

// workbookStructureOutput is one get_workbook_structure response carrying two
// sub-sheets plus the sibling fields +sheet-list drops.
const workbookStructureOutput = `{"revision":60,"sheets":[` +
	`{"sheet_id":"sh1","title":"Sheet1","row_count":1000,"column_count":26,"index":0,"hidden":false},` +
	`{"sheet_id":"sh2","title":"Q1","row_count":200,"column_count":8,"index":1,"hidden":true}]}`

func TestSheetListProjectSheets(t *testing.T) {
	t.Parallel()

	t.Run("extracts the sheets array from a workbook-structure object", func(t *testing.T) {
		out := map[string]interface{}{
			"revision": float64(60),
			"sheets": []interface{}{
				map[string]interface{}{"sheet_id": "sh1"},
				map[string]interface{}{"sheet_id": "sh2"},
			},
		}
		got, err := projectSheets(out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		sheets, ok := got.([]interface{})
		if !ok || len(sheets) != 2 {
			t.Fatalf("sheets = %#v, want a 2-element array", got)
		}
	})

	t.Run("errors when sheets is absent", func(t *testing.T) {
		_, err := projectSheets(map[string]interface{}{"revision": float64(60)})
		requireProblem(t, err, errs.CategoryInternal, errs.SubtypeInvalidResponse, "sheets array")
	})

	t.Run("errors when sheets is not an array", func(t *testing.T) {
		_, err := projectSheets(map[string]interface{}{"sheets": "Sheet1"})
		requireProblem(t, err, errs.CategoryInternal, errs.SubtypeInvalidResponse, "sheets array")
	})

	t.Run("errors on a non-object output", func(t *testing.T) {
		_, err := projectSheets("not-an-object")
		requireProblem(t, err, errs.CategoryInternal, errs.SubtypeInvalidResponse, "non-object")
	})
}

// TestExecute_SheetList_EmitsSheetsArray locks the output contract that makes
// +sheet-list worth having next to +workbook-info: envelope data is the bare
// sheets array, with each entry passed through exactly as the tool returned it.
func TestExecute_SheetList_EmitsSheetsArray(t *testing.T) {
	t.Parallel()
	stub := toolOutputStub(testToken, "read", workbookStructureOutput)
	out, err := runShortcutWithStubs(t, SheetList, []string{"--url", testURL}, stub)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}

	var envelope struct {
		OK   bool          `json:"ok"`
		Data []interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("failed to decode envelope: %v\nraw=%s", err, out)
	}
	if !envelope.OK {
		t.Fatalf("envelope.ok=false: %s", out)
	}
	if len(envelope.Data) != 2 {
		t.Fatalf("data len = %d, want 2; out=%s", len(envelope.Data), out)
	}
	first, _ := envelope.Data[0].(map[string]interface{})
	if first["sheet_id"] != "sh1" || first["title"] != "Sheet1" ||
		first["row_count"] != float64(1000) || first["index"] != float64(0) {
		t.Errorf("first sheet lost fields on the way through: %#v", first)
	}
}

// TestSheetList_MatchesWorkbookInfoEntries pins the "same per-sheet shape"
// promise: the entries +sheet-list emits are the ones +workbook-info nests
// under `sheets`, not a re-projected subset.
func TestSheetList_MatchesWorkbookInfoEntries(t *testing.T) {
	t.Parallel()

	listOut, err := runShortcutWithStubs(t, SheetList, []string{"--url", testURL},
		toolOutputStub(testToken, "read", workbookStructureOutput))
	if err != nil {
		t.Fatalf("+sheet-list failed: %v\nout=%s", err, listOut)
	}
	infoOut, err := runShortcutWithStubs(t, WorkbookInfo, []string{"--url", testURL},
		toolOutputStub(testToken, "read", workbookStructureOutput))
	if err != nil {
		t.Fatalf("+workbook-info failed: %v\nout=%s", err, infoOut)
	}

	var list struct {
		Data []interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(listOut), &list); err != nil {
		t.Fatalf("decode +sheet-list envelope: %v\nraw=%s", err, listOut)
	}
	info := decodeEnvelopeData(t, infoOut)
	infoSheets, _ := info["sheets"].([]interface{})

	got, _ := json.Marshal(list.Data)
	want, _ := json.Marshal(infoSheets)
	if string(got) != string(want) {
		t.Errorf("+sheet-list entries = %s, want +workbook-info's sheets = %s", got, want)
	}
}

// TestSheetList_DryRunInvokesWorkbookStructure asserts the shortcut reuses the
// existing read tool rather than reaching for a dedicated backend endpoint.
func TestSheetList_DryRunInvokesWorkbookStructure(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, SheetList, []string{"--url", testURL})
	input := decodeToolInput(t, body, "get_workbook_structure")
	if input["excel_id"] != testToken {
		t.Errorf("excel_id = %v, want %s", input["excel_id"], testToken)
	}
}

// TestSheetList_StaysOffTheHelpSurface guards the half of the design Go owns:
// +sheet-list answers callers who already typed it and is advertised to nobody.
// (The skill-doc half lives in sheet-skill-spec's doc_hidden_shortcuts.)
func TestSheetList_StaysOffTheHelpSurface(t *testing.T) {
	t.Parallel()
	if !shortcutFromRegistry(t, "+sheet-list").Hidden {
		t.Error("+sheet-list must stay hidden: it is a fallback for a guessed name, not a second documented way to read workbook structure")
	}
}
