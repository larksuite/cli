// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestValidateDriveImportSpecRejectsMismatchedType(t *testing.T) {
	t.Parallel()

	err := validateDriveImportSpec(driveImportSpec{
		FilePath: "./data.xlsx",
		DocType:  "docx",
	})
	if err == nil || !strings.Contains(err.Error(), "file type mismatch") {
		t.Fatalf("expected file type mismatch error, got %v", err)
	}
}

func TestParseDriveImportStatus(t *testing.T) {
	t.Parallel()

	status := parseDriveImportStatus("tk_123", map[string]interface{}{
		"result": map[string]interface{}{
			"type":          "sheet",
			"job_status":    0,
			"job_error_msg": "",
			"token":         "sheet_123",
			"url":           "https://example.com/sheets/sheet_123",
			"extra":         []interface{}{"2000"},
		},
	})

	if !status.Ready() {
		t.Fatal("expected import status to be ready")
	}
	if status.StatusLabel() != "success" {
		t.Fatalf("status label = %q, want %q", status.StatusLabel(), "success")
	}
	if status.Token != "sheet_123" {
		t.Fatalf("token = %q, want %q", status.Token, "sheet_123")
	}
}

func TestDriveImportStatusPendingWithoutToken(t *testing.T) {
	t.Parallel()

	status := driveImportStatus{JobStatus: 0}
	if status.Ready() {
		t.Fatal("expected status without token to be not ready")
	}
	if !status.Pending() {
		t.Fatal("expected status without token to be pending")
	}
	if got := status.StatusLabel(); got != "pending" {
		t.Fatalf("StatusLabel() = %q, want %q", got, "pending")
	}
}

func TestDriveImportTimeoutReturnsFollowUpCommand(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	registerDriveBotTokenStub(reg)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"file_token": "file_123"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/import_tasks",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"ticket": "tk_import"},
		},
	})
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

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.WriteFile("data.xlsx", []byte("fake-xlsx"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	prevAttempts, prevInterval := driveImportPollAttempts, driveImportPollInterval
	driveImportPollAttempts, driveImportPollInterval = 1, 0
	t.Cleanup(func() {
		driveImportPollAttempts, driveImportPollInterval = prevAttempts, prevInterval
	})

	err := mountAndRunDrive(t, DriveImport, []string{
		"+import",
		"--file", "data.xlsx",
		"--type", "sheet",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"ready": false`)) {
		t.Fatalf("stdout missing ready=false: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"timed_out": true`)) {
		t.Fatalf("stdout missing timed_out=true: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"next_command": "lark-cli drive +task_result --scenario import --ticket tk_import"`)) {
		t.Fatalf("stdout missing follow-up command: %s", stdout.String())
	}
}
