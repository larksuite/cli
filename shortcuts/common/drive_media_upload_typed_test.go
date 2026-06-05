// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestUploadDriveMediaAllTypedWithInMemoryContent(t *testing.T) {
	runtime, reg := newDriveMediaUploadTestRuntime(t)
	withDriveMediaUploadWorkingDir(t, t.TempDir())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"file_token": "file_typed_123"},
		},
	})

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
}

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
			if err == nil || !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("err = %v, want substring %q", err, tt.wantText)
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("expected typed problem, got %T (%v)", err, err)
			}
			if p.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("subtype = %s, want invalid_response", p.Subtype)
			}
		})
	}
}

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
