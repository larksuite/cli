// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestDriveUploadBotAutoGrantSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, drivePermissionGrantTestConfig(t, "ou_current_user"))
	registerDriveBotTokenStub(reg)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"file_token": "file_uploaded",
			},
		},
	})

	permStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/file_uploaded/members",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
	}
	reg.Register(permStub)

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.WriteFile("report.pdf", []byte("pdf"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDrive(t, DriveUpload, []string{
		"+upload",
		"--file", "report.pdf",
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
	if grant["message"] != "Granted the current CLI user full_access (可管理权限) on the new file." {
		t.Fatalf("permission_grant.message = %#v", grant["message"])
	}

	body := decodeCapturedJSONBody(t, permStub)
	if body["member_type"] != "openid" || body["member_id"] != "ou_current_user" || body["perm"] != "full_access" || body["type"] != "user" {
		t.Fatalf("unexpected permission request body: %#v", body)
	}
}

func TestDriveImportBotAutoGrantSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, drivePermissionGrantTestConfig(t, "ou_current_user"))
	registerDriveBotTokenStub(reg)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"file_token": "file_media",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/import_tasks",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"ticket": "tk_import",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/import_tasks/tk_import",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"result": map[string]interface{}{
					"type":       "docx",
					"job_status": 0,
					"token":      "doxcn_imported",
					"url":        "https://example.feishu.cn/docx/doxcn_imported",
				},
			},
		},
	})

	permStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/doxcn_imported/members",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
	}
	reg.Register(permStub)

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.WriteFile("README.md", []byte("# Title"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	prevAttempts, prevInterval := driveImportPollAttempts, driveImportPollInterval
	driveImportPollAttempts, driveImportPollInterval = 1, 0
	t.Cleanup(func() {
		driveImportPollAttempts, driveImportPollInterval = prevAttempts, prevInterval
	})

	err := mountAndRunDrive(t, DriveImport, []string{
		"+import",
		"--file", "README.md",
		"--type", "docx",
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

func TestDriveUploadUserSkipsPermissionGrantAugmentation(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, drivePermissionGrantTestConfig(t, "ou_current_user"))
	registerDriveBotTokenStub(reg)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"file_token": "file_uploaded",
			},
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.WriteFile("report.pdf", []byte("pdf"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDrive(t, DriveUpload, []string{
		"+upload",
		"--file", "report.pdf",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if _, ok := data["permission_grant"]; ok {
		t.Fatalf("did not expect permission_grant in user mode output: %#v", data)
	}
}

func drivePermissionGrantTestConfig(t *testing.T, userOpenID string) *core.CliConfig {
	t.Helper()

	replacer := strings.NewReplacer("/", "-", " ", "-")
	suffix := replacer.Replace(strings.ToLower(t.Name()))
	return &core.CliConfig{
		AppID:      "drive-permission-test-" + suffix,
		AppSecret:  "drive-permission-secret-" + suffix,
		Brand:      core.BrandFeishu,
		UserOpenId: userOpenID,
	}
}

func decodeDriveEnvelope(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
	t.Helper()

	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode output: %v\nraw=%s", err, stdout.String())
	}
	data, _ := envelope["data"].(map[string]interface{})
	if data == nil {
		t.Fatalf("missing data in output envelope: %#v", envelope)
	}
	return data
}

func registerDriveBotTokenStub(reg *httpmock.Registry) {
	_ = reg
}

func decodeCapturedJSONBody(t *testing.T, stub *httpmock.Stub) map[string]interface{} {
	t.Helper()

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("failed to decode captured request body: %v\nraw=%s", err, string(stub.CapturedBody))
	}
	return body
}
