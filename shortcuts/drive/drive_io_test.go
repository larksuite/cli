// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

var driveTaskCheckPollMu sync.Mutex

func driveTestConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID: "drive-test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
}

func mountAndRunDrive(t *testing.T, s common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "drive"}
	s.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

func withSingleDriveTaskCheckPoll(t *testing.T) {
	t.Helper()
	driveTaskCheckPollMu.Lock()

	prevAttempts, prevInterval := driveTaskCheckPollAttempts, driveTaskCheckPollInterval
	driveTaskCheckPollAttempts, driveTaskCheckPollInterval = 1, 0
	t.Cleanup(func() {
		driveTaskCheckPollAttempts, driveTaskCheckPollInterval = prevAttempts, prevInterval
		driveTaskCheckPollMu.Unlock()
	})
}

func withDriveWorkingDir(t *testing.T, dir string) {
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

func TestDriveUploadLargeFileUsesMultipart(t *testing.T) {
	// Use a distinct AppID to avoid Lark SDK global token cache collision with other tests.
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)

	// Step 1: upload_prepare
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_prepare",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"upload_id":  "test-upload-id",
				"block_size": float64(common.MaxDriveMediaUploadSinglePartSize),
				"block_num":  float64(2),
			},
		},
	})

	// Step 2: upload_part (block 0)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_part",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})

	// Step 2: upload_part (block 1)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_part",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})

	// Step 3: upload_finish
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_finish",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"file_token": "file_multipart_token",
			},
		},
	})

	tmpDir := t.TempDir()
	// Use Chdir directly (not withDriveWorkingDir) to avoid cleanup order interference with other tests.
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error: %v", err)
	}
	defer os.Chdir(origDir)

	fh, err := os.Create("large.bin")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := fh.Truncate(common.MaxDriveMediaUploadSinglePartSize + 1); err != nil {
		t.Fatalf("Truncate() error: %v", err)
	}
	if err := fh.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	err = mountAndRunDrive(t, DriveUpload, []string{
		"+upload",
		"--file", "large.bin",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected multipart upload to succeed, got error: %v", err)
	}
	if !strings.Contains(stdout.String(), "file_multipart_token") {
		t.Fatalf("stdout missing file_token: %s", stdout.String())
	}
}

func TestDriveUploadLargeFileToWikiUsesMultipart(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-large-wiki-test", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)

	prepareStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_prepare",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"upload_id":  "test-upload-id",
				"block_size": float64(common.MaxDriveMediaUploadSinglePartSize),
				"block_num":  float64(2),
			},
		},
	}
	reg.Register(prepareStub)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_part",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_part",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_finish",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"file_token": "file_multipart_wiki_token",
			},
		},
	})

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error: %v", err)
	}
	defer os.Chdir(origDir)

	fh, err := os.Create("large.bin")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := fh.Truncate(common.MaxDriveMediaUploadSinglePartSize + 1); err != nil {
		t.Fatalf("Truncate() error: %v", err)
	}
	if err := fh.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	err = mountAndRunDrive(t, DriveUpload, []string{
		"+upload",
		"--file", "large.bin",
		"--wiki-token", "wikcn_multipart_upload_test",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected multipart wiki upload to succeed, got error: %v", err)
	}

	body := decodeCapturedJSONBody(t, prepareStub)
	if got := body["parent_type"]; got != driveUploadParentTypeWiki {
		t.Fatalf("parent_type = %#v, want %q", got, driveUploadParentTypeWiki)
	}
	if got := body["parent_node"]; got != "wikcn_multipart_upload_test" {
		t.Fatalf("parent_node = %#v, want %q", got, "wikcn_multipart_upload_test")
	}
}

