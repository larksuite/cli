// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bytes"
	"encoding/json"
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
		{
			name: "base bitable ok",
			spec: driveExportSpec{Token: "base123", DocType: "bitable", FileExtension: "base"},
		},
		{
			name:    "base non bitable rejected",
			spec:    driveExportSpec{Token: "sheet123", DocType: "sheet", FileExtension: "base"},
			wantErr: "only supports --doc-type bitable",
		},
		{
			name:    "unknown file extension rejected",
			spec:    driveExportSpec{Token: "docx123", DocType: "docx", FileExtension: "rtf"},
			wantErr: "invalid --file-extension",
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

func TestDriveExportMarkdownUsesProvidedFileName(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docs/v1/content",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"content": "# custom\n",
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
		"--file-name", "custom-notes",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "custom-notes.md"))
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "# custom\n" {
		t.Fatalf("markdown content = %q", string(data))
	}
	if !strings.Contains(stdout.String(), `"file_name": "custom-notes.md"`) {
		t.Fatalf("stdout missing provided file name: %s", stdout.String())
	}
}

func TestDriveExportDryRunIncludesLocalFileNameMetadata(t *testing.T) {
	tests := []struct {
		name         string
		wantURL      string
		wantFileName string
		args         []string
	}{
		{
			name:         "markdown",
			wantURL:      "/open-apis/docs/v1/content",
			wantFileName: `"file_name": "notes.md"`,
			args: []string{
				"+export",
				"--token", "docx123",
				"--doc-type", "docx",
				"--file-extension", "markdown",
				"--file-name", "notes",
				"--output-dir", "./exports",
				"--dry-run",
				"--as", "bot",
			},
		},
		{
			name:         "async export",
			wantURL:      "/open-apis/drive/v1/export_tasks",
			wantFileName: `"file_name": "report.pdf"`,
			args: []string{
				"+export",
				"--token", "docx123",
				"--doc-type", "docx",
				"--file-extension", "pdf",
				"--file-name", "report",
				"--output-dir", "./exports",
				"--dry-run",
				"--as", "bot",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())

			err := mountAndRunDrive(t, DriveExport, tt.args, f, stdout)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			out := stdout.String()
			if !strings.Contains(out, tt.wantURL) {
				t.Fatalf("stdout missing URL %q: %s", tt.wantURL, out)
			}
			if !strings.Contains(out, tt.wantFileName) {
				t.Fatalf("stdout missing file_name metadata %q: %s", tt.wantFileName, out)
			}
			if !strings.Contains(out, `"output_dir": "./exports"`) {
				t.Fatalf("stdout missing output_dir metadata: %s", out)
			}
		})
	}
}

func TestDriveExportMarkdownFallsBackToTokenWhenTitleLookupFails(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docs/v1/content",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"content": "# fallback\n",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Status: 500,
		Body: map[string]interface{}{
			"code": 999,
			"msg":  "metadata unavailable",
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

	data, err := os.ReadFile(filepath.Join(tmpDir, "docx123.md"))
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "# fallback\n" {
		t.Fatalf("markdown content = %q", string(data))
	}
	if !strings.Contains(stdout.String(), `"file_name": "docx123.md"`) {
		t.Fatalf("stdout missing fallback file name: %s", stdout.String())
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

func TestDriveExportAsyncUsesProvidedFileName(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/export_tasks",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"ticket": "tk_custom"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/export_tasks/tk_custom",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": map[string]interface{}{
					"job_status":     0,
					"file_token":     "box_custom",
					"file_name":      "server-name",
					"file_extension": "pdf",
					"type":           "docx",
					"file_size":      3,
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/export_tasks/file/box_custom/download",
		Status:  200,
		RawBody: []byte("pdf"),
		Headers: http.Header{
			"Content-Type":        []string{"application/pdf"},
			"Content-Disposition": []string{`attachment; filename="server-name.pdf"`},
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
		"--file-name", "custom-report",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "custom-report.pdf"))
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "pdf" {
		t.Fatalf("downloaded content = %q", string(data))
	}
	if !strings.Contains(stdout.String(), `"file_name": "custom-report.pdf"`) {
		t.Fatalf("stdout missing provided file name: %s", stdout.String())
	}
}

func TestDriveExportBitableBaseAsyncSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/export_tasks",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"ticket": "tk_base"},
		},
	}
	reg.Register(createStub)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/export_tasks/tk_base",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": map[string]interface{}{
					"job_status":     0,
					"file_token":     "box_base",
					"file_name":      "crm",
					"file_extension": "base",
					"type":           "bitable",
					"file_size":      8,
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/export_tasks/file/box_base/download",
		Status:  200,
		RawBody: []byte("snapshot"),
		Headers: http.Header{
			"Content-Type":        []string{"application/octet-stream"},
			"Content-Disposition": []string{`attachment; filename="crm.base"`},
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
		"--token", "bitable123",
		"--doc-type", "bitable",
		"--file-extension", "base",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var createBody map[string]interface{}
	if err := json.Unmarshal(createStub.CapturedBody, &createBody); err != nil {
		t.Fatalf("unmarshal export_tasks body: %v", err)
	}
	if createBody["file_extension"] != "base" {
		t.Fatalf("export_tasks body file_extension = %v, want %q", createBody["file_extension"], "base")
	}
	if createBody["type"] != "bitable" {
		t.Fatalf("export_tasks body type = %v, want %q", createBody["type"], "bitable")
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "crm.base"))
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "snapshot" {
		t.Fatalf("downloaded content = %q", string(data))
	}
	if !strings.Contains(stdout.String(), `"file_extension": "base"`) {
		t.Fatalf("stdout missing base file_extension: %s", stdout.String())
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

func TestDriveExportTimeoutPreservesProvidedFileName(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/export_tasks",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"ticket": "tk_name"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/export_tasks/tk_name",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": map[string]interface{}{
					"job_status": 2,
				},
			},
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
		"--file-name", "quarterly-report",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), `"file_name": "quarterly-report.pdf"`) {
		t.Fatalf("stdout missing preserved file name: %s", stdout.String())
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
