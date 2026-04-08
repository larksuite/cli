// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"strings"
	"testing"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
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

func TestValidateDriveImportSpecRejectsXlsBitable(t *testing.T) {
	t.Parallel()

	err := validateDriveImportSpec(driveImportSpec{
		FilePath: "./data.xls",
		DocType:  "bitable",
	})
	if err == nil || !strings.Contains(err.Error(), ".xls files can only be imported as 'sheet'") {
		t.Fatalf("expected xls-only-sheet validation error, got %v", err)
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

func TestDriveImportUsesMultipartUploadForLargeFile(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	registerDriveBotTokenStub(reg)

	prepareStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_prepare",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"upload_id":  "upload_123",
				"block_size": 4 * 1024 * 1024,
				"block_num":  6,
			},
		},
	}
	reg.Register(prepareStub)

	partStubs := make([]*httpmock.Stub, 0, 6)
	for i := 0; i < 6; i++ {
		stub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/drive/v1/medias/upload_part",
			Body: map[string]interface{}{
				"code": 0,
				"msg":  "ok",
			},
		}
		partStubs = append(partStubs, stub)
		reg.Register(stub)
	}

	finishStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_finish",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"file_token": "file_123",
			},
		},
	}
	reg.Register(finishStub)
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
					"job_status": 0,
					"token":      "sheet_123",
					"url":        "https://example.com/sheets/sheet_123",
				},
			},
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	writeSizedDriveImportFile(t, "large.xlsx", int64(maxDriveUploadFileSize)+1)

	err := mountAndRunDrive(t, DriveImport, []string{
		"+import",
		"--file", "large.xlsx",
		"--type", "sheet",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Contains(stdout.Bytes(), []byte(`"token": "sheet_123"`)) {
		t.Fatalf("stdout missing imported token: %s", stdout.String())
	}

	prepareBody := decodeCapturedJSONBody(t, prepareStub)
	if got, _ := prepareBody["parent_type"].(string); got != "ccm_import_open" {
		t.Fatalf("prepare parent_type = %q, want %q", got, "ccm_import_open")
	}
	if got, _ := prepareBody["file_name"].(string); got != "large.xlsx" {
		t.Fatalf("prepare file_name = %q, want %q", got, "large.xlsx")
	}
	if got, _ := prepareBody["size"].(float64); got != float64(maxDriveUploadFileSize+1) {
		t.Fatalf("prepare size = %v, want %d", got, maxDriveUploadFileSize+1)
	}

	firstPart := decodeCapturedMultipartBody(t, partStubs[0])
	if got := firstPart.Fields["upload_id"]; got != "upload_123" {
		t.Fatalf("first part upload_id = %q, want %q", got, "upload_123")
	}
	if got := firstPart.Fields["seq"]; got != "0" {
		t.Fatalf("first part seq = %q, want %q", got, "0")
	}
	if got := firstPart.Fields["size"]; got != "4194304" {
		t.Fatalf("first part size = %q, want %q", got, "4194304")
	}
	if got := len(firstPart.Files["file"]); got != 4*1024*1024 {
		t.Fatalf("first part file size = %d, want %d", got, 4*1024*1024)
	}

	lastPart := decodeCapturedMultipartBody(t, partStubs[len(partStubs)-1])
	if got := lastPart.Fields["seq"]; got != "5" {
		t.Fatalf("last part seq = %q, want %q", got, "5")
	}
	if got := lastPart.Fields["size"]; got != "1" {
		t.Fatalf("last part size = %q, want %q", got, "1")
	}
	if got := len(lastPart.Files["file"]); got != 1 {
		t.Fatalf("last part file size = %d, want %d", got, 1)
	}

	finishBody := decodeCapturedJSONBody(t, finishStub)
	if got, _ := finishBody["upload_id"].(string); got != "upload_123" {
		t.Fatalf("finish upload_id = %q, want %q", got, "upload_123")
	}
	if got, _ := finishBody["block_num"].(float64); got != 6 {
		t.Fatalf("finish block_num = %v, want %d", got, 6)
	}
}

func TestDriveImportMultipartPrepareValidatesResponseFields(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]interface{}
		wantText string
	}{
		{
			name: "missing upload id",
			data: map[string]interface{}{
				"block_size": 4 * 1024 * 1024,
				"block_num":  6,
			},
			wantText: "upload prepare failed: no upload_id returned",
		},
		{
			name: "missing block size",
			data: map[string]interface{}{
				"upload_id": "upload_123",
				"block_num": 6,
			},
			wantText: "upload prepare failed: invalid block_size returned",
		},
		{
			name: "missing block num",
			data: map[string]interface{}{
				"upload_id":  "upload_123",
				"block_size": 4 * 1024 * 1024,
			},
			wantText: "upload prepare failed: invalid block_num returned",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
			registerDriveBotTokenStub(reg)
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/drive/v1/medias/upload_prepare",
				Body: map[string]interface{}{
					"code": 0,
					"data": tt.data,
				},
			})

			tmpDir := t.TempDir()
			withDriveWorkingDir(t, tmpDir)
			writeSizedDriveImportFile(t, "large.xlsx", int64(maxDriveUploadFileSize)+1)

			err := mountAndRunDrive(t, DriveImport, []string{
				"+import",
				"--file", "large.xlsx",
				"--type", "sheet",
				"--as", "bot",
			}, f, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantText)
			}
		})
	}
}