func TestDriveUploadLargeFileOverwriteUsesMultipart(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-large-overwrite-test", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)

	prepareStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_prepare",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"upload_id":  "test-upload-id",
				"block_size": float64(common.MaxDriveMediaUploadSinglePartSize),
				"block_num":  float64(2),
			},
		},
	}
	reg.Register(prepareStub)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_part",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_part",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_finish",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"file_token": "file_multipart_overwrite_token",
			},
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	fh, err := os.Create("large.bin")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := fh.Truncate(common.MaxDriveMediaUploadSinglePartSize + 1); err != nil {
		t.Fatalf("Truncate() error: %v", err)
	}
	if err := fh.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	err = mountAndRunDrive(t, DriveUpload, []string{
		"+upload",
		"--file", "large.bin",
		"--file-token", "box_existing_large_upload",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected multipart overwrite upload to succeed, got error: %v", err)
	}

	body := decodeCapturedJSONBody(t, prepareStub)
	if got := body["file_token"]; got != "box_existing_large_upload" {
		t.Fatalf("file_token = %#v, want %q", got, "box_existing_large_upload")
	}
}

func TestDriveUploadLargeFileOverwriteReturnsVersionFromUploadFinish(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-large-overwrite-version-test", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_prepare",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"upload_id":  "test-upload-id",
				"block_size": float64(common.MaxDriveMediaUploadSinglePartSize),
				"block_num":  float64(1),
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_part",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_finish",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"file_token": "file_multipart_overwrite_version_token",
				"version":    "v44",
			},
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	fh, err := os.Create("large.bin")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := fh.Truncate(common.MaxDriveMediaUploadSinglePartSize + 1); err != nil {
		t.Fatalf("Truncate() error: %v", err)
	}
	if err := fh.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	err = mountAndRunDrive(t, DriveUpload, []string{
		"+upload",
		"--file", "large.bin",
		"--file-token", "box_existing_large_upload",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected multipart overwrite upload to succeed, got error: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if got := data["version"]; got != "v44" {
		t.Fatalf("data.version = %#v, want %q", got, "v44")
	}
}

func TestDriveUploadLargeFileOverwriteReturnsVersionFromUploadFinishAlias(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-large-overwrite-data-version-test", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_prepare",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"upload_id":  "test-upload-id",
				"block_size": float64(common.MaxDriveMediaUploadSinglePartSize),
				"block_num":  float64(1),
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_part",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_finish",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"file_token":   "file_multipart_overwrite_alias_token",
				"data_version": "v45",
			},
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	fh, err := os.Create("large.bin")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := fh.Truncate(common.MaxDriveMediaUploadSinglePartSize + 1); err != nil {
		t.Fatalf("Truncate() error: %v", err)
	}
	if err := fh.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	err = mountAndRunDrive(t, DriveUpload, []string{
		"+upload",
		"--file", "large.bin",
		"--file-token", "box_existing_large_upload",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected multipart overwrite upload to succeed, got error: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if got := data["version"]; got != "v45" {
		t.Fatalf("data.version = %#v, want %q", got, "v45")
	}
}

func TestDriveUploadSmallFile(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-small-test", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"file_token": "file_small_token",
			},
		},
	})

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error: %v", err)
	}
	defer os.Chdir(origDir)

	if err := os.WriteFile("small.bin", make([]byte, 1024), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDrive(t, DriveUpload, []string{
		"+upload", "--file", "small.bin", "--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected small upload to succeed, got error: %v", err)
	}
	if !strings.Contains(stdout.String(), "file_small_token") {
		t.Fatalf("stdout missing file_token: %s", stdout.String())
	}
}

