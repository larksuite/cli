// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"reflect"
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

func TestProjectReleaseDetailAliases(t *testing.T) {
	current := map[string]interface{}{
		"release": map[string]interface{}{
			"releaseID": "release_new", "release_id": "release_old", "status": "publishing",
			"createdAt": json.Number("1788264000000"), "updatedAt": json.Number("1788264060000"),
			"onlineUrl": "https://example.feishu.cn/app/release_new", "commitID": "abc123",
			"future_field": "preserved",
		},
		"errorLogs": []interface{}{map[string]interface{}{
			"step": "build", "errorLog": "compile error", "future_log_field": true,
		}},
		"currentNodeInfo": map[string]interface{}{
			"currentNode": "deploy", "currentStatus": "PENDING",
			"result":      map[string]interface{}{"approvalURL": "https://approval.example.com/task/1"},
			"submittedBy": map[string]interface{}{"username": "张三", "email": "zhangsan@example.com", "openID": "ou_xxx"},
			"createdAt":   json.Number("1788264060"),
		},
	}
	legacy := map[string]interface{}{
		"release": map[string]interface{}{
			"release_id": "release_new", "status": "publishing",
			"created_at": json.Number("1788264000000"), "updated_at": json.Number("1788264060000"),
			"online_url": "https://example.feishu.cn/app/release_new", "commit_id": "abc123",
			"future_field": "preserved",
		},
		"error_logs": []interface{}{map[string]interface{}{
			"step": "build", "error_log": "compile error", "future_log_field": true,
		}},
		"current_node_info": map[string]interface{}{
			"current_node": "deploy", "current_status": "PENDING",
			"result":       map[string]interface{}{"approval_url": "https://approval.example.com/task/1"},
			"submitted_by": map[string]interface{}{"username": "张三", "email": "zhangsan@example.com", "open_id": "ou_xxx"},
			"created_at":   json.Number("1788264060"),
		},
	}

	var currentData map[string]interface{}
	for _, tc := range []struct {
		name string
		data map[string]interface{}
	}{
		{name: "current camel aliases", data: current},
		{name: "legacy snake aliases", data: legacy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := cloneReleaseTestValue(t, tc.data)
			projection := projectReleaseDetail(tc.data)
			got := releaseTestJSONMap(t, projection.Data)

			if tc.name == "current camel aliases" {
				currentData = got
			} else if !reflect.DeepEqual(got, currentData) {
				t.Fatalf("legacy projection differs from current:\nlegacy=%#v\ncurrent=%#v", got, currentData)
			}
			if got["release_id"] != "release_new" || got["future_field"] != "preserved" {
				t.Errorf("root fields = %#v", got)
			}
			assertNoReleaseCamelAliases(t, got)
			if _, ok := got["release"]; ok {
				t.Errorf("release wrapper leaked: %#v", got)
			}
			logs := got["error_logs"].([]interface{})
			logEntry := logs[0].(map[string]interface{})
			if logEntry["error_log"] != "compile error" || logEntry["future_log_field"] != true {
				t.Errorf("error log = %#v", logEntry)
			}
			if projection.CurrentNode == nil || projection.CurrentNode.CurrentNode != "deploy" ||
				projection.CurrentNode.CurrentStatus != "PENDING" || projection.CurrentNode.Result == nil ||
				projection.CurrentNode.Result.ApprovalURL != "https://approval.example.com/task/1" ||
				projection.CurrentNode.SubmittedBy == nil || projection.CurrentNode.SubmittedBy.OpenID != "ou_xxx" ||
				projection.CurrentNode.CreatedAt != json.Number("1788264060") {
				t.Errorf("typed current node = %#v", projection.CurrentNode)
			}
			if !reflect.DeepEqual(releaseTestJSONMap(t, tc.data), before) {
				t.Errorf("projectReleaseDetail mutated input\nbefore=%#v\nafter=%#v", before, releaseTestJSONMap(t, tc.data))
			}
		})
	}
}

