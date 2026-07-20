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

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
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
		{
			name: "wiki move to Drive missing task id",
			flags: map[string]string{
				"scenario": "wiki_move_to_drive",
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
			var vErr *errs.ValidationError
			if !errors.As(err, &vErr) {
				t.Fatalf("expected *errs.ValidationError, got %T", err)
			}
			if vErr.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("Subtype = %q, want %q", vErr.Subtype, errs.SubtypeInvalidArgument)
			}
			if got := output.ExitCodeOf(err); got != output.ExitValidation {
				t.Fatalf("exit code = %d, want ExitValidation (%d)", got, output.ExitValidation)
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

func TestDriveTaskResultDryRunWikiMoveToDriveIncludesTaskTypeParam(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +task_result"}
	cmd.Flags().String("scenario", "", "")
	cmd.Flags().String("ticket", "", "")
	cmd.Flags().String("task-id", "", "")
	cmd.Flags().String("file-token", "", "")
	if err := cmd.Flags().Set("scenario", "wiki_move_to_drive"); err != nil {
		t.Fatalf("set --scenario: %v", err)
	}
	if err := cmd.Flags().Set("task-id", "raw-task-signature"); err != nil {
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
	if len(got.API) != 1 || got.API[0].Params["task_type"] != "move_wiki_to_docs" {
		t.Fatalf("wiki move-to-drive dry run = %#v", got.API)
	}
}

func TestDriveTaskResultWikiMoveToDriveStatuses(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		statusMsg  string
		wantReady  bool
		wantFailed bool
	}{
		{name: "success", status: 0, statusMsg: "success", wantReady: true},
		{name: "processing fallback label", status: 1, wantReady: false},
		{name: "failure", status: -1, statusMsg: "failure", wantFailed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, _, registry := cmdutil.TestFactory(t, driveTestConfig())
			registry.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/wiki/v2/tasks/raw-task-signature",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"task": map[string]interface{}{
							// The external handler may omit task.task_id, so the
							// result must retain the signed request ID.
							"move_wiki_to_docs_result": map[string]interface{}{
								"status":     tt.status,
								"status_msg": tt.statusMsg,
								"obj_token":  "docxABC",
								"obj_type":   "docx",
								"url":        "https://example.feishu.cn/docx/docxABC",
							},
						},
					},
				},
			})

			err := mountAndRunDrive(t, DriveTaskResult, []string{
				"+task_result",
				"--scenario", "wiki_move_to_drive",
				"--task-id", "raw-task-signature",
				"--as", "user",
			}, factory, stdout)
			if err != nil {
				t.Fatalf("mountAndRunDrive() error = %v", err)
			}

			data := decodeDriveEnvelope(t, stdout)
			if data["scenario"] != "wiki_move_to_drive" || data["task_id"] != "raw-task-signature" {
				t.Fatalf("unexpected envelope = %#v", data)
			}
			if data["ready"] != tt.wantReady || data["failed"] != tt.wantFailed {
				t.Fatalf("readiness fields = %#v", data)
			}
			if tt.statusMsg == "" && data["status_msg"] != "processing" {
				t.Fatalf("status_msg = %#v, want processing fallback", data["status_msg"])
			}
			if data["obj_token"] != "docxABC" || data["obj_type"] != "docx" || data["url"] == "" {
				t.Fatalf("result fields = %#v", data)
			}
		})
	}
}

func TestDriveTaskResultWikiMoveToDriveRejectsMissingResult(t *testing.T) {
	factory, stdout, _, registry := cmdutil.TestFactory(t, driveTestConfig())
	registry.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/tasks/raw-task-signature",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"task": map[string]interface{}{}},
		},
	})

	err := mountAndRunDrive(t, DriveTaskResult, []string{
		"+task_result",
		"--scenario", "wiki_move_to_drive",
		"--task-id", "raw-task-signature",
		"--as", "user",
	}, factory, stdout)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("error = %T %v, want internal/invalid_response", err, err)
	}
}

func TestDriveTaskResultWikiMoveToDriveRejectsMalformedStatus(t *testing.T) {
	for name, rawStatus := range map[string]interface{}{
		"null":          nil,
		"string":        "processing",
		"fractional":    0.5,
		"unknown value": 2,
	} {
		t.Run(name, func(t *testing.T) {
			factory, stdout, _, registry := cmdutil.TestFactory(t, driveTestConfig())
			registry.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/wiki/v2/tasks/raw-task-signature",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"task": map[string]interface{}{
							"move_wiki_to_docs_result": map[string]interface{}{"status": rawStatus},
						},
					},
				},
			})

			err := mountAndRunDrive(t, DriveTaskResult, []string{
				"+task_result",
				"--scenario", "wiki_move_to_drive",
				"--task-id", "raw-task-signature",
				"--as", "user",
			}, factory, stdout)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("error = %T %v, want internal/invalid_response", err, err)
			}
		})
	}
}

