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

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

func docsTestConfigWithAppID(appID string) *core.CliConfig {
	return &core.CliConfig{
		AppID: appID, AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
}

type docMediaScopedTokenResolver struct {
	scopes string
}

func (r *docMediaScopedTokenResolver) ResolveToken(context.Context, credential.TokenSpec) (*credential.TokenResult, error) {
	return &credential.TokenResult{Token: "test-token", Scopes: r.scopes}, nil
}

func registerDocMediaExportAuth(reg *httpmock.Registry, token string, allowed bool) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/drive/v1/permissions/" + token + "/members/auth",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"auth_result": allowed},
		},
	}
	reg.Register(stub)
	return stub
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
	// UploadDriveMediaAllTyped when small enough. This also exercises the
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
	// Clipboard path: in-memory bytes route through UploadDriveMediaMultipartTyped
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
	registerDocMediaExportAuth(reg, "tok_123", true)
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
	registerDocMediaExportAuth(reg, "tok_123", true)
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
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if strings.Contains(problem.Hint, "docs +media-preview") || strings.Contains(problem.Hint, "exponential backoff") {
		t.Fatalf("hint=%q, want no 403 or rate-limit guidance for HTTP 404", problem.Hint)
	}
}

func TestDocMediaDownloadExportDeniedFailsBeforeDownload(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-download-export-denied-app"))
	registerDocMediaExportAuth(reg, "media_export_denied", false)
	downloadCalls := 0
	reg.Register(&httpmock.Stub{
		Method:   http.MethodGet,
		URL:      "/open-apis/drive/v1/medias/media_export_denied/download",
		Optional: true,
		OnMatch: func(*http.Request) {
			downloadCalls++
		},
	})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)
	err := mountAndRunDocs(t, DocMediaDownload, []string{
		"+media-download",
		"--token", "media_export_denied",
		"--output", "blocked.bin",
		"--as", "bot",
	}, f, nil)
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if problem.Category != errs.CategoryAuthorization || problem.Subtype != errs.SubtypePermissionDenied {
		t.Fatalf("problem = category %q subtype %q, want authorization/permission_denied", problem.Category, problem.Subtype)
	}
	for _, want := range []string{"docs +media-preview", "--token <MEDIA_TOKEN>", "--output <path>"} {
		if !strings.Contains(problem.Hint, want) {
			t.Fatalf("hint=%q, want %q", problem.Hint, want)
		}
	}
	if strings.Contains(problem.Hint, "media_export_denied") {
		t.Fatalf("hint=%q, want placeholder media token", problem.Hint)
	}
	if downloadCalls != 0 {
		t.Fatalf("download calls = %d, want 0", downloadCalls)
	}
}

func TestDocMediaDownloadHTTP403SuggestsPreview(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-download-403-app"))
	registerDocMediaExportAuth(reg, "media_403", true)
	reg.Register(&httpmock.Stub{
		Method:  http.MethodGet,
		URL:     "/open-apis/drive/v1/medias/media_403/download",
		Status:  http.StatusForbidden,
		RawBody: []byte("permission denied"),
	})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)
	err := mountAndRunDocs(t, DocMediaDownload, []string{
		"+media-download",
		"--token", "media_403",
		"--output", "blocked.bin",
		"--as", "bot",
	}, f, nil)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryNetwork || problem.Code != http.StatusForbidden {
		t.Fatalf("problem=%+v ok=%v, want network HTTP 403", problem, ok)
	}
	if !strings.Contains(problem.Hint, "docs +media-preview") || !strings.Contains(problem.Hint, "--token <MEDIA_TOKEN>") || !strings.Contains(problem.Hint, "--output <path>") {
		t.Fatalf("hint=%q, want media preview command with placeholders", problem.Hint)
	}
	if strings.Contains(problem.Hint, "media_403") {
		t.Fatalf("hint=%q, want placeholder media token", problem.Hint)
	}
}

