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

func TestProjectReleaseDetailSnakeCaseProjection(t *testing.T) {
	data := map[string]interface{}{
		"release": map[string]interface{}{
			"release_id": "9001", "status": "publishing",
			"created_at": json.Number("1788264000000"), "updated_at": json.Number("1788264060000"),
			"online_url": "https://example.feishu.cn/app/9001", "commit_id": "abc123",
			"future_field": "preserved",
		},
		"error_logs": []interface{}{map[string]interface{}{
			"step": "build", "error_log": "compile error", "future_log_field": true,
		}},
		"current_node_info": map[string]interface{}{
			"current_node": "deploy", "current_status": "PENDING",
			"result": map[string]interface{}{
				"approval_url": "https://example.feishu.cn/approval/task/1", "future_result_field": "kept",
			},
			"submitted_by": map[string]interface{}{
				"username": "张三", "email": "zhangsan@example.com", "open_id": "ou_xxx", "future_submitter_field": true,
			},
			"created_at": json.Number("1788264060"), "future_node_field": "kept",
		},
	}
	before := cloneReleaseTestValue(t, data)
	projection := projectReleaseDetail(data)
	got := releaseTestJSONMap(t, projection.Data)
	if got["release_id"] != "9001" || got["future_field"] != "preserved" {
		t.Errorf("root fields = %#v", got)
	}
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
		projection.CurrentNode.Result.ApprovalURL != "https://example.feishu.cn/approval/task/1" ||
		projection.CurrentNode.SubmittedBy == nil || projection.CurrentNode.SubmittedBy.OpenID != "ou_xxx" ||
		projection.CurrentNode.CreatedAt != json.Number("1788264060") {
		t.Errorf("typed current node = %#v", projection.CurrentNode)
	}
	nodeJSON := got["current_node_info"].(map[string]interface{})
	if nodeJSON["future_node_field"] != "kept" ||
		nodeJSON["result"].(map[string]interface{})["future_result_field"] != "kept" ||
		nodeJSON["submitted_by"].(map[string]interface{})["future_submitter_field"] != true {
		t.Errorf("raw current node fields were not preserved: %#v", nodeJSON)
	}
	if !reflect.DeepEqual(releaseTestJSONMap(t, data), before) {
		t.Errorf("projectReleaseDetail mutated input\nbefore=%#v\nafter=%#v", before, releaseTestJSONMap(t, data))
	}
}

