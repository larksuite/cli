// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/vfs/localfileio"
)

func TestValidateDriveExportSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		spec    driveExportSpec
		wantErr string
	}{
		{
			name: "markdown docx ok",
			spec: driveExportSpec{Token: "docx123", DocType: "docx", FileExtension: "markdown"},
		},
		{
			name:    "markdown non docx rejected",
			spec:    driveExportSpec{Token: "doc123", DocType: "doc", FileExtension: "markdown"},
			wantErr: "only supports --doc-type docx",
		},
		{
			name:    "csv without sub id rejected",
			spec:    driveExportSpec{Token: "sheet123", DocType: "sheet", FileExtension: "csv"},
			wantErr: "--sub-id is required",
		},
		{
			name:    "sub id on non csv rejected",
			spec:    driveExportSpec{Token: "docx123", DocType: "docx", FileExtension: "pdf", SubID: "tbl_1"},
			wantErr: "--sub-id is only used",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateDriveExportSpec(tt.spec)
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

func TestDriveExportMarkdownWritesFile(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docs/v1/content",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"content": "# hello\n",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"title": "Weekly Notes"},
				},
			},
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	err := mountAndRunDrive(t, DriveExport, []string{
		"+export",
		"--token", "docx123",
		"--doc-type", "docx",
		"--file-extension", "markdown",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "Weekly Notes.md"))
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "# hello\n" {
		t.Fatalf("markdown content = %q", string(data))
	}
	if !strings.Contains(stdout.String(), "Weekly Notes.md") {
		t.Fatalf("stdout missing file name: %s", stdout.String())
	}
}

func TestDriveExportAsyncSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/export_tasks",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"ticket": "tk_123"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/export_tasks/tk_123",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": map[string]interface{}{
					"job_status":     0,
					"file_token":     "box_123",
					"file_name":      "report",
					"file_extension": "pdf",
					"type":           "docx",
					"file_size":      3,
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/export_tasks/file/box_123/download",
		Status:  200,
		RawBody: []byte("pdf"),
		Headers: http.Header{
			"Content-Type":        []string{"application/pdf"},
			"Content-Disposition": []string{`attachment; filename="report.pdf"`},
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	prevAttempts, prevInterval := driveExportPollAttempts, driveExportPollInterval
	driveExportPollAttempts, driveExportPollInterval = 1, 0
	t.Cleanup(func() {
		driveExportPollAttempts, driveExportPollInterval = prevAttempts, prevInterval
	})

	err := mountAndRunDrive(t, DriveExport, []string{
		"+export",
		"--token", "docx123",
		"--doc-type", "docx",
		"--file-extension", "pdf",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "report.pdf"))
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "pdf" {
		t.Fatalf("downloaded content = %q", string(data))
	}
	if !strings.Contains(stdout.String(), `"ticket": "tk_123"`) {
		t.Fatalf("stdout missing ticket: %s", stdout.String())
	}
}

