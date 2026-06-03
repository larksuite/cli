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

func TestBuildHistoryQuery(t *testing.T) {
	// status, limit, page_token omitted when zero/empty; app_id is in the path
	q := buildHistoryQuery("", 0, "")
	if _, ok := q["status"]; ok {
		t.Errorf("status should be omitted when empty, got %v", q)
	}
	if _, ok := q["limit"]; ok {
		t.Errorf("limit should be omitted when 0, got %v", q)
	}
	if _, ok := q["page_token"]; ok {
		t.Errorf("page_token should be omitted when empty, got %v", q)
	}
	// all params included; page_token uses snake_case key
	q2 := buildHistoryQuery("finished", 30, "tok")
	if q2["status"] != "finished" {
		t.Errorf("status = %v, want finished", q2["status"])
	}
	if q2["limit"] != 30 {
		t.Errorf("limit = %v, want 30", q2["limit"])
	}
	if q2["page_token"] != "tok" {
		t.Errorf("page_token = %v, want tok", q2["page_token"])
	}
	if _, ok := q2["app_id"]; ok {
		t.Errorf("app_id must not be in query params, got %v", q2)
	}
}

func TestValidateHistoryLimit(t *testing.T) {
	if err := validateHistoryLimit(0); err != nil {
		t.Errorf("limit 0 (unset) should pass, got %v", err)
	}
	if err := validateHistoryLimit(500); err != nil {
		t.Errorf("limit 500 should pass, got %v", err)
	}
	if err := validateHistoryLimit(501); err == nil {
		t.Error("limit 501 should fail")
	}
	if err := validateHistoryLimit(-1); err == nil {
		t.Error("limit -1 should fail")
	}
}

// newHistoryRuntimeContext builds a RuntimeContext for AppsPublishHistory.Execute tests.
func newHistoryRuntimeContext(t *testing.T, appID string) (*common.RuntimeContext, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	cfg := &core.CliConfig{
		AppID:      "test-app-" + strings.ToLower(t.Name()),
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_test",
	}
	factory, stdoutBuf, _, reg := cmdutil.TestFactory(t, cfg)

	cmd := &cobra.Command{Use: "test-publish-history"}
	cmd.SetContext(context.Background())
	cmd.Flags().String("app-id", "", "")
	cmd.Flags().String("status", "", "")
	cmd.Flags().Int("limit", 0, "")
	cmd.Flags().String("page-token", "", "")
	_ = cmd.Flags().Set("app-id", appID)

	rctx := common.TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, factory, core.AsUser)
	return rctx, stdoutBuf, reg
}

func TestAppsPublishHistoryExecute_Success(t *testing.T) {
	rctx, stdoutBuf, reg := newHistoryRuntimeContext(t, "app_x")
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

	err := AppsPublishHistory.Execute(context.Background(), rctx)
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
