// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/vfs/localfileio"
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
	if err := os.WriteFile("data.xlsx", []byte("PK\x03\x04fake-xlsx"), 0o644); err != nil {
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

// TestCorrectedWorkbookExtension covers the content-sniffing that corrects (or
// rejects) a mislabeled Excel file before its extension reaches the backend.
func TestCorrectedWorkbookExtension(t *testing.T) {
	ooxml := []byte("PK\x03\x04rest")                      // zip -> .xlsx
	ole2 := []byte("\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1rest") // compound doc -> .xls

	tests := []struct {
		name       string
		fileName   string
		content    []byte
		wantExt    string // expected override ("" == leave the declared extension)
		wantErrSub string // non-empty == expect a validation error containing this
	}{
		{name: "xlsx content mislabeled as xls corrects to xlsx", fileName: "book.xls", content: ooxml, wantExt: "xlsx"},
		{name: "xls content mislabeled as xlsx corrects to xls", fileName: "book.xlsx", content: ole2, wantExt: "xls"},
		{name: "genuine xlsx left untouched", fileName: "book.xlsx", content: ooxml, wantExt: ""},
		{name: "genuine xls left untouched", fileName: "book.xls", content: ole2, wantExt: ""},
		{name: "unrecognized content on .xls rejected", fileName: "book.xls", content: []byte("<html><table></table></html>"), wantErrSub: "neither an OOXML"},
		{name: "non-excel extension never sniffed", fileName: "notes.csv", content: []byte("a,b\n1,2\n"), wantExt: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chdirTemp(t)
			if err := os.WriteFile(tt.fileName, tt.content, 0o644); err != nil {
				t.Fatalf("write file: %v", err)
			}

			ext, err := correctedWorkbookExtension(&localfileio.LocalFileIO{}, "./"+tt.fileName)
			if tt.wantErrSub != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Fatalf("error = %v, want substring %q", err, tt.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ext != tt.wantExt {
				t.Fatalf("override ext = %q, want %q", ext, tt.wantExt)
			}
		})
	}
}

// TestWorkbookImport_DryRunCorrectsMislabeledXls verifies an .xls file whose
// bytes are actually OOXML is imported with file_extension=xlsx end to end.
func TestWorkbookImport_DryRunCorrectsMislabeledXls(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("book.xls", []byte("PK\x03\x04zip-body"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	calls := parseDryRunAPI(t, WorkbookImport, []string{"--file", "./book.xls"})

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
	if createBody["file_extension"] != "xlsx" {
		t.Errorf("file_extension = %v, want xlsx (mislabeled .xls must be corrected)", createBody["file_extension"])
	}
}

// TestWorkbookImport_RejectsUnrecognizedExcel ensures a file whose .xls/.xlsx
// name matches neither Excel container is rejected locally with a prescriptive
// error rather than deferring to the backend's opaque failure.
func TestWorkbookImport_RejectsUnrecognizedExcel(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("bogus.xls", []byte("<html>not excel</html>"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, _, err := runShortcutCapturingErr(t, WorkbookImport, []string{"--file", "./bogus.xls", "--dry-run"})
	requireValidation(t, err, "neither an OOXML")
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
