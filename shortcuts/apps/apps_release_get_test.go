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

func TestAppsReleaseGetMeta(t *testing.T) {
	if AppsReleaseGet.Command != "+release-get" || AppsReleaseGet.Risk != "read" {
		t.Errorf("meta mismatch: %+v", AppsReleaseGet)
	}
	if len(AppsReleaseGet.Scopes) != 1 || AppsReleaseGet.Scopes[0] != "spark:app:read" {
		t.Errorf("scopes = %v", AppsReleaseGet.Scopes)
	}
	// both --app-id and --release-id must be required
	req := map[string]bool{}
	for _, f := range AppsReleaseGet.Flags {
		req[f.Name] = f.Required
	}
	if !req["app-id"] || !req["release-id"] {
		t.Errorf("app-id and release-id must be Required; flags=%+v", AppsReleaseGet.Flags)
	}
}

// newStatusRuntimeContext builds a RuntimeContext for AppsReleaseGet.Execute tests.
func newStatusRuntimeContext(t *testing.T, appID, releaseID string) (*common.RuntimeContext, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	cfg := &core.CliConfig{
		AppID:      "test-app-" + strings.ToLower(t.Name()),
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_test",
	}
	factory, stdoutBuf, _, reg := cmdutil.TestFactory(t, cfg)

	cmd := &cobra.Command{Use: "test-release-get"}
	cmd.SetContext(context.Background())
	cmd.Flags().String("app-id", "", "")
	cmd.Flags().String("release-id", "", "")
	_ = cmd.Flags().Set("app-id", appID)
	_ = cmd.Flags().Set("release-id", releaseID)

	rctx := common.TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, factory, core.AsUser)
	return rctx, stdoutBuf, reg
}

func TestAppsReleaseGetExecute_Success(t *testing.T) {
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

	err := AppsReleaseGet.Execute(context.Background(), rctx)
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

func TestAppsReleaseGetPrettyFinishedOnlineURL(t *testing.T) {
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
	if err := AppsReleaseGet.Execute(context.Background(), rctx); err != nil {
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

func TestAppsReleaseGetPrettyFailedErrorLogs(t *testing.T) {
	rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "6")
	rctx.Format = "pretty"
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/releases/6",
		Body: map[string]interface{}{
			"code": 0, "msg": "",
			"data": map[string]interface{}{
				"release": map[string]interface{}{
					"release_id": "6", "status": "failed",
					"created_at": "1700000000000", "updated_at": "1700000000050",
				},
				"error_logs": []interface{}{
					map[string]interface{}{"step": "build", "error_log": "compile error"},
				},
			},
		},
	})
	if err := AppsReleaseGet.Execute(context.Background(), rctx); err != nil {
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

func TestAppsReleaseGetPrettyPublishingNoExtra(t *testing.T) {
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
	if err := AppsReleaseGet.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	out := stdoutBuf.String()
	if strings.Contains(out, "online_url:") || strings.Contains(out, "error_log") {
		t.Errorf("publishing must not add extra fields, got:\n%s", out)
	}
}

func TestAppsReleaseGetPrettyFinishedNoURL(t *testing.T) {
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
	if err := AppsReleaseGet.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if strings.Contains(stdoutBuf.String(), "online_url:") {
		t.Errorf("finished without online_url must not print the line, got:\n%s", stdoutBuf.String())
	}
}

func TestAppsReleaseGetPrettyFailedEmptyLogs(t *testing.T) {
	rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "9")
	rctx.Format = "pretty"
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/releases/9",
		Body: map[string]interface{}{"code": 0, "msg": "",
			"data": map[string]interface{}{
				"release": map[string]interface{}{
					"release_id": "9", "status": "failed",
					"created_at": "1700000000000", "updated_at": "1700000000050",
				},
				"error_logs": []interface{}{},
			}},
	})
	if err := AppsReleaseGet.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if strings.Contains(stdoutBuf.String(), "compile error") {
		t.Errorf("empty error_logs must not render row content, got:\n%s", stdoutBuf.String())
	}
}