func TestProjectReleaseDetailCamelPresenceWins(t *testing.T) {
	data := map[string]interface{}{
		"release": map[string]interface{}{
			"releaseID": "", "release_id": "legacy-release",
			"createdAt": nil, "created_at": json.Number("1"),
			"commitID": 42, "commit_id": "legacy-commit",
			"status": "failed",
		},
		"errorLogs": []interface{}{map[string]interface{}{
			"errorLog": nil, "error_log": "legacy error", "extra": "kept",
		}},
		"error_logs":        []interface{}{map[string]interface{}{"error_log": "outer legacy"}},
		"currentNodeInfo":   "wrong type",
		"current_node_info": map[string]interface{}{"current_node": "legacy node"},
	}
	projection := projectReleaseDetail(data)
	got := releaseTestJSONMap(t, projection.Data)
	if value, ok := got["release_id"]; !ok || value != "" {
		t.Errorf("release_id = %#v, present=%v; camel presence must win", value, ok)
	}
	if value, ok := got["created_at"]; !ok || value != nil {
		t.Errorf("created_at = %#v, present=%v; camel nil must win", value, ok)
	}
	if value := got["commit_id"]; value != json.Number("42") {
		t.Errorf("commit_id = %#v; camel wrong type must win", value)
	}
	logs := got["error_logs"].([]interface{})
	entry := logs[0].(map[string]interface{})
	if value, ok := entry["error_log"]; !ok || value != nil || entry["extra"] != "kept" {
		t.Errorf("error log aliases = %#v", entry)
	}
	if projection.CurrentNode != nil {
		t.Errorf("camel wrong-type currentNodeInfo must block snake fallback: %#v", projection.CurrentNode)
	}
	if _, ok := got["current_node_info"]; ok {
		t.Errorf("unusable current node must be omitted: %#v", got)
	}
}

func TestProjectReleaseDetailOptionalNestedObjects(t *testing.T) {
	tests := []struct {
		name  string
		data  map[string]interface{}
		check func(*testing.T, map[string]interface{}, *releaseCurrentNodeInfo)
	}{
		{
			name: "missing current node",
			data: map[string]interface{}{"release_id": "1"},
			check: func(t *testing.T, got map[string]interface{}, node *releaseCurrentNodeInfo) {
				if node != nil {
					t.Fatalf("node = %#v, want nil", node)
				}
				if _, ok := got["current_node_info"]; ok {
					t.Errorf("current_node_info must be omitted: %#v", got)
				}
			},
		},
		{
			name: "empty result and submitter omitted",
			data: map[string]interface{}{
				"release_id": "2",
				"currentNodeInfo": map[string]interface{}{
					"currentNode": "review", "result": map[string]interface{}{"approvalURL": ""},
					"submittedBy": map[string]interface{}{"username": "", "email": "", "openID": ""},
				},
			},
			check: func(t *testing.T, got map[string]interface{}, node *releaseCurrentNodeInfo) {
				if node == nil || node.Result != nil || node.SubmittedBy != nil {
					t.Fatalf("node = %#v", node)
				}
				nodeJSON := got["current_node_info"].(map[string]interface{})
				if _, ok := nodeJSON["result"]; ok {
					t.Errorf("empty result leaked: %#v", nodeJSON)
				}
				if _, ok := nodeJSON["submitted_by"]; ok {
					t.Errorf("empty submitted_by leaked: %#v", nodeJSON)
				}
			},
		},
		{
			name: "partial submitter",
			data: map[string]interface{}{
				"release_id": "3",
				"current_node_info": map[string]interface{}{
					"submitted_by": map[string]interface{}{"email": "only@example.com"},
				},
			},
			check: func(t *testing.T, got map[string]interface{}, node *releaseCurrentNodeInfo) {
				if node == nil || node.SubmittedBy == nil || node.SubmittedBy.Email != "only@example.com" {
					t.Fatalf("node = %#v", node)
				}
				submitter := got["current_node_info"].(map[string]interface{})["submitted_by"].(map[string]interface{})
				if !reflect.DeepEqual(submitter, map[string]interface{}{"email": "only@example.com"}) {
					t.Errorf("submitter = %#v", submitter)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			projection := projectReleaseDetail(tc.data)
			tc.check(t, releaseTestJSONMap(t, projection.Data), projection.CurrentNode)
		})
	}
}

func TestProjectReleaseDetailErrorLogsPresence(t *testing.T) {
	tests := []struct {
		name        string
		data        map[string]interface{}
		wantPresent bool
		wantLen     int
	}{
		{name: "missing", data: map[string]interface{}{"release_id": "1"}},
		{name: "empty", data: map[string]interface{}{"release_id": "1", "errorLogs": []interface{}{}}, wantPresent: true},
		{name: "present wrong type", data: map[string]interface{}{"release_id": "1", "errorLogs": nil}, wantPresent: true},
		{name: "entry", data: map[string]interface{}{"release_id": "1", "errorLogs": []interface{}{map[string]interface{}{"errorLog": "boom", "code": 7}}}, wantPresent: true, wantLen: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := releaseTestJSONMap(t, projectReleaseDetail(tc.data).Data)
			value, present := got["error_logs"]
			if present != tc.wantPresent {
				t.Fatalf("error_logs present = %v, want %v; data=%#v", present, tc.wantPresent, got)
			}
			if present {
				logs, ok := value.([]interface{})
				if !ok || len(logs) != tc.wantLen {
					t.Fatalf("error_logs = %#v, want len %d", value, tc.wantLen)
				}
				if tc.wantLen == 1 {
					entry := logs[0].(map[string]interface{})
					if entry["error_log"] != "boom" || entry["code"] != json.Number("7") {
						t.Errorf("entry = %#v", entry)
					}
				}
			}
		})
	}
}

