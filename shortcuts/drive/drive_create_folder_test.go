// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestValidateDriveCreateFolderSpecRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		spec    driveCreateFolderSpec
		wantErr string
	}{
		{
			name:    "empty name",
			spec:    driveCreateFolderSpec{},
			wantErr: "--name must not be empty",
		},
		{
			name: "name too long",
			spec: driveCreateFolderSpec{
				Name: strings.Repeat("a", 257),
			},
			wantErr: "maximum of 256 bytes",
		},
		{
			name: "invalid folder token",
			spec: driveCreateFolderSpec{
				Name:        "Reports",
				FolderToken: "../bad",
			},
			wantErr: "--folder-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateDriveCreateFolderSpec(tt.spec)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestDriveCreateFolderDryRunIncludesCreateRequest(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +create-folder"}
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("folder-token", "", "")
	if err := cmd.Flags().Set("name", " Weekly Reports "); err != nil {
		t.Fatalf("set --name: %v", err)
	}
	if err := cmd.Flags().Set("folder-token", " fld_parent "); err != nil {
		t.Fatalf("set --folder-token: %v", err)
	}

	runtime := common.TestNewRuntimeContextWithIdentity(cmd, nil, core.AsBot)
	dry := DriveCreateFolder.DryRun(context.Background(), runtime)
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
			URL    string                 `json:"url"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run json: %v", err)
	}
	if len(got.API) != 1 {
		t.Fatalf("expected 1 API call, got %d", len(got.API))
	}
	if got.API[0].Method != "POST" || got.API[0].URL != "/open-apis/drive/v1/files/create_folder" {
		t.Fatalf("unexpected dry-run API call: %#v", got.API[0])
	}
	if got.API[0].Body["name"] != "Weekly Reports" {
		t.Fatalf("name = %#v, want %q", got.API[0].Body["name"], "Weekly Reports")
	}
	if got.API[0].Body["folder_token"] != "fld_parent" {
		t.Fatalf("folder_token = %#v, want %q", got.API[0].Body["folder_token"], "fld_parent")
	}
}

func TestDriveCreateFolderBotAutoGrantSuccess(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, drivePermissionGrantTestConfig(t, "ou_current_user"))

	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/create_folder",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"token": "fld_created",
				"url":   "https://example.feishu.cn/drive/folder/fld_created",
			},
		},
	}
	reg.Register(createStub)

	permStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/fld_created/members",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
	}
	reg.Register(permStub)

	err := mountAndRunDrive(t, DriveCreateFolder, []string{
		"+create-folder",
		"--name", " Weekly Reports ",
		"--folder-token", " fld_parent ",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeCapturedJSONBody(t, createStub)
	if body["name"] != "Weekly Reports" {
		t.Fatalf("name = %#v, want %q", body["name"], "Weekly Reports")
	}
	if body["folder_token"] != "fld_parent" {
		t.Fatalf("folder_token = %#v, want %q", body["folder_token"], "fld_parent")
	}

	data := decodeDriveEnvelope(t, stdout)
	if data["folder_token"] != "fld_created" {
		t.Fatalf("folder_token = %#v, want %q", data["folder_token"], "fld_created")
	}
	if data["parent_folder_token"] != "fld_parent" {
		t.Fatalf("parent_folder_token = %#v, want %q", data["parent_folder_token"], "fld_parent")
	}
	if data["name"] != "Weekly Reports" {
		t.Fatalf("name = %#v, want %q", data["name"], "Weekly Reports")
	}
	if data["url"] != "https://example.feishu.cn/drive/folder/fld_created" {
		t.Fatalf("url = %#v, want folder url", data["url"])
	}

	grant, _ := data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantGranted {
		t.Fatalf("permission_grant.status = %#v, want %q", grant["status"], common.PermissionGrantGranted)
	}
	if grant["user_open_id"] != "ou_current_user" {
		t.Fatalf("permission_grant.user_open_id = %#v, want %q", grant["user_open_id"], "ou_current_user")
	}
	if grant["message"] != "Granted the current CLI user full_access (可管理权限) on the new folder." {
		t.Fatalf("permission_grant.message = %#v", grant["message"])
	}

	permBody := decodeCapturedJSONBody(t, permStub)
	if permBody["member_type"] != "openid" || permBody["member_id"] != "ou_current_user" || permBody["perm"] != "full_access" || permBody["type"] != "user" {
		t.Fatalf("unexpected permission request body: %#v", permBody)
	}
}

func TestDriveCreateFolderUsesRootWhenParentIsOmitted(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, drivePermissionGrantTestConfig(t, "ou_current_user"))

	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/create_folder",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"token": "fld_root_child",
			},
		},
	}
	reg.Register(createStub)

	err := mountAndRunDrive(t, DriveCreateFolder, []string{
		"+create-folder",
		"--name", "Inbox",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeCapturedJSONBody(t, createStub)
	if body["folder_token"] != "" {
		t.Fatalf("folder_token = %#v, want empty string for root create", body["folder_token"])
	}

	data := decodeDriveEnvelope(t, stdout)
	if data["folder_token"] != "fld_root_child" {
		t.Fatalf("folder_token = %#v, want %q", data["folder_token"], "fld_root_child")
	}
	if data["parent_folder_token"] != "" {
		t.Fatalf("parent_folder_token = %#v, want empty string", data["parent_folder_token"])
	}
	if _, ok := data["permission_grant"]; ok {
		t.Fatalf("did not expect permission_grant in user mode output: %#v", data)
	}
}

func TestDriveCreateFolderRejectsCreateResponseWithoutToken(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, drivePermissionGrantTestConfig(t, "ou_current_user"))

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/create_folder",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"url": "https://example.feishu.cn/drive/folder/unknown",
			},
		},
	})

	err := mountAndRunDrive(t, DriveCreateFolder, []string{
		"+create-folder",
		"--name", "Broken Folder",
		"--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "returned no folder token") {
		t.Fatalf("err = %v, want missing folder token error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout should be empty on error, got %s", stdout.String())
	}
}
