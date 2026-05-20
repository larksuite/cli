// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

// TestWorkbookShortcuts_DryRun covers all 9 lark_sheet_workbook shortcuts
// (WorkbookInfo + 8 sheet-* variants) by asserting the One-OpenAPI body
// the dry-run renders. Together they exercise every dispatch arm of
// modify_workbook_structure plus the read tool.
func TestWorkbookShortcuts_DryRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sc        common.Shortcut
		args      []string
		toolName  string
		wantInput map[string]interface{}
	}{
		{
			name:     "+workbook-info read",
			sc:       WorkbookInfo,
			args:     []string{"--url", testURL},
			toolName: "get_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id": testToken,
			},
		},
		{
			name:     "+sheet-create with all options",
			sc:       SheetCreate,
			args:     []string{"--url", testURL, "--title", "Q1", "--index", "1", "--row-count", "300", "--col-count", "10"},
			toolName: "modify_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id":     testToken,
				"operation":    "create",
				"sheet_name":   "Q1",
				"target_index": float64(1),
				"rows":         float64(300),
				"columns":      float64(10),
			},
		},
		{
			name:     "+sheet-delete by id",
			sc:       SheetDelete,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID},
			toolName: "modify_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "delete",
				"sheet_id":  testSheetID,
			},
		},
		{
			name:     "+sheet-rename by name",
			sc:       SheetRename,
			args:     []string{"--url", testURL, "--sheet-name", "汇总", "--title", "Q1 汇总"},
			toolName: "modify_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id":   testToken,
				"operation":  "rename",
				"sheet_name": "汇总",
				"new_name":   "Q1 汇总",
			},
		},
		{
			name:     "+sheet-copy without explicit title",
			sc:       SheetCopy,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID},
			toolName: "modify_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "duplicate",
				"sheet_id":  testSheetID,
			},
		},
		{
			name:     "+sheet-copy with new title and index",
			sc:       SheetCopy,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--title", "副本", "--index", "0"},
			toolName: "modify_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id":     testToken,
				"operation":    "duplicate",
				"sheet_id":     testSheetID,
				"new_name":     "副本",
				"target_index": float64(0),
			},
		},
		{
			name:     "+sheet-hide",
			sc:       SheetHide,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID},
			toolName: "modify_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "hide",
				"sheet_id":  testSheetID,
			},
		},
		{
			name:     "+sheet-unhide",
			sc:       SheetUnhide,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID},
			toolName: "modify_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "unhide",
				"sheet_id":  testSheetID,
			},
		},
		{
			name:     "+sheet-set-tab-color hex",
			sc:       SheetSetTabColor,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--color", "#FF0000"},
			toolName: "modify_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "set_tab_color",
				"sheet_id":  testSheetID,
				"tab_color": "#FF0000",
			},
		},
		{
			name:     "+sheet-set-tab-color empty clears",
			sc:       SheetSetTabColor,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--color", ""},
			toolName: "modify_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "set_tab_color",
				"sheet_id":  testSheetID,
				"tab_color": "",
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := parseDryRunBody(t, tt.sc, tt.args)
			got := decodeToolInput(t, body, tt.toolName)
			assertInputEquals(t, got, tt.wantInput)
		})
	}
}

// TestSheetMove_DryRunResolvePlaceholders verifies the move shortcut emits
// <resolve> placeholders for fields it would otherwise have to look up
// at execute time. DryRun must stay network-free.
func TestSheetMove_DryRunResolvePlaceholders(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		args          []string
		wantSheetID   string
		wantSourceIdx interface{}
	}{
		{
			name:          "id only, no source-index → both literal + placeholder",
			args:          []string{"--url", testURL, "--sheet-id", testSheetID, "--index", "0"},
			wantSheetID:   testSheetID,
			wantSourceIdx: "<resolve>",
		},
		{
			name:          "name only → sheet_id placeholder + source_index placeholder",
			args:          []string{"--url", testURL, "--sheet-name", "汇总", "--index", "0"},
			wantSheetID:   "<resolve:汇总>",
			wantSourceIdx: "<resolve>",
		},
		{
			name:          "id + source-index → both literal",
			args:          []string{"--url", testURL, "--sheet-id", testSheetID, "--index", "0", "--source-index", "5"},
			wantSheetID:   testSheetID,
			wantSourceIdx: float64(5),
		},
	}
	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := parseDryRunBody(t, SheetMove, tt.args)
			input := decodeToolInput(t, body, "modify_workbook_structure")
			if got := input["sheet_id"]; got != tt.wantSheetID {
				t.Errorf("sheet_id = %#v, want %#v", got, tt.wantSheetID)
			}
			if got := input["source_index"]; got != tt.wantSourceIdx {
				t.Errorf("source_index = %#v, want %#v", got, tt.wantSourceIdx)
			}
			if got := input["target_index"]; got != float64(0) {
				t.Errorf("target_index = %#v, want 0", got)
			}
		})
	}
}