func TestAppsReleaseGetPrettyPendingApprovalContext(t *testing.T) {
	rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "pending")
	rctx.Format = "pretty"
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/releases/pending",
		Body: map[string]interface{}{"code": 0, "msg": "", "data": map[string]interface{}{
			"release": map[string]interface{}{
				"releaseID": "release_new", "status": "publishing",
				"createdAt": json.Number("1788264000000"), "updatedAt": json.Number("1788264060000"), "commitID": "abc123",
			},
			"currentNodeInfo": map[string]interface{}{
				"currentNode": "deploy", "currentStatus": "PENDING",
				"result":      map[string]interface{}{"approvalURL": "https://approval.example.com/task/1"},
				"submittedBy": map[string]interface{}{"username": "张三", "email": "zhangsan@example.com", "openID": "ou_xxx"},
				"createdAt":   json.Number("1788264060"),
			},
		}},
	})
	if err := AppsReleaseGet.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	want := strings.Join([]string{
		"release_id: release_new",
		"status: publishing",
		"created_at: 1788264000000",
		"updated_at: 1788264060000",
		"commit_id: abc123",
		"current_node: deploy",
		"current_status: PENDING",
		"approval_url: https://approval.example.com/task/1",
		"submitted_by_username: 张三",
		"submitted_by_email: zhangsan@example.com",
		"submitted_by_open_id: ou_xxx",
		"current_node_created_at: 1788264060",
	}, "\n") + "\n"
	if got := stdoutBuf.String(); got != want {
		t.Errorf("pretty output mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestAppsReleaseGetPrettyNodeWithoutApprovalURL(t *testing.T) {
	projection := projectReleaseDetail(map[string]interface{}{
		"release_id": "1", "status": "publishing", "created_at": "10", "updated_at": "11",
		"current_node_info": map[string]interface{}{
			"current_node": "review", "current_status": "PENDING",
			"submitted_by": map[string]interface{}{"email": "only@example.com"},
		},
	})
	var out bytes.Buffer
	writeReleaseDetailPretty(&out, projection)
	want := "release_id: 1\nstatus: publishing\ncreated_at: 10\nupdated_at: 11\n" +
		"current_node: review\ncurrent_status: PENDING\napproval_url: --\nsubmitted_by_email: only@example.com\n"
	if out.String() != want {
		t.Errorf("pretty output mismatch\ngot:\n%s\nwant:\n%s", out.String(), want)
	}
	if strings.Contains(out.String(), "submitted_by_username") || strings.Contains(out.String(), "submitted_by_open_id") {
		t.Errorf("missing submitter fields must be omitted:\n%s", out.String())
	}
}

func TestAppsReleaseGetExecuteFormatsUseNormalizedData(t *testing.T) {
	for _, format := range []string{"json", "pretty", "table", "csv", "ndjson"} {
		t.Run(format, func(t *testing.T) {
			rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "formats")
			rctx.Format = format
			reg.Register(&httpmock.Stub{
				Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/releases/formats",
				Body: map[string]interface{}{"code": 0, "msg": "", "data": map[string]interface{}{
					"release": map[string]interface{}{
						"releaseID": "release_formats", "status": "publishing", "createdAt": "10", "updatedAt": "11", "unknown": "kept",
					},
					"currentNodeInfo": map[string]interface{}{
						"currentNode": "review", "currentStatus": "PENDING",
						"result": map[string]interface{}{"approvalURL": "https://approval.example.com/task/2"},
					},
				}},
			})
			if err := AppsReleaseGet.Execute(context.Background(), rctx); err != nil {
				t.Fatalf("Execute() = %v", err)
			}
			out := stdoutBuf.String()
			for _, camel := range []string{"releaseID", "createdAt", "updatedAt", "errorLogs", "errorLog", "currentNodeInfo", "currentNode", "currentStatus", "approvalURL"} {
				if strings.Contains(out, camel) {
					t.Errorf("%s output leaks %q:\n%s", format, camel, out)
				}
			}
			for _, snake := range []string{"release_id", "created_at", "updated_at", "current_node", "current_status", "approval_url"} {
				if !strings.Contains(out, snake) {
					t.Errorf("%s output missing %q:\n%s", format, snake, out)
				}
			}

			switch format {
			case "json":
				var env struct {
					Data map[string]interface{} `json:"data"`
				}
				if err := json.Unmarshal(stdoutBuf.Bytes(), &env); err != nil {
					t.Fatalf("decode JSON: %v", err)
				}
				assertNormalizedReleaseFormatData(t, env.Data)
			case "ndjson":
				var data map[string]interface{}
				if err := json.Unmarshal(stdoutBuf.Bytes(), &data); err != nil {
					t.Fatalf("decode NDJSON: %v", err)
				}
				assertNormalizedReleaseFormatData(t, data)
			case "csv":
				records, err := csv.NewReader(strings.NewReader(out)).ReadAll()
				if err != nil {
					t.Fatalf("decode CSV: %v", err)
				}
				keys := map[string]string{}
				for _, record := range records[1:] {
					keys[record[0]] = record[1]
				}
				if keys["release_id"] != "release_formats" || keys["unknown"] != "kept" ||
					keys["current_node_info.result.approval_url"] != "https://approval.example.com/task/2" {
					t.Errorf("CSV normalized values = %#v", keys)
				}
			case "table":
				if !strings.Contains(out, "release_id") || !strings.Contains(out, "release_formats") || !strings.Contains(out, "unknown") || !strings.Contains(out, "kept") {
					t.Errorf("table normalized output:\n%s", out)
				}
			}
		})
	}
}

