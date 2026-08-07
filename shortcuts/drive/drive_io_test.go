// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/brand"
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	configpkg "github.com/larksuite/cli/internal/config"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/identity"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

type driveRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn driveRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

var driveTaskCheckPollMu sync.Mutex

func driveTestConfig() *configpkg.CliConfig {
	return &configpkg.CliConfig{
		AppID: "drive-test-app", AppSecret: "test-secret", Brand: brand.Feishu,
	}
}

func mountAndRunDrive(t *testing.T, s common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	return mountAndRunDriveWithContext(t, context.Background(), s, args, f, stdout)
}

func mountAndRunDriveWithContext(t *testing.T, ctx context.Context, s common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "drive"}
	s.Mount(parent, f)
	parent.SetContext(ctx)
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
	uploadTestConfig := &configpkg.CliConfig{
		AppID: "drive-upload-test-app", AppSecret: "test-secret", Brand: brand.Feishu,
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
	uploadTestConfig := &configpkg.CliConfig{
		AppID: "drive-upload-large-wiki-test", AppSecret: "test-secret", Brand: brand.Feishu,
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
	uploadTestConfig := &configpkg.CliConfig{
		AppID: "drive-upload-large-overwrite-test", AppSecret: "test-secret", Brand: brand.Feishu,
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
	uploadTestConfig := &configpkg.CliConfig{
		AppID: "drive-upload-large-overwrite-version-test", AppSecret: "test-secret", Brand: brand.Feishu,
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
	uploadTestConfig := &configpkg.CliConfig{
		AppID: "drive-upload-large-overwrite-data-version-test", AppSecret: "test-secret", Brand: brand.Feishu,
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
	uploadTestConfig := &configpkg.CliConfig{
		AppID: "drive-upload-small-test", AppSecret: "test-secret", Brand: brand.Feishu,
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
	uploadTestConfig := &configpkg.CliConfig{
		AppID: "drive-upload-small-overwrite-test", AppSecret: "test-secret", Brand: brand.Feishu,
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
	uploadTestConfig := &configpkg.CliConfig{
		AppID: "drive-upload-small-data-version-test", AppSecret: "test-secret", Brand: brand.Feishu,
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
	uploadTestConfig := &configpkg.CliConfig{
		AppID: "drive-upload-small-wiki-test", AppSecret: "test-secret", Brand: brand.Feishu,
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

func TestDriveUploadUsesMetaURLForExplorerParent(t *testing.T) {
	uploadTestConfig := &configpkg.CliConfig{
		AppID: "drive-upload-explorer-meta-url", AppSecret: "test-secret", Brand: brand.Feishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"file_token": "file_explorer_small"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"doc_token": "file_explorer_small", "doc_type": "file", "url": "https://tenant.example.com/file/file_explorer_small"},
				},
			},
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
	if got, want := data["url"], "https://tenant.example.com/file/file_explorer_small"; got != want {
		t.Fatalf("data.url = %#v, want %q (metadata URL)", got, want)
	}
}

func TestDriveUploadUsesMetaURLForWikiParent(t *testing.T) {
	uploadTestConfig := &configpkg.CliConfig{
		AppID: "drive-upload-wiki-meta-url", AppSecret: "test-secret", Brand: brand.Feishu,
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
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"doc_token": "file_wiki_small", "doc_type": "file", "url": "https://tenant.example.com/file/file_wiki_small"},
				},
			},
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
	if got, want := data["url"], "https://tenant.example.com/file/file_wiki_small"; got != want {
		t.Fatalf("data.url = %#v, want %q (metadata URL)", got, want)
	}
}

func TestDriveUploadSmallFileAPIError(t *testing.T) {
	uploadTestConfig := &configpkg.CliConfig{
		AppID: "drive-upload-small-err", AppSecret: "test-secret", Brand: brand.Feishu,
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
	uploadTestConfig := &configpkg.CliConfig{
		AppID: "drive-upload-small-notoken", AppSecret: "test-secret", Brand: brand.Feishu,
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
	uploadTestConfig := &configpkg.CliConfig{
		AppID: "drive-upload-small-json", AppSecret: "test-secret", Brand: brand.Feishu,
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
	uploadTestConfig := &configpkg.CliConfig{
		AppID: "drive-upload-prepare-bad", AppSecret: "test-secret", Brand: brand.Feishu,
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
	uploadTestConfig := &configpkg.CliConfig{
		AppID: "drive-upload-part-err", AppSecret: "test-secret", Brand: brand.Feishu,
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
	uploadTestConfig := &configpkg.CliConfig{
		AppID: "drive-upload-part-json", AppSecret: "test-secret", Brand: brand.Feishu,
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
	uploadTestConfig := &configpkg.CliConfig{
		AppID: "drive-upload-finish-notoken", AppSecret: "test-secret", Brand: brand.Feishu,
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
	uploadTestConfig := &configpkg.CliConfig{
		AppID: "drive-upload-name-test", AppSecret: "test-secret", Brand: brand.Feishu,
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

	runtime := common.TestNewRuntimeContextWithIdentity(cmd, nil, identity.AsBot)
	dry := DriveUpload.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}

	var got struct {
		PostUploadNote string `json:"post_upload_note"`
		API            []struct {
			URL  string                 `json:"url"`
			Body map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run json: %v", err)
	}
	if len(got.API) != 2 {
		t.Fatalf("expected 2 API calls, got %d", len(got.API))
	}
	if got.API[0].Body["parent_type"] != driveUploadParentTypeWiki {
		t.Fatalf("parent_type = %#v, want %q", got.API[0].Body["parent_type"], driveUploadParentTypeWiki)
	}
	if got.API[0].Body["parent_node"] != "wikcn_dryrun_upload_target" {
		t.Fatalf("parent_node = %#v, want %q", got.API[0].Body["parent_node"], "wikcn_dryrun_upload_target")
	}
	if got.API[1].URL != "/open-apis/drive/v1/metas/batch_query" {
		t.Fatalf("metadata URL = %q, want metas/batch_query", got.API[1].URL)
	}
	if got.API[1].Body["with_url"] != true {
		t.Fatalf("metadata with_url = %#v, want true", got.API[1].Body["with_url"])
	}
	wantPostUploadNote := "After file upload succeeds in bot mode, the CLI will also try to grant the current CLI user full_access on the new file."
	if got.PostUploadNote != wantPostUploadNote {
		t.Fatalf("post_upload_note = %q, want %q", got.PostUploadNote, wantPostUploadNote)
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
			URL  string                 `json:"url"`
			Body map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run json: %v", err)
	}
	if len(got.API) != 2 {
		t.Fatalf("expected 2 API calls, got %d", len(got.API))
	}
	if got.API[0].Body["file_token"] != "boxcn_dryrun_overwrite" {
		t.Fatalf("file_token = %#v, want %q", got.API[0].Body["file_token"], "boxcn_dryrun_overwrite")
	}
	if got.API[1].URL != "/open-apis/drive/v1/metas/batch_query" {
		t.Fatalf("metadata URL = %q, want metas/batch_query", got.API[1].URL)
	}
	if got.API[1].Body["with_url"] != true {
		t.Fatalf("metadata with_url = %#v, want true", got.API[1].Body["with_url"])
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
	if len(got.API) != 2 {
		t.Fatalf("expected 2 API calls, got %d", len(got.API))
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
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("Validate() error = %T %v, want *errs.ValidationError", err, err)
	}
	if verr.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype = %q, want %q", verr.Subtype, errs.SubtypeInvalidArgument)
	}
	if !strings.Contains(verr.Error(), "mutually exclusive") {
		t.Fatalf("Validate() error = %v, want mutually exclusive error", err)
	}
	// Multi-flag conflict carries no single Param.
	if verr.Param != "" {
		t.Fatalf("Param = %q, want empty for multi-flag conflict", verr.Param)
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
	assertDriveValidationParam(t, err, "--wiki-token", "--wiki-token cannot be empty")
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
	assertDriveValidationParam(t, err, "--file-token", "--file-token cannot be empty")
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
	assertDriveValidationParam(t, err, "--folder-token", "--folder-token cannot be empty")
}

// assertDriveValidationParam asserts err is a typed *errs.ValidationError with
// SubtypeInvalidArgument, the given Param, and a message containing wantMsg.
func assertDriveValidationParam(t *testing.T, err error, wantParam, wantMsg string) {
	t.Helper()
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error = %T %v, want *errs.ValidationError", err, err)
	}
	if verr.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype = %q, want %q", verr.Subtype, errs.SubtypeInvalidArgument)
	}
	if verr.Param != wantParam {
		t.Fatalf("Param = %q, want %q", verr.Param, wantParam)
	}
	if !strings.Contains(verr.Error(), wantMsg) {
		t.Fatalf("error = %q, want substring %q", verr.Error(), wantMsg)
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

func TestDriveDownloadHTTP403SuggestsPreview(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/file_403/download",
		Status:  http.StatusForbidden,
		RawBody: []byte("permission denied"),
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_403",
		"--output", "blocked.md",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected HTTP 403 error, got nil")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if problem.Category != errs.CategoryNetwork {
		t.Fatalf("category=%q, want network", problem.Category)
	}
	if problem.Code != http.StatusForbidden {
		t.Fatalf("code=%d, want %d", problem.Code, http.StatusForbidden)
	}
	if !strings.Contains(problem.Hint, "drive +preview") {
		t.Fatalf("hint=%q, want preview guidance", problem.Hint)
	}
	if strings.Contains(problem.Hint, "file_403") {
		t.Fatalf("hint=%q, want placeholder file token", problem.Hint)
	}
	if !strings.Contains(problem.Hint, "--file-token <FILE_TOKEN>") {
		t.Fatalf("hint=%q, want file token placeholder", problem.Hint)
	}
	if !strings.Contains(problem.Hint, "--type source_file") || !strings.Contains(problem.Hint, "--output <path>") {
		t.Fatalf("hint=%q, want source_file output command", problem.Hint)
	}
}

func TestDriveDownloadHTTP404DoesNotSuggestPreview(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/file_missing/download",
		Status:  http.StatusNotFound,
		RawBody: []byte("not found"),
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_missing",
		"--output", "missing.md",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected HTTP 404 error, got nil")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if problem.Code != http.StatusNotFound {
		t.Fatalf("code=%d, want %d", problem.Code, http.StatusNotFound)
	}
	if strings.Contains(problem.Hint, "drive +preview") {
		t.Fatalf("hint=%q, want no preview guidance for non-403", problem.Hint)
	}
}

func TestDriveDownloadDefaultOutputPathSanitizesSlashOnlyNames(t *testing.T) {
	header := http.Header{
		"Content-Disposition": []string{`attachment; filename="////"`},
		"Content-Type":        []string{"application/octet-stream"},
	}
	if got := mustDriveDownloadDefaultOutputPath(t, header, "////", "file_token", nil); got != "file_token" {
		t.Fatalf("default output path = %q, want file_token", got)
	}
	if got := driveDownloadFallbackFileName(`\\`, "file_token"); got != "file_token" {
		t.Fatalf("fallback filename = %q, want file_token", got)
	}
}

func TestDriveDownloadDefaultOutputPathSanitizesWindowsReservedCharacters(t *testing.T) {
	header := http.Header{
		"Content-Disposition": []string{`attachment; filename="Q1: forecast?.txt"`},
		"Content-Type":        []string{"text/plain"},
	}
	if got := mustDriveDownloadDefaultOutputPath(t, header, "Metadata Title", "file_token", nil); got != "Q1_ forecast_.txt" {
		t.Fatalf("default output path = %q, want Q1_ forecast_.txt", got)
	}

	header = http.Header{
		"Content-Type": []string{"text/plain; charset=utf-8"},
	}
	if got := mustDriveDownloadDefaultOutputPath(t, header, "Q1: forecast?", "file_token", nil); got != "Q1_ forecast_.txt" {
		t.Fatalf("metadata fallback output path = %q, want Q1_ forecast_.txt", got)
	}
}

func TestDriveDownloadDefaultOutputPathRejectsWindowsReservedDeviceNames(t *testing.T) {
	header := http.Header{
		"Content-Disposition": []string{`attachment; filename="CON.txt"`},
		"Content-Type":        []string{"text/plain"},
	}
	if got := mustDriveDownloadDefaultOutputPath(t, header, "Metadata Title", "file_token", nil); got != "Metadata Title.txt" {
		t.Fatalf("default output path = %q, want Metadata Title.txt", got)
	}

	header = http.Header{
		"Content-Type": []string{"application/octet-stream"},
	}
	if got := mustDriveDownloadDefaultOutputPath(t, header, "COM1.pdf", "file_token", nil); got != "file_token" {
		t.Fatalf("metadata fallback output path = %q, want file_token", got)
	}
}

func TestDriveDownloadDefaultOutputPathFallsBackWhenHeaderCandidateFailsPathValidation(t *testing.T) {
	validatePath := func(path string) error {
		_, err := validate.SafeOutputPath(path)
		return err
	}

	header := http.Header{
		"Content-Disposition": []string{"attachment; filename=\"evil\u202etxt\""},
		"Content-Type":        []string{"text/plain"},
	}
	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	got := mustDriveDownloadDefaultOutputPath(t, header, "Metadata Title", "file_token", validatePath)
	if got != "Metadata Title.txt" {
		t.Fatalf("default output path = %q, want Metadata Title.txt", got)
	}

	header = http.Header{
		"Content-Type": []string{"text/plain"},
	}
	got = mustDriveDownloadDefaultOutputPath(t, header, "evil\u202etxt", "file_token", validatePath)
	if got != "file_token.txt" {
		t.Fatalf("metadata fallback output path = %q, want file_token.txt", got)
	}
}

func mustDriveDownloadDefaultOutputPath(t *testing.T, header http.Header, title, fileToken string, validatePath driveDownloadOutputPathValidator) string {
	t.Helper()
	got, err := driveDownloadDefaultOutputPath(header, title, fileToken, validatePath)
	if err != nil {
		t.Fatalf("driveDownloadDefaultOutputPath() error = %v", err)
	}
	return got
}

func TestDriveDownloadDryRunPlansMetadataWhenOutputOmitted(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_dryrun",
		"--dry-run",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	apis, _ := data["api"].([]interface{})
	if len(apis) != 2 {
		t.Fatalf("api count = %d, want 2\nstdout=%s", len(apis), stdout.String())
	}
	first, _ := apis[0].(map[string]interface{})
	if first["method"] != "POST" || first["url"] != "/open-apis/drive/v1/metas/batch_query" {
		t.Fatalf("first api = %#v, want metadata batch_query", first)
	}
	second, _ := apis[1].(map[string]interface{})
	if second["method"] != "GET" || second["url"] != "/open-apis/drive/v1/files/file_dryrun/download" {
		t.Fatalf("second api = %#v, want file download", second)
	}
	if second["desc"] != "[2] Download file bytes; Content-Disposition filename wins over metadata title when present" {
		t.Fatalf("second desc = %#v, want metadata-aware step 2", second["desc"])
	}
}

func TestDriveDownloadDryRunExplicitOutputSkipsMetadata(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_dryrun",
		"--output", "report.bin",
		"--dry-run",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	apis, _ := data["api"].([]interface{})
	if len(apis) != 1 {
		t.Fatalf("api count = %d, want 1\nstdout=%s", len(apis), stdout.String())
	}
	first, _ := apis[0].(map[string]interface{})
	if first["method"] != "GET" || first["url"] != "/open-apis/drive/v1/files/file_dryrun/download" {
		t.Fatalf("api = %#v, want file download", first)
	}
	if first["desc"] != "[1] Download file bytes to the explicit output path" {
		t.Fatalf("api desc = %#v, want explicit-output step 1", first["desc"])
	}
	if data["output"] != "report.bin" {
		t.Fatalf("output = %#v, want report.bin", data["output"])
	}
}

func TestDriveDownloadOmittedOutputRequiresMetadataScope(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	f.Credential = credential.NewCredentialProvider(nil, nil, &driveStatusScopedTokenResolver{scopes: "drive:file:download"}, nil)

	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_no_scope",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected missing metadata scope error, got nil")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if problem.Category != errs.CategoryAuthorization || problem.Subtype != errs.SubtypeMissingScope {
		t.Fatalf("problem = category %q subtype %q, want authorization/missing_scope", problem.Category, problem.Subtype)
	}
}

func TestDriveDownloadRejectsInvalidFileToken(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "../bad",
		"--output", "report.bin",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected invalid file-token error, got nil")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T: %v", err, err)
	}
	if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument || validationErr.Param != "--file-token" {
		t.Fatalf("problem = category %q subtype %q param %q, want validation/invalid_argument/--file-token", problem.Category, problem.Subtype, validationErr.Param)
	}
}

func TestDriveDownloadRejectsUnsafeExplicitOutput(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_safe",
		"--output", "../report.bin",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected unsafe output error, got nil")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T: %v", err, err)
	}
	if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument || validationErr.Param != "--output" {
		t.Fatalf("problem = category %q subtype %q param %q, want validation/invalid_argument/--output", problem.Category, problem.Subtype, validationErr.Param)
	}
}

func TestDriveDownloadExplicitOutputSkipsMetadataScope(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	f.Credential = credential.NewCredentialProvider(nil, nil, &driveStatusScopedTokenResolver{scopes: "drive:file:download"}, nil)
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/file_no_meta_scope/download",
		Status:  200,
		RawBody: []byte("bytes"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_no_meta_scope",
		"--output", "explicit.bin",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(tmpDir, "explicit.bin")); err != nil || string(data) != "bytes" {
		t.Fatalf("explicit output content = %q, err=%v; want bytes", string(data), err)
	}
}

func TestDriveDownloadRejectsExistingDefaultOutputWithoutOverwrite(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"doc_token": "file_existing_title", "doc_type": "file", "title": "Existing Report"},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/file_existing_title/download",
		Status:  200,
		RawBody: []byte("new"),
		Headers: http.Header{"Content-Type": []string{"text/plain"}},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.WriteFile(filepath.Join(tmpDir, "Existing Report.txt"), []byte("old"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_existing_title",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected overwrite protection error, got nil")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T: %v", err, err)
	}
	if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument || validationErr.Param != "--output" {
		t.Fatalf("problem = category %q subtype %q param %q, want validation/invalid_argument/--output", problem.Category, problem.Subtype, validationErr.Param)
	}
}

func TestDriveDownloadUsesContentDispositionWhenOutputOmitted(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	metaStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"doc_token": "file_named", "doc_type": "file", "title": "Metadata Report"},
				},
			},
		},
	}
	reg.Register(metaStub)
	metadataSeenBeforeDownload := false
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/file_named/download",
		Status:  200,
		RawBody: []byte("downloaded"),
		Headers: http.Header{
			"Content-Type":        []string{"application/octet-stream"},
			"Content-Disposition": []string{`attachment; filename="server-report.md"`},
		},
		OnMatch: func(req *http.Request) {
			metadataSeenBeforeDownload = len(metaStub.CapturedBody) > 0
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_named",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !metadataSeenBeforeDownload {
		t.Fatal("metadata title lookup must happen before download")
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "server-report.md"))
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "downloaded" {
		t.Fatalf("downloaded content = %q, want downloaded", string(data))
	}
	out := decodeDriveEnvelope(t, stdout)
	if got := filepath.Base(common.GetString(out, "saved_path")); got != "server-report.md" {
		t.Fatalf("saved_path base=%q, want server-report.md\nstdout=%s", got, stdout.String())
	}
}

