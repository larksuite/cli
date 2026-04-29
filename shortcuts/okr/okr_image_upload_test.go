// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"bytes"
	"mime"
	"mime/multipart"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

func uploadImageTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	return &core.CliConfig{
		AppID:     "test-okr-upload-image",
		AppSecret: "secret-okr-upload-image",
		Brand:     core.BrandFeishu,
	}
}

func runUploadImageShortcut(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()
	parent := &cobra.Command{Use: "okr"}
	OKRUploadImage.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

// --- Validate tests ---

func TestUploadImageValidate_MissingFile(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, uploadImageTestConfig(t))
	// --file is a Required flag, so cobra rejects before our Validate runs.
	err := runUploadImageShortcut(t, f, stdout, []string{
		"+upload-image",
		"--target-id", "123",
		"--target-type", "objective",
	})
	if err == nil {
		t.Fatal("expected error for missing --file")
	}
}

func TestUploadImageValidate_InvalidExtension(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, uploadImageTestConfig(t))
	err := runUploadImageShortcut(t, f, stdout, []string{
		"+upload-image",
		"--file", "document.pdf",
		"--target-id", "123",
		"--target-type", "objective",
	})
	if err == nil {
		t.Fatal("expected error for invalid --file extension")
	}
	if !strings.Contains(err.Error(), "--file must be an image") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUploadImageValidate_MissingTargetID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, uploadImageTestConfig(t))
	err := runUploadImageShortcut(t, f, stdout, []string{
		"+upload-image",
		"--file", "./photo.png",
		"--target-type", "objective",
	})
	if err == nil {
		t.Fatal("expected error for missing --target-id")
	}
}

func TestUploadImageValidate_InvalidTargetID_NonNumeric(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, uploadImageTestConfig(t))
	err := runUploadImageShortcut(t, f, stdout, []string{
		"+upload-image",
		"--file", "./photo.png",
		"--target-id", "abc",
		"--target-type", "objective",
	})
	if err == nil {
		t.Fatal("expected error for non-numeric --target-id")
	}
	if !strings.Contains(err.Error(), "--target-id must be a positive int64") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUploadImageValidate_InvalidTargetType(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, uploadImageTestConfig(t))
	err := runUploadImageShortcut(t, f, stdout, []string{
		"+upload-image",
		"--file", "./photo.png",
		"--target-id", "123",
		"--target-type", "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid --target-type")
	}
	if !strings.Contains(err.Error(), "--target-type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUploadImageValidate_ValidObjective(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadImageTestConfig(t))

	tmpDir := t.TempDir()
	cmdutil.TestChdir(t, tmpDir)
	if err := os.WriteFile("photo.png", []byte("png-bytes"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v1/images/upload",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"file_token": "test_token",
				"url":        "https://example.com/download",
			},
		},
	})

	err := runUploadImageShortcut(t, f, stdout, []string{
		"+upload-image",
		"--file", "./photo.png",
		"--target-id", "6974586812998174252",
		"--target-type", "objective",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- DryRun tests ---

func TestUploadImageDryRun(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, uploadImageTestConfig(t))
	err := runUploadImageShortcut(t, f, stdout, []string{
		"+upload-image",
		"--file", "./photo.png",
		"--target-id", "6974586812998174252",
		"--target-type", "objective",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "/open-apis/okr/v1/images/upload") {
		t.Fatalf("dry-run output should contain API path, got: %s", output)
	}
	if !strings.Contains(output, "POST") {
		t.Fatalf("dry-run output should contain POST method, got: %s", output)
	}
	if !strings.Contains(output, "target_id") {
		t.Fatalf("dry-run output should contain target_id, got: %s", output)
	}
}

func TestUploadImageDryRun_KeyResult(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, uploadImageTestConfig(t))
	err := runUploadImageShortcut(t, f, stdout, []string{
		"+upload-image",
		"--file", "./image.jpg",
		"--target-id", "123",
		"--target-type", "key_result",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "key_result") {
		t.Fatalf("dry-run output should mention key_result, got: %s", output)
	}
}

// --- Execute tests ---

func TestUploadImageExecute_Success(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadImageTestConfig(t))

	tmpDir := t.TempDir()
	cmdutil.TestChdir(t, tmpDir)
	if err := os.WriteFile("photo.png", []byte("png-bytes"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v1/images/upload",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"file_token": "test_token",
				"url":        "https://example.com/download?file_token=test_token",
			},
		},
	})

	err := runUploadImageShortcut(t, f, stdout, []string{
		"+upload-image",
		"--file", "./photo.png",
		"--target-id", "6974586812998174252",
		"--target-type", "objective",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeEnvelope(t, stdout)
	if data["file_token"] != "test_token" {
		t.Fatalf("file_token = %v, want test_token", data["file_token"])
	}
	if data["file_name"] != "photo.png" {
		t.Fatalf("file_name = %v, want photo.png", data["file_name"])
	}
	if data["url"] == "" {
		t.Fatal("url should not be empty")
	}
}

func TestUploadImageExecute_KeyResultType(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadImageTestConfig(t))

	tmpDir := t.TempDir()
	cmdutil.TestChdir(t, tmpDir)
	if err := os.WriteFile("img.jpeg", []byte("jpeg-bytes"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v1/images/upload",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"file_token": "boxTestKRToken",
				"url":        "https://example.com/download",
			},
		},
	}
	reg.Register(uploadStub)

	err := runUploadImageShortcut(t, f, stdout, []string{
		"+upload-image",
		"--file", "./img.jpeg",
		"--target-id", "999",
		"--target-type", "key_result",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeEnvelope(t, stdout)
	if data["file_token"] != "boxTestKRToken" {
		t.Fatalf("file_token = %v, want boxTestKRToken", data["file_token"])
	}

	// Verify multipart body contains correct target_type value
	body := decodeUploadImageMultipart(t, uploadStub)
	if body.Fields["target_type"] != "3" {
		t.Fatalf("target_type = %q, want 3 (key_result)", body.Fields["target_type"])
	}
	if body.Fields["target_id"] != "999" {
		t.Fatalf("target_id = %q, want 999", body.Fields["target_id"])
	}
}

func TestUploadImageExecute_ObjectiveType(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadImageTestConfig(t))

	tmpDir := t.TempDir()
	cmdutil.TestChdir(t, tmpDir)
	if err := os.WriteFile("img.gif", []byte("gif-bytes"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v1/images/upload",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"file_token": "boxOToken",
				"url":        "https://example.com/download",
			},
		},
	}
	reg.Register(uploadStub)

	err := runUploadImageShortcut(t, f, stdout, []string{
		"+upload-image",
		"--file", "./img.gif",
		"--target-id", "456",
		"--target-type", "objective",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeUploadImageMultipart(t, uploadStub)
	if body.Fields["target_type"] != "2" {
		t.Fatalf("target_type = %q, want 2 (objective)", body.Fields["target_type"])
	}
}

func TestUploadImageExecute_APIError(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadImageTestConfig(t))

	tmpDir := t.TempDir()
	cmdutil.TestChdir(t, tmpDir)
	if err := os.WriteFile("photo.png", []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v1/images/upload",
		Status: 400,
		Body: map[string]interface{}{
			"code": 1001001,
			"msg":  "invalid parameters",
		},
	})

	err := runUploadImageShortcut(t, f, stdout, []string{
		"+upload-image",
		"--file", "./photo.png",
		"--target-id", "789",
		"--target-type", "objective",
	})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

