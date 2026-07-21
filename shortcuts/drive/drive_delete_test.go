// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
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

func TestDriveDeleteDryRunIncludesAsyncAndTaskCheckParams(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +delete"}
	cmd.Flags().String("file-token", "", "")
	cmd.Flags().String("type", "", "")
	if err := cmd.Flags().Set("file-token", "docx_src"); err != nil {
		t.Fatalf("set --file-token: %v", err)
	}
	if err := cmd.Flags().Set("type", "docx"); err != nil {
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
	if got.API[0].Params["type"] != "docx" {
		t.Fatalf("delete params = %#v", got.API[0].Params)
	}
	if got.API[0].Params["async"] != true {
		t.Fatalf("delete params = %#v, want async=true", got.API[0].Params)
	}
	if got.API[1].Params["task_id"] != "<task_id>" {
		t.Fatalf("task check params = %#v", got.API[1].Params)
	}
}

func TestDriveDeleteScopesIncludeTaskCheckReadScope(t *testing.T) {
	t.Parallel()

	wantScopes := map[string]bool{
		"space:document:delete":         false,
		"drive:drive.metadata:readonly": false,
	}
	for _, scope := range DriveDelete.Scopes {
		if _, ok := wantScopes[scope]; ok {
			wantScopes[scope] = true
		}
	}
	for scope, seen := range wantScopes {
		if !seen {
			t.Fatalf("DriveDelete.Scopes missing %q: %#v", scope, DriveDelete.Scopes)
		}
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
			"data": map[string]interface{}{"task_id": "task_file_123"},
		},
		OnMatch: func(req *http.Request) {
			query := req.URL.Query()
			if got := query.Get("type"); got != "file" {
				t.Errorf("delete query type=%q, want file", got)
			}
			if got := query.Get("async"); got != "true" {
				t.Errorf("delete query async=%q, want true", got)
			}
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/files/task_check",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"status": "success"},
		},
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("task_id"); got != "task_file_123" {
				t.Errorf("task_check task_id=%q, want task_file_123", got)
			}
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
	if !bytes.Contains(stdout.Bytes(), []byte(`"task_id": "task_file_123"`)) {
		t.Fatalf("stdout missing task_id: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"deleted": true`)) {
		t.Fatalf("stdout missing deleted=true: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"ready": true`)) {
		t.Fatalf("stdout missing ready=true: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"file_token": "file_token_test"`)) {
		t.Fatalf("stdout missing file token: %s", stdout.String())
	}
}

func TestDriveDeleteWithoutTaskIDFallsBackToSyncSuccess(t *testing.T) {
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
	for _, needle := range []string{
		`"deleted": true`,
		`"file_token": "file_token_test"`,
		`"type": "file"`,
	} {
		if !bytes.Contains(stdout.Bytes(), []byte(needle)) {
			t.Fatalf("stdout missing %q: %s", needle, stdout.String())
		}
	}
	if bytes.Contains(stdout.Bytes(), []byte(`"task_id"`)) {
		t.Fatalf("stdout should not include task_id for sync success fallback: %s", stdout.String())
	}
}

func TestDriveDeleteTaskCheckOutcomes(t *testing.T) {
	tests := []struct {
		name            string
		fileType        string
		fileToken       string
		taskCheckBody   map[string]interface{}
		wantErrContains string
		wantStdout      []string
	}{
		{
			name:      "docx success",
			fileType:  "docx",
			fileToken: "docx_src",
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
			name:      "folder timeout",
			fileType:  "folder",
			fileToken: "fld_src",
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
			name:      "folder failed",
			fileType:  "folder",
			fileToken: "fld_src",
			taskCheckBody: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"status": "fail"},
			},
			wantErrContains: "drive task failed",
		},
		{
			name:      "docx task_check error",
			fileType:  "docx",
			fileToken: "docx_src",
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
				URL:    "/open-apis/drive/v1/files/" + tt.fileToken,
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
				"--file-token", tt.fileToken,
				"--type", tt.fileType,
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

func TestDriveDeleteTimedOutTaskCanBeResumedWithTaskResult(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/drive/v1/files/fld_token_test",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"task_id": "task_resume_123"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/files/task_check",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"status": "process"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/files/task_check",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"status": "success"},
		},
	})

	withSingleDriveTaskCheckPoll(t)

	err := mountAndRunDrive(t, DriveDelete, []string{
		"+delete",
		"--file-token", "fld_token_test",
		"--type", "folder",
		"--yes",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected delete error: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"ready": false`)) {
		t.Fatalf("stdout missing ready=false: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"next_command": "lark-cli drive +task_result --scenario task_check --task-id task_resume_123 --as bot"`)) {
		t.Fatalf("stdout missing next_command: %s", stdout.String())
	}

	err = mountAndRunDrive(t, DriveTaskResult, []string{
		"+task_result",
		"--scenario", "task_check",
		"--task-id", "task_resume_123",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected task_result error: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"task_id": "task_resume_123"`)) {
		t.Fatalf("task_result stdout missing task_id: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"ready": true`)) {
		t.Fatalf("task_result stdout missing ready=true: %s", stdout.String())
	}
}
