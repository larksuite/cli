// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"context"
	"encoding/json"
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
