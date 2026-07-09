// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"mime"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func newAppsClientRuntime(t *testing.T) (*common.RuntimeContext, *httpmock.Registry) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cfg := &core.CliConfig{
		AppID:      "test-app-" + strings.ToLower(t.Name()),
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_test",
	}
	factory, _, _, reg := cmdutil.TestFactory(t, cfg)
	rctx := common.TestNewRuntimeContextForAPI(context.Background(), nil, cfg, factory, core.AsUser)
	return rctx, reg
}

func TestAppsHTMLPublishAPI_Success(t *testing.T) {
	rctx, reg := newAppsClientRuntime(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/upload_and_release_html_code",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"url": "https://miaoda.feishu.cn/app/app_x",
			},
		},
	}
	reg.Register(stub)

	api := appsHTMLPublishAPI{runtime: rctx}
	tarball := &htmlPublishTarball{Body: []byte("fake"), Size: 4, SHA256: "abc"}
	resp, err := api.HTMLPublish(context.Background(), "app_x", tarball)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if resp.URL != "https://miaoda.feishu.cn/app/app_x" {
		t.Fatalf("url=%q", resp.URL)
	}

	ct := stub.CapturedHeaders.Get("Content-Type")
	mt, params, err := mime.ParseMediaType(ct)
	if err != nil || mt != "multipart/form-data" {
		t.Fatalf("content type %q wrong", ct)
	}
	mr := multipart.NewReader(bytes.NewReader(stub.CapturedBody), params["boundary"])
	saw := false
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		if p.FormName() == "file" {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("multipart missing 'file' part")
	}
}

func TestAppsHTMLPublishAPI_BusinessErrorHasHint(t *testing.T) {
	rctx, reg := newAppsClientRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/upload_and_release_html_code",
		Body: map[string]interface{}{
			"code": 90001,
			"msg":  "build failed: dependency conflict",
		},
	})

	api := appsHTMLPublishAPI{runtime: rctx}
	_, err := api.HTMLPublish(context.Background(), "app_x", &htmlPublishTarball{Body: []byte("fake")})
	if err == nil {
		t.Fatalf("expected error")
	}
	problem := requireAppsAPIProblem(t, err)
	if problem.Code != errCodeBuildFailed {
		t.Fatalf("code = %d, want %d", problem.Code, errCodeBuildFailed)
	}
	if problem.Hint == "" {
		t.Fatalf("expected non-empty hint on code 90001")
	}
	if !strings.Contains(problem.Message, "build failed") {
		t.Fatalf("missing failure message: %v", problem.Message)
	}
}

func TestAppsHTMLPublishAPI_AppNotFoundClassified(t *testing.T) {
	rctx, reg := newAppsClientRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_missing/upload_and_release_html_code",
		Body: map[string]interface{}{
			"code": errCodeAppNotFound,
			"msg":  "app not found",
		},
	})

	api := appsHTMLPublishAPI{runtime: rctx}
	_, err := api.HTMLPublish(context.Background(), "app_missing", &htmlPublishTarball{Body: []byte("fake")})
	problem := requireAppsAPIProblem(t, err)
	if problem.Subtype != errs.SubtypeNotFound {
		t.Fatalf("subtype = %q, want %q", problem.Subtype, errs.SubtypeNotFound)
	}
	if problem.Hint == "" {
		t.Fatalf("expected app-not-found recovery hint")
	}
}

func TestAppsHTMLPublishAPI_MissingURLIsInvalidResponse(t *testing.T) {
	rctx, reg := newAppsClientRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/upload_and_release_html_code",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{},
		},
	})

	api := appsHTMLPublishAPI{runtime: rctx}
	_, err := api.HTMLPublish(context.Background(), "app_x", &htmlPublishTarball{Body: []byte("fake")})
	problem := requireAppsProblem(t, err, errs.CategoryInternal)
	if problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("subtype = %q, want %q", problem.Subtype, errs.SubtypeInvalidResponse)
	}
}

func TestBuildHTMLPublishFailureHint_UnknownCodeReturnsEmpty(t *testing.T) {
	// 默认分支：未识别的 code 返回空 hint，让 Agent 用 message 兜底。
	if hint := buildHTMLPublishFailureHint(99999); hint != "" {
		t.Fatalf("unknown code should return empty hint, got %q", hint)
	}
	if hint := buildHTMLPublishFailureHint(0); hint != "" {
		t.Fatalf("zero code should return empty hint, got %q", hint)
	}
}

func TestBuildHTMLPublishFailureHint_KnownCodes(t *testing.T) {
	if hint := buildHTMLPublishFailureHint(90001); hint == "" {
		t.Fatalf("code 90001 should return non-empty hint")
	}
	if hint := buildHTMLPublishFailureHint(90002); hint == "" {
		t.Fatalf("code 90002 should return non-empty hint")
	}
}

func TestBuildHTMLPublishFailureHint_NotFoundHintNoLongerMentionsList(t *testing.T) {
	hint := buildHTMLPublishFailureHint(90002)
	if hint == "" {
		t.Fatalf("code 90002 should return non-empty hint")
	}
	if strings.Contains(hint, "+list") {
		t.Fatalf("hint must not point at hidden +list command, got: %q", hint)
	}
	if !strings.Contains(hint, "app_id") {
		t.Fatalf("hint should reference app_id, got: %q", hint)
	}
}

func TestAppsHTMLPublishAPI_MalformedResponseIsInvalidResponse(t *testing.T) {
	rctx, reg := newAppsClientRuntime(t)
	reg.Register(&httpmock.Stub{
		Method:  "POST",
		URL:     "/open-apis/spark/v1/apps/app_x/upload_and_release_html_code",
		RawBody: []byte("{not json"),
	})

	api := appsHTMLPublishAPI{runtime: rctx}
	_, err := api.HTMLPublish(context.Background(), "app_x", &htmlPublishTarball{Body: []byte("fake")})
	problem := requireAppsProblem(t, err, errs.CategoryInternal)
	if problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("subtype = %q, want %q", problem.Subtype, errs.SubtypeInvalidResponse)
	}
}
