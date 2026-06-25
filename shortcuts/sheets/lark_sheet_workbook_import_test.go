// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	_ "github.com/larksuite/cli/internal/vfs/localfileio"
)

// chdirTemp switches into a fresh temp dir for the duration of the test and
// restores the original cwd afterwards. +workbook-import is the first sheets
// shortcut that stat()s a real local file, so these tests need a working dir.
func chdirTemp(t *testing.T) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

// TestWorkbookImport_DryRunPinsSheetType verifies the shortcut delegates to the
// shared drive import core and hard-codes the import target type to "sheet".
func TestWorkbookImport_DryRunPinsSheetType(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("data.xlsx", []byte("fake-xlsx"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	calls := parseDryRunAPI(t, WorkbookImport, []string{"--file", "./data.xlsx"})

	var createBody map[string]interface{}
	for _, c := range calls {
		cm, _ := c.(map[string]interface{})
		if u, _ := cm["url"].(string); u == "/open-apis/drive/v1/import_tasks" {
			createBody, _ = cm["body"].(map[string]interface{})
		}
	}
	if createBody == nil {
		t.Fatalf("no import_tasks create call in dry-run: %#v", calls)
	}
	if createBody["type"] != "sheet" {
		t.Errorf("import type = %v, want sheet (must be pinned regardless of file)", createBody["type"])
	}
	if createBody["file_extension"] != "xlsx" {
		t.Errorf("file_extension = %v, want xlsx", createBody["file_extension"])
	}
}

// TestWorkbookImport_RejectsNonSheetFile ensures a file that cannot become a
// spreadsheet (e.g. .docx) is rejected up front by the pinned-sheet validation.
func TestWorkbookImport_RejectsNonSheetFile(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("notes.docx", []byte("fake-docx"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Validate runs before DryRun, so the pinned-sheet check rejects .docx up
	// front and the error surfaces through the normal envelope/err path.
	_, _, err := runShortcutCapturingErr(t, WorkbookImport, []string{"--file", "./notes.docx", "--dry-run"})
	requireValidation(t, err, "can only be imported")
}

// TestWorkbookImport_ExecuteCreatesSheet runs the full upload → create → poll
// flow against stubs and asserts the resulting URL is a /sheets/ link.
func TestWorkbookImport_ExecuteCreatesSheet(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("data.csv", []byte("a,b\n1,2\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	stubs := []*httpmock.Stub{
		{
			Method: "POST",
			URL:    "/open-apis/drive/v1/medias/upload_all",
			Body: map[string]interface{}{
				"code": 0, "msg": "ok",
				"data": map[string]interface{}{"file_token": "file_import_media"},
			},
		},
		{
			Method: "POST",
			URL:    "/open-apis/drive/v1/import_tasks",
			Body: map[string]interface{}{
				"code": 0, "msg": "ok",
				"data": map[string]interface{}{"ticket": "tk_sheet"},
			},
		},
		{
			Method: "GET",
			URL:    "/open-apis/drive/v1/import_tasks/tk_sheet",
			Body: map[string]interface{}{
				"code": 0, "msg": "ok",
				"data": map[string]interface{}{"result": map[string]interface{}{
					"token":      "shtcn_imported",
					"type":       "sheet",
					"job_status": float64(0),
				}},
			},
		},
	}

	out, err := runShortcutWithStubs(t, WorkbookImport, []string{"--file", "./data.csv", "--as", "user"}, stubs...)
	if err != nil {
		t.Fatalf("import execute failed: %v\n%s", err, out)
	}

	idx := strings.Index(out, "{")
	if idx < 0 {
		t.Fatalf("execute output has no JSON envelope:\n%s", out)
	}
	var env struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out[idx:]), &env); err != nil {
		t.Fatalf("decode envelope: %v\nraw=%s", err, out)
	}
	if url, _ := env.Data["url"].(string); !strings.Contains(url, "/sheets/") {
		t.Errorf("imported url = %q, want a /sheets/ link", url)
	}
	if tok, _ := env.Data["token"].(string); tok != "shtcn_imported" {
		t.Errorf("token = %q, want shtcn_imported", tok)
	}
}
