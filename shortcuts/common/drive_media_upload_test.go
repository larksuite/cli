// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

var commonDriveMediaUploadTestSeq atomic.Int64

func TestUploadDriveMediaAllBuildsMultipartBody(t *testing.T) {
	tests := []struct {
		name           string
		parentNode     *string
		wantParentNode string
		wantParentSet  bool
	}{
		{
			name:           "includes parent_node when provided",
			parentNode:     strPtr("blk_parent"),
			wantParentNode: "blk_parent",
			wantParentSet:  true,
		},
		{
			name:          "omits parent_node when not provided",
			parentNode:    nil,
			wantParentSet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime, reg := newDriveMediaUploadTestRuntime(t)
			withDriveMediaUploadWorkingDir(t, t.TempDir())

			uploadStub := &httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/drive/v1/medias/upload_all",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"file_token": "file_all_123"},
				},
			}
			reg.Register(uploadStub)

			filePath := writeDriveMediaUploadTestFile(t, "small.bin", 3)
			fileToken, err := UploadDriveMediaAll(runtime, DriveMediaUploadAllConfig{
				FilePath:   filePath,
				FileName:   "small.bin",
				FileSize:   3,
				ParentType: "docx_file",
				ParentNode: tt.parentNode,
				Extra:      `{"drive_route_token":"doxcn123"}`,
			})
			if err != nil {
				t.Fatalf("UploadDriveMediaAll() error: %v", err)
			}
			if fileToken != "file_all_123" {
				t.Fatalf("fileToken = %q, want %q", fileToken, "file_all_123")
			}

			body := decodeCapturedDriveMediaMultipartBody(t, uploadStub)
			if got := body.Fields["file_name"]; got != "small.bin" {
				t.Fatalf("file_name = %q, want %q", got, "small.bin")
			}
			if got := body.Fields["parent_type"]; got != "docx_file" {
				t.Fatalf("parent_type = %q, want %q", got, "docx_file")
			}
			if got := body.Fields["size"]; got != "3" {
				t.Fatalf("size = %q, want %q", got, "3")
			}
			if got := body.Fields["extra"]; got != `{"drive_route_token":"doxcn123"}` {
				t.Fatalf("extra = %q, want drive route token payload", got)
			}
			if got := len(body.Files["file"]); got != 3 {
				t.Fatalf("file size = %d, want %d", got, 3)
			}

			gotParentNode, hasParentNode := body.Fields["parent_node"]
			if hasParentNode != tt.wantParentSet {
				t.Fatalf("parent_node present = %v, want %v", hasParentNode, tt.wantParentSet)
			}
			if hasParentNode && gotParentNode != tt.wantParentNode {
				t.Fatalf("parent_node = %q, want %q", gotParentNode, tt.wantParentNode)
			}
		})
	}
}

func TestUploadDriveMediaMultipartBuildsPreparePartsAndFinish(t *testing.T) {
	runtime, reg := newDriveMediaUploadTestRuntime(t)
	withDriveMediaUploadWorkingDir(t, t.TempDir())

	prepareStub := &httpmock.Stub{
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
			"data": map[string]interface{}{"file_token": "file_multi_123"},
		},
	}
	reg.Register(finishStub)

	filePath := writeDriveMediaUploadSizedFile(t, "large.bin", MaxDriveMediaUploadSinglePartSize+1)
	fileToken, err := UploadDriveMediaMultipart(runtime, DriveMediaMultipartUploadConfig{
		FilePath:   filePath,
		FileName:   "large.bin",
		FileSize:   MaxDriveMediaUploadSinglePartSize + 1,
		ParentType: "ccm_import_open",
		ParentNode: "",
		Extra:      `{"obj_type":"sheet","file_extension":"xlsx"}`,
	})
	if err != nil {
		t.Fatalf("UploadDriveMediaMultipart() error: %v", err)
	}
	if fileToken != "file_multi_123" {
		t.Fatalf("fileToken = %q, want %q", fileToken, "file_multi_123")
	}

	prepareBody := decodeCapturedDriveMediaJSONBody(t, prepareStub)
	if got, _ := prepareBody["parent_type"].(string); got != "ccm_import_open" {
		t.Fatalf("prepare parent_type = %q, want %q", got, "ccm_import_open")
	}
	rawParentNode, ok := prepareBody["parent_node"]
	if !ok {
		t.Fatal("prepare body missing parent_node")
	}
	if got, ok := rawParentNode.(string); !ok || got != "" {
		t.Fatalf("prepare parent_node = %#v, want empty string", rawParentNode)
	}
	if got, _ := prepareBody["extra"].(string); got != `{"obj_type":"sheet","file_extension":"xlsx"}` {
		t.Fatalf("prepare extra = %q, want import payload", got)
	}
	if got, _ := prepareBody["size"].(float64); got != float64(MaxDriveMediaUploadSinglePartSize+1) {
		t.Fatalf("prepare size = %v, want %d", got, MaxDriveMediaUploadSinglePartSize+1)
	}

	firstPart := decodeCapturedDriveMediaMultipartBody(t, partStubs[0])
	if got := firstPart.Fields["upload_id"]; got != "upload_123" {
		t.Fatalf("first part upload_id = %q, want %q", got, "upload_123")
	}
	if got := firstPart.Fields["seq"]; got != "0" {
		t.Fatalf("first part seq = %q, want %q", got, "0")
	}
	if got := firstPart.Fields["size"]; got != "4194304" {
		t.Fatalf("first part size = %q, want %q", got, "4194304")
	}

	lastPart := decodeCapturedDriveMediaMultipartBody(t, partStubs[len(partStubs)-1])
	if got := lastPart.Fields["seq"]; got != "5" {
		t.Fatalf("last part seq = %q, want %q", got, "5")
	}
	if got := lastPart.Fields["size"]; got != "1" {
		t.Fatalf("last part size = %q, want %q", got, "1")
	}
	if got := len(lastPart.Files["file"]); got != 1 {
		t.Fatalf("last part file size = %d, want %d", got, 1)
	}

	finishBody := decodeCapturedDriveMediaJSONBody(t, finishStub)
	if got, _ := finishBody["upload_id"].(string); got != "upload_123" {
		t.Fatalf("finish upload_id = %q, want %q", got, "upload_123")
	}
	if got, _ := finishBody["block_num"].(float64); got != 6 {
		t.Fatalf("finish block_num = %v, want %d", got, 6)
	}
}

