// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func testRuntimeWithNameDesc(t *testing.T, name, desc string) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "update"}
	cmd.Flags().String("name", name, "")
	cmd.Flags().String("description", desc, "")
	return common.TestNewRuntimeContext(cmd, nil)
}

func TestBuildAppsUpdateBody_FieldCombos(t *testing.T) {
	t.Run("both empty -> empty body", func(t *testing.T) {
		if body := buildAppsUpdateBody(testRuntimeWithNameDesc(t, "  ", "")); len(body) != 0 {
			t.Errorf("empty inputs should yield empty body, got %v", body)
		}
	})
	t.Run("name only", func(t *testing.T) {
		body := buildAppsUpdateBody(testRuntimeWithNameDesc(t, "App", ""))
		if body["name"] != "App" || len(body) != 1 {
			t.Errorf("name-only body=%v", body)
		}
	})
	t.Run("description only", func(t *testing.T) {
		body := buildAppsUpdateBody(testRuntimeWithNameDesc(t, "", "desc"))
		if body["description"] != "desc" || len(body) != 1 {
			t.Errorf("desc-only body=%v", body)
		}
	})
	t.Run("both set and trimmed", func(t *testing.T) {
		body := buildAppsUpdateBody(testRuntimeWithNameDesc(t, "  App  ", "  d  "))
		if body["name"] != "App" || body["description"] != "d" {
			t.Errorf("both body=%v", body)
		}
	})
}

func TestAppsUpdate_PartialFields(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/spark/v1/apps/app_x",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"app": map[string]interface{}{
					"app_id":     "app_x",
					"name":       "renamed",
					"updated_at": "2026-05-18T10:05:00Z",
				},
			},
		},
	}
	reg.Register(stub)

	if err := runAppsShortcut(t, AppsUpdate,
		[]string{"+update", "--app-id", "app_x", "--name", "renamed", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	var sent map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &sent); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if sent["name"] != "renamed" {
		t.Fatalf("body.name = %v", sent["name"])
	}
	if _, present := sent["description"]; present {
		t.Fatalf("description should not be in body when not provided: %v", sent)
	}
}

func TestAppsUpdate_RequiresAppID(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsUpdate,
		[]string{"+update", "--name", "renamed", "--as", "user"}, factory, stdout)
	// cobra Required:true may match "app-id" instead of "--app-id"
	if err == nil || !strings.Contains(err.Error(), "app-id") {
		t.Fatalf("expected --app-id required, got %v", err)
	}
}

func TestAppsUpdate_RequiresAtLeastOneField(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsUpdate,
		[]string{"+update", "--app-id", "app_x", "--as", "user"}, factory, stdout)
	if err == nil {
		t.Fatalf("expected error when no field provided")
	}
}

func TestAppsUpdate_TrimsAppIDInPath(t *testing.T) {
	// 钉死 --app-id 在拼进 URL 前要先 TrimSpace —— 与 create / access-scope-* 等保持一致，
	// 避免 " app_x " 这种取值被原样 EncodePathSegment 编进 path 出现空格转义。
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/spark/v1/apps/app_x",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"app": map[string]interface{}{"app_id": "app_x"},
			},
		},
	}
	reg.Register(stub)

	if err := runAppsShortcut(t, AppsUpdate,
		[]string{"+update", "--app-id", "  app_x  ", "--name", "renamed", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
}

// TestAppsUpdate_PrettyOutputReadsNestedAppID exercises the prettyFn callback
// passed to OutFormat (only invoked under --format pretty) so the new
// data.app.app_id nesting is actually read by the text writer.
func TestAppsUpdate_PrettyOutputReadsNestedAppID(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/spark/v1/apps/app_x",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"app": map[string]interface{}{"app_id": "app_x", "name": "renamed"},
			},
		},
	})

	if err := runAppsShortcut(t, AppsUpdate,
		[]string{"+update", "--app-id", "app_x", "--name", "renamed", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "updated: app_x") {
		t.Fatalf("pretty output should read app_id from data.app.app_id, got: %q", got)
	}
}

const (
	iconAppPath    = "/open-apis/spark/v1/apps/app_x"
	iconUploadPath = "/open-apis/spark/v1/icon"
	iconHostedURL  = "https://cdn.example.com/icon/abc.png"
)

// pngHeader / jpegHeader are the magic bytes http.DetectContentType keys on.
var (
	pngHeader  = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	jpegHeader = []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00")
)

func appGetStub() *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    iconAppPath,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"app": map[string]interface{}{"app_id": "app_x", "app_type": "full_stack"}},
		},
	}
}

func appPatchStub(iconURL string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "PATCH",
		URL:    iconAppPath,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"app": map[string]interface{}{"app_id": "app_x", "icon_url": iconURL}},
		},
	}
}

func iconUploadStub() *httpmock.Stub {
	return &httpmock.Stub{
		Method: "POST",
		URL:    iconUploadPath,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"icon_url": iconHostedURL},
		},
	}
}

