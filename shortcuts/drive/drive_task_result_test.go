// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestDriveTaskResultValidateErrorsByScenario(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		flags   map[string]string
		wantErr string
	}{
		{
			name: "unsupported scenario",
			flags: map[string]string{
				"scenario": "unknown",
			},
			wantErr: "unsupported scenario",
		},
		{
			name: "import missing ticket",
			flags: map[string]string{
				"scenario": "import",
			},
			wantErr: "--ticket is required",
		},
		{
			name: "export missing file token",
			flags: map[string]string{
				"scenario": "export",
				"ticket":   "ticket_export_test",
			},
			wantErr: "--file-token is required",
		},
		{
			name: "task check missing task id",
			flags: map[string]string{
				"scenario": "task_check",
			},
			wantErr: "--task-id is required",
		},
		{
			name: "wiki move missing task id",
			flags: map[string]string{
				"scenario": "wiki_move",
			},
			wantErr: "--task-id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := &cobra.Command{Use: "drive +task_result"}
			cmd.Flags().String("scenario", "", "")
			cmd.Flags().String("ticket", "", "")
			cmd.Flags().String("task-id", "", "")
			cmd.Flags().String("file-token", "", "")
			for key, value := range tt.flags {
				if err := cmd.Flags().Set(key, value); err != nil {
					t.Fatalf("set --%s: %v", key, err)
				}
			}

			runtime := common.TestNewRuntimeContext(cmd, nil)
			err := DriveTaskResult.Validate(context.Background(), runtime)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestDriveTaskResultDryRunExportIncludesTokenParam(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +task_result"}
	cmd.Flags().String("scenario", "", "")
	cmd.Flags().String("ticket", "", "")
	cmd.Flags().String("task-id", "", "")
	cmd.Flags().String("file-token", "", "")
	if err := cmd.Flags().Set("scenario", "export"); err != nil {
		t.Fatalf("set --scenario: %v", err)
	}
	if err := cmd.Flags().Set("ticket", "tk_export"); err != nil {
		t.Fatalf("set --ticket: %v", err)
	}
	if err := cmd.Flags().Set("file-token", "doc_123"); err != nil {
		t.Fatalf("set --file-token: %v", err)
	}

	runtime := common.TestNewRuntimeContext(cmd, nil)
	dry := DriveTaskResult.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}

	var got struct {
		API []struct {
			Params map[string]interface{} `json:"params"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run json: %v", err)
	}
	if len(got.API) != 1 {
		t.Fatalf("expected 1 API call, got %d", len(got.API))
	}
	if got.API[0].Params["token"] != "doc_123" {
		t.Fatalf("export status params = %#v", got.API[0].Params)
	}
}

func TestDriveTaskResultImportIncludesReadyFlags(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/import_tasks/tk_import",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": map[string]interface{}{
					"type":       "sheet",
					"job_status": 2,
				},
			},
		},
	})

	err := mountAndRunDrive(t, DriveTaskResult, []string{
		"+task_result",
		"--scenario", "import",
		"--ticket", "tk_import",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"ready": false`)) {
		t.Fatalf("stdout missing ready=false: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"job_status_label": "processing"`)) {
		t.Fatalf("stdout missing job_status_label: %s", stdout.String())
	}
	if bytes.Contains(stdout.Bytes(), []byte(`"permission_grant"`)) {
		t.Fatalf("stdout should not include permission_grant before import is ready: %s", stdout.String())
	}
}

func TestDriveTaskResultImportBotAutoGrantSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, drivePermissionGrantTestConfig(t, "ou_current_user"))
	registerDriveBotTokenStub(reg)

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/import_tasks/tk_import_ready",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": map[string]interface{}{
					"type":       "sheet",
					"job_status": 0,
					"token":      "sheet_imported",
					"url":        "https://example.feishu.cn/sheets/sheet_imported",
				},
			},
		},
	})

	permStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/sheet_imported/members",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
	}
	reg.Register(permStub)

	err := mountAndRunDrive(t, DriveTaskResult, []string{
		"+task_result",
		"--scenario", "import",
		"--ticket", "tk_import_ready",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	grant, _ := data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantGranted {
		t.Fatalf("permission_grant.status = %#v, want %q", grant["status"], common.PermissionGrantGranted)
	}
	if grant["user_open_id"] != "ou_current_user" {
		t.Fatalf("permission_grant.user_open_id = %#v, want %q", grant["user_open_id"], "ou_current_user")
	}

	body := decodeCapturedJSONBody(t, permStub)
	if body["member_type"] != "openid" || body["member_id"] != "ou_current_user" || body["perm"] != "full_access" || body["type"] != "user" {
		t.Fatalf("unexpected permission request body: %#v", body)
	}
}

func TestDriveTaskResultTaskCheckIncludesReadyFlags(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/files/task_check",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"status": "pending"},
		},
	})

	err := mountAndRunDrive(t, DriveTaskResult, []string{
		"+task_result",
		"--scenario", "task_check",
		"--task-id", "task_123",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"status": "pending"`)) {
		t.Fatalf("stdout missing pending status: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"ready": false`)) {
		t.Fatalf("stdout missing ready=false: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"failed": false`)) {
		t.Fatalf("stdout missing failed=false: %s", stdout.String())
	}
}

