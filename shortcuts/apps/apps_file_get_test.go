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

const fileGetURL = "/open-apis/spark/v1/apps/app_x/storage/file"

// TestAppsFileGet_RequiresAppIDAndPath 验证空白 --app-id 与空白 --path 分别触发对应的 typed 校验错误。
func TestAppsFileGet_RequiresAppIDAndPath(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsFileGet,
		[]string{"+file-get", "--app-id", "  ", "--path", "/x.png", "--as", "user"}, factory, stdout)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %T %v, want *errs.ValidationError", err, err)
	}
	if ve.Param != "--app-id" {
		t.Fatalf("Param = %q, want --app-id", ve.Param)
	}
	factory2, stdout2, _ := newAppsExecuteFactory(t)
	err2 := runAppsShortcut(t, AppsFileGet,
		[]string{"+file-get", "--app-id", "app_x", "--path", "  ", "--as", "user"}, factory2, stdout2)
	var ve2 *errs.ValidationError
	if !errors.As(err2, &ve2) {
		t.Fatalf("err = %T %v, want *errs.ValidationError", err2, err2)
	}
	if ve2.Param != "--path" {
		t.Fatalf("Param = %q, want --path", ve2.Param)
	}
}

// TestAppsFileGet_DryRunSendsPathQuery 验证 dry-run 输出 GET file，path 作为 query 参数下发。
func TestAppsFileGet_DryRunSendsPathQuery(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsFileGet,
		[]string{"+file-get", "--app-id", "app_x", "--path", "/x.png", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Params map[string]interface{} `json:"params"`
		} `json:"api"`
	}
	_ = json.Unmarshal([]byte(stdout.String()), &env)
	if env.API[0].Method != "GET" || env.API[0].URL != fileGetURL || env.API[0].Params["path"] != "/x.png" {
		t.Fatalf("dry-run = %s %s params=%v", env.API[0].Method, env.API[0].URL, env.API[0].Params)
	}
}

// TestAppsFileGet_SuccessAndPrettyKeyValue 验证 pretty key/value 展示 size 含 bytes、uploaded_by 只显示 name 且不泄漏 user id。
func TestAppsFileGet_SuccessAndPrettyKeyValue(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: fileGetURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"file_name": "logo.png", "path": "/1858537546760216.png",
			"size_bytes": 24580, "type": "image/png",
			"created_at": "2026-04-15T10:30:00Z",
			"created_by": `{"id":"7311","name":"alice"}`,
		}},
	})
	if err := runAppsShortcut(t, AppsFileGet,
		[]string{"+file-get", "--app-id", "app_x", "--path", "/1858537546760216.png", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	// pretty key/value：size 含 bytes、uploaded_by 只展示 name。
	for _, want := range []string{"file_name:", "24 KB (24580 bytes)", "uploaded_by: alice", "uploaded_at: 2026-04-15T10:30:00Z"} {
		if !strings.Contains(got, want) {
			t.Errorf("pretty missing %q:\n%s", want, got)
		}
	}
	// pretty 不该泄漏 user id。
	if strings.Contains(got, "7311") {
		t.Errorf("pretty should show name only, not id:\n%s", got)
	}
}