func TestDriveUploadSmallFileOverwriteUsesFileToken(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-small-overwrite-test", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"file_token": "file_small_overwrite_token",
				"version":    "v42",
			},
		},
	}
	reg.Register(stub)

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	if err := os.WriteFile("small.bin", make([]byte, 1024), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDrive(t, DriveUpload, []string{
		"+upload",
		"--file", "small.bin",
		"--file-token", "box_existing_small_upload",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected small overwrite upload to succeed, got error: %v", err)
	}

	body := decodeDriveMultipartBody(t, stub)
	if got := body.Fields["file_token"]; got != "box_existing_small_upload" {
		t.Fatalf("file_token = %q, want %q", got, "box_existing_small_upload")
	}
	data := decodeDriveEnvelope(t, stdout)
	if got := data["version"]; got != "v42" {
		t.Fatalf("data.version = %#v, want %q", got, "v42")
	}
}

func TestDriveUploadReturnsVersionFromDataVersionAlias(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-small-data-version-test", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"file_token":   "file_small_alias_token",
				"data_version": "v43",
			},
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	if err := os.WriteFile("small.bin", make([]byte, 1024), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDrive(t, DriveUpload, []string{
		"+upload",
		"--file", "small.bin",
		"--file-token", "box_existing_alias_upload",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected overwrite upload to succeed, got error: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if got := data["version"]; got != "v43" {
		t.Fatalf("data.version = %#v, want %q", got, "v43")
	}
}

func TestDriveUploadSmallFileToWiki(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-small-wiki-test", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"file_token": "file_small_wiki_token",
			},
		},
	}
	reg.Register(stub)

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	if err := os.WriteFile("small.bin", make([]byte, 1024), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDrive(t, DriveUpload, []string{
		"+upload",
		"--file", "small.bin",
		"--wiki-token", "wikcn_target_upload_test",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected wiki upload to succeed, got error: %v", err)
	}

	body := decodeDriveMultipartBody(t, stub)
	if got := body.Fields["parent_type"]; got != driveUploadParentTypeWiki {
		t.Fatalf("parent_type = %q, want %q", got, driveUploadParentTypeWiki)
	}
	if got := body.Fields["parent_node"]; got != "wikcn_target_upload_test" {
		t.Fatalf("parent_node = %q, want %q", got, "wikcn_target_upload_test")
	}
	if got := body.Fields["file_name"]; got != "small.bin" {
		t.Fatalf("file_name = %q, want %q", got, "small.bin")
	}
	if got := body.Fields["size"]; got != "1024" {
		t.Fatalf("size = %q, want %q", got, "1024")
	}
}

func TestDriveUploadFallbackURLForExplorerParent(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-explorer-fallback-url", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			// upload_all only ever returns file_token; url is never present —
			// this exercises the fallback path unconditionally for explorer
			// parents.
			"data": map[string]interface{}{"file_token": "file_explorer_small"},
		},
	})

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error: %v", err)
	}
	defer os.Chdir(origDir)

	if err := os.WriteFile("hello.bin", make([]byte, 64), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDrive(t, DriveUpload, []string{
		"+upload", "--file", "hello.bin", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("upload should succeed, got: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if got, want := data["url"], "https://www.feishu.cn/file/file_explorer_small"; got != want {
		t.Fatalf("data.url = %#v, want %q (brand-standard fallback)", got, want)
	}
}

func TestDriveUploadOmitsURLForWikiParent(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-wiki-no-url", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"file_token": "file_wiki_small"},
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.WriteFile("hello.bin", make([]byte, 64), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDrive(t, DriveUpload, []string{
		"+upload", "--file", "hello.bin",
		"--wiki-token", "wikcn_parent",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("upload should succeed, got: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if _, ok := data["url"]; ok {
		t.Fatalf("data.url should be omitted for wiki-hosted files (no standalone URL); got %#v", data["url"])
	}
}

func TestDriveUploadSmallFileAPIError(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-small-err", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 1001, "msg": "quota exceeded",
		},
	})

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error: %v", err)
	}
	defer os.Chdir(origDir)

	if err := os.WriteFile("small.bin", make([]byte, 1024), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDrive(t, DriveUpload, []string{
		"+upload", "--file", "small.bin", "--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error for API error code, got nil")
	}
	if !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDriveUploadSmallFileNoToken(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-small-notoken", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{},
		},
	})

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error: %v", err)
	}
	defer os.Chdir(origDir)

	if err := os.WriteFile("small.bin", make([]byte, 1024), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDrive(t, DriveUpload, []string{
		"+upload", "--file", "small.bin", "--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error for missing file_token, got nil")
	}
	if !strings.Contains(err.Error(), "no file_token") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDriveUploadSmallFileInvalidJSON(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-small-json", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)

	reg.Register(&httpmock.Stub{
		Method:  "POST",
		URL:     "/open-apis/drive/v1/files/upload_all",
		RawBody: []byte("not valid json"),
	})

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error: %v", err)
	}
	defer os.Chdir(origDir)

	if err := os.WriteFile("small.bin", make([]byte, 1024), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDrive(t, DriveUpload, []string{
		"+upload", "--file", "small.bin", "--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDriveUploadPrepareInvalidResponse(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-prepare-bad", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_prepare",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"upload_id":  "",
				"block_size": float64(0),
				"block_num":  float64(0),
			},
		},
	})

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error: %v", err)
	}
	defer os.Chdir(origDir)

	fh, err := os.Create("large.bin")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := fh.Truncate(common.MaxDriveMediaUploadSinglePartSize + 1); err != nil {
		t.Fatalf("Truncate() error: %v", err)
	}
	fh.Close()

	err = mountAndRunDrive(t, DriveUpload, []string{
		"+upload", "--file", "large.bin", "--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error for invalid prepare response, got nil")
	}
	if !strings.Contains(err.Error(), "upload_prepare returned invalid data") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDriveUploadPartAPIError(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-part-err", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_prepare",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"upload_id":  "test-upload-id",
				"block_size": float64(common.MaxDriveMediaUploadSinglePartSize),
				"block_num":  float64(2),
			},
		},
	})

	// First part succeeds
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_part",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})

	// Second part fails with API error
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_part",
		Body: map[string]interface{}{
			"code": 5001, "msg": "part upload failed",
		},
	})

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error: %v", err)
	}
	defer os.Chdir(origDir)

	fh, err := os.Create("large.bin")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := fh.Truncate(common.MaxDriveMediaUploadSinglePartSize + 1); err != nil {
		t.Fatalf("Truncate() error: %v", err)
	}
	fh.Close()

	err = mountAndRunDrive(t, DriveUpload, []string{
		"+upload", "--file", "large.bin", "--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error for part upload failure, got nil")
	}
	if !strings.Contains(err.Error(), "part upload failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDriveUploadPartInvalidJSON(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-part-json", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_prepare",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"upload_id":  "test-upload-id",
				"block_size": float64(common.MaxDriveMediaUploadSinglePartSize + 1),
				"block_num":  float64(1),
			},
		},
	})

	reg.Register(&httpmock.Stub{
		Method:  "POST",
		URL:     "/open-apis/drive/v1/files/upload_part",
		RawBody: []byte("not json"),
	})

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error: %v", err)
	}
	defer os.Chdir(origDir)

	fh, err := os.Create("large.bin")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := fh.Truncate(common.MaxDriveMediaUploadSinglePartSize + 1); err != nil {
		t.Fatalf("Truncate() error: %v", err)
	}
	fh.Close()

	err = mountAndRunDrive(t, DriveUpload, []string{
		"+upload", "--file", "large.bin", "--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error for invalid part JSON, got nil")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDriveUploadFinishNoToken(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-finish-notoken", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_prepare",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"upload_id":  "test-upload-id",
				"block_size": float64(common.MaxDriveMediaUploadSinglePartSize + 1),
				"block_num":  float64(1),
			},
		},
	})

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_part",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_finish",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{},
		},
	})

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error: %v", err)
	}
	defer os.Chdir(origDir)

	fh, err := os.Create("large.bin")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := fh.Truncate(common.MaxDriveMediaUploadSinglePartSize + 1); err != nil {
		t.Fatalf("Truncate() error: %v", err)
	}
	fh.Close()

	err = mountAndRunDrive(t, DriveUpload, []string{
		"+upload", "--file", "large.bin", "--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error for missing file_token, got nil")
	}
	if !strings.Contains(err.Error(), "no file_token") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDriveUploadWithCustomName(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-name-test", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"file_token": "file_named_token",
			},
		},
	})

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error: %v", err)
	}
	defer os.Chdir(origDir)

	if err := os.WriteFile("small.bin", make([]byte, 1024), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDrive(t, DriveUpload, []string{
		"+upload", "--file", "small.bin", "--name", "custom.bin", "--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected upload to succeed, got error: %v", err)
	}
	if !strings.Contains(stdout.String(), "custom.bin") {
		t.Fatalf("stdout missing custom name: %s", stdout.String())
	}
}