func TestProjectReleaseDetailNormalizesBOEApprovalNodeShape(t *testing.T) {
	data := map[string]interface{}{
		"release_id": "release_pending", "status": "pending",
		"created_at": json.Number("1788264000000"), "updated_at": json.Number("1788264060000"),
		"online_url": "https://example.feishu.cn/app/release_pending",
		"current_node_info": map[string]interface{}{
			"currentNode": "approval", "currentStatus": "PENDING",
			"result": map[string]interface{}{
				"approvalURL": "https://example.feishu.cn/approval/task/1", "future_result_field": "kept",
			},
			"submittedBy": map[string]interface{}{
				"username": "申请人", "openID": "ou_xxx", "future_submitter_field": true,
			},
			"createdAt": json.Number("1788264060"), "future_node_field": "kept",
		},
	}
	before := cloneReleaseTestValue(t, data)

	projection := projectReleaseDetail(data)
	got := releaseTestJSONMap(t, projection.Data)
	node := got["current_node_info"].(map[string]interface{})
	result := node["result"].(map[string]interface{})
	submitter := node["submitted_by"].(map[string]interface{})
	if node["current_node"] != "approval" || node["current_status"] != "PENDING" ||
		node["created_at"] != json.Number("1788264060") ||
		result["approval_url"] != "https://example.feishu.cn/approval/task/1" ||
		submitter["username"] != "申请人" || submitter["open_id"] != "ou_xxx" {
		t.Fatalf("normalized approval node = %#v", node)
	}
	for _, key := range []string{"currentNode", "currentStatus", "submittedBy", "createdAt"} {
		if _, present := node[key]; present {
			t.Errorf("compatible node key %q leaked into canonical JSON: %#v", key, node)
		}
	}
	if _, present := result["approvalURL"]; present {
		t.Errorf("compatible result key leaked into canonical JSON: %#v", result)
	}
	if _, present := submitter["openID"]; present {
		t.Errorf("compatible submitter key leaked into canonical JSON: %#v", submitter)
	}
	if node["future_node_field"] != "kept" || result["future_result_field"] != "kept" ||
		submitter["future_submitter_field"] != true {
		t.Errorf("normalization lost unknown approval fields: %#v", node)
	}
	if !reflect.DeepEqual(releaseTestJSONMap(t, data), before) {
		t.Errorf("projectReleaseDetail mutated BOE input\nbefore=%#v\nafter=%#v", before, releaseTestJSONMap(t, data))
	}

	var pretty bytes.Buffer
	writeReleaseDetailPretty(&pretty, projection)
	for _, line := range []string{
		"status: pending\n",
		"current_node: approval\n",
		"current_status: PENDING\n",
		"approval_url: https://example.feishu.cn/approval/task/1\n",
		"submitted_by_username: 申请人\n",
		"submitted_by_open_id: ou_xxx\n",
		"current_node_created_at: 1788264060\n",
	} {
		if !strings.Contains(pretty.String(), line) {
			t.Errorf("pretty output missing %q:\n%s", line, pretty.String())
		}
	}
	if strings.Contains(pretty.String(), "online_url:") {
		t.Errorf("pending online_url must not be presented as deployed:\n%s", pretty.String())
	}
}

