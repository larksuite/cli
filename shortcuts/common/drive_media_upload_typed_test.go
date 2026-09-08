// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/fileevent"
	"github.com/larksuite/cli/internal/httpmock"
)

const driveMediaTestCapacityExpansionURL = "https://example.com/space/upload/pay/prepare"

// TestUploadDriveMediaAllTypedWithInMemoryContent verifies single-part uploads from in-memory content.
func TestUploadDriveMediaAllTypedWithInMemoryContent(t *testing.T) {
	runtime, reg := newDriveMediaUploadTestRuntime(t)
	withDriveMediaUploadWorkingDir(t, t.TempDir())

	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"file_token": "file_typed_123"},
		},
	}
	reg.Register(uploadStub)

	payload := []byte{0x89, 0x50, 0x4e, 0x47}
	fileToken, err := UploadDriveMediaAllTyped(runtime, DriveMediaUploadAllConfig{
		Reader:     bytes.NewReader(payload),
		FileName:   "clipboard.png",
		FileSize:   int64(len(payload)),
		ParentType: "docx_image",
		ParentNode: strPtr("blk_parent"),
	})
	if err != nil {
		t.Fatalf("UploadDriveMediaAllTyped() error: %v", err)
	}
	if fileToken != "file_typed_123" {
		t.Fatalf("fileToken = %q, want %q", fileToken, "file_typed_123")
	}

	// The in-memory reader is streamed directly into the multipart form.
	body := decodeCapturedDriveMediaMultipartBody(t, uploadStub)
	if got := body.Fields["file_name"]; got != "clipboard.png" {
		t.Fatalf("file_name = %q, want %q", got, "clipboard.png")
	}
	if got := body.Files["file"]; !bytes.Equal(got, payload) {
		t.Fatalf("uploaded file bytes mismatch; got %v, want %v", got, payload)
	}
}

// TestUploadDriveMediaAllTypedClassifiesAPIFailure verifies typed classification of upload API errors.
func TestUploadDriveMediaAllTypedClassifiesAPIFailure(t *testing.T) {
	runtime, reg := newDriveMediaUploadTestRuntime(t)
	withDriveMediaUploadWorkingDir(t, t.TempDir())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body: map[string]interface{}{
			"code": 999,
			"msg":  "upload rejected",
		},
	})

	payload := []byte{0x01}
	_, err := UploadDriveMediaAllTyped(runtime, DriveMediaUploadAllConfig{
		Reader:     bytes.NewReader(payload),
		FileName:   "clipboard.png",
		FileSize:   int64(len(payload)),
		ParentType: "docx_image",
		ParentNode: strPtr("blk_parent"),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T (%v)", err, err)
	}
	if p.Category != errs.CategoryAPI {
		t.Fatalf("category = %s, want api", p.Category)
	}
	if p.Code != 999 {
		t.Fatalf("code = %d, want 999", p.Code)
	}
	if !strings.HasPrefix(p.Message, "upload media failed: ") || !strings.Contains(p.Message, "upload rejected") {
		t.Fatalf("message = %q, want action prefix and server msg", p.Message)
	}
}