func TestDriveTaskResultTaskCheckTreatsFailAsFailed(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/files/task_check",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"status": "fail"},
		},
	})

	err := mountAndRunDrive(t, DriveTaskResult, []string{
		"+task_result",
		"--scenario", "task_check",
		"--task-id", "task_123",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"status": "fail"`)) {
		t.Fatalf("stdout missing fail status: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"failed": true`)) {
		t.Fatalf("stdout missing failed=true: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"ready": false`)) {
		t.Fatalf("stdout missing ready=false: %s", stdout.String())
	}
}

type mockDriveTaskResultTokenResolver struct {
	token  string
	scopes string
	err    error
}

func (m *mockDriveTaskResultTokenResolver) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.TokenResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	token := m.token
	if token == "" {
		token = "test-token"
	}
	return &credential.TokenResult{Token: token, Scopes: m.scopes}, nil
}

func newDriveTaskResultRuntimeWithScopes(t *testing.T, as core.Identity, scopes string) *common.RuntimeContext {
	t.Helper()

	cfg := driveTestConfig()
	factory, _, _, _ := cmdutil.TestFactory(t, cfg)
	factory.Credential = credential.NewCredentialProvider(nil, nil, &mockDriveTaskResultTokenResolver{scopes: scopes}, nil)

	runtime := common.TestNewRuntimeContextWithIdentity(&cobra.Command{Use: "drive +task_result"}, cfg, as)
	runtime.Factory = factory
	return runtime
}

