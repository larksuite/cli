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

func TestShapeErrorLog(t *testing.T) {
	// gateway returns snake_case error_logs
	in := map[string]interface{}{
		"status": "failed",
		"error_logs": []interface{}{
			map[string]interface{}{"step": "build", "error_log": "boom"},
		},
	}
	out := shapeErrorLog(in)
	if out["status"] != "failed" {
		t.Errorf("status = %v, want failed", out["status"])
	}
	logs, ok := out["error_logs"].([]interface{})
	if !ok || len(logs) != 1 {
		t.Fatalf("error_logs = %v", out["error_logs"])
	}
	// missing error_logs -> empty slice, not nil
	out2 := shapeErrorLog(map[string]interface{}{"status": "finished"})
	if logs2, ok := out2["error_logs"].([]interface{}); !ok || len(logs2) != 0 {
		t.Errorf("error_logs should default to empty slice, got %v", out2["error_logs"])
	}
}

func TestAppsPublishErrorLogMeta(t *testing.T) {
	if AppsPublishErrorLog.Command != "+publish-error-log" || AppsPublishErrorLog.Risk != "read" {
		t.Errorf("meta mismatch: %+v", AppsPublishErrorLog)
	}
	if len(AppsPublishErrorLog.Scopes) != 1 || AppsPublishErrorLog.Scopes[0] != "spark:app:read" {
		t.Errorf("scopes = %v", AppsPublishErrorLog.Scopes)
	}
	req := map[string]bool{}
	for _, f := range AppsPublishErrorLog.Flags {
		req[f.Name] = f.Required
	}
	if !req["app-id"] || !req["release-id"] {
		t.Errorf("app-id and release-id must be Required; flags=%+v", AppsPublishErrorLog.Flags)
	}
}

// newErrorLogRuntimeContext builds a RuntimeContext for AppsPublishErrorLog.Execute tests.
func newErrorLogRuntimeContext(t *testing.T, appID, releaseID string) (*common.RuntimeContext, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	cfg := &core.CliConfig{
		AppID:      "test-app-" + strings.ToLower(t.Name()),
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_test",
	}
	factory, stdoutBuf, _, reg := cmdutil.TestFactory(t, cfg)

	cmd := &cobra.Command{Use: "test-publish-error-log"}
	cmd.SetContext(context.Background())
	cmd.Flags().String("app-id", "", "")
	cmd.Flags().String("release-id", "", "")
	_ = cmd.Flags().Set("app-id", appID)
	_ = cmd.Flags().Set("release-id", releaseID)

	rctx := common.TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, factory, core.AsUser)
	return rctx, stdoutBuf, reg
}

func TestAppsPublishErrorLogExecute_Success(t *testing.T) {
	rctx, stdoutBuf, reg := newErrorLogRuntimeContext(t, "app_x", "7")
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/releases/7/error_logs",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"status": "failed",
				"error_logs": []interface{}{
					map[string]interface{}{
						"step":      "build",
						"error_log": "boom",
					},
				},
			},
		},
	})

	err := AppsPublishErrorLog.Execute(context.Background(), rctx)
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
	if env.Data["status"] != "failed" {
		t.Errorf("status = %v, want failed", env.Data["status"])
	}
	logs, ok := env.Data["error_logs"].([]interface{})
	if !ok || len(logs) != 1 {
		t.Fatalf("error_logs = %v", env.Data["error_logs"])
	}
	entry := logs[0].(map[string]interface{})
	if entry["step"] != "build" {
		t.Errorf("error_logs[0].step = %v, want build", entry["step"])
	}
	if entry["error_log"] != "boom" {
		t.Errorf("error_logs[0].error_log = %v, want boom", entry["error_log"])
	}
}