// TestUploadDriveMediaAllTypedFileOpenFailure verifies typed handling of local file-open errors.
func TestUploadDriveMediaAllTypedFileOpenFailure(t *testing.T) {
	runtime, _ := newDriveMediaUploadTestRuntime(t)
	withDriveMediaUploadWorkingDir(t, t.TempDir())

	_, err := UploadDriveMediaAllTyped(runtime, DriveMediaUploadAllConfig{
		FilePath:   "missing.bin",
		FileName:   "missing.bin",
		FileSize:   1,
		ParentType: "docx_image",
		ParentNode: strPtr("blk_parent"),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected typed validation error, got %T (%v)", err, err)
	}
}

// TestUploadDriveMediaMultipartTypedBuildsPreparePartsAndFinish verifies the complete multipart request sequence.
func TestUploadDriveMediaMultipartTypedBuildsPreparePartsAndFinish(t *testing.T) {
	runtime, reg := newDriveMediaUploadTestRuntime(t)
	withDriveMediaUploadWorkingDir(t, t.TempDir())

	size := MaxDriveMediaUploadSinglePartSize + 1
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_prepare",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"upload_id":  "upload_typed_1",
				"block_size": float64(4 * 1024 * 1024),
				"block_num":  float64(6),
			},
		},
	})
	for i := 0; i < 6; i++ {
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/drive/v1/medias/upload_part",
			Body:   map[string]interface{}{"code": 0, "msg": "ok"},
		})
	}
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_finish",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"file_token": "file_typed_multi"},
		},
	})

	payload := bytes.Repeat([]byte{0xCD}, int(size))
	fileToken, err := UploadDriveMediaMultipartTyped(runtime, DriveMediaMultipartUploadConfig{
		Reader:     bytes.NewReader(payload),
		FileName:   "clipboard.png",
		FileSize:   size,
		ParentType: "docx_image",
		ParentNode: "",
	})
	if err != nil {
		t.Fatalf("UploadDriveMediaMultipartTyped() error: %v", err)
	}
	if fileToken != "file_typed_multi" {
		t.Fatalf("fileToken = %q, want %q", fileToken, "file_typed_multi")
	}
}

// TestParseDriveMediaMultipartUploadSessionTypedValidatesResponseFields verifies required prepare-response fields.
func TestParseDriveMediaMultipartUploadSessionTypedValidatesResponseFields(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			_, err := parseDriveMediaMultipartUploadSessionTyped(tt.data)
			requireProblem(t, err, errs.CategoryInternal, errs.SubtypeInvalidResponse, tt.wantText)
		})
	}
}

// TestUploadDriveMediaMultipartTypedPartAPIFailure verifies typed errors from multipart part uploads.
func TestUploadDriveMediaMultipartTypedPartAPIFailure(t *testing.T) {
	runtime, reg := newDriveMediaUploadTestRuntime(t)
	withDriveMediaUploadWorkingDir(t, t.TempDir())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_prepare",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"upload_id":  "upload_123",
				"block_size": float64(4 * 1024 * 1024),
				"block_num":  float64(6),
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

	filePath := writeDriveMediaUploadSizedFile(t, "large.bin", MaxDriveMediaUploadSinglePartSize+1)
	_, err := UploadDriveMediaMultipartTyped(runtime, DriveMediaMultipartUploadConfig{
		FilePath:   filePath,
		FileName:   "large.bin",
		FileSize:   MaxDriveMediaUploadSinglePartSize + 1,
		ParentType: "ccm_import_open",
		ParentNode: "",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T (%v)", err, err)
	}
	if p.Category != errs.CategoryAPI || p.Code != 999 {
		t.Fatalf("category/code = %s/%d, want api/999", p.Category, p.Code)
	}
	if !strings.HasPrefix(p.Message, "upload media part failed: ") || !strings.Contains(p.Message, "chunk rejected") {
		t.Fatalf("message = %q, want action prefix and server msg", p.Message)
	}
}

// TestUploadDriveMediaMultipartTypedFinishRequiresFileToken verifies that the finish response must include a file token.
func TestUploadDriveMediaMultipartTypedFinishRequiresFileToken(t *testing.T) {
	runtime, reg := newDriveMediaUploadTestRuntime(t)
	withDriveMediaUploadWorkingDir(t, t.TempDir())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_prepare",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"upload_id":  "upload_123",
				"block_size": float64(4 * 1024 * 1024),
				"block_num":  float64(6),
			},
		},
	})
	for i := 0; i < 6; i++ {
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/drive/v1/medias/upload_part",
			Body:   map[string]interface{}{"code": 0, "msg": "ok"},
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

	filePath := writeDriveMediaUploadSizedFile(t, "large.bin", MaxDriveMediaUploadSinglePartSize+1)
	_, err := UploadDriveMediaMultipartTyped(runtime, DriveMediaMultipartUploadConfig{
		FilePath:   filePath,
		FileName:   "large.bin",
		FileSize:   MaxDriveMediaUploadSinglePartSize + 1,
		ParentType: "ccm_import_open",
		ParentNode: "",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T (%v)", err, err)
	}
	if p.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("subtype = %s, want invalid_response", p.Subtype)
	}
	if !strings.Contains(p.Message, "upload media finish failed: no file_token returned") {
		t.Fatalf("message = %q", p.Message)
	}
}