func TestAppsReleaseGetPrettySanitizationIsDisplayOnly(t *testing.T) {
	releaseID := "release\rforged\nstatus: forged\t\x1b[31mred\x1b[0m\u202e"
	status := "publishing\nrelease_id: forged"
	createdAt := "10\nupdated_at: forged"
	updatedAt := "11\tcommit_id: forged"
	commitID := "abc\ncurrent_node: forged"
	currentNode := "review\napproval_url: forged"
	currentStatus := "PENDING\tstatus: forged"
	approvalURL := "https://approval.example\nsubmitted_by_username: forged"
	username := "张\ncurrent_node_created_at: forged"
	email := "user@example.com\tstatus: forged"
	openID := "ou_xxx\nonline_url: forged"
	nodeCreatedAt := "12\tstatus: forged"

	fixture := func() map[string]interface{} {
		return map[string]interface{}{
			"release": map[string]interface{}{
				"releaseID": releaseID, "status": status, "createdAt": createdAt,
				"updatedAt": updatedAt, "commitID": commitID,
			},
			"currentNodeInfo": map[string]interface{}{
				"currentNode": currentNode, "currentStatus": currentStatus,
				"result": map[string]interface{}{"approvalURL": approvalURL},
				"submittedBy": map[string]interface{}{
					"username": username, "email": email, "openID": openID,
				},
				"createdAt": nodeCreatedAt,
			},
		}
	}

	t.Run("pretty flattens and sanitizes every displayed server scalar", func(t *testing.T) {
		projection := projectReleaseDetail(fixture())
		var out bytes.Buffer
		writeReleaseDetailPretty(&out, projection)
		want := strings.Join([]string{
			"release_id: releaseforged status: forged red",
			"status: publishing release_id: forged",
			"created_at: 10 updated_at: forged",
			"updated_at: 11 commit_id: forged",
			"commit_id: abc current_node: forged",
			"current_node: review approval_url: forged",
			"current_status: PENDING status: forged",
			"approval_url: https://approval.example submitted_by_username: forged",
			"submitted_by_username: 张 current_node_created_at: forged",
			"submitted_by_email: user@example.com status: forged",
			"submitted_by_open_id: ou_xxx online_url: forged",
			"current_node_created_at: 12 status: forged",
		}, "\n") + "\n"
		if got := out.String(); got != want {
			t.Fatalf("sanitized pretty output mismatch\ngot:\n%q\nwant:\n%q", got, want)
		}
		if strings.ContainsAny(out.String(), "\r\t\x1b") || strings.ContainsRune(out.String(), '\u202e') {
			t.Errorf("pretty output contains terminal-unsafe content: %q", out.String())
		}
		for _, forged := range []string{"\nstatus: forged", "\nrelease_id: forged", "\nupdated_at: forged", "\ncommit_id: forged", "\ncurrent_node: forged", "\napproval_url: forged", "\nsubmitted_by_username: forged", "\ncurrent_node_created_at: forged", "\nonline_url: forged"} {
			if strings.Contains(out.String(), forged) {
				t.Errorf("pretty output permits forged labelled line %q: %q", forged, out.String())
			}
		}
	})

	t.Run("online URL is sanitized", func(t *testing.T) {
		projection := projectReleaseDetail(map[string]interface{}{
			"releaseID": "release", "status": "finished", "createdAt": "10", "updatedAt": "11",
			"onlineUrl": " https://example.feishu.cn/app\nonline_url: forged\t\x1b[2J ",
		})
		var out bytes.Buffer
		writeReleaseDetailPretty(&out, projection)
		if !strings.Contains(out.String(), "online_url: https://example.feishu.cn/app online_url: forged\n") || strings.Contains(out.String(), "\x1b") {
			t.Errorf("online URL was not rendered safely: %q", out.String())
		}
	})

	for _, format := range []string{"json", "ndjson"} {
		t.Run(format+" preserves raw strings", func(t *testing.T) {
			rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "raw")
			rctx.Format = format
			reg.Register(&httpmock.Stub{
				Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/releases/raw",
				Body: map[string]interface{}{"code": 0, "msg": "", "data": fixture()},
			})
			if err := AppsReleaseGet.Execute(context.Background(), rctx); err != nil {
				t.Fatalf("Execute() = %v", err)
			}
			var got map[string]interface{}
			if format == "json" {
				var env struct {
					Data map[string]interface{} `json:"data"`
				}
				if err := json.Unmarshal(stdoutBuf.Bytes(), &env); err != nil {
					t.Fatalf("decode JSON: %v", err)
				}
				got = env.Data
			} else if err := json.Unmarshal(stdoutBuf.Bytes(), &got); err != nil {
				t.Fatalf("decode NDJSON: %v", err)
			}
			if got["release_id"] != releaseID || got["status"] != status || got["created_at"] != createdAt ||
				got["updated_at"] != updatedAt || got["commit_id"] != commitID {
				t.Errorf("%s root data did not preserve raw strings: %#v", format, got)
			}
			node := got["current_node_info"].(map[string]interface{})
			result := node["result"].(map[string]interface{})
			submitter := node["submitted_by"].(map[string]interface{})
			if node["current_node"] != currentNode || node["current_status"] != currentStatus ||
				node["created_at"] != nodeCreatedAt || result["approval_url"] != approvalURL ||
				submitter["username"] != username || submitter["email"] != email || submitter["open_id"] != openID {
				t.Errorf("%s node data did not preserve raw strings: %#v", format, node)
			}
		})
	}
}

