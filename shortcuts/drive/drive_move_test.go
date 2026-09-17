// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestDriveMoveUsesRootFolderWhenFolderTokenMissing(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/explorer/v2/root_folder/meta",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"token": "folder_root_token_test",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/file_token_test/move",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRunDrive(t, DriveMove, []string{
		"+move",
		"--file-token", "file_token_test",
		"--type", "file",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), `"folder_token": "folder_root_token_test"`) {
		t.Fatalf("stdout missing resolved root folder token: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"file_token": "file_token_test"`) {
		t.Fatalf("stdout missing file token: %s", stdout.String())
	}
}

func TestDriveMoveRootFolderLookupRequiresToken(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/explorer/v2/root_folder/meta",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRunDrive(t, DriveMove, []string{
		"+move",
		"--file-token", "file_token_test",
		"--type", "file",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected missing root folder token error, got nil")
	}
	if !strings.Contains(err.Error(), "root_folder/meta returned no token") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDriveMoveFolderWithoutTaskCompletesSynchronously(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]interface{}
		identity string
		useRoot  bool
		apiCode  int
	}{
		{name: "empty data", data: map[string]interface{}{}, identity: "bot"},
		{name: "missing data", identity: "bot"},
		{name: "empty task id", data: map[string]interface{}{"task_id": ""}, identity: "bot"},
		{name: "null task id", data: map[string]interface{}{"task_id": nil}, identity: "bot"},
		{name: "user explicit destination", data: map[string]interface{}{}, identity: "user"},
		{name: "user default root", data: map[string]interface{}{}, identity: "user", useRoot: true},
		{name: "API error without task is preserved", data: map[string]interface{}{}, identity: "bot", apiCode: 1061004},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
			if tt.useRoot {
				reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/drive/explorer/v2/root_folder/meta", Body: map[string]interface{}{
					"code": 0, "data": map[string]interface{}{"token": "fld_dst"},
				}})
			}
			response := map[string]interface{}{"code": tt.apiCode, "log_id": "move-log-id"}
			if tt.data != nil {
				response["data"] = tt.data
			}
			// No metadata, listing, or task stubs: any follow-up request fails.
			move := &httpmock.Stub{Method: "POST", URL: "/open-apis/drive/v1/files/fld_src/move", Body: response}
			reg.Register(move)
			args := []string{"+move", "--file-token", "fld_src", "--type", "folder", "--as", tt.identity}
			if !tt.useRoot {
				args = append(args, "--folder-token", "fld_dst")
			}
			err := mountAndRunDrive(t, DriveMove, args, f, stdout)
			var body struct {
				Type        string `json:"type"`
				FolderToken string `json:"folder_token"`
			}
			if decodeErr := json.Unmarshal(move.CapturedBody, &body); decodeErr != nil {
				t.Fatalf("decode move request: %v", decodeErr)
			}
			if body.Type != "folder" || body.FolderToken != "fld_dst" {
				t.Fatalf("unexpected move request: %+v", body)
			}
			if tt.apiCode != 0 {
				problem, ok := errs.ProblemOf(err)
				if !ok || problem.Category != errs.CategoryAuthorization || problem.Subtype != errs.SubtypePermissionDenied || problem.Code != tt.apiCode || problem.LogID != "move-log-id" {
					t.Fatalf("move error was not preserved: %v", err)
				}
				if stdout.Len() != 0 {
					t.Fatalf("failure emitted success output: %s", stdout.String())
				}
				return
			}
			if err != nil {
				t.Fatalf("move failed: %v", err)
			}
			var envelope struct {
				OK       bool                   `json:"ok"`
				Identity string                 `json:"identity"`
				Data     map[string]interface{} `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatalf("decode output: %v", err)
			}
			if !envelope.OK || envelope.Identity != tt.identity || envelope.Data["ready"] != true || envelope.Data["status"] != "success" || envelope.Data["file_token"] != "fld_src" || envelope.Data["folder_token"] != "fld_dst" {
				t.Fatalf("unexpected synchronous output: %s", stdout.String())
			}
			for _, key := range []string{"task_id", "timed_out", "next_command", "already_at_destination"} {
				if _, ok := envelope.Data[key]; ok {
					t.Errorf("synchronous output must not contain %q: %s", key, stdout.String())
				}
			}
		})
	}
}

func TestDriveMoveFolderRejectsInvalidTaskID(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/drive/v1/files/fld_src/move", Body: map[string]interface{}{
		"code": 0, "data": map[string]interface{}{"task_id": 123},
	}})
	err := mountAndRunDrive(t, DriveMove, []string{"+move", "--file-token", "fld_src", "--type", "folder", "--folder-token", "fld_dst", "--as", "bot"}, f, stdout)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("expected internal invalid_response, got %v", err)
	}
	var cause *json.UnmarshalTypeError
	if !errors.As(err, &cause) {
		t.Fatalf("missing decode cause: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("invalid task response emitted output: %s", stdout.String())
	}
}