// TestSheetDelete_HighRiskWriteRequiresYes verifies the framework gate on
// high-risk-write — exit code 10 (confirmation_required) without --yes.
func TestSheetDelete_HighRiskWriteRequiresYes(t *testing.T) {
	t.Parallel()
	stdout, stderr, err := runShortcutCapturingErr(t, SheetDelete, []string{"--url", testURL, "--sheet-id", testSheetID})
	if err == nil {
		t.Fatalf("expected confirmation_required error; got nil. stdout=%s stderr=%s", stdout, stderr)
	}
	combined := stdout + stderr + err.Error()
	if !strings.Contains(combined, "confirmation_required") && !strings.Contains(combined, "requires confirmation") {
		t.Errorf("expected confirmation envelope; got=%s|%s|%v", stdout, stderr, err)
	}
}

// TestWorkbook_Validation covers a few critical validation paths shared
// across the package's helpers (XOR token, XOR sheet selector, required
// flags).
func TestWorkbook_Validation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		sc      common.Shortcut
		args    []string
		wantMsg string
	}{
		{
			name:    "+workbook-info needs --url or --spreadsheet-token",
			sc:      WorkbookInfo,
			args:    []string{},
			wantMsg: "at least one of --url or --spreadsheet-token",
		},
		{
			name:    "+workbook-info rejects both url and token",
			sc:      WorkbookInfo,
			args:    []string{"--url", testURL, "--spreadsheet-token", testToken},
			wantMsg: "mutually exclusive",
		},
		{
			name:    "+sheet-delete needs sheet selector",
			sc:      SheetDelete,
			args:    []string{"--url", testURL},
			wantMsg: "at least one of --sheet-id or --sheet-name",
		},
		{
			name:    "+sheet-create requires --title",
			sc:      SheetCreate,
			args:    []string{"--url", testURL},
			wantMsg: "required flag(s) \"title\" not set",
		},
		{
			name:    "+sheet-create row-count over cap",
			sc:      SheetCreate,
			args:    []string{"--url", testURL, "--title", "X", "--row-count", "999999"},
			wantMsg: "--row-count must be between",
		},
	}
	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stdout, stderr, err := runShortcutCapturingErr(t, tt.sc, append(tt.args, "--dry-run"))
			if err == nil {
				t.Fatalf("expected validation error; got nil. stdout=%s stderr=%s", stdout, stderr)
			}
			combined := stdout + stderr + err.Error()
			if !strings.Contains(combined, tt.wantMsg) {
				t.Errorf("error message missing %q; got=%s", tt.wantMsg, combined)
			}
		})
	}
}

// ─── +workbook-create / +workbook-export (legacy OAPI) ───────────────

// TestWorkbookCreate_DryRun verifies the two-step plan (create
// spreadsheet + optional set_cell_range follow-up) is rendered.
func TestWorkbookCreate_DryRun(t *testing.T) {
	t.Parallel()

	t.Run("minimal title only", func(t *testing.T) {
		t.Parallel()
		calls := parseDryRunAPI(t, WorkbookCreate, []string{"--title", "MySheet"})
		if len(calls) != 1 {
			t.Fatalf("api calls = %d, want 1 (no headers/data)", len(calls))
		}
		c := calls[0].(map[string]interface{})
		if c["url"] != "/open-apis/sheets/v3/spreadsheets" {
			t.Errorf("url = %v, want /open-apis/sheets/v3/spreadsheets", c["url"])
		}
		body, _ := c["body"].(map[string]interface{})
		if body["title"] != "MySheet" {
			t.Errorf("body.title = %v, want MySheet", body["title"])
		}
	})

	t.Run("with headers and data → 2-step plan", func(t *testing.T) {
		t.Parallel()
		calls := parseDryRunAPI(t, WorkbookCreate, []string{
			"--title", "Sales",
			"--headers", `["Name","Score"]`,
			"--values", `[["alice",95],["bob",88]]`,
		})
		if len(calls) != 2 {
			t.Fatalf("api calls = %d, want 2 (create + fill)", len(calls))
		}
		fill := calls[1].(map[string]interface{})
		if !strings.Contains(fill["url"].(string), "/sheet_ai/v2/spreadsheets/") {
			t.Errorf("fill url = %v, want sheet_ai/v2 path", fill["url"])
		}
		body, _ := fill["body"].(map[string]interface{})
		input := decodeToolInput(t, body, "set_cell_range")
		if input["range"] != "A1:B3" {
			t.Errorf("fill range = %v, want A1:B3 (1 header + 2 data rows × 2 cols)", input["range"])
		}
	})
}

