// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"errors"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
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
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "pkg.tar.gz")
	_ = os.WriteFile(tarPath, []byte("fake"), 0o644)

	rctx, reg := newAppsClientRuntime(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/miaoda/v1/apps/app_x/upload_and_release_html_code",
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
	tarball := &htmlPublishTarball{Path: tarPath, Size: 4, SHA256: "abc"}
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
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "pkg.tar.gz")
	_ = os.WriteFile(tarPath, []byte("fake"), 0o644)

	rctx, reg := newAppsClientRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/miaoda/v1/apps/app_x/upload_and_release_html_code",
		Body: map[string]interface{}{
			"code": 90001,
			"msg":  "build failed: dependency conflict",
		},
	})

	api := appsHTMLPublishAPI{runtime: rctx}
	_, err := api.HTMLPublish(context.Background(), "app_x", &htmlPublishTarball{Path: tarPath})
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
