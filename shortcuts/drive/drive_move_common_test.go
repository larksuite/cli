// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestParseDriveTaskCheckStatusFallback(t *testing.T) {
	t.Parallel()

	status := parseDriveTaskCheckStatus("task_123", map[string]interface{}{
		"status": "success",
	})

	if !status.Ready() {
		t.Fatal("expected task check status to be ready")
	}
	if status.StatusLabel() != "success" {
		t.Fatalf("status label = %q, want %q", status.StatusLabel(), "success")
	}
}

func TestDriveTaskCheckStatusPendingAndUnknownLabel(t *testing.T) {
	t.Parallel()

	status := driveTaskCheckStatus{}
	if !status.Pending() {
		t.Fatal("expected empty status to be treated as pending")
	}
	if got := status.StatusLabel(); got != "unknown" {
		t.Fatalf("StatusLabel() = %q, want %q", got, "unknown")
	}
}

func TestValidateDriveMoveSpecRejectsUnsupportedType(t *testing.T) {
	t.Parallel()

	err := validateDriveMoveSpec(driveMoveSpec{
		FileToken: "file_token_test",
		FileType:  "unsupported_type",
	})
	if err == nil {
		t.Fatal("expected unsupported type error, got nil")
	}
	if got := err.Error(); !bytes.Contains([]byte(got), []byte("unsupported file type")) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDriveMoveDryRunFolderIncludesTaskCheckParams(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +move"}
	cmd.Flags().String("file-token", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().String("folder-token", "", "")
	if err := cmd.Flags().Set("file-token", "fld_src"); err != nil {
		t.Fatalf("set --file-token: %v", err)
	}
	if err := cmd.Flags().Set("type", "folder"); err != nil {
		t.Fatalf("set --type: %v", err)
	}
	if err := cmd.Flags().Set("folder-token", "fld_dst"); err != nil {
		t.Fatalf("set --folder-token: %v", err)
	}

	runtime := common.TestNewRuntimeContext(cmd, nil)
	dry := DriveMove.DryRun(context.Background(), runtime)
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
	if len(got.API) != 2 {
		t.Fatalf("expected 2 API calls, got %d", len(got.API))
	}
	if got.API[1].Params["task_id"] != "<task_id>" {
		t.Fatalf("task check params = %#v", got.API[1].Params)
	}
}

func TestDriveMoveFolderTaskCheckOutcomes(t *testing.T) {
	tests := []struct {
		name            string
		taskCheckBody   map[string]interface{}
		wantErrContains string
		wantStdout      []string
	}{
		{
			name: "success",
			taskCheckBody: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"status": "success"},
			},
			wantStdout: []string{
				`"task_id": "task_123"`,
				`"ready": true`,
			},
		},
		{
			name: "timeout",
			taskCheckBody: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"status": "pending"},
			},
			wantStdout: []string{
				`"ready": false`,
				`"timed_out": true`,
				`"next_command": "lark-cli drive +task_result --scenario task_check --task-id task_123 --as bot"`,
			},
		},
		{
			name: "all polls fail",
			taskCheckBody: map[string]interface{}{
				"code": 1061001,
				"msg":  "internal error",
			},
			wantErrContains: "internal error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/drive/v1/files/fld_src/move",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"task_id": "task_123"},
				},
			})
			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/drive/v1/files/task_check",
				Body:   tt.taskCheckBody,
			})

			withSingleDriveTaskCheckPoll(t)

			err := mountAndRunDrive(t, DriveMove, []string{
				"+move",
				"--file-token", "fld_src",
				"--type", "folder",
				"--folder-token", "fld_dst",
				"--as", "bot",
			}, f, stdout)

			if tt.wantErrContains != "" {
				if err == nil {
					t.Fatal("expected task_check polling error, got nil")
				}
				if !bytes.Contains([]byte(err.Error()), []byte(tt.wantErrContains)) {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, needle := range tt.wantStdout {
				if !bytes.Contains(stdout.Bytes(), []byte(needle)) {
					t.Fatalf("stdout missing %q: %s", needle, stdout.String())
				}
			}
		})
	}
}