func TestDriveDownloadFallsBackToMetadataTitleWhenOutputOmitted(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"doc_token": "file_title", "doc_type": "file", "title": "Quarterly Report"},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/file_title/download",
		Status:  200,
		RawBody: []byte("plain text"),
		Headers: http.Header{
			"Content-Type": []string{"text/plain; charset=utf-8"},
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_title",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "Quarterly Report.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "plain text" {
		t.Fatalf("downloaded content = %q, want plain text", string(data))
	}
	out := decodeDriveEnvelope(t, stdout)
	if got := filepath.Base(common.GetString(out, "saved_path")); got != "Quarterly Report.txt" {
		t.Fatalf("saved_path base=%q, want Quarterly Report.txt\nstdout=%s", got, stdout.String())
	}
}

func TestDriveDownloadFallsBackToTokenWhenOutputOmittedAndMetadataEmpty(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/file_empty/download",
		Status:  200,
		RawBody: []byte("bytes"),
		Headers: http.Header{
			"Content-Type": []string{"application/octet-stream"},
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_empty",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "file_empty"))
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "bytes" {
		t.Fatalf("downloaded content = %q, want bytes", string(data))
	}
}

func TestDriveDownloadMetadataNonPermissionErrorContinuesWithTokenFallback(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 99991400,
			"msg":  "rate limit",
		},
	})
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/file_rate_limited/download",
		Status:  200,
		RawBody: []byte("bytes"),
		Headers: http.Header{
			"Content-Type": []string{"application/octet-stream"},
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_rate_limited",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr.String(), "warning: metadata title lookup failed") {
		t.Fatalf("stderr missing metadata warning: %s", stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(tmpDir, "file_rate_limited"))
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "bytes" {
		t.Fatalf("downloaded content = %q, want bytes", string(data))
	}
	out := decodeDriveEnvelope(t, stdout)
	if got := filepath.Base(common.GetString(out, "saved_path")); got != "file_rate_limited" {
		t.Fatalf("saved_path base=%q, want file_rate_limited\nstdout=%s", got, stdout.String())
	}
}

