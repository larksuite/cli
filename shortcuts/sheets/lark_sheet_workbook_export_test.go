// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/vfs/localfileio"
	"github.com/larksuite/cli/shortcuts/drive"
)

// TestApplyWorkbookOutputPath pins the --output-path → OutputDir/FileName
// contract the dry-run plan cannot show (it stays I/O-free and carries the
// unsplit path): empty = no download, an existing directory = download under
// the server-provided name, anything else = split into dir + base name.
func TestApplyWorkbookOutputPath(t *testing.T) {
	t.Parallel()
	fio := &localfileio.LocalFileIO{}

	p := drive.ExportParams{}
	applyWorkbookOutputPath(&p, fio, "")
	if p.OutputDir != "" || p.FileName != "" {
		t.Errorf("empty path must mean no download, got dir=%q name=%q", p.OutputDir, p.FileName)
	}

	p = drive.ExportParams{}
	applyWorkbookOutputPath(&p, fio, "./out.xlsx")
	if p.OutputDir != "." || p.FileName != "out.xlsx" {
		t.Errorf("file path must split into dir+name, got dir=%q name=%q", p.OutputDir, p.FileName)
	}

	p = drive.ExportParams{}
	applyWorkbookOutputPath(&p, fio, ".")
	if p.OutputDir != "." || p.FileName != "" {
		t.Errorf("existing dir must keep the server-provided name, got dir=%q name=%q", p.OutputDir, p.FileName)
	}
}

// TestWorkbookExport_ExecuteExportOnly covers the no-download path: without
// --output-path, +workbook-export delegates to the shared drive export core
// with OutputDir="" so it creates + polls the export task and returns the ready
// file token without writing a local file (downloaded=false).
func TestWorkbookExport_ExecuteExportOnly(t *testing.T) {
	stubs := []*httpmock.Stub{
		{
			Method: "POST",
			URL:    "/open-apis/drive/v1/export_tasks",
			Body: map[string]interface{}{
				"code": 0, "msg": "ok",
				"data": map[string]interface{}{"ticket": "tk_export"},
			},
		},
		{
			Method: "GET",
			URL:    "/open-apis/drive/v1/export_tasks/tk_export",
			Body: map[string]interface{}{
				"code": 0, "msg": "ok",
				"data": map[string]interface{}{"result": map[string]interface{}{
					"job_status": float64(0),
					"file_token": "ftk_xlsx",
					"file_name":  "report.xlsx",
					"file_size":  float64(2048),
				}},
			},
		},
	}

	out, err := runShortcutWithStubs(t, WorkbookExport, []string{
		"--url", testURL, "--file-extension", "xlsx", "--as", "user",
	}, stubs...)
	if err != nil {
		t.Fatalf("export-only execute failed: %v\n%s", err, out)
	}

	idx := strings.Index(out, "{")
	if idx < 0 {
		t.Fatalf("no JSON envelope:\n%s", out)
	}
	var env struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out[idx:]), &env); err != nil {
		t.Fatalf("decode envelope: %v\nraw=%s", err, out)
	}
	if env.Data["ready"] != true {
		t.Errorf("ready = %v, want true", env.Data["ready"])
	}
	if env.Data["downloaded"] != false {
		t.Errorf("downloaded = %v, want false (no --output-path)", env.Data["downloaded"])
	}
	if env.Data["file_token"] != "ftk_xlsx" {
		t.Errorf("file_token = %v, want ftk_xlsx", env.Data["file_token"])
	}
	if env.Data["doc_type"] != "sheet" {
		t.Errorf("doc_type = %v, want sheet", env.Data["doc_type"])
	}
}

func TestWorkbookExport_CreateRateLimitKeepsCallerRecovery(t *testing.T) {
	stubs := []*httpmock.Stub{
		{
			Method: "POST",
			URL:    "/open-apis/drive/v1/export_tasks",
			Status: http.StatusBadRequest,
			Body: map[string]interface{}{
				"code": 9499,
				"msg":  "too many request",
			},
		},
	}

	_, err := runShortcutWithStubs(t, WorkbookExport, []string{
		"--url", testURL, "--file-extension", "xlsx", "--as", "user",
	}, stubs...)
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed rate-limit error, got %T: %v", err, err)
	}
	if problem.Category != errs.CategoryAPI || problem.Subtype != errs.SubtypeRateLimit || problem.Code != 9499 || !problem.Retryable {
		t.Fatalf("problem = %+v, want api/rate_limit code 9499 retryable", problem)
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T, want *errs.APIError", err)
	}
	if !strings.Contains(problem.Hint, "rerun the original command with the same arguments") {
		t.Fatalf("hint should preserve the workbook-export caller: %q", problem.Hint)
	}
	if strings.Contains(problem.Hint, "drive +export") {
		t.Fatalf("workbook-export hint must not redirect users to drive +export: %q", problem.Hint)
	}
}
