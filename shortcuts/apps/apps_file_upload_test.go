// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

// TestAppsFileUpload_RequiresAppIDAndFile 验证仅含空白的 --file 经 Validate 去空后触发 --file typed 校验错误。
func TestAppsFileUpload_RequiresAppIDAndFile(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	// --file is a cobra-required flag; pass whitespace so cobra's required check
	// passes and our Validate (which trims) rejects it with a typed error.
	err := runAppsShortcut(t, AppsFileUpload,
		[]string{"+file-upload", "--app-id", "app_x", "--file", "  ", "--as", "user"}, factory, stdout)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %T %v, want *errs.ValidationError", err, err)
	}
	if ve.Param != "--file" {
		t.Fatalf("Param = %q, want --file", ve.Param)
	}
}

// TestAppsFileUpload_RejectsDirectory 验证 --file 指向目录时触发 --file typed 校验错误。
func TestAppsFileUpload_RejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	oldWD, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsFileUpload,
		[]string{"+file-upload", "--app-id", "app_x", "--file", "sub", "--as", "user"}, factory, stdout)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %T %v, want *errs.ValidationError", err, err)
	}
	if ve.Param != "--file" {
		t.Fatalf("Param = %q, want --file", ve.Param)
	}
}

// TestAppsFileUpload_DryRunPreUpload 验证 dry-run 输出 POST file_pre_upload，body.file_name 取文件 basename。
func TestAppsFileUpload_DryRunPreUpload(t *testing.T) {
	// Validate 会 Stat --file（在 DryRun 之前），故 dry-run 也需要真实存在的文件。
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "logo.png"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldWD, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsFileUpload,
		[]string{"+file-upload", "--app-id", "app_x", "--file", "logo.png", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env dryRunAPIEnvelope
	_ = json.Unmarshal([]byte(stdout.String()), &env)
	a := env.API[0]
	if a.Method != "POST" || a.URL != "/open-apis/spark/v1/apps/app_x/storage/file_pre_upload" {
		t.Fatalf("dry-run = %s %s", a.Method, a.URL)
	}
	if a.Body["file_name"] != "logo.png" {
		t.Fatalf("dry-run body.file_name = %v, want logo.png (basename)", a.Body["file_name"])
	}
}

// 三步直传：pre-upload → 客户端 PUT 字节 → callback。
func TestAppsFileUpload_EndToEnd(t *testing.T) {
	var putBody []byte
	var putContentType, putCD string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		putBody, _ = io.ReadAll(r.Body)
		putContentType = r.Header.Get("Content-Type")
		putCD = r.Header.Get("Content-Disposition")
		w.Header().Set("ETag", `"etag-123"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "logo.png"), []byte("PNGBYTES"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldWD, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/spark/v1/apps/app_x/storage/file_pre_upload",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"upload_url": srv.URL, "upload_id": "up-1"}},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/spark/v1/apps/app_x/storage/file_upload_callback",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"file_name": "logo.png", "path": "/1858537546760216.png", "size_bytes": 8, "type": "image/png",
			"download_url": "/spark/app/x/1858537546760216.png",
		}},
	})

	if err := runAppsShortcut(t, AppsFileUpload,
		[]string{"+file-upload", "--app-id", "app_x", "--file", "logo.png", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if string(putBody) != "PNGBYTES" {
		t.Fatalf("PUT body = %q, want file bytes", putBody)
	}
	if putContentType != "image/png" {
		t.Errorf("PUT Content-Type = %q, want image/png", putContentType)
	}
	// 原始文件名必须经 Content-Disposition 透传给 TOS（否则后端用 storage key 当文件名）。
	// 断言按解析结果（format-agnostic）：mime.FormatMediaType 对无 tspecial 的名不加引号，
	// 旧的写死字符串 `filename="logo.png"` 不再成立，但 filename 参数仍须等于原名。
	if disp, params, err := mime.ParseMediaType(putCD); err != nil || disp != "attachment" || params["filename"] != "logo.png" {
		t.Errorf("PUT Content-Disposition = %q, want disposition=attachment filename=logo.png (parse err=%v)", putCD, err)
	}
	got := stdout.String()
	if !strings.Contains(got, `"path": "/1858537546760216.png"`) {
		t.Errorf("output missing uploaded path:\n%s", got)
	}
}

// TestSanitizeUploadFileName_Cases 验证 sanitizeUploadFileName：空格转 %20、去 TOS 非法字符、全非法兜底、非 ASCII 百分号编码。
func TestSanitizeUploadFileName_Cases(t *testing.T) {
	cases := []struct{ in, want string }{
		{"logo.png", "logo.png"},
		{"a b.png", "a%20b.png"},     // 空格 → %20（encodeURIComponent）
		{`a:b/c*d?.png`, "abcd.png"}, // 去掉 TOS 非法字符
		{"///", "download_file"},     // 全非法 → 兜底
		{"中.txt", "%E4%B8%AD.txt"},   // 非 ASCII → UTF-8 百分号编码
	}
	for _, c := range cases {
		if got := sanitizeUploadFileName(c.in); got != c.want {
			t.Errorf("sanitizeUploadFileName(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

// TestMimeByExt_Cases 验证 mimeByExt：按扩展名识别 image/png，未知扩展名兜底 application/octet-stream。
func TestMimeByExt_Cases(t *testing.T) {
	if got := mimeByExt("a.png"); !strings.HasPrefix(got, "image/png") {
		t.Errorf("mimeByExt(a.png)=%q want image/png", got)
	}
	if got := mimeByExt("data.unknownext"); got != "application/octet-stream" {
		t.Errorf("mimeByExt(unknown)=%q want application/octet-stream", got)
	}
}
