// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

const fileDeleteURL = "/open-apis/spark/v1/apps/app_x/storage/file_batch_remove"

// TestAppsFileDelete_RequiresAppIDAndPath 验证仅含空白的 --path 去空后为空时，Validate 报 --path typed 校验错误。
func TestAppsFileDelete_RequiresAppIDAndPath(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	// 传入仅含空白的 --path：满足 cobra 的 Required 检查，但 cleanDeletePaths 去空后为空，
	// 触发 Validate 内的 typed --path 校验。
	err := runAppsShortcut(t, AppsFileDelete,
		[]string{"+file-delete", "--app-id", "app_x", "--path", "  ", "--yes", "--as", "user"}, factory, stdout)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %T %v, want *errs.ValidationError", err, err)
	}
	if ve.Param != "--path" {
		t.Fatalf("Param = %q, want --path", ve.Param)
	}
}

// high-risk-write：无 --yes → confirmation_required（exit 10）。
func TestAppsFileDelete_RequiresConfirmation(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsFileDelete,
		[]string{"+file-delete", "--app-id", "app_x", "--path", "/a.png", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "requires confirmation") {
		t.Fatalf("expected confirmation_required, got %v", err)
	}
}

// TestAppsFileDelete_DryRunSendsPaths 验证 dry-run 输出 POST file_batch_remove，body.paths 按序携带多个 --path。
func TestAppsFileDelete_DryRunSendsPaths(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsFileDelete,
		[]string{"+file-delete", "--app-id", "app_x", "--path", "/a.png", "--path", "/b.png", "--yes", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env dryRunAPIEnvelope
	_ = json.Unmarshal([]byte(stdout.String()), &env)
	a := env.API[0]
	if a.Method != "POST" || a.URL != fileDeleteURL {
		t.Fatalf("dry-run = %s %s", a.Method, a.URL)
	}
	paths, _ := a.Body["paths"].([]interface{})
	if len(paths) != 2 || paths[0] != "/a.png" || paths[1] != "/b.png" {
		t.Fatalf("body.paths = %v", a.Body["paths"])
	}
}

// 部分失败仍 ok:true；results 按下标 zip 回 path；失败项带 error{code,message}。
func TestAppsFileDelete_PartialFailureStillOK(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: fileDeleteURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"results": []interface{}{
				map[string]interface{}{"status": "ok", "file": map[string]interface{}{"file_name": "a.png", "path": "/a.png"}},
				map[string]interface{}{"status": "error", "error_code": "FILE_NOT_FOUND"},
			},
		}},
	})
	err := runAppsShortcut(t, AppsFileDelete,
		[]string{"+file-delete", "--app-id", "app_x", "--path", "/a.png", "--path", "/missing.png", "--yes", "--as", "user"}, factory, stdout)
	if err != nil {
		t.Fatalf("partial failure should NOT error (ok:true semantics), got %v", err)
	}
	got := stdout.String()
	var env struct {
		Data struct {
			Results []map[string]interface{} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(got), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, got)
	}
	if len(env.Data.Results) != 2 {
		t.Fatalf("want 2 results, got %d: %s", len(env.Data.Results), got)
	}
	r0, r1 := env.Data.Results[0], env.Data.Results[1]
	if r0["status"] != "ok" || r0["path"] != "/a.png" {
		t.Errorf("result[0] = %v", r0)
	}
	if r1["status"] != "error" || r1["path"] != "/missing.png" {
		t.Errorf("result[1] = %v (path must be back-filled by index)", r1)
	}
	if e, ok := r1["error"].(map[string]interface{}); !ok || e["code"] != "FILE_NOT_FOUND" {
		t.Errorf("result[1].error = %v (want code FILE_NOT_FOUND)", r1["error"])
	}
}

// TestAppsFileDelete_PrettySummary 验证 pretty 输出逐项 ✓/✗ 标记并汇总 "1/2 deleted"。
func TestAppsFileDelete_PrettySummary(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: fileDeleteURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"results": []interface{}{
				map[string]interface{}{"status": "ok", "file": map[string]interface{}{"file_name": "a.png"}},
				map[string]interface{}{"status": "error", "error_code": "FILE_NOT_FOUND"},
			},
		}},
	})
	if err := runAppsShortcut(t, AppsFileDelete,
		[]string{"+file-delete", "--app-id", "app_x", "--path", "/a.png", "--path", "/missing.png", "--yes", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	for _, want := range []string{"✓ /a.png", "✗ /missing.png (FILE_NOT_FOUND)", "1/2 deleted"} {
		if !strings.Contains(got, want) {
			t.Errorf("pretty missing %q:\n%s", want, got)
		}
	}
}