// TestWorkbookCreate_DataValidation rejects bad JSON shape.
func TestWorkbookCreate_DataValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"headers not array", []string{"--title", "X", "--headers", `"abc"`}, "must be a JSON array"},
		{"values not 2D", []string{"--title", "X", "--values", `["a","b"]`}, "must be an array"},
	}
	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stdout, stderr, err := runShortcutCapturingErr(t, WorkbookCreate, append(tt.args, "--dry-run"))
			if err == nil || !strings.Contains(stdout+stderr+err.Error(), tt.want) {
				t.Errorf("expected %q; got=%s|%s|%v", tt.want, stdout, stderr, err)
			}
		})
	}
}

// TestWorkbookExport_DryRun checks the 2-or-3 step plan depending on
// --output-path. The order should be: POST → GET (poll) → optional GET
// (download).
func TestWorkbookExport_DryRun(t *testing.T) {
	t.Parallel()

	t.Run("xlsx without --output-path → 2 steps", func(t *testing.T) {
		t.Parallel()
		calls := parseDryRunAPI(t, WorkbookExport, []string{"--url", testURL, "--file-extension", "xlsx"})
		if len(calls) != 2 {
			t.Fatalf("api calls = %d, want 2 (create + poll)", len(calls))
		}
		create := calls[0].(map[string]interface{})
		if create["url"] != "/open-apis/drive/v1/export_tasks" {
			t.Errorf("first url = %v", create["url"])
		}
		body, _ := create["body"].(map[string]interface{})
		if body["type"] != "sheet" || body["file_extension"] != "xlsx" || body["token"] != testToken {
			t.Errorf("create body = %#v", body)
		}
	})

	t.Run("csv → 3 steps, with sub_id", func(t *testing.T) {
		t.Parallel()
		calls := parseDryRunAPI(t, WorkbookExport, []string{
			"--url", testURL, "--file-extension", "csv", "--sheet-id", "sh1",
			"--output-path", "/tmp/out.csv",
		})
		if len(calls) != 3 {
			t.Fatalf("api calls = %d, want 3", len(calls))
		}
		body, _ := calls[0].(map[string]interface{})["body"].(map[string]interface{})
		if body["sub_id"] != "sh1" {
			t.Errorf("csv export missing sub_id: %#v", body)
		}
		dl := calls[2].(map[string]interface{})
		if !strings.Contains(dl["url"].(string), "/export_tasks/file/") {
			t.Errorf("download url = %v", dl["url"])
		}
	})

	t.Run("csv requires --sheet-id", func(t *testing.T) {
		t.Parallel()
		stdout, stderr, err := runShortcutCapturingErr(t, WorkbookExport, []string{
			"--url", testURL, "--file-extension", "csv", "--dry-run",
		})
		if err == nil || !strings.Contains(stdout+stderr+err.Error(), "--sheet-id is required") {
			t.Errorf("expected sheet-id guard; got=%s|%s|%v", stdout, stderr, err)
		}
	})
}

// assertInputEquals compares the decoded tool input map against the wanted
// fields. Extra fields in `got` are allowed (defaults, optional fields);
// every key in `want` must match exactly.
func assertInputEquals(t *testing.T, got, want map[string]interface{}) {
	t.Helper()
	for k, wv := range want {
		gv, ok := got[k]
		if !ok {
			t.Errorf("missing input key %q (got=%#v)", k, got)
			continue
		}
		if !deepEqualJSON(gv, wv) {
			t.Errorf("input[%q] = %#v, want %#v", k, gv, wv)
		}
	}
}

// deepEqualJSON compares JSON-shaped values (post-Unmarshal) — handles
// the fact that numbers come back as float64 and maps as map[string]interface{}.
func deepEqualJSON(a, b interface{}) bool {
	switch av := a.(type) {
	case map[string]interface{}:
		bv, ok := b.(map[string]interface{})
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if !deepEqualJSON(v, bv[k]) {
				return false
			}
		}
		return true
	case []interface{}:
		bv, ok := b.([]interface{})
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !deepEqualJSON(av[i], bv[i]) {
				return false
			}
		}
		return true
	}
	return a == b
}
