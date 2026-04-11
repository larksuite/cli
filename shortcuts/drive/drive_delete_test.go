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

func TestValidateDriveDeleteSpecRejectsWiki(t *testing.T) {
	t.Parallel()

	err := validateDriveDeleteSpec(driveDeleteSpec{
		FileToken: "wiki_token_test",
		FileType:  "wiki",
	})
	if err == nil {
		t.Fatal("expected wiki type error, got nil")
	}
	if !strings.Contains(err.Error(), "wiki documents are not supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDriveDeleteDryRunFolderIncludesTaskCheckParams(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +delete"}
	cmd.Flags().String("file-token", "", "")
	cmd.Flags().String("type", "", "")
	if err := cmd.Flags().Set("file-token", "fld_src"); err != nil {
		t.Fatalf("set --file-token: %v", err)
	}
	if err := cmd.Flags().Set("type", "folder"); err != nil {
		t.Fatalf("set --type: %v", err)
	}

	runtime := common.TestNewRuntimeContext(cmd, nil)
	dry := DriveDelete.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}

	var got struct {
		API []struct {
			Method string                 `json:"method"`
			Params map[string]interface{} `json:"params"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run json: %v", err)
	}
	if len(got.API) != 2 {
		t.Fatalf("expected 2 API calls, got %d", len(got.API))
	}
	if got.API[0].Method != "DELETE" {
		t.Fatalf("first method = %q, want DELETE", got.API[0].Method)
	}
	if got.API[0].Params["type"] != "folder" {
		t.Fatalf("delete params = %#v", got.API[0].Params)
	}
	if got.API[1].Params["task_id"] != "<task_id>" {
		t.Fatalf("task check params = %#v", got.API[1].Params)
	}
}

func TestDriveDeleteRequiresYes(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	err := mountAndRunDrive(t, DriveDelete, []string{
		"+delete",
		"--file-token", "file_token_test",
		"--type", "file",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected confirmation error, got nil")
	}
	if !strings.Contains(err.Error(), "requires confirmation") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDriveDeleteFileSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/drive/v1/files/file_token_test",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRunDrive(t, DriveDelete, []string{
		"+delete",
		"--file-token", "file_token_test",
		"--type", "file",
		"--yes",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"deleted": true`)) {
		t.Fatalf("stdout missing deleted=true: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"file_token": "file_token_test"`)) {
		t.Fatalf("stdout missing file token: %s", stdout.String())
	}
}

func TestDriveDeleteFolderTaskCheckOutcomes(t *testing.T) {
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
				`"deleted": true`,
				`"ready": true`,
			},
		},
		{
			name: "timeout",
			taskCheckBody: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"status": "process"},
			},
			wantStdout: []string{
				`"ready": false`,
				`"timed_out": true`,
				`"next_command": "lark-cli drive +task_result --scenario task_check --task-id task_123 --as bot"`,
			},
		},
		{
			name: "failed",
			taskCheckBody: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"status": "fail"},
			},
			wantErrContains: "folder task failed",
		},
		{
			name: "task_check error",
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
				Method: "DELETE",
				URL:    "/open-apis/drive/v1/files/fld_src",
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

			err := mountAndRunDrive(t, DriveDelete, []string{
				"+delete",
				"--file-token", "fld_src",
				"--type", "folder",
				"--yes",
				"--as", "bot",
			}, f, stdout)

			if tt.wantErrContains != "" {
				if err == nil {
					t.Fatal("expected delete failure, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErrContains) {
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
