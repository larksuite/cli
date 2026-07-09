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

const fileSignURL = "/open-apis/spark/v1/apps/app_x/storage/file_sign"

// TestAppsFileSign_DryRunBody 验证 dry-run 输出 POST file_sign，body 携带 path 与 expires_in。
func TestAppsFileSign_DryRunBody(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsFileSign,
		[]string{"+file-sign", "--app-id", "app_x", "--path", "/x.png", "--expires-in", "3600", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	_ = json.Unmarshal([]byte(stdout.String()), &env)
	a := env.API[0]
	if a.Method != "POST" || a.URL != fileSignURL || a.Body["path"] != "/x.png" {
		t.Fatalf("dry-run = %s %s body=%v", a.Method, a.URL, a.Body)
	}
	if ei, _ := a.Body["expires_in"].(float64); int(ei) != 3600 {
		t.Fatalf("body.expires_in = %v, want 3600", a.Body["expires_in"])
	}
}

// TestAppsFileSign_RejectsDurationOverMax 验证 --expires-in 超过上限时触发 --expires-in typed 校验错误。
func TestAppsFileSign_RejectsDurationOverMax(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsFileSign,
		[]string{"+file-sign", "--app-id", "app_x", "--path", "/x.png", "--expires-in", "9999999", "--as", "user"}, factory, stdout)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %T %v, want *errs.ValidationError", err, err)
	}
	if ve.Param != "--expires-in" {
		t.Fatalf("Param = %q, want --expires-in", ve.Param)
	}
}

// TestAppsFileSign_PrettyPrintsSignedURL 验证 pretty 只输出 signed_url 本身。
func TestAppsFileSign_PrettyPrintsSignedURL(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: fileSignURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"file_name": "x.png", "path": "/x.png",
			"signed_url": "https://tos.example/x.png?sig=abc", "expires_at": "2026-04-16T10:30:00Z",
		}},
	})
	if err := runAppsShortcut(t, AppsFileSign,
		[]string{"+file-sign", "--app-id", "app_x", "--path", "/x.png", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := strings.TrimSpace(stdout.String())
	if got != "https://tos.example/x.png?sig=abc" {
		t.Fatalf("pretty should print only signed_url, got: %q", got)
	}
}