func TestDriveTaskResultDryRunWikiMoveIncludesTaskTypeParam(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +task_result"}
	cmd.Flags().String("scenario", "", "")
	cmd.Flags().String("ticket", "", "")
	cmd.Flags().String("task-id", "", "")
	cmd.Flags().String("file-token", "", "")
	if err := cmd.Flags().Set("scenario", "wiki_move"); err != nil {
		t.Fatalf("set --scenario: %v", err)
	}
	if err := cmd.Flags().Set("task-id", "task_123"); err != nil {
		t.Fatalf("set --task-id: %v", err)
	}

	runtime := common.TestNewRuntimeContext(cmd, nil)
	dry := DriveTaskResult.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}

	var got struct {
		API []struct {
			Params map[string]interface{} `json:"params"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run json: %v", err)
	}
	if len(got.API) != 1 {
		t.Fatalf("expected 1 API call, got %d", len(got.API))
	}
	if got.API[0].Params["task_type"] != "move" {
		t.Fatalf("wiki move params = %#v, want task_type=move", got.API[0].Params)
	}
}

func TestDriveTaskResultWikiMoveIncludesFlattenedNodeFields(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/tasks/task_123",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"task": map[string]interface{}{
					"task_id": "task_123",
					"move_result": []interface{}{
						map[string]interface{}{
							"status":     0,
							"status_msg": "success",
							"node": map[string]interface{}{
								"space_id":   "space_dst",
								"node_token": "wik_done",
								"obj_token":  "sheet_token",
								"obj_type":   "sheet",
								"node_type":  "origin",
								"title":      "Roadmap",
							},
						},
					},
				},
			},
		},
	})

	err := mountAndRunDrive(t, DriveTaskResult, []string{
		"+task_result",
		"--scenario", "wiki_move",
		"--task-id", "task_123",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if data["scenario"] != "wiki_move" || data["task_id"] != "task_123" {
		t.Fatalf("unexpected wiki_move envelope: %#v", data)
	}
	if data["ready"] != true || data["failed"] != false || data["wiki_token"] != "wik_done" {
		t.Fatalf("unexpected readiness fields: %#v", data)
	}
	if data["title"] != "Roadmap" || data["obj_type"] != "sheet" || data["space_id"] != "space_dst" {
		t.Fatalf("flattened node fields missing: %#v", data)
	}
	moveResults, ok := data["move_results"].([]interface{})
	if !ok || len(moveResults) != 1 {
		t.Fatalf("move_results = %#v, want one result", data["move_results"])
	}
}

func TestValidateDriveTaskResultScopesWikiScenariosRequireWikiScope(t *testing.T) {
	t.Parallel()

	// wiki_move and wiki_delete_space both read wiki task status, so both must
	// require wiki:space:read. A single table keeps this invariant explicit
	// without duplicating near-identical test functions per scenario.
	for _, scenario := range []string{"wiki_move", "wiki_delete_space"} {
		t.Run(scenario+"/rejects missing scope", func(t *testing.T) {
			t.Parallel()
			runtime := newDriveTaskResultRuntimeWithScopes(t, core.AsUser, "drive:drive.metadata:readonly")
			err := validateDriveTaskResultScopes(context.Background(), runtime, scenario)
			if err == nil || !strings.Contains(err.Error(), "missing required scope(s): wiki:space:read") {
				t.Fatalf("expected missing wiki scope error, got %v", err)
			}
		})
		t.Run(scenario+"/accepts wiki scope", func(t *testing.T) {
			t.Parallel()
			runtime := newDriveTaskResultRuntimeWithScopes(t, core.AsUser, "wiki:space:read")
			err := validateDriveTaskResultScopes(context.Background(), runtime, scenario)
			if err != nil {
				t.Fatalf("validateDriveTaskResultScopes() error = %v", err)
			}
		})
	}
}

func TestDriveTaskResultDryRunWikiDeleteSpaceIncludesTaskTypeParam(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +task_result"}
	cmd.Flags().String("scenario", "", "")
	cmd.Flags().String("ticket", "", "")
	cmd.Flags().String("task-id", "", "")
	cmd.Flags().String("file-token", "", "")
	if err := cmd.Flags().Set("scenario", "wiki_delete_space"); err != nil {
		t.Fatalf("set --scenario: %v", err)
	}
	if err := cmd.Flags().Set("task-id", "task_del_1"); err != nil {
		t.Fatalf("set --task-id: %v", err)
	}

	runtime := common.TestNewRuntimeContext(cmd, nil)
	dry := DriveTaskResult.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}

	var got struct {
		API []struct {
			Params map[string]interface{} `json:"params"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run json: %v", err)
	}
	if len(got.API) != 1 {
		t.Fatalf("expected 1 API call, got %d", len(got.API))
	}
	if got.API[0].Params["task_type"] != "delete_space" {
		t.Fatalf("wiki delete-space params = %#v, want task_type=delete_space", got.API[0].Params)
	}
}

