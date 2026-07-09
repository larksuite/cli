// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

const fileSignURLForDownload = "/open-apis/spark/v1/apps/app_x/storage/file_sign"

// TestAppsFileDownload_RequiresAppIDAndPath 验证仅含空白的 --path 触发 --path typed 校验错误。
func TestAppsFileDownload_RequiresAppIDAndPath(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsFileDownload,
		[]string{"+file-download", "--app-id", "app_x", "--path", "  ", "--as", "user"}, factory, stdout)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %T %v, want *errs.ValidationError", err, err)
	}
	if ve.Param != "--path" {
		t.Fatalf("Param = %q, want --path", ve.Param)
	}
}

// TestAppsFileDownload_DryRunSignsFirst 验证 dry-run 第一步是 POST file_sign。
func TestAppsFileDownload_DryRunSignsFirst(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsFileDownload,
		[]string{"+file-download", "--app-id", "app_x", "--path", "/x.png", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env struct {
		API []struct {
			Method string `json:"method"`
			URL    string `json:"url"`
		} `json:"api"`
	}
	_ = json.Unmarshal([]byte(stdout.String()), &env)
	if env.API[0].Method != "POST" || env.API[0].URL != fileSignURLForDownload {
		t.Fatalf("dry-run = %s %s (want POST sign)", env.API[0].Method, env.API[0].URL)
	}
}

// sign → 客户端 GET presigned signed_url → 落盘 --output。
func TestAppsFileDownload_EndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		io.WriteString(w, "PNGDATA")
	}))
	defer srv.Close()

	dir := t.TempDir()
	oldWD, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: fileSignURLForDownload,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"signed_url": srv.URL}},
	})
	if err := runAppsShortcut(t, AppsFileDownload,
		[]string{"+file-download", "--app-id", "app_x", "--path", "/x.png", "--output", "out.png", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "out.png"))
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(b) != "PNGDATA" {
		t.Fatalf("downloaded content = %q, want PNGDATA", b)
	}
	if !strings.Contains(stdout.String(), `"size_bytes": 7`) {
		t.Errorf("output json missing size_bytes:7\n%s", stdout.String())
	}
}

// 不传 --output → 默认远端 basename。
func TestAppsFileDownload_DefaultsOutputToBasename(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "DATA")
	}))
	defer srv.Close()

	dir := t.TempDir()
	oldWD, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: fileSignURLForDownload,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"signed_url": srv.URL}},
	})
	if err := runAppsShortcut(t, AppsFileDownload,
		[]string{"+file-download", "--app-id", "app_x", "--path", "/1858537546760216.png", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "1858537546760216.png")); err != nil {
		t.Fatalf("default output basename not written: %v", err)
	}
}