func TestDriveExportReadyDownloadFailureIncludesRecoveryHint(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/export_tasks",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"ticket": "tk_ready"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/export_tasks/tk_ready",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": map[string]interface{}{
					"job_status":     0,
					"file_token":     "box_ready",
					"file_name":      "report",
					"file_extension": "pdf",
					"type":           "docx",
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/export_tasks/file/box_ready/download",
		Status:  200,
		RawBody: []byte("pdf"),
		Headers: http.Header{
			"Content-Type":        []string{"application/pdf"},
			"Content-Disposition": []string{`attachment; filename="report.pdf"`},
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.WriteFile(filepath.Join(tmpDir, "report.pdf"), []byte("old"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	prevAttempts, prevInterval := driveExportPollAttempts, driveExportPollInterval
	driveExportPollAttempts, driveExportPollInterval = 1, 0
	t.Cleanup(func() {
		driveExportPollAttempts, driveExportPollInterval = prevAttempts, prevInterval
	})

	err := mountAndRunDrive(t, DriveExport, []string{
		"+export",
		"--token", "docx123",
		"--doc-type", "docx",
		"--file-extension", "pdf",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected download recovery error, got nil")
	}

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected structured exit error, got %v", err)
	}
	if !strings.Contains(exitErr.Detail.Message, "already exists") {
		t.Fatalf("message missing overwrite guidance: %q", exitErr.Detail.Message)
	}
	if !strings.Contains(exitErr.Detail.Hint, "ticket=tk_ready") {
		t.Fatalf("hint missing ticket: %q", exitErr.Detail.Hint)
	}
	if !strings.Contains(exitErr.Detail.Hint, "file_token=box_ready") {
		t.Fatalf("hint missing file token: %q", exitErr.Detail.Hint)
	}
	if !strings.Contains(exitErr.Detail.Hint, `lark-cli drive +export-download --file-token "box_ready" --file-name "report.pdf"`) {
		t.Fatalf("hint missing recovery command: %q", exitErr.Detail.Hint)
	}
}

func TestDriveExportTimeoutReturnsFollowUpCommand(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/export_tasks",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"ticket": "tk_456"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/export_tasks/tk_456",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": map[string]interface{}{
					"job_status": 2,
				},
			},
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	prevAttempts, prevInterval := driveExportPollAttempts, driveExportPollInterval
	driveExportPollAttempts, driveExportPollInterval = 1, 0
	t.Cleanup(func() {
		driveExportPollAttempts, driveExportPollInterval = prevAttempts, prevInterval
	})

	err := mountAndRunDrive(t, DriveExport, []string{
		"+export",
		"--token", "docx123",
		"--doc-type", "docx",
		"--file-extension", "pdf",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), `"ticket": "tk_456"`) {
		t.Fatalf("stdout missing ticket: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"timed_out": true`) {
		t.Fatalf("stdout missing timed_out=true: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"failed": false`) {
		t.Fatalf("stdout missing failed=false: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"job_status": 2`) {
		t.Fatalf("stdout missing numeric job_status: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"job_status_label": "processing"`) {
		t.Fatalf("stdout missing processing job_status_label: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"next_command": "lark-cli drive +task_result --scenario export --ticket tk_456 --file-token docx123"`) {
		t.Fatalf("stdout missing follow-up command: %s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "report.pdf")); !os.IsNotExist(err) {
		t.Fatalf("unexpected downloaded file, err=%v", err)
	}
}

func TestDriveExportPollErrorsReturnLastErrorWithRecoveryHint(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/export_tasks",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"ticket": "tk_poll_fail"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/export_tasks/tk_poll_fail",
		Status: 500,
		Body: map[string]interface{}{
			"code": 999,
			"msg":  "temporary backend failure",
		},
	})

	prevAttempts, prevInterval := driveExportPollAttempts, driveExportPollInterval
	driveExportPollAttempts, driveExportPollInterval = 1, 0
	t.Cleanup(func() {
		driveExportPollAttempts, driveExportPollInterval = prevAttempts, prevInterval
	})

	err := mountAndRunDrive(t, DriveExport, []string{
		"+export",
		"--token", "docx123",
		"--doc-type", "docx",
		"--file-extension", "pdf",
		"--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected persistent poll error, got nil")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout should stay empty on persistent poll error: %s", stdout.String())
	}

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected structured exit error, got %v", err)
	}
	if !strings.Contains(exitErr.Detail.Message, "temporary backend failure") {
		t.Fatalf("message missing last poll error: %q", exitErr.Detail.Message)
	}
	if !strings.Contains(exitErr.Detail.Hint, "ticket=tk_poll_fail") {
		t.Fatalf("hint missing ticket: %q", exitErr.Detail.Hint)
	}
	if !strings.Contains(exitErr.Detail.Hint, "lark-cli drive +task_result --scenario export --ticket tk_poll_fail --file-token docx123") {
		t.Fatalf("hint missing recovery command: %q", exitErr.Detail.Hint)
	}
}

func TestDriveExportDownloadUsesProvidedFileName(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/export_tasks/file/box_789/download",
		Status:  200,
		RawBody: []byte("csv"),
		Headers: http.Header{
			"Content-Type": []string{"text/csv"},
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	err := mountAndRunDrive(t, DriveExportDownload, []string{
		"+export-download",
		"--file-token", "box_789",
		"--file-name", "custom.csv",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "custom.csv"))
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "csv" {
		t.Fatalf("downloaded content = %q", string(data))
	}
}

func TestDriveExportDownloadRejectsOverwriteWithoutFlag(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/export_tasks/file/box_dup/download",
		Status:  200,
		RawBody: []byte("new"),
		Headers: http.Header{
			"Content-Type":        []string{"application/pdf"},
			"Content-Disposition": []string{`attachment; filename="dup.pdf"`},
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.WriteFile("dup.pdf", []byte("old"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDrive(t, DriveExportDownload, []string{
		"+export-download",
		"--file-token", "box_dup",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected overwrite protection error, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSaveContentToOutputDirRejectsOverwriteWithoutFlag(t *testing.T) {

	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "exists.txt")
	if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	fio := &localfileio.LocalFileIO{}
	_, err = saveContentToOutputDir(fio, ".", "exists.txt", []byte("new"), false)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected overwrite error, got %v", err)
	}
}

func TestDriveTaskResultExportIncludesReadyFlags(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/export_tasks/tk_export",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": map[string]interface{}{
					"job_status": 2,
				},
			},
		},
	})

	err := mountAndRunDrive(t, DriveTaskResult, []string{
		"+task_result",
		"--scenario", "export",
		"--ticket", "tk_export",
		"--file-token", "docx123",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"ready": false`)) {
		t.Fatalf("stdout missing ready=false: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"failed": false`)) {
		t.Fatalf("stdout missing failed=false: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"job_status_label": "processing"`)) {
		t.Fatalf("stdout missing job_status_label: %s", stdout.String())
	}
}