func TestDriveUploadDryRunUsesWikiTarget(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +upload"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("file-token", "", "")
	cmd.Flags().String("folder-token", "", "")
	cmd.Flags().String("wiki-token", "", "")
	cmd.Flags().String("name", "", "")
	if err := cmd.Flags().Set("file", "./report.pdf"); err != nil {
		t.Fatalf("set --file: %v", err)
	}
	if err := cmd.Flags().Set("wiki-token", "wikcn_dryrun_upload_target"); err != nil {
		t.Fatalf("set --wiki-token: %v", err)
	}

	runtime := common.TestNewRuntimeContextWithCtx(context.Background(), cmd, nil)
	dry := DriveUpload.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}

	var got struct {
		API []struct {
			Body map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run json: %v", err)
	}
	if len(got.API) != 1 {
		t.Fatalf("expected 1 API call, got %d", len(got.API))
	}
	if got.API[0].Body["parent_type"] != driveUploadParentTypeWiki {
		t.Fatalf("parent_type = %#v, want %q", got.API[0].Body["parent_type"], driveUploadParentTypeWiki)
	}
	if got.API[0].Body["parent_node"] != "wikcn_dryrun_upload_target" {
		t.Fatalf("parent_node = %#v, want %q", got.API[0].Body["parent_node"], "wikcn_dryrun_upload_target")
	}
}

