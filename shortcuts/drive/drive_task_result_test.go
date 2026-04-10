// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
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
