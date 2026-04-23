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

func TestValidateDriveImportSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		spec    driveImportSpec
		wantErr string
	}{
		{
			name:    "xlsx as docx rejected",
			spec:    driveImportSpec{FilePath: "./data.xlsx", DocType: "docx"},
			wantErr: "file type mismatch",
		},
		{
			name:    "xls bitable rejected",
			spec:    driveImportSpec{FilePath: "./data.xls", DocType: "bitable"},
			wantErr: ".xls files can only be imported as 'sheet'",
		},
		{
			name: "base bitable ok",
			spec: driveImportSpec{FilePath: "./snapshot.base", DocType: "bitable"},
		},
		{
			name:    "base non bitable rejected",
			spec:    driveImportSpec{FilePath: "./snapshot.base", DocType: "sheet"},
			wantErr: ".base files can only be imported as 'bitable'",
		},
		{
			name:    "unknown extension rejected",
			spec:    driveImportSpec{FilePath: "./data.rtf", DocType: "docx"},
			wantErr: "unsupported file extension",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateDriveImportSpec(tt.spec)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestValidateDriveImportFileSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filePath string
		docType  string
		fileSize int64
		wantText string
	}{
		{
			name:     "docx exceeds 600mb limit",
			filePath: "./report.docx",
			docType:  "docx",
			fileSize: driveImport600MBFileSizeLimit + 1,
			wantText: "exceeds 600.0 MB import limit for .docx",
		},
		{
			name:     "csv sheet exceeds 20mb limit",
			filePath: "./data.csv",
			docType:  "sheet",
			fileSize: driveImport20MBFileSizeLimit + 1,
			wantText: "exceeds 20.0 MB import limit for .csv when importing as sheet",
		},
		{
			name:     "csv bitable exceeds 100mb limit",
			filePath: "./data.csv",
			docType:  "bitable",
			fileSize: driveImport100MBFileSizeLimit + 1,
			wantText: "exceeds 100.0 MB import limit for .csv when importing as bitable",
		},
		{
			name:     "xlsx within 800mb limit",
			filePath: "./data.xlsx",
			docType:  "sheet",
			fileSize: driveImport800MBFileSizeLimit,
		},
		{
			name:     "base exceeds 20mb limit",
			filePath: "./snapshot.base",
			docType:  "bitable",
			fileSize: driveImport20MBFileSizeLimit + 1,
			wantText: "exceeds 20.0 MB import limit for .base",
		},
		{
			name:     "base within 20mb limit",
			filePath: "./snapshot.base",
			docType:  "bitable",
			fileSize: driveImport20MBFileSizeLimit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateDriveImportFileSize(tt.filePath, tt.docType, tt.fileSize)
			if tt.wantText == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantText)
			}
		})
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
	if bytes.Contains(stdout.Bytes(), []byte(`"permission_grant"`)) {
		t.Fatalf("stdout should not include permission_grant before import is ready: %s", stdout.String())
	}
}

func TestDriveImportRejectsOversizedFileByImportLimit(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	writeSizedDriveImportFile(t, "too-large.csv", driveImport100MBFileSizeLimit+1)

	err := mountAndRunDrive(t, DriveImport, []string{
		"+import",
		"--file", "too-large.csv",
		"--type", "bitable",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected size limit error, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds 100.0 MB import limit for .csv when importing as bitable") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDriveImportRejectsOversizedBaseFile(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	writeSizedDriveImportFile(t, "too-large.base", driveImport20MBFileSizeLimit+1)

	err := mountAndRunDrive(t, DriveImport, []string{
		"+import",
		"--file", "too-large.base",
		"--type", "bitable",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected size limit error, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds 20.0 MB import limit for .base") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeSizedDriveImportFile(t *testing.T, name string, size int64) {
	t.Helper()

	fh, err := os.Create(name)
	if err != nil {
		t.Fatalf("Create(%q) error: %v", name, err)
	}
	if err := fh.Truncate(size); err != nil {
		t.Fatalf("Truncate(%q) error: %v", name, err)
	}
	if err := fh.Close(); err != nil {
		t.Fatalf("Close(%q) error: %v", name, err)
	}
}