func TestDriveTaskResultWikiDeleteSpaceSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/tasks/task_del_1",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"task": map[string]interface{}{
					"delete_space_result": map[string]interface{}{
						"status": "success",
					},
				},
			},
		},
	})

	err := mountAndRunDrive(t, DriveTaskResult, []string{
		"+task_result",
		"--scenario", "wiki_delete_space",
		"--task-id", "task_del_1",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if data["scenario"] != "wiki_delete_space" || data["task_id"] != "task_del_1" {
		t.Fatalf("unexpected wiki_delete_space envelope: %#v", data)
	}
	if data["ready"] != true || data["failed"] != false || data["status"] != "success" {
		t.Fatalf("unexpected readiness fields: %#v", data)
	}
}

func TestValidateDriveTaskResultScopesDriveScenariosRequireDriveScope(t *testing.T) {
	t.Parallel()

	runtime := newDriveTaskResultRuntimeWithScopes(t, core.AsUser, "wiki:space:read")
	err := validateDriveTaskResultScopes(context.Background(), runtime, "import")
	if err == nil || !strings.Contains(err.Error(), "missing required scope(s): drive:drive.metadata:readonly") {
		t.Fatalf("expected missing drive scope error, got %v", err)
	}
}

func TestParseWikiMoveTaskQueryStatusFallbackTaskIDAndNode(t *testing.T) {
	t.Parallel()

	status, err := parseWikiMoveTaskQueryStatus("task_fallback", map[string]interface{}{
		"move_result": []interface{}{
			map[string]interface{}{
				"status":     0,
				"status_msg": "success",
				"node": map[string]interface{}{
					"space_id":   "space_dst",
					"node_token": "wik_done",
					"obj_token":  "sheet_token",
					"obj_type":   "sheet",
					"title":      "Roadmap",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("parseWikiMoveTaskQueryStatus() error = %v", err)
	}
	if status.TaskID != "task_fallback" || !status.Ready() || status.PrimaryStatusLabel() != "success" {
		t.Fatalf("unexpected parsed status: %+v", status)
	}
	if first := status.FirstResult(); first == nil || first.Node == nil || first.Node["node_token"] != "wik_done" {
		t.Fatalf("parsed node = %+v", first)
	}
}

func TestParseWikiMoveTaskQueryStatusRejectsMissingTask(t *testing.T) {
	t.Parallel()

	_, err := parseWikiMoveTaskQueryStatus("task_123", nil)
	if err == nil || !strings.Contains(err.Error(), "missing task") {
		t.Fatalf("expected missing task error, got %v", err)
	}
}

func TestWikiMoveTaskQueryStatusPrimarySurfacesFailureOverEarlierSuccess(t *testing.T) {
	t.Parallel()

	status := wikiMoveTaskQueryStatus{
		MoveResults: []wikiMoveTaskResultStatus{
			{Status: 0, StatusMsg: "success"},
			{Status: -3, StatusMsg: "permission denied"},
			{Status: 1, StatusMsg: "processing"},
		},
	}
	if got := status.PrimaryStatusCode(); got != -3 {
		t.Fatalf("PrimaryStatusCode = %d, want -3", got)
	}
	if got := status.PrimaryStatusLabel(); got != "permission denied" {
		t.Fatalf("PrimaryStatusLabel = %q, want permission denied", got)
	}
	// FirstResult must keep its literal "first entry" semantics for callers
	// that flatten node fields from the first move_result.
	if first := status.FirstResult(); first == nil || first.StatusMsg != "success" {
		t.Fatalf("FirstResult = %+v, want first success entry", first)
	}
}

func TestWikiMoveTaskQueryStatusPrimaryPrefersProcessingOverFirstSuccess(t *testing.T) {
	t.Parallel()

	status := wikiMoveTaskQueryStatus{
		MoveResults: []wikiMoveTaskResultStatus{
			{Status: 0, StatusMsg: "success"},
			{Status: 1, StatusMsg: "processing"},
		},
	}
	if got := status.PrimaryStatusCode(); got != 1 {
		t.Fatalf("PrimaryStatusCode = %d, want 1", got)
	}
	if got := status.PrimaryStatusLabel(); got != "processing" {
		t.Fatalf("PrimaryStatusLabel = %q, want processing", got)
	}
}

type cancelingTokenResolver struct{}

func (cancelingTokenResolver) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.TokenResult, error) {
	return nil, context.Canceled
}

func TestValidateDriveTaskResultScopesPropagatesContextCancellation(t *testing.T) {
	t.Parallel()

	cfg := driveTestConfig()
	factory, _, _, _ := cmdutil.TestFactory(t, cfg)
	factory.Credential = credential.NewCredentialProvider(nil, nil, cancelingTokenResolver{}, nil)

	runtime := common.TestNewRuntimeContextWithIdentity(&cobra.Command{Use: "drive +task_result"}, cfg, core.AsUser)
	runtime.Factory = factory

	err := validateDriveTaskResultScopes(context.Background(), runtime, "wiki_move")
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