func TestAppsReleaseGetJSONErrorLogsPassthrough(t *testing.T) {
	rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "6")
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/releases/6",
		Body: map[string]interface{}{"code": 0, "msg": "",
			"data": map[string]interface{}{
				"release": map[string]interface{}{
					"release_id": "6", "status": "failed",
					"created_at": "1700000000000", "updated_at": "1700000000050",
				},
				"error_logs": []interface{}{
					map[string]interface{}{"step": "build", "error_log": "compile error"},
				},
			}},
	})
	if err := AppsReleaseGet.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdoutBuf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, stdoutBuf.String())
	}
	logs, ok := env.Data["error_logs"].([]interface{})
	if !ok || len(logs) != 1 {
		t.Fatalf("JSON must passthrough data.error_logs, got: %v", env.Data["error_logs"])
	}
	first, _ := logs[0].(map[string]interface{})
	if first["step"] != "build" || first["error_log"] != "compile error" {
		t.Errorf("error_logs content mismatch: %v", logs[0])
	}
	// flattened release fields must still be present alongside error_logs
	if env.Data["release_id"] != "6" || env.Data["status"] != "failed" {
		t.Errorf("flattened release fields missing: %v", env.Data)
	}
}

func TestAppsReleaseGetJSONNoErrorLogsKeyWhenAbsent(t *testing.T) {
	rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "5")
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/releases/5",
		Body: map[string]interface{}{"code": 0, "msg": "",
			"data": map[string]interface{}{"release": map[string]interface{}{
				"release_id": "5", "status": "finished",
				"created_at": "1700000000000", "updated_at": "1700000000001",
			}}},
	})
	if err := AppsReleaseGet.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	var env struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdoutBuf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, stdoutBuf.String())
	}
	if _, present := env.Data["error_logs"]; present {
		t.Errorf("error_logs key must be absent when API omits it, got: %v", env.Data)
	}
}

func TestAppsReleaseGetPrettyCommitID(t *testing.T) {
	rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "10")
	rctx.Format = "pretty"
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/releases/10",
		Body: map[string]interface{}{"code": 0, "msg": "",
			"data": map[string]interface{}{"release": map[string]interface{}{
				"release_id": "10", "status": "publishing",
				"created_at": "1700000000000", "updated_at": "1700000000000",
				"commit_id": "1230aisdkjah9123913hi193",
			}}},
	})
	if err := AppsReleaseGet.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if !strings.Contains(stdoutBuf.String(), "commit_id: 1230aisdkjah9123913hi193") {
		t.Errorf("expected commit_id line, got:\n%s", stdoutBuf.String())
	}
}

func TestAppsReleaseGetPrettyNoCommitID(t *testing.T) {
	rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "11")
	rctx.Format = "pretty"
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/releases/11",
		Body: map[string]interface{}{"code": 0, "msg": "",
			"data": map[string]interface{}{"release": map[string]interface{}{
				"release_id": "11", "status": "publishing",
				"created_at": "1700000000000", "updated_at": "1700000000000",
			}}},
	})
	if err := AppsReleaseGet.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if strings.Contains(stdoutBuf.String(), "commit_id:") {
		t.Errorf("absent commit_id must not print commit_id line, got:\n%s", stdoutBuf.String())
	}
}

func TestAppsReleaseGetPrettyEmptyCommitID(t *testing.T) {
	rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "12")
	rctx.Format = "pretty"
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/releases/12",
		Body: map[string]interface{}{"code": 0, "msg": "",
			"data": map[string]interface{}{"release": map[string]interface{}{
				"release_id": "12", "status": "publishing",
				"created_at": "1700000000000", "updated_at": "1700000000000",
				"commit_id": "",
			}}},
	})
	if err := AppsReleaseGet.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if strings.Contains(stdoutBuf.String(), "commit_id:") {
		t.Errorf("empty commit_id must not print commit_id line, got:\n%s", stdoutBuf.String())
	}
}

func TestAppsReleaseGetJSONOnlineURLPassthrough(t *testing.T) {
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
	if err := AppsReleaseGet.Execute(context.Background(), rctx); err != nil {
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