func TestDocWhiteboardDownloadSkipsExportAuth(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-whiteboard-no-auth-app"))
	f.Credential = credential.NewCredentialProvider(nil, nil, &docMediaScopedTokenResolver{scopes: "docs:document.media:download"}, nil)
	reg.Register(&httpmock.Stub{
		Method:  http.MethodGet,
		URL:     "/open-apis/board/v1/whiteboards/board_no_auth/download_as_image",
		Status:  http.StatusOK,
		RawBody: []byte("png-bytes"),
		Headers: http.Header{"Content-Type": []string{"image/png"}},
	})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)
	err := mountAndRunDocs(t, DocMediaDownload, []string{
		"+media-download",
		"--token", "board_no_auth",
		"--type", "whiteboard",
		"--output", "board",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("whiteboard download error = %v", err)
	}
	if data, readErr := os.ReadFile(filepath.Join(tmpDir, "board.png")); readErr != nil || string(data) != "png-bytes" {
		t.Fatalf("whiteboard content = %q, err=%v; want png-bytes", string(data), readErr)
	}
}

func TestDocMediaDownloadDeclaresConditionalPermissionMemberAuthScope(t *testing.T) {
	if len(DocMediaDownload.ConditionalScopes) != 1 || DocMediaDownload.ConditionalScopes[0] != common.DrivePermissionMemberAuthScope {
		t.Fatalf("ConditionalScopes = %v, want [%q]", DocMediaDownload.ConditionalScopes, common.DrivePermissionMemberAuthScope)
	}
}

func TestDocMediaDownloadPermissionAuthScopeErrorsWarnAndContinue(t *testing.T) {
	tests := []struct {
		name string
		code int
		msg  string
	}{
		{name: "app_scope_not_applied", code: 99991672, msg: "app scope not applied"},
		{name: "token_scope_insufficient", code: 99991676, msg: "token scope insufficient"},
		{name: "missing_scope", code: 99991679, msg: "missing scope"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _, stderr, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-download-"+tt.name+"-app"))
			f.Credential = credential.NewCredentialProvider(nil, nil, &docMediaScopedTokenResolver{scopes: "docs:document.media:download"}, nil)
			token := "media_" + tt.name
			reg.Register(&httpmock.Stub{
				Method: http.MethodGet,
				URL:    "/open-apis/drive/v1/permissions/" + token + "/members/auth",
				Body: map[string]interface{}{
					"code": tt.code,
					"msg":  tt.msg,
				},
			})
			reg.Register(&httpmock.Stub{
				Method:  http.MethodGet,
				URL:     "/open-apis/drive/v1/medias/" + token + "/download",
				Status:  http.StatusOK,
				RawBody: []byte("downloaded without permission auth scope"),
				Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
			})

			tmpDir := t.TempDir()
			withDocsWorkingDir(t, tmpDir)
			err := mountAndRunDocs(t, DocMediaDownload, []string{
				"+media-download",
				"--token", token,
				"--output", "downloaded.bin",
				"--as", "bot",
			}, f, nil)
			if err != nil {
				t.Fatalf("media download error = %v, want permission auth scope error %d to be non-blocking", err, tt.code)
			}
			if !strings.Contains(stderr.String(), "warning: export permission check failed; continuing with download:") {
				t.Fatalf("stderr=%q, want permission scope warning", stderr.String())
			}
			data, readErr := os.ReadFile(filepath.Join(tmpDir, "downloaded.bin"))
			if readErr != nil || string(data) != "downloaded without permission auth scope" {
				t.Fatalf("downloaded content = %q, err=%v", string(data), readErr)
			}
		})
	}
}