// registerDriveMediaReportStub registers a successful report_file_event stub.
func registerDriveMediaReportStub(t *testing.T, reg *httpmock.Registry) *httpmock.Stub {
	t.Helper()
	return registerDriveMediaReportStubWithMsg(t, reg, "")
}

// registerDriveMediaReportStubWithMsg registers a report_file_event stub that
// returns code 0 and, when msg is non-empty, carries it as the top-level msg
// (the capacity-expansion URL for tenant-capacity-exceeded uploads).
func registerDriveMediaReportStubWithMsg(t *testing.T, reg *httpmock.Registry, msg string) *httpmock.Stub {
	t.Helper()
	body := map[string]interface{}{"code": 0, "data": map[string]interface{}{}}
	if msg != "" {
		body["msg"] = msg
	}
	stub := &httpmock.Stub{
		Method:   "POST",
		URL:      fileevent.ReportPath,
		Body:     body,
		Reusable: true,
	}
	reg.Register(stub)
	return stub
}

// assertSingleReport verifies one upload report with the expected status and
// returns its decoded tags for additional assertions.
func assertSingleReport(t *testing.T, reportStub *httpmock.Stub, wantStatus string) map[string]interface{} {
	t.Helper()
	if len(reportStub.CapturedBodies) != 1 {
		t.Fatalf("report call count = %d, want 1", len(reportStub.CapturedBodies))
	}
	body := decodeCapturedDriveMediaJSONBody(t, reportStub)
	if body["file_scene"] != "lark-cli" || body["scene"] != "upload" || body["operation"] != "upload" {
		t.Fatalf("unexpected report envelope: %#v", body)
	}
	if _, ok := body["user_id"]; ok {
		t.Fatalf("user_id must be omitted, got %v", body["user_id"])
	}
	if _, ok := body["tenant_id"]; ok {
		t.Fatalf("tenant_id must be omitted, got %v", body["tenant_id"])
	}
	tags, ok := body["tags"].(map[string]interface{})
	if !ok {
		t.Fatalf("tags = %#v, want object", body["tags"])
	}
	if got := tags["status"]; got != wantStatus {
		t.Fatalf("tags.status = %v, want %s", got, wantStatus)
	}
	return tags
}

// TestUploadDriveMediaAllTypedReportsFileEventOnSuccess verifies reporting after a single-part upload succeeds.
func TestUploadDriveMediaAllTypedReportsFileEventOnSuccess(t *testing.T) {
	runtime, reg := newDriveMediaUploadTestRuntime(t)
	withDriveMediaUploadWorkingDir(t, t.TempDir())
	reportStub := registerDriveMediaReportStub(t, reg)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"file_token": "file_ok"},
		},
	})

	payload := []byte{0x89, 0x50}
	fileToken, err := UploadDriveMediaAllTyped(runtime, DriveMediaUploadAllConfig{
		Reader:     bytes.NewReader(payload),
		FileName:   "clipboard.png",
		FileSize:   int64(len(payload)),
		ParentType: "docx_image",
		ParentNode: strPtr("blk_parent"),
	})
	if err != nil {
		t.Fatalf("UploadDriveMediaAllTyped() error: %v", err)
	}
	if fileToken != "file_ok" {
		t.Fatalf("fileToken = %q, want file_ok", fileToken)
	}

	tags := assertSingleReport(t, reportStub, fileevent.StatusSuccess)
	if got := tags["api_path"]; got != "/open-apis/drive/v1/medias/upload_all" {
		t.Fatalf("tags.api_path = %v", got)
	}
	if _, ok := tags["upload_mode"]; ok {
		t.Fatal("tags.upload_mode must be omitted")
	}
	if got := tags["resource_type"]; got != "media" {
		t.Fatalf("tags.resource_type = %v, want media", got)
	}
	if got := tags["mount_point"]; got != "docx_image" {
		t.Fatalf("tags.mount_point = %v, want docx_image", got)
	}
	if got := tags["file_token"]; got != "file_ok" {
		t.Fatalf("tags.file_token = %v, want file_ok", got)
	}
}

