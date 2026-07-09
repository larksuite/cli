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

func TestBuildReleaseListQuery(t *testing.T) {
	// page_size always present; status/page_token omitted when empty; app_id is in the path
	q := buildReleaseListQuery("", 0, "")
	if q["page_size"] != 0 {
		t.Errorf("page_size should always be present, got %v", q)
	}
	if _, ok := q["status"]; ok {
		t.Errorf("status should be omitted when empty, got %v", q)
	}
	if _, ok := q["page_token"]; ok {
		t.Errorf("page_token should be omitted when empty, got %v", q)
	}
	q2 := buildReleaseListQuery("finished", 30, "tok")
	if q2["page_size"] != 30 {
		t.Errorf("page_size = %v, want 30", q2["page_size"])
	}
	if q2["status"] != "finished" {
		t.Errorf("status = %v, want finished", q2["status"])
	}
	if q2["page_token"] != "tok" {
		t.Errorf("page_token = %v, want tok", q2["page_token"])
	}
	if _, ok := q2["app_id"]; ok {
		t.Errorf("app_id must not be in query params, got %v", q2)
	}
}

// newReleaseListRuntimeContext builds a RuntimeContext for AppsReleaseList.Execute tests.
func newReleaseListRuntimeContext(t *testing.T, appID string) (*common.RuntimeContext, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	cfg := &core.CliConfig{
		AppID:      "test-app-" + strings.ToLower(t.Name()),
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_test",
	}
	factory, stdoutBuf, _, reg := cmdutil.TestFactory(t, cfg)

	cmd := &cobra.Command{Use: "test-release-list"}
	cmd.SetContext(context.Background())
	cmd.Flags().String("app-id", "", "")
	cmd.Flags().String("status", "", "")
	cmd.Flags().Int("page-size", 20, "")
	cmd.Flags().String("page-token", "", "")
	_ = cmd.Flags().Set("app-id", appID)

	rctx := common.TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, factory, core.AsUser)
	return rctx, stdoutBuf, reg
}

func TestAppsReleaseListExecute_Success(t *testing.T) {
	rctx, stdoutBuf, reg := newReleaseListRuntimeContext(t, "app_x")
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/releases",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"releases": []interface{}{
					map[string]interface{}{
						"release_id": "1",
						"status":     "finished",
						"created_at": "1700000000000",
						"updated_at": "1700000000000",
					},
				},
				"next_page_token": "tok",
				"has_more":        true,
			},
		},
	})

	err := AppsReleaseList.Execute(context.Background(), rctx)
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

	// releases passthrough
	releases, ok := env.Data["releases"].([]interface{})
	if !ok || len(releases) != 1 {
		t.Fatalf("releases = %v", env.Data["releases"])
	}
	r0 := releases[0].(map[string]interface{})
	if r0["release_id"] != "1" {
		t.Errorf("releases[0].release_id = %v, want 1", r0["release_id"])
	}
	if r0["status"] != "finished" {
		t.Errorf("releases[0].status = %v, want finished", r0["status"])
	}

	// pagination fields passthrough
	if env.Data["next_page_token"] != "tok" {
		t.Errorf("next_page_token = %v, want tok", env.Data["next_page_token"])
	}
	if env.Data["has_more"] != true {
		t.Errorf("has_more = %v, want true", env.Data["has_more"])
	}
}
