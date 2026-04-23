// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

func docsTestConfigWithAppID(appID string) *core.CliConfig {
	return &core.CliConfig{
		AppID: appID, AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
}

func mountAndRunDocs(t *testing.T, s common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "docs"}
	s.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

func withDocsWorkingDir(t *testing.T, dir string) {
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

func TestDocMediaInsertRejectsOldDocURL(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-test-app"))

	err := mountAndRunDocs(t, DocMediaInsert, []string{
		"+media-insert",
		"--doc", "https://example.larksuite.com/doc/xxxxxx",
		"--file", "dummy.png",
		"--dry-run",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "only supports docx documents") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDocMediaInsertValidateRequiresFileOrClipboard(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-test-app"))

	err := mountAndRunDocs(t, DocMediaInsert, []string{
		"+media-insert",
		"--doc", "https://example.larksuite.com/docx/doxcnXXXXXXXXXXXXXXXXXX",
		"--dry-run",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "one of --file or --from-clipboard is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDocMediaInsertValidateRejectsFileAndClipboardTogether(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-test-app"))

	err := mountAndRunDocs(t, DocMediaInsert, []string{
		"+media-insert",
		"--doc", "https://example.larksuite.com/docx/doxcnXXXXXXXXXXXXXXXXXX",
		"--file", "dummy.png",
		"--from-clipboard",
		"--dry-run",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected mutual-exclusion error, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDocMediaInsertDryRunWithClipboardUsesPlaceholder(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-test-app"))

	err := mountAndRunDocs(t, DocMediaInsert, []string{
		"+media-insert",
		"--doc", "https://example.larksuite.com/docx/doxcnXXXXXXXXXXXXXXXXXX",
		"--from-clipboard",
		"--dry-run",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// JSON output escapes "<" and ">" as \u003c / \u003e by default.
	out := stdout.String()
	if !strings.Contains(out, `\u003cclipboard image\u003e`) && !strings.Contains(out, "<clipboard image>") {
		t.Fatalf("dry-run output missing <clipboard image> placeholder: %s", out)
	}
}

func TestDocMediaInsertDryRunWikiAddsResolveStep(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-test-app"))

	err := mountAndRunDocs(t, DocMediaInsert, []string{
		"+media-insert",
		"--doc", "https://example.larksuite.com/wiki/xxxxxx",
		"--file", "dummy.png",
		"--dry-run",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "Resolve wiki node to docx document") {
		t.Fatalf("dry-run output missing wiki resolve step: %s", out)
	}
	if !strings.Contains(out, "resolved_docx_token") {
		t.Fatalf("dry-run output missing resolved docx token placeholder: %s", out)
	}
}

func TestDocMediaUploadDryRunUsesMultipartForLargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)
	writeSizedDocTestFile(t, "large.bin", common.MaxDriveMediaUploadSinglePartSize+1)

	cmd := &cobra.Command{Use: "docs +media-upload"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("parent-type", "", "")
	cmd.Flags().String("parent-node", "", "")
	cmd.Flags().String("doc-id", "", "")
	if err := cmd.Flags().Set("file", "./large.bin"); err != nil {
		t.Fatalf("set --file: %v", err)
	}
	if err := cmd.Flags().Set("parent-type", "docx_file"); err != nil {
		t.Fatalf("set --parent-type: %v", err)
	}
	if err := cmd.Flags().Set("parent-node", "blk_parent"); err != nil {
		t.Fatalf("set --parent-node: %v", err)
	}

	dry := decodeDocDryRun(t, DocMediaUpload.DryRun(context.Background(), common.TestNewRuntimeContext(cmd, nil)))
	if dry.Description != "chunked media upload (files > 20MB)" {
		t.Fatalf("dry-run description = %q", dry.Description)
	}
	if len(dry.API) != 3 {
		t.Fatalf("expected 3 API calls, got %d", len(dry.API))
	}
	if dry.API[0].URL != "/open-apis/drive/v1/medias/upload_prepare" {
		t.Fatalf("first URL = %q, want upload_prepare", dry.API[0].URL)
	}
	if dry.API[1].URL != "/open-apis/drive/v1/medias/upload_part" {
		t.Fatalf("second URL = %q, want upload_part", dry.API[1].URL)
	}
	if dry.API[2].URL != "/open-apis/drive/v1/medias/upload_finish" {
		t.Fatalf("third URL = %q, want upload_finish", dry.API[2].URL)
	}
	if got, _ := dry.API[0].Body["parent_node"].(string); got != "blk_parent" {
		t.Fatalf("prepare parent_node = %q, want %q", got, "blk_parent")
	}
}

func TestDocMediaInsertDryRunUsesMultipartForLargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)
	writeSizedDocTestFile(t, "large.bin", common.MaxDriveMediaUploadSinglePartSize+1)

	cmd := &cobra.Command{Use: "docs +media-insert"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("doc", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().String("align", "", "")
	cmd.Flags().String("caption", "", "")
	if err := cmd.Flags().Set("doc", "doxcnDryRunLarge"); err != nil {
		t.Fatalf("set --doc: %v", err)
	}
	if err := cmd.Flags().Set("file", "./large.bin"); err != nil {
		t.Fatalf("set --file: %v", err)
	}

	dry := decodeDocDryRun(t, DocMediaInsert.DryRun(context.Background(), common.TestNewRuntimeContext(cmd, nil)))
	if dry.Description != "4-step orchestration: query root → create block → upload file → bind to block (auto-rollback on failure)" {
		t.Fatalf("dry-run description = %q", dry.Description)
	}
	if len(dry.API) != 6 {
		t.Fatalf("expected 6 API calls, got %d", len(dry.API))
	}
	if dry.API[2].URL != "/open-apis/drive/v1/medias/upload_prepare" {
		t.Fatalf("third URL = %q, want upload_prepare", dry.API[2].URL)
	}
	if dry.API[3].URL != "/open-apis/drive/v1/medias/upload_part" {
		t.Fatalf("fourth URL = %q, want upload_part", dry.API[3].URL)
	}
	if dry.API[4].URL != "/open-apis/drive/v1/medias/upload_finish" {
		t.Fatalf("fifth URL = %q, want upload_finish", dry.API[4].URL)
	}
	if dry.API[5].URL != "/open-apis/docx/v1/documents/doxcnDryRunLarge/blocks/batch_update" {
		t.Fatalf("last URL = %q, want batch_update", dry.API[5].URL)
	}
	if !strings.Contains(dry.API[2].Desc, "[3a]") {
		t.Fatalf("upload_prepare desc = %q, want [3a] step marker", dry.API[2].Desc)
	}
	if !strings.Contains(dry.API[3].Desc, "[3b]") {
		t.Fatalf("upload_part desc = %q, want [3b] step marker", dry.API[3].Desc)
	}
	if !strings.Contains(dry.API[4].Desc, "[3c]") {
		t.Fatalf("upload_finish desc = %q, want [3c] step marker", dry.API[4].Desc)
	}
	if !strings.Contains(dry.API[5].Desc, "[4]") {
		t.Fatalf("batch_update desc = %q, want [4] step marker", dry.API[5].Desc)
	}
}

func TestUploadDocMediaFileWithContentUsesSinglePartUpload(t *testing.T) {
	// Clipboard path: in-memory bytes (no FilePath) route through
	// UploadDriveMediaAll when small enough. This also exercises the
	// drive_route_token extra built from docID.
	f, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-upload-content-app"))
	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"file_token": "file_content_123"},
		},
	}
	reg.Register(uploadStub)

	runtime := common.TestNewRuntimeContextForAPI(
		context.Background(),
		&cobra.Command{Use: "docs +media-upload"},
		docsTestConfigWithAppID("docs-upload-content-app"),
		f,
		core.AsBot,
	)

	payload := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a} // PNG magic bytes
	fileToken, err := uploadDocMediaFile(runtime, UploadDocMediaFileConfig{
		Reader:     bytes.NewReader(payload),
		FileName:   "clipboard.png",
		FileSize:   int64(len(payload)),
		ParentType: "docx_image",
		ParentNode: "blk_parent",
		DocID:      "doxcnDocID123",
	})
	if err != nil {
		t.Fatalf("uploadDocMediaFile() error: %v", err)
	}
	if fileToken != "file_content_123" {
		t.Fatalf("fileToken = %q, want %q", fileToken, "file_content_123")
	}

	if !strings.Contains(string(uploadStub.CapturedBody), `drive_route_token`) {
		t.Fatalf("expected drive_route_token in extra, captured body did not include it")
	}
}

func TestUploadDocMediaFileWithContentUsesMultipart(t *testing.T) {
	// Clipboard path: in-memory bytes route through UploadDriveMediaMultipart
	// when size exceeds the single-part threshold.
	f, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-upload-content-multi"))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_prepare",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"upload_id":  "upload_content_multi",
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
			"data": map[string]interface{}{"file_token": "file_content_multi_done"},
		},
	})

	runtime := common.TestNewRuntimeContextForAPI(
		context.Background(),
		&cobra.Command{Use: "docs +media-upload"},
		docsTestConfigWithAppID("docs-upload-content-multi"),
		f,
		core.AsBot,
	)

	size := common.MaxDriveMediaUploadSinglePartSize + 1
	payload := bytes.Repeat([]byte{0xAB}, int(size))
	fileToken, err := uploadDocMediaFile(runtime, UploadDocMediaFileConfig{
		Reader:     bytes.NewReader(payload),
		FileName:   "clipboard.png",
		FileSize:   size,
		ParentType: "docx_image",
		ParentNode: "blk_parent",
		// no DocID → no drive_route_token extra
	})
	if err != nil {
		t.Fatalf("uploadDocMediaFile() error: %v", err)
	}
	if fileToken != "file_content_multi_done" {
		t.Fatalf("fileToken = %q, want %q", fileToken, "file_content_multi_done")
	}
}

func TestDocMediaInsertExecuteFromClipboard(t *testing.T) {
	// Covers the Execute clipboard branch end-to-end: read synthetic bytes,
	// resolve docx root, create block, upload in-memory content, bind to block.
	prev := readClipboardImage
	t.Cleanup(func() { readClipboardImage = prev })
	payload := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0xAA, 0xBB}
	readClipboardImage = func() ([]byte, error) { return payload, nil }

	f, stdout, stderr, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-clipboard-exec-app"))
	documentID := "doxcnClipboardExec1"

	// Step 1: GET root block
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docx/v1/documents/" + documentID + "/blocks/" + documentID,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"block": map[string]interface{}{
					"block_id": documentID,
					"children": []interface{}{"existing_block"},
				},
			},
		},
	})
	// Step 2: POST create child block
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docx/v1/documents/" + documentID + "/blocks/" + documentID + "/children",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"children": []interface{}{
					map[string]interface{}{"block_id": "new_image_block"},
				},
			},
		},
	})
	// Step 3: POST upload_all for in-memory bytes
	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"file_token": "file_clip_abc"},
		},
	}
	reg.Register(uploadStub)
	// Step 4: PATCH batch_update
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/docx/v1/documents/" + documentID + "/blocks/batch_update",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})

	err := mountAndRunDocs(t, DocMediaInsert, []string{
		"+media-insert",
		"--doc", documentID,
		"--from-clipboard",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v — stderr: %s", err, stderr.String())
	}

	// stderr should show clipboard read + file name "clipboard.png"
	if !strings.Contains(stderr.String(), "Reading image from clipboard") {
		t.Errorf("stderr missing clipboard-read log: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "clipboard.png") {
		t.Errorf("stderr missing clipboard.png file name: %s", stderr.String())
	}
	// stdout should include the file_token
	if !strings.Contains(stdout.String(), "file_clip_abc") {
		t.Errorf("stdout missing file_token: %s", stdout.String())
	}

	// Upload multipart body should contain the synthetic payload bytes.
	if !bytes.Contains(uploadStub.CapturedBody, payload) {
		t.Errorf("upload body missing clipboard payload bytes")
	}
}

func TestDocMediaInsertExecuteClipboardReadError(t *testing.T) {
	// Covers the early-return when clipboard read fails (no osascript etc).
	prev := readClipboardImage
	t.Cleanup(func() { readClipboardImage = prev })
	readClipboardImage = func() ([]byte, error) {
		return nil, fmt.Errorf("clipboard image upload is not supported on test")
	}

	f, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-clipboard-err-app"))
	err := mountAndRunDocs(t, DocMediaInsert, []string{
		"+media-insert",
		"--doc", "doxcnXXXXXXXXXXXXXXXXXX",
		"--from-clipboard",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected clipboard read error, got nil")
	}
	if !strings.Contains(err.Error(), "clipboard image upload is not supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDocMediaInsertExecuteResolvesWikiBeforeFileCheck(t *testing.T) {
	f, _, stderr, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-insert-exec-app"))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "docx",
					"obj_token": "doxcnResolved123",
				},
			},
		},
	})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)

	err := mountAndRunDocs(t, DocMediaInsert, []string{
		"+media-insert",
		"--doc", "https://example.larksuite.com/wiki/xxxxxx",
		"--file", "missing.png",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected file-not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "file not found") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr.String(), "Resolved wiki to docx") {
		t.Fatalf("stderr missing wiki resolution log: %s", stderr.String())
	}
}

