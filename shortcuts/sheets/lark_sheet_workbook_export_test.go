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

// TestWorkbookExport_LocalOfficeTokenRejected pins the up-front refusal for a
// locally opened Office file: drive export can never serve those tokens, so the
// command must fail with a typed failed_precondition telling the caller the
// file is already local — before any export_tasks round trip (no stubs are
// registered, so any HTTP call would fail the run with a different error).
func TestWorkbookExport_LocalOfficeTokenRejected(t *testing.T) {
	// The two token classes are both unexportable, but they imply different
	// recovery: a synthetic prefix means the caller already holds the file on
	// disk, while an OFL0X token names a file stored in Lark that the caller
	// may have never downloaded. Telling the second class to "use the local
	// file" hands them an action they cannot take.
	cases := []struct {
		name        string
		token       string
		wantMessage string
		wantHint    []string
		denyHint    string
	}{
		{
			name:        "local_office_ prefix is a file on the caller's disk",
			token:       "local_office_" + strings.Repeat("a", 12),
			wantMessage: "locally opened Office file",
			wantHint:    []string{"already a file on your own disk", "no export needed"},
		},
		{
			name:        "fake_office_ prefix is the same class",
			token:       "fake_office_" + strings.Repeat("b", 12),
			wantMessage: "locally opened Office file",
			wantHint:    []string{"already a file on your own disk"},
		},
		{
			name:        "interleaved OFL0X token is stored in Lark",
			token:       "aaaaOaaaaFaaaaLaaaa0aaaaXaaa",
			wantMessage: "Office file stored in Lark",
			wantHint:    []string{"drive +download --file-token aaaaOaaaaFaaaaLaaaa0aaaaXaaa", "+workbook-import"},
			denyHint:    "on your own disk",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runShortcutWithStubs(t, WorkbookExport, []string{
				"--spreadsheet-token", tc.token, "--as", "user",
			})
			problem := requireProblem(t, err, errs.CategoryValidation, errs.SubtypeFailedPrecondition, tc.wantMessage)
			for _, want := range tc.wantHint {
				if !strings.Contains(problem.Hint, want) {
					t.Errorf("hint should contain %q, got: %q", want, problem.Hint)
				}
			}
			if tc.denyHint != "" && strings.Contains(problem.Hint, tc.denyHint) {
				t.Errorf("hint must not assume a local copy exists (%q): %q", tc.denyHint, problem.Hint)
			}
		})
	}
}

// TestWorkbookExport_LocalOfficeTokenRejectedInDryRun keeps --dry-run honest:
// the guard lives in Validate, which runs before the dry-run preview, so a
// local-office token must not render a plan that could never work.
func TestWorkbookExport_LocalOfficeTokenRejectedInDryRun(t *testing.T) {
	_, err := runShortcutWithStubs(t, WorkbookExport, []string{
		"--spreadsheet-token", "local_office_abcdefghijkl", "--dry-run", "--as", "user",
	})
	requireProblem(t, err, errs.CategoryValidation, errs.SubtypeFailedPrecondition, "cannot be exported")
}

// TestWorkbookExport_SuccessLeavesStderrEmpty covers the shared drive export
// core this shortcut delegates to: creating the task, polling it and finishing
// used to be narrated on stderr, which made a completed export read as a
// failure to runners that key on stderr. Every one of those facts (ticket,
// ready, file_token) is in the payload.
func TestWorkbookExport_SuccessLeavesStderrEmpty(t *testing.T) {
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

	parent, stdout, stderr, reg := newTestRig(t, WorkbookExport)
	for _, s := range stubs {
		reg.Register(s)
	}
	parent.SetArgs([]string{"+workbook-export", "--url", testURL, "--file-extension", "xlsx", "--as", "user"})
	if err := parent.Execute(); err != nil {
		t.Fatalf("export execute failed: %v\n%s", err, stdout.String())
	}
	if got := stderr.String(); got != "" {
		t.Errorf("a successful export must leave stderr empty, got: %q", got)
	}
	if !strings.Contains(stdout.String(), `"ticket": "tk_export"`) {
		t.Errorf("the ticket must still be reported on stdout, got: %s", stdout.String())
	}
}

// TestWorkbookExport_LocalOfficeBehindWikiNodeRejected covers the second guard
// call site: a /wiki/ URL carries a node_token that Validate cannot classify,
// so a wiki node backed by a locally opened Office file only reveals itself
// after get_node runs in Execute. The export must be refused there, before any
// export_tasks request goes out — the create stub is registered so an
// unexpected call would surface as a passing export instead of a rejection.
func TestWorkbookExport_LocalOfficeBehindWikiNodeRejected(t *testing.T) {
	getNode := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "sheet",
					"obj_token": "local_office_abcdefghijkl",
				},
			},
		},
	}
	createTask := &httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/drive/v1/export_tasks",
		Optional: true,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"ticket": "tk_should_not_happen"},
		},
	}

	_, err := runShortcutWithStubs(t, WorkbookExport, []string{
		"--url", "https://example.feishu.cn/wiki/wikTestNODE", "--as", "user",
	}, getNode, createTask)

	requireProblem(t, err, errs.CategoryValidation, errs.SubtypeFailedPrecondition, "cannot be exported")
	if len(createTask.CapturedBodies) != 0 {
		t.Errorf("no export task may be created for a local Office file, got %d request(s)", len(createTask.CapturedBodies))
	}
}
