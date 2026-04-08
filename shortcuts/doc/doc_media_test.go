// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func docsTestConfig() *core.CliConfig {
	return docsTestConfigWithAppID("docs-test-app")
}

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
	f, _, _, _ := cmdutil.TestFactory(t, docsTestConfig())

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
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfig())

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