// TestUploadDriveMediaAllTypedReportsFileEventOnError verifies reporting after a single-part upload fails.
func TestUploadDriveMediaAllTypedReportsFileEventOnError(t *testing.T) {
	runtime, reg := newDriveMediaUploadTestRuntime(t)
	withDriveMediaUploadWorkingDir(t, t.TempDir())
	reportStub := registerDriveMediaReportStub(t, reg)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body:   map[string]interface{}{"code": 999, "msg": "upload rejected"},
	})

	payload := []byte{0x01}
	_, err := UploadDriveMediaAllTyped(runtime, DriveMediaUploadAllConfig{
		Reader:     bytes.NewReader(payload),
		FileName:   "clipboard.png",
		FileSize:   int64(len(payload)),
		ParentType: "docx_image",
		ParentNode: strPtr("blk_parent"),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Code != 999 {
		t.Fatalf("expected typed api error code 999, got %T (%v)", err, err)
	}

	tags := assertSingleReport(t, reportStub, fileevent.StatusError)
	if got := tags["code"]; got != "999" {
		t.Fatalf("tags.code = %v, want 999", got)
	}
}

// TestUploadDriveMediaAllTypedReportFailureKeepsUploadError verifies that reporting cannot replace the upload error.
func TestUploadDriveMediaAllTypedReportFailureKeepsUploadError(t *testing.T) {
	runtime, reg := newDriveMediaUploadTestRuntime(t)
	withDriveMediaUploadWorkingDir(t, t.TempDir())
	reg.Register(&httpmock.Stub{
		Method:   "POST",
		URL:      fileevent.ReportPath,
		Body:     map[string]interface{}{"code": 500, "msg": "report rejected"},
		Reusable: true,
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body:   map[string]interface{}{"code": 1061101, "msg": "tenant capacity exceeded"},
	})

	payload := []byte{0x01}
	_, err := UploadDriveMediaAllTyped(runtime, DriveMediaUploadAllConfig{
		Reader:     bytes.NewReader(payload),
		FileName:   "clipboard.png",
		FileSize:   int64(len(payload)),
		ParentType: "docx_image",
		ParentNode: strPtr("blk_parent"),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T (%v)", err, err)
	}
	if p.Code != 1061101 {
		t.Fatalf("code = %d, want original 1061101", p.Code)
	}
	// The report failed (code 500), so no capacity-expansion URL is available.
	// Keep the quota hint produced by API error classification unchanged.
	const wantHint = "reduce the request volume or free quota, then retry after the relevant quota resets"
	if p.Hint != wantHint {
		t.Fatalf("hint = %q, want original classified hint %q", p.Hint, wantHint)
	}
}

// TestUploadDriveMediaMultipartTypedReportsFileEventOnPrepareError verifies reporting when multipart preparation fails.
func TestUploadDriveMediaMultipartTypedReportsFileEventOnPrepareError(t *testing.T) {
	runtime, reg := newDriveMediaUploadTestRuntime(t)
	withDriveMediaUploadWorkingDir(t, t.TempDir())
	reportStub := registerDriveMediaReportStubWithMsg(t, reg, driveMediaTestCapacityExpansionURL)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_prepare",
		Body:   map[string]interface{}{"code": 1061101, "msg": "tenant capacity exceeded"},
	})

	filePath := writeDriveMediaUploadSizedFile(t, "large.bin", MaxDriveMediaUploadSinglePartSize+1)
	_, err := UploadDriveMediaMultipartTyped(runtime, DriveMediaMultipartUploadConfig{
		FilePath:   filePath,
		FileName:   "large.bin",
		FileSize:   MaxDriveMediaUploadSinglePartSize + 1,
		ParentType: "ccm_import_open",
		ParentNode: "",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Code != 1061101 {
		t.Fatalf("expected typed api error code 1061101, got %T (%v)", err, err)
	}
	if !strings.Contains(p.Hint, driveMediaTestCapacityExpansionURL) {
		t.Fatalf("hint = %q, want capacity expansion URL", p.Hint)
	}

	tags := assertSingleReport(t, reportStub, fileevent.StatusError)
	if _, ok := tags["upload_mode"]; ok {
		t.Fatal("tags.upload_mode must be omitted")
	}
	if got := tags["api_path"]; got != "/open-apis/drive/v1/medias/upload_prepare" {
		t.Fatalf("tags.api_path = %v, want upload_prepare", got)
	}
	if got := tags["code"]; got != "1061101" {
		t.Fatalf("tags.code = %v, want 1061101", got)
	}
}

// TestUploadDriveMediaMultipartTypedReportsFileEventAndHonorsQuietOnSuccess verifies reporting and the published Quiet contract.
func TestUploadDriveMediaMultipartTypedReportsFileEventAndHonorsQuietOnSuccess(t *testing.T) {
	runtime, reg := newDriveMediaUploadTestRuntime(t)
	runtime.IO().StderrIsTerminal = true
	withDriveMediaUploadWorkingDir(t, t.TempDir())
	reportStub := registerDriveMediaReportStub(t, reg)

	size := MaxDriveMediaUploadSinglePartSize + 1
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_prepare",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"upload_id":  "upload_ok",
				"block_size": float64(4 * 1024 * 1024),
				"block_num":  float64(6),
			},
		},
	})
	for i := 0; i < 6; i++ {
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/drive/v1/medias/upload_part",
			Body:   map[string]interface{}{"code": 0, "msg": "ok"},
		})
	}
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_finish",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"file_token": "file_multi_ok"},
		},
	})

	payload := bytes.Repeat([]byte{0xCD}, int(size))
	fileToken, err := UploadDriveMediaMultipartTyped(runtime, DriveMediaMultipartUploadConfig{
		Reader:     bytes.NewReader(payload),
		FileName:   "clipboard.png",
		FileSize:   size,
		ParentType: "docx_image",
		ParentNode: "",
		Quiet:      true,
	})
	if err != nil {
		t.Fatalf("UploadDriveMediaMultipartTyped() error: %v", err)
	}
	if fileToken != "file_multi_ok" {
		t.Fatalf("fileToken = %q, want file_multi_ok", fileToken)
	}
	stderr, ok := runtime.IO().ErrOut.(*bytes.Buffer)
	if !ok {
		t.Fatalf("stderr writer = %T, want *bytes.Buffer", runtime.IO().ErrOut)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no multipart progress", stderr.String())
	}

	tags := assertSingleReport(t, reportStub, fileevent.StatusSuccess)
	if _, ok := tags["upload_mode"]; ok {
		t.Fatal("tags.upload_mode must be omitted")
	}
	if got := tags["api_path"]; got != "/open-apis/drive/v1/medias/upload_finish" {
		t.Fatalf("tags.api_path = %v, want upload_finish", got)
	}
	if got := tags["file_token"]; got != "file_multi_ok" {
		t.Fatalf("tags.file_token = %v, want file_multi_ok", got)
	}
}