func TestDocWhiteboardDownloadHTTP403DoesNotSuggestMediaPreview(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-whiteboard-403-app"))
	reg.Register(&httpmock.Stub{
		Method:  http.MethodGet,
		URL:     "/open-apis/board/v1/whiteboards/board_403/download_as_image",
		Status:  http.StatusForbidden,
		RawBody: []byte("permission denied"),
	})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)
	err := mountAndRunDocs(t, DocMediaDownload, []string{
		"+media-download",
		"--token", "board_403",
		"--type", "whiteboard",
		"--output", "blocked.png",
		"--as", "bot",
	}, f, nil)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Code != http.StatusForbidden {
		t.Fatalf("problem=%+v ok=%v, want HTTP 403", problem, ok)
	}
	if strings.Contains(problem.Hint, "docs +media-preview") {
		t.Fatalf("hint=%q, want no media preview guidance for whiteboard", problem.Hint)
	}
}

func TestDocMediaDownloadHTTP429SuggestsBackoff(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-download-429-app"))
	registerDocMediaExportAuth(reg, "media_rate_limited", true)
	reg.Register(&httpmock.Stub{
		Method:  http.MethodGet,
		URL:     "/open-apis/drive/v1/medias/media_rate_limited/download",
		Status:  http.StatusTooManyRequests,
		RawBody: []byte("rate limited"),
	})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)
	err := mountAndRunDocs(t, DocMediaDownload, []string{
		"+media-download",
		"--token", "media_rate_limited",
		"--output", "blocked.bin",
		"--as", "bot",
	}, f, nil)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryNetwork || problem.Code != http.StatusTooManyRequests {
		t.Fatalf("problem=%+v ok=%v, want network HTTP 429", problem, ok)
	}
	for _, want := range []string{"stop immediate retries", "retry later with exponential backoff"} {
		if !strings.Contains(problem.Hint, want) {
			t.Fatalf("hint=%q, want %q", problem.Hint, want)
		}
	}
	if strings.Contains(problem.Hint, "1 minute") {
		t.Fatalf("hint=%q, want no fixed retry duration", problem.Hint)
	}
}

func TestDocMediaDownloadExportAuthRateLimitPreservesAPIErrorAndSuggestsBackoff(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-auth-429-app"))
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/drive/v1/permissions/media_auth_limited/members/auth",
		Body: map[string]interface{}{
			"code":   99991400,
			"msg":    "rate limited",
			"log_id": "log-doc-auth-limited",
		},
	})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)
	err := mountAndRunDocs(t, DocMediaDownload, []string{
		"+media-download",
		"--token", "media_auth_limited",
		"--output", "blocked.bin",
		"--as", "bot",
	}, f, nil)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryAPI || problem.Subtype != errs.SubtypeRateLimit || problem.Code != 99991400 {
		t.Fatalf("problem=%+v ok=%v, want api/rate_limit/99991400", problem, ok)
	}
	if problem.LogID != "log-doc-auth-limited" || !problem.Retryable {
		t.Fatalf("problem=%+v, want preserved log_id and retryable", problem)
	}
	for _, want := range []string{"stop immediate retries", "retry later with exponential backoff"} {
		if !strings.Contains(problem.Hint, want) {
			t.Fatalf("hint=%q, want %q", problem.Hint, want)
		}
	}
	if strings.Contains(problem.Hint, "1 minute") {
		t.Fatalf("hint=%q, want no fixed retry duration", problem.Hint)
	}
}

func TestDocMediaDownloadTypedRateLimitSuggestsBackoff(t *testing.T) {
	err := errs.NewAPIError(errs.SubtypeRateLimit, "request trigger frequency limit").
		WithCode(99991400).
		WithRetryable().
		WithHint("upstream hint")

	got := withDocMediaDownloadRecoveryHint(err, "media")
	problem, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", got, got)
	}
	if problem.Category != errs.CategoryAPI || problem.Subtype != errs.SubtypeRateLimit || problem.Code != 99991400 || !problem.Retryable {
		t.Fatalf("problem=%+v, want preserved API rate-limit metadata", problem)
	}
	for _, want := range []string{"upstream hint", "stop immediate retries", "retry later with exponential backoff"} {
		if !strings.Contains(problem.Hint, want) {
			t.Fatalf("hint=%q, want %q", problem.Hint, want)
		}
	}
	if strings.Contains(problem.Hint, "1 minute") {
		t.Fatalf("hint=%q, want no fixed retry duration", problem.Hint)
	}
}