func requireIconFileParam(t *testing.T, err error, param, contains string) {
	t.Helper()
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %T %v, want *errs.ValidationError", err, err)
	}
	if ve.Param != param {
		t.Fatalf("Param = %q, want %q (err=%v)", ve.Param, param, err)
	}
	if !strings.Contains(err.Error(), contains) {
		t.Fatalf("err = %v, want it to contain %q", err, contains)
	}
}

func decodeSentBody(t *testing.T, raw []byte) map[string]interface{} {
	t.Helper()
	var sent map[string]interface{}
	if err := json.Unmarshal(raw, &sent); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return sent
}

func TestAppsUpdate_IconURLOnly(t *testing.T) {
	factory, stdout, stderr, reg := newAppsExecuteFactoryWithStderr(t)
	get := appGetStub()
	patch := appPatchStub("https://example.com/i.png")
	reg.Register(get)
	reg.Register(patch)

	if err := runAppsShortcut(t, AppsUpdate,
		[]string{"+update", "--app-id", "app_x", "--icon-url", "https://example.com/i.png", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	reg.Verify(t)
	sent := decodeSentBody(t, patch.CapturedBody)
	if sent["icon_url"] != "https://example.com/i.png" || len(sent) != 1 {
		t.Fatalf("PATCH body = %v, want only icon_url", sent)
	}
	if strings.Contains(stderr.String(), "warning") {
		t.Fatalf("unexpected warning: %s", stderr.String())
	}
}

func TestAppsUpdate_IconFileOnly(t *testing.T) {
	chdirTemp(t)
	content := append(append([]byte{}, pngHeader...), make([]byte, 64)...)
	_ = os.WriteFile("ok.png", content, 0o600)
	factory, stdout, stderr, reg := newAppsExecuteFactoryWithStderr(t)
	upload := iconUploadStub()
	patch := appPatchStub(iconHostedURL)
	reg.Register(appGetStub())
	reg.Register(upload)
	reg.Register(patch)

	if err := runAppsShortcut(t, AppsUpdate,
		[]string{"+update", "--app-id", "app_x", "--icon-file", "ok.png", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	reg.Verify(t)
	if ct := upload.CapturedHeaders.Get("Content-Type"); !strings.HasPrefix(ct, "multipart/form-data") {
		t.Fatalf("upload Content-Type = %q, want multipart/form-data", ct)
	}
	if !bytes.Contains(upload.CapturedBody, []byte(`name="file"`)) || !bytes.Contains(upload.CapturedBody, pngHeader) {
		t.Fatalf("upload body missing file part: %q", upload.CapturedBody)
	}
	sent := decodeSentBody(t, patch.CapturedBody)
	if sent["icon_url"] != iconHostedURL {
		t.Fatalf("PATCH icon_url = %v, want %s", sent["icon_url"], iconHostedURL)
	}
	if strings.Contains(stderr.String(), "warning") {
		t.Fatalf("unexpected warning: %s", stderr.String())
	}
}

func TestAppsUpdate_IconFileJPEGWithNameAndDescription(t *testing.T) {
	chdirTemp(t)
	_ = os.WriteFile("ok.jpg", append(append([]byte{}, jpegHeader...), make([]byte, 64)...), 0o600)
	factory, stdout, reg := newAppsExecuteFactory(t)
	patch := appPatchStub(iconHostedURL)
	reg.Register(appGetStub())
	reg.Register(iconUploadStub())
	reg.Register(patch)

	if err := runAppsShortcut(t, AppsUpdate,
		[]string{"+update", "--app-id", "app_x", "--name", "n", "--description", "d", "--icon-file", "ok.jpg", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	sent := decodeSentBody(t, patch.CapturedBody)
	if sent["name"] != "n" || sent["description"] != "d" || sent["icon_url"] != iconHostedURL {
		t.Fatalf("PATCH body = %v", sent)
	}
}

func TestAppsUpdate_NameOnlySkipsIconPrecheck(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(appPatchStub(""))
	// No GET stub: a name-only update must not precheck the app.
	if err := runAppsShortcut(t, AppsUpdate,
		[]string{"+update", "--app-id", "app_x", "--name", "n", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
}

func TestAppsUpdate_IconURLAndFileMutuallyExclusive(t *testing.T) {
	chdirTemp(t)
	_ = os.WriteFile("ok.png", pngHeader, 0o600)
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsUpdate,
		[]string{"+update", "--app-id", "app_x", "--icon-url", "https://example.com/i.png", "--icon-file", "ok.png", "--as", "user"},
		factory, stdout)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutually exclusive validation error, got %T %v", err, err)
	}
}

func TestAppsUpdate_RejectsNoFieldsMentionsIconFlags(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsUpdate,
		[]string{"+update", "--app-id", "app_x", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "--icon-file") {
		t.Fatalf("expected at-least-one error naming --icon-file, got %v", err)
	}
}

func TestAppsUpdate_RejectsInvalidIconURL(t *testing.T) {
	for _, raw := range []string{"ftp://example.com/i.png", "example.com/i.png", "https://", "./icon.png"} {
		t.Run(raw, func(t *testing.T) {
			factory, stdout, _ := newAppsExecuteFactory(t)
			err := runAppsShortcut(t, AppsUpdate,
				[]string{"+update", "--app-id", "app_x", "--icon-url", raw, "--as", "user"}, factory, stdout)
			requireIconFileParam(t, err, "--icon-url", "http(s)")
		})
	}
}

func TestAppsUpdate_RejectsBadIconFilesLocally(t *testing.T) {
	cases := []struct {
		name     string
		file     string
		content  []byte
		contains string
	}{
		{"gif", "bad.gif", []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;"), "png or jpeg"},
		{"fake png", "fake.png", []byte("this is plain text, not an image"), "png or jpeg"},
		{"oversize", "big.png", append(append([]byte{}, pngHeader...), make([]byte, appIconMaxBytes)...), "2 MB"},
		{"missing", "missing.png", nil, "cannot read file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chdirTemp(t)
			if tc.content != nil {
				_ = os.WriteFile(tc.file, tc.content, 0o600)
			}
			// No stubs registered: any request would fail the run with an
			// unmatched-request error instead of the local validation error.
			factory, stdout, _ := newAppsExecuteFactory(t)
			err := runAppsShortcut(t, AppsUpdate,
				[]string{"+update", "--app-id", "app_x", "--icon-file", tc.file, "--as", "user"}, factory, stdout)
			requireIconFileParam(t, err, "--icon-file", tc.contains)
		})
	}
}

func TestAppsUpdate_UnsupportedAppTypeBlocksUpload(t *testing.T) {
	chdirTemp(t)
	_ = os.WriteFile("ok.png", pngHeader, 0o600)
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    iconAppPath,
		Body:   map[string]interface{}{"code": appIconUnsupportedCode, "msg": "app type not supported"},
	})
	upload := iconUploadStub()
	upload.Optional = true
	reg.Register(upload)

	err := runAppsShortcut(t, AppsUpdate,
		[]string{"+update", "--app-id", "app_x", "--icon-file", "ok.png", "--as", "user"}, factory, stdout)
	p, ok := errs.ProblemOf(err)
	if !ok || p.Code != appIconUnsupportedCode {
		t.Fatalf("expected code %d problem, got %T %v", appIconUnsupportedCode, err, err)
	}
	if !strings.Contains(p.Hint, "OpenClaw") || !strings.Contains(p.Hint, "Miaoda editor") {
		t.Fatalf("hint = %q, want editor guidance", p.Hint)
	}
	if len(upload.CapturedBodies) != 0 {
		t.Fatalf("icon must not be uploaded when the precheck rejects the app type")
	}
}

func TestAppsUpdate_UnsupportedAppTypeOnPatchGetsHint(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(appGetStub())
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    iconAppPath,
		Body:   map[string]interface{}{"code": appIconUnsupportedCode, "msg": "app type not supported"},
	})
	err := runAppsShortcut(t, AppsUpdate,
		[]string{"+update", "--app-id", "app_x", "--icon-url", "https://example.com/i.png", "--as", "user"}, factory, stdout)
	p, ok := errs.ProblemOf(err)
	if !ok || !strings.Contains(p.Hint, "Miaoda editor") {
		t.Fatalf("expected editor hint, got %T %v", err, err)
	}
}

func TestAppsUpdate_WarnsWhenIconURLReadbackDiffers(t *testing.T) {
	factory, stdout, stderr, reg := newAppsExecuteFactoryWithStderr(t)
	reg.Register(appGetStub())
	reg.Register(appPatchStub("https://example.com/other.png"))

	if err := runAppsShortcut(t, AppsUpdate,
		[]string{"+update", "--app-id", "app_x", "--icon-url", "https://example.com/i.png", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := stderr.String(); !strings.Contains(got, "warning: icon_url in the update response") {
		t.Fatalf("stderr missing readback warning: %q", got)
	}
	if strings.Contains(stdout.String(), "warning") {
		t.Fatalf("warning leaked to stdout: %q", stdout.String())
	}
}

func TestAppsUpdate_DryRunIconFileShowsUploadAndPatch(t *testing.T) {
	chdirTemp(t)
	_ = os.WriteFile("ok.png", pngHeader, 0o600)
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsUpdate,
		[]string{"+update", "--app-id", "app_x", "--icon-file", "ok.png", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env struct {
		Data struct {
			API []struct {
				Method string `json:"method"`
				URL    string `json:"url"`
			} `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode dry-run: %v\n%s", err, stdout.String())
	}
	want := []string{"GET " + iconAppPath, "POST " + iconUploadPath, "PATCH " + iconAppPath}
	if len(env.Data.API) != len(want) {
		t.Fatalf("dry-run calls = %+v, want %v", env.Data.API, want)
	}
	for i, c := range env.Data.API {
		if got := c.Method + " " + c.URL; got != want[i] {
			t.Fatalf("dry-run call %d = %q, want %q", i, got, want[i])
		}
	}
	if !strings.Contains(stdout.String(), "icon_url from upload response") {
		t.Fatalf("dry-run PATCH body missing upload placeholder: %s", stdout.String())
	}
}
