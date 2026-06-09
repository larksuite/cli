// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestAppsPublishStatusMeta(t *testing.T) {
	if AppsPublishStatus.Command != "+publish-status" || AppsPublishStatus.Risk != "read" {
		t.Errorf("meta mismatch: %+v", AppsPublishStatus)
	}
	if len(AppsPublishStatus.Scopes) != 1 || AppsPublishStatus.Scopes[0] != "spark:app:read" {
		t.Errorf("scopes = %v", AppsPublishStatus.Scopes)
	}
	// both --app-id and --release-id must be required
	req := map[string]bool{}
	for _, f := range AppsPublishStatus.Flags {
		req[f.Name] = f.Required
	}
	if !req["app-id"] || !req["release-id"] {
		t.Errorf("app-id and release-id must be Required; flags=%+v", AppsPublishStatus.Flags)
	}
}

// newStatusRuntimeContext builds a RuntimeContext for AppsPublishStatus.Execute tests.
func newStatusRuntimeContext(t *testing.T, appID, releaseID string) (*common.RuntimeContext, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	cfg := &core.CliConfig{
		AppID:      "test-app-" + strings.ToLower(t.Name()),
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_test",
	}
	factory, stdoutBuf, _, reg := cmdutil.TestFactory(t, cfg)

	cmd := &cobra.Command{Use: "test-publish-status"}
	cmd.SetContext(context.Background())
	cmd.Flags().String("app-id", "", "")
	cmd.Flags().String("release-id", "", "")
	_ = cmd.Flags().Set("app-id", appID)
	_ = cmd.Flags().Set("release-id", releaseID)

	rctx := common.TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, factory, core.AsUser)
	return rctx, stdoutBuf, reg
}

func TestAppsPublishStatusExecute_Success(t *testing.T) {
	rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "5")
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/releases/5",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"release": map[string]interface{}{
					"release_id": "5",
					"status":     "finished",
					"created_at": "1700000000000",
					"updated_at": "1700000000001",
				},
			},
		},
	})

	err := AppsPublishStatus.Execute(context.Background(), rctx)
	if err != nil {
		t.Fatalf("Execute() = %v", err)
	}

	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdoutBuf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal output: %v\nraw: %s", err, stdoutBuf.String())
	}
	if !env.OK {
		t.Fatalf("expected ok=true, got: %s", stdoutBuf.String())
	}
	// Execute unwraps the nested "release" object
	if env.Data["release_id"] != "5" {
		t.Errorf("release_id = %v, want 5", env.Data["release_id"])
	}
	if env.Data["status"] != "finished" {
		t.Errorf("status = %v, want finished", env.Data["status"])
	}
}

func TestAppsPublishStatusPrettyFinishedOnlineURL(t *testing.T) {
	rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "5")
	rctx.Format = "pretty"
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/releases/5",
		Body: map[string]interface{}{
			"code": 0, "msg": "",
			"data": map[string]interface{}{"release": map[string]interface{}{
				"release_id": "5", "status": "finished",
				"created_at": "1700000000000", "updated_at": "1700000000001",
				"online_url": "https://example.feishu.cn/spark/faas/app_x",
			}},
		},
	})
	if err := AppsPublishStatus.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	out := stdoutBuf.String()
	if !strings.Contains(out, "status: finished") {
		t.Errorf("missing base fields:\n%s", out)
	}
	if !strings.Contains(out, "online_url: https://example.feishu.cn/spark/faas/app_x") {
		t.Errorf("expected online_url line, got:\n%s", out)
	}
}

func TestAppsPublishStatusPrettyFailedErrorLogs(t *testing.T) {
	rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "6")
	rctx.Format = "pretty"
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/releases/6",
		Body: map[string]interface{}{
			"code": 0, "msg": "",
			"data": map[string]interface{}{"release": map[string]interface{}{
				"release_id": "6", "status": "failed",
				"created_at": "1700000000000", "updated_at": "1700000000050",
				"error_logs": []interface{}{
					map[string]interface{}{"step": "build", "error_log": "compile error"},
				},
			}},
		},
	})
	if err := AppsPublishStatus.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	out := stdoutBuf.String()
	if !strings.Contains(out, "status: failed") {
		t.Errorf("missing base fields:\n%s", out)
	}
	if !strings.Contains(out, "build") || !strings.Contains(out, "compile error") {
		t.Errorf("expected error_logs table with step/error_log, got:\n%s", out)
	}
}

func TestAppsPublishStatusPrettyPublishingNoExtra(t *testing.T) {
	rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "7")
	rctx.Format = "pretty"
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/releases/7",
		Body: map[string]interface{}{"code": 0, "msg": "",
			"data": map[string]interface{}{"release": map[string]interface{}{
				"release_id": "7", "status": "publishing",
				"created_at": "1700000000000", "updated_at": "1700000000000",
			}}},
	})
	if err := AppsPublishStatus.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	out := stdoutBuf.String()
	if strings.Contains(out, "online_url:") || strings.Contains(out, "error_log") {
		t.Errorf("publishing must not add extra fields, got:\n%s", out)
	}
}

func TestAppsPublishStatusPrettyFinishedNoURL(t *testing.T) {
	rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "8")
	rctx.Format = "pretty"
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/releases/8",
		Body: map[string]interface{}{"code": 0, "msg": "",
			"data": map[string]interface{}{"release": map[string]interface{}{
				"release_id": "8", "status": "finished",
				"created_at": "1700000000000", "updated_at": "1700000000001",
			}}},
	})
	if err := AppsPublishStatus.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if strings.Contains(stdoutBuf.String(), "online_url:") {
		t.Errorf("finished without online_url must not print the line, got:\n%s", stdoutBuf.String())
	}
}

func TestAppsPublishStatusPrettyFailedEmptyLogs(t *testing.T) {
	rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "9")
	rctx.Format = "pretty"
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/releases/9",
		Body: map[string]interface{}{"code": 0, "msg": "",
			"data": map[string]interface{}{"release": map[string]interface{}{
				"release_id": "9", "status": "failed",
				"created_at": "1700000000000", "updated_at": "1700000000050",
				"error_logs": []interface{}{},
			}}},
	})
	if err := AppsPublishStatus.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if strings.Contains(stdoutBuf.String(), "compile error") {
		t.Errorf("empty error_logs must not render row content, got:\n%s", stdoutBuf.String())
	}
}

func TestAppsPublishStatusJSONOnlineURLPassthrough(t *testing.T) {
	rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "5")
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/releases/5",
		Body: map[string]interface{}{"code": 0, "msg": "",
			"data": map[string]interface{}{"release": map[string]interface{}{
				"release_id": "5", "status": "finished",
				"created_at": "1700000000000", "updated_at": "1700000000001",
				"online_url": "https://example.feishu.cn/spark/faas/app_x",
			}}},
	})
	if err := AppsPublishStatus.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdoutBuf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, stdoutBuf.String())
	}
	if env.Data["online_url"] != "https://example.feishu.cn/spark/faas/app_x" {
		t.Errorf("JSON must passthrough online_url, got: %v", env.Data["online_url"])
	}
}