func TestProjectReleaseDetailApprovalAliasPrecedenceAndFallback(t *testing.T) {
	projection := projectReleaseDetail(map[string]interface{}{
		"release_id": "release_aliases",
		"current_node_info": map[string]interface{}{
			"current_node": "snake-node", "currentNode": "camel-node",
			"current_status": 7, "currentStatus": "PENDING",
			"created_at": "", "createdAt": json.Number("1788264060"),
			"result": map[string]interface{}{
				"approval_url": "", "approvalURL": "https://example.feishu.cn/approval/task/fallback",
			},
			"submitted_by": "invalid", "submittedBy": map[string]interface{}{
				"open_id": "ou_snake", "openID": "ou_camel",
			},
		},
	})
	got := releaseTestJSONMap(t, projection.Data)
	node := got["current_node_info"].(map[string]interface{})
	result := node["result"].(map[string]interface{})
	submitter := node["submitted_by"].(map[string]interface{})
	if node["current_node"] != "snake-node" {
		t.Errorf("valid canonical current_node must win: %#v", node)
	}
	if node["current_status"] != "PENDING" || node["created_at"] != json.Number("1788264060") {
		t.Errorf("invalid canonical values must fall back to compatible values: %#v", node)
	}
	if result["approval_url"] != "https://example.feishu.cn/approval/task/fallback" {
		t.Errorf("empty canonical approval_url must fall back: %#v", result)
	}
	if submitter["open_id"] != "ou_snake" {
		t.Errorf("valid canonical open_id must win: %#v", submitter)
	}
	if projection.CurrentNode == nil || projection.CurrentNode.CurrentNode != "snake-node" ||
		projection.CurrentNode.CurrentStatus != "PENDING" || projection.CurrentNode.Result == nil ||
		projection.CurrentNode.Result.ApprovalURL != "https://example.feishu.cn/approval/task/fallback" ||
		projection.CurrentNode.SubmittedBy == nil || projection.CurrentNode.SubmittedBy.OpenID != "ou_snake" {
		t.Errorf("typed approval node did not consume canonical values: %#v", projection.CurrentNode)
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
				"current_node_info": map[string]interface{}{
					"current_node": "review", "result": map[string]interface{}{"approval_url": ""},
					"submitted_by": map[string]interface{}{"username": "", "email": "", "open_id": ""},
				},
			},
			check: func(t *testing.T, got map[string]interface{}, node *releaseCurrentNodeInfo) {
				if node == nil || node.Result != nil || node.SubmittedBy != nil {
					t.Fatalf("node = %#v", node)
				}
				nodeJSON := got["current_node_info"].(map[string]interface{})
				if !reflect.DeepEqual(nodeJSON["result"], map[string]interface{}{"approval_url": ""}) {
					t.Errorf("raw empty result was not preserved: %#v", nodeJSON)
				}
				if !reflect.DeepEqual(nodeJSON["submitted_by"], map[string]interface{}{"username": "", "email": "", "open_id": ""}) {
					t.Errorf("raw empty submitter was not preserved: %#v", nodeJSON)
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
		want        interface{}
	}{
		{name: "missing", data: map[string]interface{}{"release_id": "1"}},
		{name: "empty", data: map[string]interface{}{"release_id": "1", "error_logs": []interface{}{}}, wantPresent: true, want: []interface{}{}},
		{name: "null", data: map[string]interface{}{"release_id": "1", "error_logs": nil}, wantPresent: true},
		{name: "unexpected scalar", data: map[string]interface{}{"release_id": "1", "error_logs": "pending"}, wantPresent: true, want: "pending"},
		{name: "entry", data: map[string]interface{}{"release_id": "1", "error_logs": []interface{}{map[string]interface{}{"error_log": "boom", "code": 7}}}, wantPresent: true, want: []interface{}{map[string]interface{}{"error_log": "boom", "code": json.Number("7")}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := releaseTestJSONMap(t, projectReleaseDetail(tc.data).Data)
			value, present := got["error_logs"]
			if present != tc.wantPresent {
				t.Fatalf("error_logs present = %v, want %v; data=%#v", present, tc.wantPresent, got)
			}
			if present && !reflect.DeepEqual(value, tc.want) {
				t.Fatalf("error_logs = %#v, want %#v", value, tc.want)
			}
		})
	}
}

func TestProjectReleaseDetailFallsBackToNestedAuxiliaryFields(t *testing.T) {
	projection := projectReleaseDetail(map[string]interface{}{
		"release": map[string]interface{}{
			"release_id": "nested", "status": "publishing",
			"error_logs": nil,
			"current_node_info": map[string]interface{}{
				"current_node": "review", "current_status": "PENDING", "future_node_field": "kept",
			},
		},
	})
	got := releaseTestJSONMap(t, projection.Data)
	if value, present := got["error_logs"]; !present || value != nil {
		t.Fatalf("nested error_logs = %#v (present=%v), want preserved null", value, present)
	}
	node := got["current_node_info"].(map[string]interface{})
	if node["future_node_field"] != "kept" {
		t.Errorf("nested current_node_info lost unknown fields: %#v", node)
	}
	if projection.CurrentNode == nil || projection.CurrentNode.CurrentStatus != "PENDING" {
		t.Fatalf("typed nested current node = %#v, want PENDING", projection.CurrentNode)
	}
}

func TestProjectReleaseDetailOuterAuxiliaryFieldsTakePrecedence(t *testing.T) {
	projection := projectReleaseDetail(map[string]interface{}{
		"release": map[string]interface{}{
			"release_id": "outer-wins", "status": "publishing",
			"current_node_info": map[string]interface{}{"current_status": "INNER"},
			"error_logs":        "inner",
		},
		"current_node_info": map[string]interface{}{"current_status": "PENDING", "source": "outer"},
		"error_logs":        nil,
	})
	got := releaseTestJSONMap(t, projection.Data)
	if got["error_logs"] != nil {
		t.Errorf("outer null error_logs did not take precedence: %#v", got["error_logs"])
	}
	node := got["current_node_info"].(map[string]interface{})
	if node["source"] != "outer" || projection.CurrentNode == nil || projection.CurrentNode.CurrentStatus != "PENDING" {
		t.Errorf("outer current_node_info did not take precedence: data=%#v typed=%#v", node, projection.CurrentNode)
	}
}

func TestAppsReleaseGetFormatsKeepReleaseWhenErrorLogsIsNull(t *testing.T) {
	for _, format := range []string{"json", "table", "csv", "ndjson"} {
		t.Run(format, func(t *testing.T) {
			rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "null-logs")
			rctx.Format = format
			reg.Register(&httpmock.Stub{
				Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/releases/null-logs",
				Body: map[string]interface{}{"code": 0, "msg": "", "data": map[string]interface{}{
					"release": map[string]interface{}{
						"release_id": "release_null_logs", "status": "publishing", "created_at": "10", "updated_at": "11",
					},
					"error_logs": nil,
				}},
			})
			if err := AppsReleaseGet.Execute(context.Background(), rctx); err != nil {
				t.Fatalf("Execute() = %v", err)
			}

			switch format {
			case "json":
				var env struct {
					Data map[string]interface{} `json:"data"`
				}
				if err := json.Unmarshal(stdoutBuf.Bytes(), &env); err != nil {
					t.Fatalf("decode JSON: %v\n%s", err, stdoutBuf.String())
				}
				if env.Data["release_id"] != "release_null_logs" || env.Data["status"] != "publishing" {
					t.Fatalf("JSON lost release fields: %#v", env.Data)
				}
				if value, present := env.Data["error_logs"]; !present || value != nil {
					t.Fatalf("JSON error_logs = %#v (present=%v), want null", value, present)
				}
			case "ndjson":
				var data map[string]interface{}
				if err := json.Unmarshal(stdoutBuf.Bytes(), &data); err != nil {
					t.Fatalf("decode NDJSON: %v\n%s", err, stdoutBuf.String())
				}
				if data["release_id"] != "release_null_logs" || data["status"] != "publishing" {
					t.Fatalf("NDJSON lost release fields: %#v", data)
				}
			case "csv":
				records, err := csv.NewReader(strings.NewReader(stdoutBuf.String())).ReadAll()
				if err != nil {
					t.Fatalf("decode CSV: %v\n%s", err, stdoutBuf.String())
				}
				values := map[string]string{}
				for _, record := range records[1:] {
					if len(record) == 2 {
						values[record[0]] = record[1]
					}
				}
				if values["release_id"] != "release_null_logs" || values["status"] != "publishing" {
					t.Fatalf("CSV lost release fields: %#v\n%s", values, stdoutBuf.String())
				}
			case "table":
				out := stdoutBuf.String()
				if !strings.Contains(out, "release_id") || !strings.Contains(out, "release_null_logs") ||
					!strings.Contains(out, "status") || !strings.Contains(out, "publishing") {
					t.Fatalf("table lost release fields:\n%s", out)
				}
			}
		})
	}
}