func TestDocMediaDownloadAppendsExtensionFromContentDispositionFilename(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-download-disposition-app"))
	registerDocMediaExportAuth(reg, "tok_123", true)
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
	registerDocMediaExportAuth(reg, "tok_123", true)
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

func TestDocMediaDownloadDryRunIncludesExportAuthBeforeDownload(t *testing.T) {
	cmd := &cobra.Command{Use: "docs +media-download"}
	cmd.Flags().String("token", "", "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().String("type", "media", "")
	if err := cmd.Flags().Set("token", "media_dryrun"); err != nil {
		t.Fatalf("set --token: %v", err)
	}
	if err := cmd.Flags().Set("output", "asset.bin"); err != nil {
		t.Fatalf("set --output: %v", err)
	}

	dry := decodeDocDryRun(t, DocMediaDownload.DryRun(context.Background(), common.TestNewRuntimeContext(cmd, nil)))
	if len(dry.API) != 2 {
		t.Fatalf("expected 2 API calls, got %d", len(dry.API))
	}
	if dry.API[0].Method != http.MethodGet || dry.API[0].URL != "/open-apis/drive/v1/permissions/media_dryrun/members/auth" {
		t.Fatalf("first API = %+v, want export permission auth", dry.API[0])
	}
	if dry.API[0].Params["type"] != "file" || dry.API[0].Params["action"] != "export" {
		t.Fatalf("first params = %#v, want type=file action=export", dry.API[0].Params)
	}
	if dry.API[1].Method != http.MethodGet || dry.API[1].URL != "/open-apis/drive/v1/medias/media_dryrun/download" {
		t.Fatalf("second API = %+v, want media download", dry.API[1])
	}
}

func TestDocWhiteboardDownloadDryRunSkipsExportAuth(t *testing.T) {
	cmd := &cobra.Command{Use: "docs +media-download"}
	cmd.Flags().String("token", "", "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().String("type", "media", "")
	if err := cmd.Flags().Set("token", "board_dryrun"); err != nil {
		t.Fatalf("set --token: %v", err)
	}
	if err := cmd.Flags().Set("output", "board.png"); err != nil {
		t.Fatalf("set --output: %v", err)
	}
	if err := cmd.Flags().Set("type", "whiteboard"); err != nil {
		t.Fatalf("set --type: %v", err)
	}

	dry := decodeDocDryRun(t, DocMediaDownload.DryRun(context.Background(), common.TestNewRuntimeContext(cmd, nil)))
	if len(dry.API) != 1 {
		t.Fatalf("expected 1 API call, got %d", len(dry.API))
	}
	if dry.API[0].URL != "/open-apis/board/v1/whiteboards/board_dryrun/download_as_image" {
		t.Fatalf("API = %+v, want whiteboard download only", dry.API[0])
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

func TestDocMediaPreviewDocumentsCommentImageTokens(t *testing.T) {
	t.Parallel()

	if !strings.Contains(DocMediaPreview.Description, "comment image") {
		t.Fatalf("description = %q, want comment image support", DocMediaPreview.Description)
	}
	for _, flag := range DocMediaPreview.Flags {
		if flag.Name != "token" {
			continue
		}
		if !strings.Contains(flag.Desc, "comment image token") {
			t.Fatalf("--token help = %q, want comment image token support", flag.Desc)
		}
		return
	}
	t.Fatal("--token flag not found")
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
	registerDocMediaExportAuth(reg, "tok_123", true)
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
		Method string                 `json:"method"`
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