func TestAppsReleaseGetFailedLogSanitizationIsDisplayOnly(t *testing.T) {
	step := "build\nstatus: forged\t\x1b[31mred\x1b[0m\u202e"
	errorLog := "compile\nrelease_id: forged\t\x1b]0;owned\x07"
	fixture := func() map[string]interface{} {
		return map[string]interface{}{
			"release": map[string]interface{}{
				"releaseID": "failed-release", "status": "failed", "createdAt": "10", "updatedAt": "11",
			},
			"errorLogs": []interface{}{map[string]interface{}{
				"step": step, "errorLog": errorLog,
			}},
		}
	}

	t.Run("pretty table keeps one terminal-safe row", func(t *testing.T) {
		projection := projectReleaseDetail(fixture())
		var out bytes.Buffer
		writeReleaseDetailPretty(&out, projection)
		got := out.String()
		if strings.ContainsAny(got, "\r\t\x1b") || strings.ContainsRune(got, '\u202e') {
			t.Fatalf("failed log table contains terminal-unsafe content: %q", got)
		}
		if strings.Contains(got, "\nstatus: forged") || strings.Contains(got, "\nrelease_id: forged") {
			t.Fatalf("failed log table permits a forged labelled line: %q", got)
		}
		lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
		if len(lines) != 7 {
			t.Fatalf("failed pretty output has %d lines, want 7 (one table row):\n%s", len(lines), got)
		}
		if !strings.Contains(lines[6], "build status: forged red") || !strings.Contains(lines[6], "compile release_id: forged") {
			t.Errorf("failed log cells were not flattened in their single row: %q", lines[6])
		}
	})

	for _, format := range []string{"json", "ndjson"} {
		t.Run(format+" preserves raw log cells", func(t *testing.T) {
			rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "failed-raw")
			rctx.Format = format
			reg.Register(&httpmock.Stub{
				Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/releases/failed-raw",
				Body: map[string]interface{}{"code": 0, "msg": "", "data": fixture()},
			})
			if err := AppsReleaseGet.Execute(context.Background(), rctx); err != nil {
				t.Fatalf("Execute() = %v", err)
			}

			var entry map[string]interface{}
			if format == "json" {
				var env struct {
					Data map[string]interface{} `json:"data"`
				}
				if err := json.Unmarshal(stdoutBuf.Bytes(), &env); err != nil {
					t.Fatalf("decode JSON: %v", err)
				}
				logs := env.Data["error_logs"].([]interface{})
				entry = logs[0].(map[string]interface{})
			} else if err := json.Unmarshal(stdoutBuf.Bytes(), &entry); err != nil {
				t.Fatalf("decode NDJSON: %v", err)
			}
			if entry["step"] != step || entry["error_log"] != errorLog {
				t.Errorf("%s changed raw failed log cells: %#v", format, entry)
			}
		})
	}
}