func TestNewDriveUploadSpecPreservesPathAndName(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +upload"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("file-token", "", "")
	cmd.Flags().String("folder-token", "", "")
	cmd.Flags().String("wiki-token", "", "")
	cmd.Flags().String("name", "", "")
	if err := cmd.Flags().Set("file", " report final.pdf "); err != nil {
		t.Fatalf("set --file: %v", err)
	}
	if err := cmd.Flags().Set("folder-token", " fld_upload_target "); err != nil {
		t.Fatalf("set --folder-token: %v", err)
	}
	if err := cmd.Flags().Set("file-token", " box_upload_target "); err != nil {
		t.Fatalf("set --file-token: %v", err)
	}
	if err := cmd.Flags().Set("wiki-token", " wikcn_upload_target "); err != nil {
		t.Fatalf("set --wiki-token: %v", err)
	}
	if err := cmd.Flags().Set("name", " final upload.pdf "); err != nil {
		t.Fatalf("set --name: %v", err)
	}

	runtime := common.TestNewRuntimeContextWithCtx(context.Background(), cmd, nil)
	got := newDriveUploadSpec(runtime)
	if got.FilePath != " report final.pdf " {
		t.Fatalf("FilePath = %q, want original value", got.FilePath)
	}
	if got.Name != " final upload.pdf " {
		t.Fatalf("Name = %q, want original value", got.Name)
	}
	if got.FolderToken != "fld_upload_target" {
		t.Fatalf("FolderToken = %q, want trimmed token", got.FolderToken)
	}
	if got.FileToken != "box_upload_target" {
		t.Fatalf("FileToken = %q, want trimmed token", got.FileToken)
	}
	if got.WikiToken != "wikcn_upload_target" {
		t.Fatalf("WikiToken = %q, want trimmed token", got.WikiToken)
	}
}

func TestDriveUploadDryRunIncludesFileToken(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +upload"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("file-token", "", "")
	cmd.Flags().String("folder-token", "", "")
	cmd.Flags().String("wiki-token", "", "")
	cmd.Flags().String("name", "", "")
	if err := cmd.Flags().Set("file", "./report.pdf"); err != nil {
		t.Fatalf("set --file: %v", err)
	}
	if err := cmd.Flags().Set("file-token", "boxcn_dryrun_overwrite"); err != nil {
		t.Fatalf("set --file-token: %v", err)
	}

	runtime := common.TestNewRuntimeContextWithCtx(context.Background(), cmd, nil)
	dry := DriveUpload.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}

	var got struct {
		API []struct {
			Body map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run json: %v", err)
	}
	if len(got.API) != 1 {
		t.Fatalf("expected 1 API call, got %d", len(got.API))
	}
	if got.API[0].Body["file_token"] != "boxcn_dryrun_overwrite" {
		t.Fatalf("file_token = %#v, want %q", got.API[0].Body["file_token"], "boxcn_dryrun_overwrite")
	}
}