func TestValidateDriveTaskResultScopesWikiScenariosRequireWikiScope(t *testing.T) {
	t.Parallel()

	// Every Wiki scenario reads Wiki task status, so all must require
	// wiki:space:read. A single table keeps this invariant explicit without
	// duplicating near-identical test functions.
	for _, scenario := range []string{"wiki_move", "wiki_move_to_drive", "wiki_delete_space", "wiki_delete_node"} {
		t.Run(scenario+"/rejects missing scope", func(t *testing.T) {
			t.Parallel()
			runtime := newDriveTaskResultRuntimeWithScopes(t, core.AsUser, "drive:drive.metadata:readonly")
			err := validateDriveTaskResultScopes(context.Background(), runtime, scenario)
			if err == nil || !strings.Contains(err.Error(), "missing required scope(s): wiki:space:read") {
				t.Fatalf("expected missing wiki scope error, got %v", err)
			}
			var permErr *errs.PermissionError
			if !errors.As(err, &permErr) {
				t.Fatalf("expected *errs.PermissionError, got %T", err)
			}
			if permErr.Subtype != errs.SubtypeMissingScope {
				t.Fatalf("Subtype = %q, want %q", permErr.Subtype, errs.SubtypeMissingScope)
			}
			if len(permErr.MissingScopes) != 1 || permErr.MissingScopes[0] != "wiki:space:read" {
				t.Fatalf("MissingScopes = %v, want [wiki:space:read]", permErr.MissingScopes)
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

func TestDriveTaskResultDryRunWikiDeleteNodeIncludesTaskTypeParam(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +task_result"}
	cmd.Flags().String("scenario", "", "")
	cmd.Flags().String("ticket", "", "")
	cmd.Flags().String("task-id", "", "")
	cmd.Flags().String("file-token", "", "")
	if err := cmd.Flags().Set("scenario", "wiki_delete_node"); err != nil {
		t.Fatalf("set --scenario: %v", err)
	}
	if err := cmd.Flags().Set("task-id", "task_del_node_1"); err != nil {
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
	if got.API[0].Params["task_type"] != "delete_node" {
		t.Fatalf("wiki delete-node params = %#v, want task_type=delete_node", got.API[0].Params)
	}
}

func TestDriveTaskResultWikiDeleteNodeSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/tasks/task_del_node_1",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"task": map[string]interface{}{
					// Gateway returns delete-node status under the generic
					// simple_task_result key (NOT delete_node_result), and it
					// carries only `status` (no status_msg).
					"simple_task_result": map[string]interface{}{
						"status": "success",
					},
				},
			},
		},
	})

	err := mountAndRunDrive(t, DriveTaskResult, []string{
		"+task_result",
		"--scenario", "wiki_delete_node",
		"--task-id", "task_del_node_1",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if data["scenario"] != "wiki_delete_node" || data["task_id"] != "task_del_node_1" {
		t.Fatalf("unexpected wiki_delete_node envelope: %#v", data)
	}
	if data["ready"] != true || data["failed"] != false || data["status"] != "success" {
		t.Fatalf("unexpected readiness fields: %#v", data)
	}
	// simple_task_result has no status_msg; label must fall back to status.
	if data["status_msg"] != "success" {
		t.Fatalf("status_msg = %#v, want fallback to status", data["status_msg"])
	}
}

func TestDriveTaskResultRejectsUnknownScenarioListsWikiDeleteNode(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	err := mountAndRunDrive(t, DriveTaskResult, []string{
		"+task_result",
		"--scenario", "bogus",
		"--task-id", "task_x",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "wiki_delete_node") {
		t.Fatalf("expected unsupported-scenario error listing wiki_delete_node, got %v", err)
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
	// A successful API call (code==0) that omits the `task` field is a
	// malformed RESPONSE, not a user error: classify as internal /
	// invalid_response (exit 5), not an API business error (exit 1).
	var iErr *errs.InternalError
	if !errors.As(err, &iErr) {
		t.Fatalf("expected *errs.InternalError, got %T", err)
	}
	if iErr.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("Subtype = %q, want %q", iErr.Subtype, errs.SubtypeInvalidResponse)
	}
	if got := output.ExitCodeOf(err); got != output.ExitInternal {
		t.Fatalf("exit code = %d, want ExitInternal (%d)", got, output.ExitInternal)
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
