// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"bytes"
	"encoding/json"
	"mime"
	"mime/multipart"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestSheetMediaUploadValidateMissingToken(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, sheetsTestConfig())
	err := mountAndRunSheets(t, SheetMediaUpload, []string{
		"+media-upload", "--file", "img.png", "--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "--url or --spreadsheet-token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestSheetMediaUploadValidateMissingFileBeforeDryRun(t *testing.T) {
	dir := t.TempDir()
	withSheetsTestWorkingDir(t, dir)

	f, stdout, _, _ := cmdutil.TestFactory(t, sheetsTestConfig())
	err := mountAndRunSheets(t, SheetMediaUpload, []string{
		"+media-upload",
		"--spreadsheet-token", "shtSTUB",
		"--file", "missing.png",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "file not found") {
		t.Fatalf("expected file-not-found error before dry-run planning, got: %v", err)
	}
}

func TestSheetMediaUploadValidateRejectsDirectoryBeforeDryRun(t *testing.T) {
	dir := t.TempDir()
	withSheetsTestWorkingDir(t, dir)
	if err := os.Mkdir("imgdir", 0o755); err != nil {
		t.Fatal(err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, sheetsTestConfig())
	err := mountAndRunSheets(t, SheetMediaUpload, []string{
		"+media-upload",
		"--spreadsheet-token", "shtSTUB",
		"--file", "imgdir",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("expected regular-file error before dry-run planning, got: %v", err)
	}
}

func TestSheetMediaUploadDryRunSmallFile(t *testing.T) {
	dir := t.TempDir()
	withSheetsTestWorkingDir(t, dir)
	if err := os.WriteFile("img.png", []byte("png-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, sheetsTestConfig())
	err := mountAndRunSheets(t, SheetMediaUpload, []string{
		"+media-upload",
		"--spreadsheet-token", "shtSTUB",
		"--file", "img.png",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "/open-apis/drive/v1/medias/upload_all") {
		t.Fatalf("dry-run should use upload_all for small file, got: %s", out)
	}
	if !strings.Contains(out, `"sheet_image"`) {
		t.Fatalf("dry-run should include parent_type=sheet_image, got: %s", out)
	}
	if strings.Contains(out, "upload_prepare") {
		t.Fatalf("dry-run should not use multipart for small file, got: %s", out)
	}
}

func TestSheetMediaUploadDryRunURLExtractsToken(t *testing.T) {
	dir := t.TempDir()
	withSheetsTestWorkingDir(t, dir)
	if err := os.WriteFile("img.png", []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, stdout, _, _ := cmdutil.TestFactory(t, sheetsTestConfig())
	err := mountAndRunSheets(t, SheetMediaUpload, []string{
		"+media-upload",
		"--url", "https://example.feishu.cn/sheets/shtFromURL?sheet=abc",
		"--file", "img.png",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "shtFromURL") {
		t.Fatalf("dry-run should extract token from URL, got: %s", stdout.String())
	}
}

func TestSheetMediaUploadDryRunLargeFileUsesMultipart(t *testing.T) {
	dir := t.TempDir()
	withSheetsTestWorkingDir(t, dir)
	// Sparse file: 20MB + 1 byte, triggers multipart path without allocating disk.
	largeFile, err := os.Create("big.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := largeFile.Truncate(20*1024*1024 + 1); err != nil {
		t.Fatal(err)
	}
	_ = largeFile.Close()

	f, stdout, _, _ := cmdutil.TestFactory(t, sheetsTestConfig())
	err = mountAndRunSheets(t, SheetMediaUpload, []string{
		"+media-upload",
		"--spreadsheet-token", "shtSTUB",
		"--file", "big.png",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"/open-apis/drive/v1/medias/upload_prepare",
		"/open-apis/drive/v1/medias/upload_part",
		"/open-apis/drive/v1/medias/upload_finish",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run should include %q for large file, got: %s", want, out)
		}
	}
	if strings.Contains(out, "upload_all") {
		t.Fatalf("dry-run should not use upload_all for large file, got: %s", out)
	}
}

func TestSheetMediaUploadExecuteSuccess(t *testing.T) {
	dir := t.TempDir()
	withSheetsTestWorkingDir(t, dir)
	if err := os.WriteFile("img.png", []byte("png-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"file_token": "boxTOK123"},
		},
	}
	reg.Register(stub)

	err := mountAndRunSheets(t, SheetMediaUpload, []string{
		"+media-upload",
		"--spreadsheet-token", "shtSTUB",
		"--file", "img.png",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("parse output: %v", err)
	}
	data, _ := envelope["data"].(map[string]interface{})
	if data["file_token"] != "boxTOK123" {
		t.Fatalf("file_token = %v, want boxTOK123", data["file_token"])
	}
	if data["spreadsheet_token"] != "shtSTUB" {
		t.Fatalf("spreadsheet_token = %v, want shtSTUB", data["spreadsheet_token"])
	}

	body := decodeSheetsMultipartBody(t, stub)
	if got := body.Fields["parent_type"]; got != sheetImageParentType {
		t.Fatalf("parent_type = %q, want %q", got, sheetImageParentType)
	}
	if got := body.Fields["parent_node"]; got != "shtSTUB" {
		t.Fatalf("parent_node = %q, want shtSTUB", got)
	}
	if got := body.Fields["file_name"]; got != "img.png" {
		t.Fatalf("file_name = %q, want img.png", got)
	}
	if got := body.Fields["size"]; got != "9" {
		t.Fatalf("size = %q, want 9 (len of png-bytes)", got)
	}
}

func TestSheetMediaUploadFileNotFound(t *testing.T) {
	dir := t.TempDir()
	withSheetsTestWorkingDir(t, dir)

	f, stdout, _, _ := cmdutil.TestFactory(t, sheetsTestConfig())
	err := mountAndRunSheets(t, SheetMediaUpload, []string{
		"+media-upload",
		"--spreadsheet-token", "shtSTUB",
		"--file", "missing.png",
		"--as", "user",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "file not found") && !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("err = %v, want file-not-found error", err)
	}
}

// withSheetsTestWorkingDir chdirs to dir for this test. Not compatible with
// t.Parallel — chdir is process-wide.
func withSheetsTestWorkingDir(t *testing.T, dir string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
}

type capturedSheetsMultipart struct {
	Fields map[string]string
	Files  map[string][]byte
}

func decodeSheetsMultipartBody(t *testing.T, stub *httpmock.Stub) capturedSheetsMultipart {
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
	body := capturedSheetsMultipart{Fields: map[string]string{}, Files: map[string][]byte{}}
	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(part)
		if part.FileName() != "" {
			body.Files[part.FormName()] = buf.Bytes()
			continue
		}
		body.Fields[part.FormName()] = buf.String()
	}
	return body
}