func TestDriveUploadDryRunBotOverwriteSkipsPermissionGrantHint(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +upload"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("file-token", "", "")
	cmd.Flags().String("folder-token", "", "")
	cmd.Flags().String("wiki-token", "", "")
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("as", "", "")
	if err := cmd.Flags().Set("file", "./report.pdf"); err != nil {
		t.Fatalf("set --file: %v", err)
	}
	if err := cmd.Flags().Set("file-token", "boxcn_dryrun_overwrite"); err != nil {
		t.Fatalf("set --file-token: %v", err)
	}
	if err := cmd.Flags().Set("as", "bot"); err != nil {
		t.Fatalf("set --as: %v", err)
	}

	runtime := common.TestNewRuntimeContextWithCtx(context.Background(), cmd, nil)
	dry := DriveUpload.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}

	var got struct {
		API []struct {
			Desc string                 `json:"desc"`
			Body map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run json: %v", err)
	}
	if len(got.API) != 1 {
		t.Fatalf("expected 1 API call, got %d", len(got.API))
	}
	if got.API[0].Body["file_token"] != "boxcn_dryrun_overwrite" {
		t.Fatalf("file_token = %#v, want %q", got.API[0].Body["file_token"], "boxcn_dryrun_overwrite")
	}
	if strings.Contains(got.API[0].Desc, "grant the current CLI user full_access") {
		t.Fatalf("dry-run desc should skip permission-grant hint for overwrite, got %q", got.API[0].Desc)
	}
}

func TestDriveUploadTargetLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target driveUploadTarget
		want   string
	}{
		{
			name: "wiki node",
			target: driveUploadTarget{
				ParentType: driveUploadParentTypeWiki,
				ParentNode: "wikcn_upload_target",
			},
			want: "wiki node " + common.MaskToken("wikcn_upload_target"),
		},
		{
			name: "root folder",
			target: driveUploadTarget{
				ParentType: driveUploadParentTypeExplorer,
			},
			want: "Drive root folder",
		},
		{
			name: "folder",
			target: driveUploadTarget{
				ParentType: driveUploadParentTypeExplorer,
				ParentNode: "fld_upload_target",
			},
			want: "folder " + common.MaskToken("fld_upload_target"),
		},
		{
			name: "unknown target",
			target: driveUploadTarget{
				ParentType: "unknown",
				ParentNode: "node_upload_target",
			},
			want: "target " + common.MaskToken("node_upload_target"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.target.Label(); got != tt.want {
				t.Fatalf("Label() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDriveUploadValidateRejectsConflictingTargets(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +upload"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("file-token", "", "")
	cmd.Flags().String("folder-token", "", "")
	cmd.Flags().String("wiki-token", "", "")
	cmd.Flags().String("name", "", "")
	if err := cmd.Flags().Set("folder-token", "fld_upload_conflict"); err != nil {
		t.Fatalf("set --folder-token: %v", err)
	}
	if err := cmd.Flags().Set("wiki-token", "wikcn_upload_conflict"); err != nil {
		t.Fatalf("set --wiki-token: %v", err)
	}

	runtime := common.TestNewRuntimeContextWithCtx(context.Background(), cmd, nil)
	err := DriveUpload.Validate(context.Background(), runtime)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("Validate() error = %v, want mutually exclusive error", err)
	}
}

func TestDriveUploadValidateRejectsExplicitEmptyWikiToken(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +upload"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("file-token", "", "")
	cmd.Flags().String("folder-token", "", "")
	cmd.Flags().String("wiki-token", "", "")
	cmd.Flags().String("name", "", "")
	if err := cmd.Flags().Set("file", "report.pdf"); err != nil {
		t.Fatalf("set --file: %v", err)
	}
	if err := cmd.Flags().Set("wiki-token", "   "); err != nil {
		t.Fatalf("set --wiki-token: %v", err)
	}

	runtime := common.TestNewRuntimeContextWithCtx(context.Background(), cmd, nil)
	err := DriveUpload.Validate(context.Background(), runtime)
	if err == nil || !strings.Contains(err.Error(), "--wiki-token cannot be empty") {
		t.Fatalf("Validate() error = %v, want empty wiki-token error", err)
	}
}

func TestDriveUploadValidateRejectsExplicitEmptyFileToken(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +upload"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("file-token", "", "")
	cmd.Flags().String("folder-token", "", "")
	cmd.Flags().String("wiki-token", "", "")
	cmd.Flags().String("name", "", "")
	if err := cmd.Flags().Set("file", "report.pdf"); err != nil {
		t.Fatalf("set --file: %v", err)
	}
	if err := cmd.Flags().Set("file-token", "   "); err != nil {
		t.Fatalf("set --file-token: %v", err)
	}

	runtime := common.TestNewRuntimeContextWithCtx(context.Background(), cmd, nil)
	err := DriveUpload.Validate(context.Background(), runtime)
	if err == nil || !strings.Contains(err.Error(), "--file-token cannot be empty") {
		t.Fatalf("Validate() error = %v, want empty file-token error", err)
	}
}

func TestDriveUploadValidateRejectsExplicitEmptyFolderToken(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +upload"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("file-token", "", "")
	cmd.Flags().String("folder-token", "", "")
	cmd.Flags().String("wiki-token", "", "")
	cmd.Flags().String("name", "", "")
	if err := cmd.Flags().Set("file", "report.pdf"); err != nil {
		t.Fatalf("set --file: %v", err)
	}
	if err := cmd.Flags().Set("folder-token", "   "); err != nil {
		t.Fatalf("set --folder-token: %v", err)
	}

	runtime := common.TestNewRuntimeContextWithCtx(context.Background(), cmd, nil)
	err := DriveUpload.Validate(context.Background(), runtime)
	if err == nil || !strings.Contains(err.Error(), "--folder-token cannot be empty") {
		t.Fatalf("Validate() error = %v, want empty folder-token error", err)
	}
}

func TestDriveUploadValidateRejectsInvalidTargetTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		flag    string
		value   string
		wantErr string
	}{
		{
			name:    "folder token",
			flag:    "folder-token",
			value:   "fld_bad?query=true",
			wantErr: "--folder-token contains invalid characters",
		},
		{
			name:    "wiki token",
			flag:    "wiki-token",
			value:   "wikcn_bad#fragment",
			wantErr: "--wiki-token contains invalid characters",
		},
		{
			name:    "file token",
			flag:    "file-token",
			value:   "box_bad?query=true",
			wantErr: "--file-token contains invalid characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := &cobra.Command{Use: "drive +upload"}
			cmd.Flags().String("file", "", "")
			cmd.Flags().String("file-token", "", "")
			cmd.Flags().String("folder-token", "", "")
			cmd.Flags().String("wiki-token", "", "")
			cmd.Flags().String("name", "", "")
			if err := cmd.Flags().Set("file", "report.pdf"); err != nil {
				t.Fatalf("set --file: %v", err)
			}
			if err := cmd.Flags().Set(tt.flag, tt.value); err != nil {
				t.Fatalf("set --%s: %v", tt.flag, err)
			}

			runtime := common.TestNewRuntimeContextWithCtx(context.Background(), cmd, nil)
			err := DriveUpload.Validate(context.Background(), runtime)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestDriveDownloadRejectsOverwriteWithoutFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	if err := os.WriteFile("existing.bin", []byte("old"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_123",
		"--output", "existing.bin",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected overwrite protection error, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDriveDownloadAllowsOverwriteFlag(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/file_123/download",
		Status:  200,
		Body:    []byte("new"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	if err := os.WriteFile("existing.bin", []byte("old"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_123",
		"--output", "existing.bin",
		"--overwrite",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile("existing.bin")
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "new" {
		t.Fatalf("downloaded file content = %q, want %q", string(data), "new")
	}
	if !strings.Contains(stdout.String(), "existing.bin") {
		t.Fatalf("stdout missing saved path: %s", stdout.String())
	}
}

type capturedDriveMultipart struct {
	Fields map[string]string
	Files  map[string][]byte
}

func decodeDriveMultipartBody(t *testing.T, stub *httpmock.Stub) capturedDriveMultipart {
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
	body := capturedDriveMultipart{Fields: map[string]string{}, Files: map[string][]byte{}}
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