func TestReleasePrettyDisplayNilCompatibility(t *testing.T) {
	if got := releasePrettyDisplayValue(nil); got != "" {
		t.Fatalf("releasePrettyDisplayValue(nil) = %q, want blank table cell", got)
	}

	var table bytes.Buffer
	writeReleaseErrorLogTable(&table, []interface{}{map[string]interface{}{
		"step": nil, "error_log": nil,
	}})
	if strings.Contains(table.String(), "<nil>") {
		t.Errorf("nil failed-log cells changed from blank to <nil>:\n%s", table.String())
	}

	var pretty bytes.Buffer
	writeReleaseDetailPretty(&pretty, releaseDetailProjection{})
	if !strings.Contains(pretty.String(), "created_at: <nil>\nupdated_at: <nil>\n") {
		t.Errorf("missing root timestamps must retain legacy pretty rendering:\n%s", pretty.String())
	}
}

func assertNormalizedReleaseFormatData(t *testing.T, data map[string]interface{}) {
	t.Helper()
	if data["release_id"] != "release_formats" || data["unknown"] != "kept" {
		t.Errorf("normalized data = %#v", data)
	}
	assertNoReleaseCamelAliases(t, data)
}

func assertNoReleaseCamelAliases(t *testing.T, value interface{}) {
	t.Helper()
	camel := map[string]bool{
		"releaseID": true, "createdAt": true, "updatedAt": true, "onlineUrl": true, "commitID": true,
		"errorLogs": true, "errorLog": true, "currentNodeInfo": true, "currentNode": true,
		"currentStatus": true, "approvalURL": true, "submittedBy": true, "openID": true,
	}
	var walk func(interface{})
	walk = func(current interface{}) {
		switch typed := current.(type) {
		case map[string]interface{}:
			for key, nested := range typed {
				if camel[key] {
					t.Errorf("camel alias %q leaked in %#v", key, value)
				}
				walk(nested)
			}
		case []interface{}:
			for _, nested := range typed {
				walk(nested)
			}
		}
	}
	walk(value)
}

func releaseTestJSONMap(t *testing.T, value interface{}) map[string]interface{} {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	var got map[string]interface{}
	if err := decoder.Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return got
}

func cloneReleaseTestValue(t *testing.T, value interface{}) map[string]interface{} {
	t.Helper()
	return releaseTestJSONMap(t, value)
}
