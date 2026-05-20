// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"errors"
	"mime"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

func newAppsClientRuntime(t *testing.T) (*common.RuntimeContext, *httpmock.Registry) {
	t.Helper()
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
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected ExitError with detail, got %v", err)
	}
	if exitErr.Detail.Hint == "" {
		t.Fatalf("expected non-empty hint on code 90001")
	}
	if !strings.Contains(exitErr.Detail.Message, "build failed") {
		t.Fatalf("missing failure message: %v", exitErr.Detail.Message)
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