func TestAppsReleaseGetPrettyUsesNestedPendingNode(t *testing.T) {
	rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "nested-pending")
	rctx.Format = "pretty"
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/releases/nested-pending",
		Body: map[string]interface{}{"code": 0, "msg": "", "data": map[string]interface{}{
			"release": map[string]interface{}{
				"release_id": "nested-pending", "status": "publishing", "created_at": "10", "updated_at": "11",
				"current_node_info": map[string]interface{}{
					"current_node": "review", "current_status": "PENDING",
				},
			},
		}},
	})
	if err := AppsReleaseGet.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	out := stdoutBuf.String()
	if !strings.Contains(out, "current_node: review") || !strings.Contains(out, "current_status: PENDING") {
		t.Fatalf("pretty output hid nested pending node:\n%s", out)
	}
}

func TestAppsReleaseGetPrettyPendingApprovalContext(t *testing.T) {
	rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "pending")
	rctx.Format = "pretty"
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/releases/pending",
		Body: map[string]interface{}{"code": 0, "msg": "", "data": map[string]interface{}{
			"release": map[string]interface{}{
				"release_id": "release_new", "status": "publishing",
				"created_at": json.Number("1788264000000"), "updated_at": json.Number("1788264060000"), "commit_id": "abc123",
			},
			"current_node_info": map[string]interface{}{
				"current_node": "deploy", "current_status": "PENDING",
				"result":       map[string]interface{}{"approval_url": "https://example.feishu.cn/approval/task/1"},
				"submitted_by": map[string]interface{}{"username": "张三", "email": "zhangsan@example.com", "open_id": "ou_xxx"},
				"created_at":   json.Number("1788264060"),
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
		"approval_url: https://example.feishu.cn/approval/task/1",
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

func TestAppsReleaseGetExecuteFormatsUseProjectedData(t *testing.T) {
	for _, format := range []string{"json", "pretty", "table", "csv", "ndjson"} {
		t.Run(format, func(t *testing.T) {
			rctx, stdoutBuf, reg := newStatusRuntimeContext(t, "app_x", "formats")
			rctx.Format = format
			reg.Register(&httpmock.Stub{
				Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/releases/formats",
				Body: map[string]interface{}{"code": 0, "msg": "", "data": map[string]interface{}{
					"release": map[string]interface{}{
						"release_id": "release_formats", "status": "publishing", "created_at": "10", "updated_at": "11", "unknown": "kept",
					},
					"current_node_info": map[string]interface{}{
						"current_node": "review", "current_status": "PENDING",
						"result": map[string]interface{}{"approval_url": "https://example.feishu.cn/approval/task/2"},
					},
				}},
			})
			if err := AppsReleaseGet.Execute(context.Background(), rctx); err != nil {
				t.Fatalf("Execute() = %v", err)
			}
			out := stdoutBuf.String()
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
				assertProjectedReleaseFormatData(t, env.Data)
			case "ndjson":
				var data map[string]interface{}
				if err := json.Unmarshal(stdoutBuf.Bytes(), &data); err != nil {
					t.Fatalf("decode NDJSON: %v", err)
				}
				assertProjectedReleaseFormatData(t, data)
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
					keys["current_node_info.result.approval_url"] != "https://example.feishu.cn/approval/task/2" {
					t.Errorf("CSV projected values = %#v", keys)
				}
			case "table":
				if !strings.Contains(out, "release_id") || !strings.Contains(out, "release_formats") || !strings.Contains(out, "unknown") || !strings.Contains(out, "kept") {
					t.Errorf("table projected output:\n%s", out)
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
				"release_id": releaseID, "status": status, "created_at": createdAt,
				"updated_at": updatedAt, "commit_id": commitID,
			},
			"current_node_info": map[string]interface{}{
				"current_node": currentNode, "current_status": currentStatus,
				"result": map[string]interface{}{"approval_url": approvalURL},
				"submitted_by": map[string]interface{}{
					"username": username, "email": email, "open_id": openID,
				},
				"created_at": nodeCreatedAt,
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
			"release_id": "release", "status": "finished", "created_at": "10", "updated_at": "11",
			"online_url": " https://example.feishu.cn/app\nonline_url: forged\t\x1b[2J ",
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
				"release_id": "failed-release", "status": "failed", "created_at": "10", "updated_at": "11",
			},
			"error_logs": []interface{}{map[string]interface{}{
				"step": step, "error_log": errorLog,
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

func assertProjectedReleaseFormatData(t *testing.T, data map[string]interface{}) {
	t.Helper()
	if data["release_id"] != "release_formats" || data["unknown"] != "kept" {
		t.Errorf("projected data = %#v", data)
	}
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
