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

func TestBuildPublishBody(t *testing.T) {
	// branch included when non-empty; app_id is NOT in body (it's in the path)
	b := buildPublishBody("feat/devops")
	if b["branch"] != "feat/devops" {
		t.Errorf("body = %v", b)
	}
	if _, ok := b["app_id"]; ok {
		t.Errorf("app_id must not be in body, got %v", b)
	}
	// branch omitted when empty
	b2 := buildPublishBody("")
	if _, ok := b2["branch"]; ok {
		t.Errorf("branch should be omitted when empty, got %v", b2)
	}
}

func TestAppsReleaseCreateMeta(t *testing.T) {
	if AppsReleaseCreate.Command != "+release-create" || AppsReleaseCreate.Risk != "write" {
		t.Errorf("meta mismatch: %+v", AppsReleaseCreate)
	}
	if len(AppsReleaseCreate.Scopes) != 1 || AppsReleaseCreate.Scopes[0] != "spark:app:write" {
		t.Errorf("scopes = %v", AppsReleaseCreate.Scopes)
	}
}

// newReleaseCreateRuntimeContext builds a RuntimeContext whose cobra.Command has the
// flags that AppsReleaseCreate.Execute reads (app-id, branch). Flag values are set
// via the returned setter helper.
func newReleaseCreateRuntimeContext(t *testing.T, appID, branch string) (*common.RuntimeContext, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	cfg := &core.CliConfig{
		AppID:      "test-app-" + strings.ToLower(t.Name()),
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_test",
	}
	factory, stdoutBuf, _, reg := cmdutil.TestFactory(t, cfg)

	cmd := &cobra.Command{Use: "test-release-create"}
	cmd.SetContext(context.Background())
	cmd.Flags().String("app-id", "", "")
	cmd.Flags().String("branch", "", "")
	_ = cmd.Flags().Set("app-id", appID)
	if branch != "" {
		_ = cmd.Flags().Set("branch", branch)
	}

	rctx := common.TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, factory, core.AsUser)
	return rctx, stdoutBuf, reg
}

func TestAppsReleaseCreateExecute_Success(t *testing.T) {
	rctx, stdoutBuf, reg := newReleaseCreateRuntimeContext(t, "app_x", "main")
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/releases",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"release_id": "123",
				"status":     "publishing",
			},
		},
	})

	err := AppsReleaseCreate.Execute(context.Background(), rctx)
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
	if env.Data["release_id"] != "123" {
		t.Errorf("release_id = %v, want 123", env.Data["release_id"])
	}
	if env.Data["status"] != "publishing" {
		t.Errorf("status = %v, want publishing", env.Data["status"])
	}
}

func TestAppsReleaseCreate_SyncField(t *testing.T) {
	rctx, stdoutBuf, reg := newReleaseCreateRuntimeContext(t, "app_sync", "main")
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_sync/releases",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"release_id": "456",
				"status":     "publishing",
				"sync":       true,
			},
		},
	})

	err := AppsReleaseCreate.Execute(context.Background(), rctx)
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
	if env.Data["release_id"] != "456" {
		t.Errorf("release_id = %v, want 456", env.Data["release_id"])
	}
	if env.Data["status"] != "publishing" {
		t.Errorf("status = %v, want publishing", env.Data["status"])
	}
	if env.Data["sync"] != true {
		t.Errorf("sync = %v, want true", env.Data["sync"])
	}
}