func TestDriveDownloadTypedMetadataTimeoutFallsBack(t *testing.T) {
	err := errs.NewNetworkError(errs.SubtypeNetworkTimeout, "metadata lookup timed out")
	if driveDownloadShouldFailOnMetadataTitleError(context.Background(), err) {
		t.Fatal("typed metadata timeout should use warning fallback")
	}
}

func TestDriveDownloadMetadataContextErrorStopsBeforeDownload(t *testing.T) {
	for _, tc := range []struct {
		name     string
		wantErr  error
		makeCtx  func() (context.Context, context.CancelFunc)
		cancelIn func(context.CancelFunc, *http.Request)
	}{
		{
			name:    "canceled",
			wantErr: context.Canceled,
			makeCtx: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(context.Background())
			},
			cancelIn: func(cancel context.CancelFunc, req *http.Request) {
				cancel()
				<-req.Context().Done()
			},
		},
		{
			name:    "deadline",
			wantErr: context.DeadlineExceeded,
			makeCtx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 20*time.Millisecond)
			},
			cancelIn: func(_ context.CancelFunc, req *http.Request) {
				<-req.Context().Done()
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runCtx, cancel := tc.makeCtx()
			defer cancel()

			cfg := driveTestConfig()
			f, _, _, _ := cmdutil.TestFactory(t, cfg)
			metadataRequests := 0
			downloadRequests := 0
			f.LarkClient = func() (*lark.Client, error) {
				return lark.NewClient(
					cfg.AppID,
					credential.RuntimeAppSecret(cfg.AppSecret),
					lark.WithEnableTokenCache(false),
					lark.WithLogLevel(larkcore.LogLevelError),
					lark.WithOpenBaseUrl(brand.ResolveOpenBaseURL(cfg.Brand)),
					lark.WithHttpClient(&http.Client{Transport: driveRoundTripFunc(func(req *http.Request) (*http.Response, error) {
						if strings.Contains(req.URL.Path, "/metas/batch_query") {
							metadataRequests++
							tc.cancelIn(cancel, req)
							return nil, req.Context().Err()
						}
						if strings.Contains(req.URL.Path, "/download") {
							downloadRequests++
						}
						return nil, errors.New("unexpected request after metadata context error")
					})}),
				), nil
			}

			tmpDir := t.TempDir()
			withDriveWorkingDir(t, tmpDir)

			err := mountAndRunDriveWithContext(t, runCtx, DriveDownload, []string{
				"+download",
				"--file-token", "file_context_error",
				"--as", "bot",
			}, f, nil)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if metadataRequests != 1 {
				t.Fatalf("metadata requests = %d, want 1", metadataRequests)
			}
			if downloadRequests != 0 {
				t.Fatalf("download requests = %d, want 0", downloadRequests)
			}
		})
	}
}

func TestDriveDownloadMetadataErrorBeforeDownloadWhenOutputOmitted(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 99991679,
			"msg":  "missing scope",
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_no_meta",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected metadata lookup error, got nil")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if problem.Category != errs.CategoryAuthorization || problem.Subtype != errs.SubtypeMissingScope || problem.Code != 99991679 {
		t.Fatalf("problem = category %q subtype %q code %d, want authorization/missing_scope/99991679", problem.Category, problem.Subtype, problem.Code)
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
