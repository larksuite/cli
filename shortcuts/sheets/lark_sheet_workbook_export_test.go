// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

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