func TestUploadImageExecute_FileNotFound(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, uploadImageTestConfig(t))

	tmpDir := t.TempDir()
	cmdutil.TestChdir(t, tmpDir)

	err := runUploadImageShortcut(t, f, stdout, []string{
		"+upload-image",
		"--file", "./missing.png",
		"--target-id", "123",
		"--target-type", "objective",
	})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestUploadImageExecute_NoFileTokenInResponse(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadImageTestConfig(t))

	tmpDir := t.TempDir()
	cmdutil.TestChdir(t, tmpDir)
	if err := os.WriteFile("photo.png", []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v1/images/upload",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{},
		},
	})

	err := runUploadImageShortcut(t, f, stdout, []string{
		"+upload-image",
		"--file", "./photo.png",
		"--target-id", "123",
		"--target-type", "objective",
	})
	if err == nil {
		t.Fatal("expected error for missing file_token in response")
	}
	if !strings.Contains(err.Error(), "no file_token returned") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Multipart body decoding helpers ---

type capturedUploadMultipart struct {
	Fields map[string]string
	Files  map[string][]byte
}

func decodeUploadImageMultipart(t *testing.T, stub *httpmock.Stub) capturedUploadMultipart {
	t.Helper()
	contentType := stub.CapturedHeaders.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("parse content-type %q: %v", contentType, err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("content type = %q, want multipart/form-data", mediaType)
	}
	reader := multipart.NewReader(bytes.NewReader(stub.CapturedBody), params["boundary"])
	body := capturedUploadMultipart{Fields: map[string]string{}, Files: map[string][]byte{}}
	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		var buf bytes.Buffer
		tmp := make([]byte, 4096)
		for {
			n, readErr := part.Read(tmp)
			if n > 0 {
				buf.Write(tmp[:n])
			}
			if readErr != nil {
				break
			}
		}
		if part.FileName() != "" {
			body.Files[part.FormName()] = buf.Bytes()
			continue
		}
		body.Fields[part.FormName()] = buf.String()
	}
	return body
}