func TestDocMediaDownloadRejectsOverwriteWithoutFlag(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-download-overwrite-app"))
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/medias/tok_123/download",
		Status:  200,
		Body:    []byte("new"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)
	if err := os.WriteFile("download.bin", []byte("old"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDocs(t, DocMediaDownload, []string{
		"+media-download",
		"--token", "tok_123",
		"--output", "download.bin",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected overwrite protection error, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDocMediaDownloadRejectsHTTPErrorBeforeWrite(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-download-app"))
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/medias/tok_123/download",
		Status:  404,
		Body:    "not found",
		Headers: http.Header{"Content-Type": []string{"text/plain"}},
	})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)

	err := mountAndRunDocs(t, DocMediaDownload, []string{
		"+media-download",
		"--token", "tok_123",
		"--output", "download.bin",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected HTTP error, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(tmpDir, "download.bin")); !os.IsNotExist(statErr) {
		t.Fatalf("download target should not be created, statErr=%v", statErr)
	}
}

func TestDocMediaDownloadAppendsExtensionFromContentDispositionFilename(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-download-disposition-app"))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/medias/tok_123/download",
		Status: 200,
		Body:   []byte("a,b,c\n1,2,3\n"),
		Headers: http.Header{
			"Content-Type":        []string{"application/octet-stream"},
			"Content-Disposition": []string{`attachment; filename="drive_registry_config_addition.csv"`},
		},
	})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)

	err := mountAndRunDocs(t, DocMediaDownload, []string{
		"+media-download",
		"--token", "tok_123",
		"--output", "download",
		"--as", "bot",
	}, f, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := decodeDocCommandOutput(t, stdout)
	wantPath := mustDocSafeOutputPath(t, "download.csv")
	if got.Data.SavedPath != wantPath {
		t.Fatalf("saved_path = %q, want %q", got.Data.SavedPath, wantPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("expected downloaded file at %q: %v", wantPath, err)
	}
}

func TestDocMediaDownloadAppendsExtensionForTrailingDotOutput(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-download-trailing-dot-app"))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/medias/tok_123/download",
		Status: 200,
		Body:   []byte("a,b,c\n1,2,3\n"),
		Headers: http.Header{
			"Content-Type": []string{"text/csv; charset=utf-8"},
		},
	})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)

	err := mountAndRunDocs(t, DocMediaDownload, []string{
		"+media-download",
		"--token", "tok_123",
		"--output", "typed.",
		"--as", "bot",
	}, f, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := decodeDocCommandOutput(t, stdout)
	wantPath := mustDocSafeOutputPath(t, "typed.csv")
	if got.Data.SavedPath != wantPath {
		t.Fatalf("saved_path = %q, want %q", got.Data.SavedPath, wantPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("expected downloaded file at %q: %v", wantPath, err)
	}
}

func TestDocMediaPreviewDryRunUsesMediaEndpoint(t *testing.T) {
	cmd := &cobra.Command{Use: "docs +media-preview"}
	cmd.Flags().String("token", "", "")
	cmd.Flags().String("output", "", "")
	if err := cmd.Flags().Set("token", "tok_preview"); err != nil {
		t.Fatalf("set --token: %v", err)
	}
	if err := cmd.Flags().Set("output", "./asset"); err != nil {
		t.Fatalf("set --output: %v", err)
	}

	dry := decodeDocDryRun(t, DocMediaPreview.DryRun(context.Background(), common.TestNewRuntimeContext(cmd, nil)))
	if len(dry.API) != 1 {
		t.Fatalf("expected 1 API call, got %d", len(dry.API))
	}
	if dry.API[0].Desc != "Preview document media file" {
		t.Fatalf("dry-run api desc = %q", dry.API[0].Desc)
	}
	if dry.API[0].URL != "/open-apis/drive/v1/medias/tok_preview/preview_download" {
		t.Fatalf("URL = %q, want media preview endpoint", dry.API[0].URL)
	}
	if got, _ := dry.API[0].Params["preview_type"].(string); got != PreviewType_SOURCE_FILE {
		t.Fatalf("preview_type = %q, want %q", got, PreviewType_SOURCE_FILE)
	}
}

func TestDocMediaPreviewRejectsOverwriteWithoutFlag(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-preview-overwrite-app"))
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/medias/tok_123/preview_download?preview_type=" + PreviewType_SOURCE_FILE,
		Status:  200,
		Body:    []byte("new"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)
	if err := os.WriteFile("preview.bin", []byte("old"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDocs(t, DocMediaPreview, []string{
		"+media-preview",
		"--token", "tok_123",
		"--output", "preview.bin",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected overwrite protection error, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDocMediaPreviewRejectsHTTPErrorBeforeWrite(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-preview-app"))
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/medias/tok_123/preview_download?preview_type=" + PreviewType_SOURCE_FILE,
		Status:  404,
		Body:    "not found",
		Headers: http.Header{"Content-Type": []string{"text/plain"}},
	})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)

	err := mountAndRunDocs(t, DocMediaPreview, []string{
		"+media-preview",
		"--token", "tok_123",
		"--output", "preview.bin",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected HTTP error, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(tmpDir, "preview.bin")); !os.IsNotExist(statErr) {
		t.Fatalf("preview target should not be created, statErr=%v", statErr)
	}
}

func TestDocMediaPreviewAppendsExtensionFromRFC5987Filename(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-preview-disposition-app"))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/medias/tok_123/preview_download?preview_type=" + PreviewType_SOURCE_FILE,
		Status: 200,
		Body:   []byte("a,b,c\n1,2,3\n"),
		Headers: http.Header{
			"Content-Type":        []string{"application/octet-stream"},
			"Content-Disposition": []string{`attachment; filename*=UTF-8''drive_registry_config_addition.csv`},
		},
	})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)

	err := mountAndRunDocs(t, DocMediaPreview, []string{
		"+media-preview",
		"--token", "tok_123",
		"--output", "preview",
		"--as", "bot",
	}, f, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := decodeDocCommandOutput(t, stdout)
	wantPath := mustDocSafeOutputPath(t, "preview.csv")
	if got.Data.SavedPath != wantPath {
		t.Fatalf("saved_path = %q, want %q", got.Data.SavedPath, wantPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("expected preview file at %q: %v", wantPath, err)
	}
}

func TestDocMediaPreviewAppendsExtensionForTrailingDotOutput(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-preview-trailing-dot-app"))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/medias/tok_123/preview_download?preview_type=" + PreviewType_SOURCE_FILE,
		Status: 200,
		Body:   []byte("a,b,c\n1,2,3\n"),
		Headers: http.Header{
			"Content-Disposition": []string{`attachment; filename*=UTF-8''drive_registry_config_addition.csv`},
			"Content-Type":        []string{"application/octet-stream"},
		},
	})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)

	err := mountAndRunDocs(t, DocMediaPreview, []string{
		"+media-preview",
		"--token", "tok_123",
		"--output", "preview.",
		"--as", "bot",
	}, f, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := decodeDocCommandOutput(t, stdout)
	wantPath := mustDocSafeOutputPath(t, "preview.csv")
	if got.Data.SavedPath != wantPath {
		t.Fatalf("saved_path = %q, want %q", got.Data.SavedPath, wantPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("expected preview file at %q: %v", wantPath, err)
	}
}

func TestDocMediaDownloadAppendsExtensionFromContentTypeMapping(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-download-content-type-app"))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/medias/tok_123/download",
		Status: 200,
		Body:   []byte("a,b,c\n1,2,3\n"),
		Headers: http.Header{
			"Content-Type": []string{"text/csv; charset=utf-8"},
		},
	})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)

	err := mountAndRunDocs(t, DocMediaDownload, []string{
		"+media-download",
		"--token", "tok_123",
		"--output", "typed",
		"--as", "bot",
	}, f, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := decodeDocCommandOutput(t, stdout)
	wantPath := mustDocSafeOutputPath(t, "typed.csv")
	if got.Data.SavedPath != wantPath {
		t.Fatalf("saved_path = %q, want %q", got.Data.SavedPath, wantPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("expected downloaded file at %q: %v", wantPath, err)
	}
}

type docDryRunOutput struct {
	Description string `json:"description"`
	API         []struct {
		Desc   string                 `json:"desc"`
		URL    string                 `json:"url"`
		Params map[string]interface{} `json:"params"`
		Body   map[string]interface{} `json:"body"`
	} `json:"api"`
}

type docCommandOutput struct {
	OK   bool `json:"ok"`
	Data struct {
		SavedPath   string `json:"saved_path"`
		SizeBytes   int64  `json:"size_bytes"`
		ContentType string `json:"content_type"`
	} `json:"data"`
}

func writeSizedDocTestFile(t *testing.T, name string, size int64) {
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

func decodeDocDryRun(t *testing.T, dryAPI *common.DryRunAPI) docDryRunOutput {
	t.Helper()

	raw, err := json.Marshal(dryAPI)
	if err != nil {
		t.Fatalf("marshal dry-run output: %v", err)
	}

	var dry docDryRunOutput
	if err := json.Unmarshal(raw, &dry); err != nil {
		t.Fatalf("decode dry-run output: %v", err)
	}
	return dry
}

func decodeDocCommandOutput(t *testing.T, stdout *bytes.Buffer) docCommandOutput {
	t.Helper()

	var out docCommandOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode command output: %v; output=%s", err, stdout.String())
	}
	return out
}

func mustDocSafeOutputPath(t *testing.T, output string) string {
	t.Helper()

	path, err := validate.SafeOutputPath(output)
	if err != nil {
		t.Fatalf("SafeOutputPath(%q) error: %v", output, err)
	}
	return path
}