func TestDriveImportMultipartUploadPartAPIFailure(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	registerDriveBotTokenStub(reg)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_prepare",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"upload_id":  "upload_123",
				"block_size": 4 * 1024 * 1024,
				"block_num":  6,
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_part",
		Body: map[string]interface{}{
			"code": 999,
			"msg":  "chunk rejected",
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	writeSizedDriveImportFile(t, "large.xlsx", int64(maxDriveUploadFileSize)+1)

	err := mountAndRunDrive(t, DriveImport, []string{
		"+import",
		"--file", "large.xlsx",
		"--type", "sheet",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "upload media part failed: [999] chunk rejected") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDriveImportMultipartFinishRequiresFileToken(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	registerDriveBotTokenStub(reg)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_prepare",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"upload_id":  "upload_123",
				"block_size": 4 * 1024 * 1024,
				"block_num":  6,
			},
		},
	})
	for i := 0; i < 6; i++ {
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/drive/v1/medias/upload_part",
			Body: map[string]interface{}{
				"code": 0,
				"msg":  "ok",
			},
		})
	}
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_finish",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	writeSizedDriveImportFile(t, "large.xlsx", int64(maxDriveUploadFileSize)+1)

	err := mountAndRunDrive(t, DriveImport, []string{
		"+import",
		"--file", "large.xlsx",
		"--type", "sheet",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "upload media finish failed: no file_token returned") {
		t.Fatalf("unexpected error: %v", err)
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

func TestParseDriveUploadResponseErrors(t *testing.T) {
	t.Parallel()

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()

		_, err := parseDriveUploadResponse(&larkcore.ApiResp{RawBody: []byte("{")}, "upload media failed")
		if err == nil || !strings.Contains(err.Error(), "invalid response JSON") {
			t.Fatalf("expected invalid JSON error, got %v", err)
		}
	})

	t.Run("api code error", func(t *testing.T) {
		t.Parallel()

		_, err := parseDriveUploadResponse(&larkcore.ApiResp{RawBody: []byte(`{"code":999,"msg":"boom","error":{"detail":"x"}}`)}, "upload media failed")
		if err == nil || !strings.Contains(err.Error(), "upload media failed: [999] boom") {
			t.Fatalf("expected API error, got %v", err)
		}
	})
}

func TestWrapDriveUploadRequestError(t *testing.T) {
	t.Parallel()

	t.Run("preserves exit error", func(t *testing.T) {
		t.Parallel()

		original := output.ErrValidation("bad input")
		got := wrapDriveUploadRequestError(original, "upload media failed")
		if got != original {
			t.Fatalf("expected same exit error pointer, got %v", got)
		}
	})

	t.Run("wraps generic error as network", func(t *testing.T) {
		t.Parallel()

		got := wrapDriveUploadRequestError(io.EOF, "upload media failed")
		var exitErr *output.ExitError
		if !errors.As(got, &exitErr) {
			t.Fatalf("expected ExitError, got %T", got)
		}
		if exitErr.Code != output.ExitNetwork {
			t.Fatalf("exit code = %d, want %d", exitErr.Code, output.ExitNetwork)
		}
		if !strings.Contains(got.Error(), "upload media failed") {
			t.Fatalf("unexpected error: %v", got)
		}
	})
}

type capturedMultipartBody struct {
	Fields map[string]string
	Files  map[string][]byte
}

func decodeCapturedJSONBody(t *testing.T, stub *httpmock.Stub) map[string]interface{} {
	t.Helper()

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode captured JSON body: %v", err)
	}
	return body
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

func decodeCapturedMultipartBody(t *testing.T, stub *httpmock.Stub) capturedMultipartBody {
	t.Helper()

	contentType := stub.CapturedHeaders.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("parse multipart content type: %v", err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("content type = %q, want multipart/form-data", mediaType)
	}

	reader := multipart.NewReader(bytes.NewReader(stub.CapturedBody), params["boundary"])
	body := capturedMultipartBody{
		Fields: map[string]string{},
		Files:  map[string][]byte{},
	}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read multipart part: %v", err)
		}

		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read multipart data: %v", err)
		}
		if part.FileName() != "" {
			body.Files[part.FormName()] = data
			continue
		}
		body.Fields[part.FormName()] = string(data)
	}
	return body
}