func TestParseDriveMediaMultipartUploadSessionValidatesResponseFields(t *testing.T) {
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

			_, err := ParseDriveMediaMultipartUploadSession(tt.data)
			if err == nil || !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("err = %v, want substring %q", err, tt.wantText)
			}
		})
	}
}

func TestUploadDriveMediaMultipartPartAPIFailure(t *testing.T) {
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
	_, err := UploadDriveMediaMultipart(runtime, DriveMediaMultipartUploadConfig{
		FilePath:   filePath,
		FileName:   "large.bin",
		FileSize:   MaxDriveMediaUploadSinglePartSize + 1,
		ParentType: "ccm_import_open",
		ParentNode: "",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "upload media part failed: [999] chunk rejected") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUploadDriveMediaMultipartFinishRequiresFileToken(t *testing.T) {
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

	filePath := writeDriveMediaUploadSizedFile(t, "large.bin", MaxDriveMediaUploadSinglePartSize+1)
	_, err := UploadDriveMediaMultipart(runtime, DriveMediaMultipartUploadConfig{
		FilePath:   filePath,
		FileName:   "large.bin",
		FileSize:   MaxDriveMediaUploadSinglePartSize + 1,
		ParentType: "ccm_import_open",
		ParentNode: "",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "upload media finish failed: no file_token returned") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseDriveMediaUploadResponseErrors(t *testing.T) {
	t.Parallel()

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()

		_, err := ParseDriveMediaUploadResponse(&larkcore.ApiResp{RawBody: []byte("{")}, "upload media failed")
		if err == nil || !strings.Contains(err.Error(), "invalid response JSON") {
			t.Fatalf("expected invalid JSON error, got %v", err)
		}
	})

	t.Run("api code error", func(t *testing.T) {
		t.Parallel()

		_, err := ParseDriveMediaUploadResponse(&larkcore.ApiResp{RawBody: []byte(`{"code":999,"msg":"boom","error":{"detail":"x"}}`)}, "upload media failed")
		if err == nil || !strings.Contains(err.Error(), "upload media failed: [999] boom") {
			t.Fatalf("expected API error, got %v", err)
		}
	})
}

func TestExtractDriveMediaUploadFileTokenRequiresToken(t *testing.T) {
	t.Parallel()

	_, err := ExtractDriveMediaUploadFileToken(map[string]interface{}{}, "upload media failed")
	if err == nil || !strings.Contains(err.Error(), "upload media failed: no file_token returned") {
		t.Fatalf("err = %v, want missing file_token error", err)
	}
}

func TestWrapDriveMediaUploadRequestError(t *testing.T) {
	t.Parallel()

	t.Run("preserves exit error", func(t *testing.T) {
		t.Parallel()

		original := output.ErrValidation("bad input")
		got := WrapDriveMediaUploadRequestError(original, "upload media failed")
		if got != original {
			t.Fatalf("expected same exit error pointer, got %v", got)
		}
	})

	t.Run("wraps generic error as network", func(t *testing.T) {
		t.Parallel()

		got := WrapDriveMediaUploadRequestError(io.EOF, "upload media failed")
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

type capturedDriveMediaMultipartBody struct {
	Fields map[string]string
	Files  map[string][]byte
}

func newDriveMediaUploadTestRuntime(t *testing.T) (*RuntimeContext, *httpmock.Registry) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	cfg := &core.CliConfig{
		AppID: fmt.Sprintf("common-drive-media-test-%d", commonDriveMediaUploadTestSeq.Add(1)), AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	runtime := &RuntimeContext{
		ctx:        context.Background(),
		Config:     cfg,
		Factory:    f,
		resolvedAs: core.AsBot,
	}
	return runtime, reg
}

func withDriveMediaUploadWorkingDir(t *testing.T, dir string) {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%q) error: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd error: %v", err)
		}
	})
}

func writeDriveMediaUploadTestFile(t *testing.T, name string, size int) string {
	t.Helper()

	if err := os.WriteFile(name, bytes.Repeat([]byte("a"), size), 0644); err != nil {
		t.Fatalf("WriteFile(%q) error: %v", name, err)
	}
	return name
}

func writeDriveMediaUploadSizedFile(t *testing.T, name string, size int64) string {
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
	return name
}

func decodeCapturedDriveMediaJSONBody(t *testing.T, stub *httpmock.Stub) map[string]interface{} {
	t.Helper()

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode captured JSON body: %v", err)
	}
	return body
}

func decodeCapturedDriveMediaMultipartBody(t *testing.T, stub *httpmock.Stub) capturedDriveMediaMultipartBody {
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
	body := capturedDriveMediaMultipartBody{
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

func strPtr(s string) *string {
	return &s
}
