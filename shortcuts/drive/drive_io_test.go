// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

type driveRoundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip delegates an HTTP request to the test transport function.
func (fn driveRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

var driveTaskCheckPollMu sync.Mutex

// driveTestConfig returns isolated credentials for Drive command tests.
func driveTestConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID: "drive-test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
}

func registerDriveDownloadExportAuth(reg *httpmock.Registry, fileToken string, allowed bool) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/drive/v1/permissions/" + fileToken + "/members/auth",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"auth_result": allowed},
		},
	}
	reg.Register(stub)
	return stub
}

// mountAndRunDrive executes a mounted Drive command with a background context.
func mountAndRunDrive(t *testing.T, s common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	return mountAndRunDriveWithContext(t, context.Background(), s, args, f, stdout)
}

// mountAndRunDriveWithContext executes a mounted Drive command with the supplied context.
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

// withSingleDriveTaskCheckPoll limits task-result polling to one attempt for a test.
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

// withDriveWorkingDir changes into dir and restores the original directory during cleanup.
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

// TestDriveUploadLargeFileUsesMultipart verifies large uploads use the multipart workflow.
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

// TestDriveUploadLargeFileToWikiUsesMultipart verifies large Wiki uploads use multipart requests.
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

// TestDriveUploadLargeFileOverwriteUsesMultipart verifies large overwrites retain multipart semantics.
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

// TestDriveUploadLargeFileOverwriteReturnsVersionFromUploadFinish verifies the finish response version is returned.
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

// TestDriveUploadLargeFileOverwriteReturnsVersionFromUploadFinishAlias verifies the version alias is accepted.
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

// TestDriveUploadSmallFile verifies the single-part Drive upload flow.
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

// TestDriveUploadSmallFileOverwriteUsesFileToken verifies overwrite requests preserve the target token.
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

// TestDriveUploadReturnsVersionFromDataVersionAlias verifies upload responses accept the data version alias.
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

// TestDriveUploadSmallFileToWiki verifies a small file can target a Wiki parent.
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

// TestDriveUploadUsesMetaURLForExplorerParent verifies Explorer URLs resolve through Drive metadata.
func TestDriveUploadUsesMetaURLForExplorerParent(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-explorer-meta-url", AppSecret: "test-secret", Brand: core.BrandFeishu,
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

// TestDriveUploadUsesMetaURLForWikiParent verifies Wiki URLs resolve through Drive metadata.
func TestDriveUploadUsesMetaURLForWikiParent(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-wiki-meta-url", AppSecret: "test-secret", Brand: core.BrandFeishu,
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

// TestDriveUploadSmallFileAPIError verifies upload API failures remain typed errors.
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

// TestDriveUploadSmallFileNoToken verifies a successful response without a token is rejected.
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

// TestDriveUploadSmallFileInvalidJSON verifies malformed upload responses are rejected.
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

// TestDriveUploadPrepareInvalidResponse verifies malformed multipart prepare responses are rejected.
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

// TestDriveUploadPartAPIError verifies multipart part failures remain typed errors.
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

// TestDriveUploadPartInvalidJSON verifies malformed multipart part responses are rejected.
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

// TestDriveUploadFinishNoToken verifies multipart completion requires a returned file token.
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

// TestDriveUploadWithCustomName verifies that an explicit remote name is preserved.
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

// TestDriveUploadDryRunUsesWikiTarget verifies the wiki mount point in the upload plan.
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

	runtime := common.TestNewRuntimeContextWithIdentity(cmd, nil, core.AsBot)
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
	if len(got.API) != 3 {
		t.Fatalf("expected 3 API calls, got %d", len(got.API))
	}
	if got.API[0].Body["parent_type"] != driveUploadParentTypeWiki {
		t.Fatalf("parent_type = %#v, want %q", got.API[0].Body["parent_type"], driveUploadParentTypeWiki)
	}
	if got.API[0].Body["parent_node"] != "wikcn_dryrun_upload_target" {
		t.Fatalf("parent_node = %#v, want %q", got.API[0].Body["parent_node"], "wikcn_dryrun_upload_target")
	}
	if got.API[1].URL != "/open-apis/drive/v1/lark_cli_file_event/report" {
		t.Fatalf("report URL = %q, want lark_cli_file_event/report", got.API[1].URL)
	}
	if got.API[2].URL != "/open-apis/drive/v1/metas/batch_query" {
		t.Fatalf("metadata URL = %q, want metas/batch_query", got.API[2].URL)
	}
	if got.API[2].Body["with_url"] != true {
		t.Fatalf("metadata with_url = %#v, want true", got.API[2].Body["with_url"])
	}
	wantPostUploadNote := "After file upload succeeds in bot mode, the CLI will also try to grant the current CLI user full_access on the new file."
	if got.PostUploadNote != wantPostUploadNote {
		t.Fatalf("post_upload_note = %q, want %q", got.PostUploadNote, wantPostUploadNote)
	}
}

// TestNewDriveUploadSpecPreservesPathAndName verifies the upload specification mirrors its flags.
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

// TestDriveUploadDryRunIncludesFileToken verifies overwrite tokens in the upload plan.
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
	if len(got.API) != 3 {
		t.Fatalf("expected 3 API calls, got %d", len(got.API))
	}
	if got.API[0].Body["file_token"] != "boxcn_dryrun_overwrite" {
		t.Fatalf("file_token = %#v, want %q", got.API[0].Body["file_token"], "boxcn_dryrun_overwrite")
	}
	if got.API[1].URL != "/open-apis/drive/v1/lark_cli_file_event/report" {
		t.Fatalf("report URL = %q, want lark_cli_file_event/report", got.API[1].URL)
	}
	if got.API[2].URL != "/open-apis/drive/v1/metas/batch_query" {
		t.Fatalf("metadata URL = %q, want metas/batch_query", got.API[2].URL)
	}
	if got.API[2].Body["with_url"] != true {
		t.Fatalf("metadata with_url = %#v, want true", got.API[2].Body["with_url"])
	}
}

// TestDriveUploadDryRunBotOverwriteSkipsPermissionGrantHint verifies that bot overwrites omit the grant hint.
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
	if len(got.API) != 3 {
		t.Fatalf("expected 3 API calls, got %d", len(got.API))
	}
	if got.API[0].Body["file_token"] != "boxcn_dryrun_overwrite" {
		t.Fatalf("file_token = %#v, want %q", got.API[0].Body["file_token"], "boxcn_dryrun_overwrite")
	}
	if strings.Contains(got.API[0].Desc, "grant the current CLI user full_access") {
		t.Fatalf("dry-run desc should skip permission-grant hint for overwrite, got %q", got.API[0].Desc)
	}
}

// TestDriveUploadTargetLabel verifies upload target labels describe the resolved destination.
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

// TestDriveUploadValidateRejectsConflictingTargets verifies mutually exclusive upload targets are rejected.
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

// TestDriveUploadValidateRejectsExplicitEmptyWikiToken verifies an explicitly empty Wiki token is rejected.
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

// TestDriveUploadValidateRejectsExplicitEmptyFileToken verifies an explicitly empty file token is rejected.
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

// TestDriveUploadValidateRejectsExplicitEmptyFolderToken verifies an explicitly empty folder token is rejected.
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

// TestDriveUploadValidateRejectsInvalidTargetTokens verifies unsafe upload target tokens are rejected.
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

// TestDriveDownloadRejectsOverwriteWithoutFlag verifies existing output is protected by default.
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

// TestDriveDownloadAllowsOverwriteFlag verifies the overwrite flag permits replacing output.
func TestDriveDownloadAllowsOverwriteFlag(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	registerDriveDownloadExportAuth(reg, "file_123", true)
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

// TestDriveDownloadHTTP403SuggestsPreview verifies forbidden downloads suggest the preview workflow.
func TestDriveDownloadHTTP403SuggestsPreview(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	registerDriveDownloadExportAuth(reg, "file_403", true)
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
	if strings.Contains(problem.Hint, "--list-only") || strings.Contains(problem.Hint, "PDF/text/image preview choices") {
		t.Fatalf("hint=%q, want only the source_file recovery command", problem.Hint)
	}
}

// TestDriveDownloadHTTP404DoesNotSuggestPreview verifies missing files do not receive permission guidance.
func TestDriveDownloadHTTP404DoesNotSuggestPreview(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	registerDriveDownloadExportAuth(reg, "file_missing", true)
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

func TestDriveDownloadExportDeniedFailsBeforeDownload(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	registerDriveDownloadExportAuth(reg, "file_export_denied", false)
	downloadCalls := 0
	reg.Register(&httpmock.Stub{
		Method:   http.MethodGet,
		URL:      "/open-apis/drive/v1/files/file_export_denied/download",
		Optional: true,
		OnMatch: func(*http.Request) {
			downloadCalls++
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_export_denied",
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
	for _, want := range []string{"drive +preview", "--file-token <FILE_TOKEN>", "--type source_file", "--output <path>"} {
		if !strings.Contains(problem.Hint, want) {
			t.Fatalf("hint=%q, want %q", problem.Hint, want)
		}
	}
	if strings.Contains(problem.Hint, "file_export_denied") || strings.Contains(problem.Hint, "--list-only") {
		t.Fatalf("hint=%q, want placeholder-only source_file guidance", problem.Hint)
	}
	if downloadCalls != 0 {
		t.Fatalf("download calls = %d, want 0", downloadCalls)
	}
	if _, statErr := os.Stat(filepath.Join(tmpDir, "blocked.bin")); !os.IsNotExist(statErr) {
		t.Fatalf("download target should not be created, statErr=%v", statErr)
	}
}

func TestDriveDownloadMalformedExportAuthStopsBeforeDownload(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/drive/v1/permissions/file_auth_malformed/members/auth",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"auth_result": "true"},
		},
	})
	downloadCalls := 0
	reg.Register(&httpmock.Stub{
		Method:   http.MethodGet,
		URL:      "/open-apis/drive/v1/files/file_auth_malformed/download",
		Optional: true,
		OnMatch: func(*http.Request) {
			downloadCalls++
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_auth_malformed",
		"--output", "blocked.bin",
		"--as", "bot",
	}, f, nil)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("problem=%+v ok=%v, want internal/invalid_response", problem, ok)
	}
	if downloadCalls != 0 {
		t.Fatalf("download calls = %d, want 0", downloadCalls)
	}
}

func TestDriveDownloadHTTP429SuggestsBackoff(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	registerDriveDownloadExportAuth(reg, "file_download_limited", true)
	reg.Register(&httpmock.Stub{
		Method:  http.MethodGet,
		URL:     "/open-apis/drive/v1/files/file_download_limited/download",
		Status:  http.StatusTooManyRequests,
		RawBody: []byte("rate limited"),
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_download_limited",
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

func TestDriveDownloadExportAuthRateLimitPreservesAPIErrorAndSuggestsBackoff(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/drive/v1/permissions/file_auth_limited/members/auth",
		Body: map[string]interface{}{
			"code":   99991400,
			"msg":    "rate limited",
			"log_id": "log-drive-auth-limited",
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_auth_limited",
		"--output", "blocked.bin",
		"--as", "bot",
	}, f, nil)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryAPI || problem.Subtype != errs.SubtypeRateLimit || problem.Code != 99991400 {
		t.Fatalf("problem=%+v ok=%v, want api/rate_limit/99991400", problem, ok)
	}
	if problem.LogID != "log-drive-auth-limited" || !problem.Retryable {
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

func TestDriveDownloadTypedRateLimitSuggestsBackoff(t *testing.T) {
	err := errs.NewAPIError(errs.SubtypeRateLimit, "request trigger frequency limit").
		WithCode(99991400).
		WithRetryable().
		WithHint("upstream hint")

	got := withDriveDownloadRecoveryHint(err, "file_secret")
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

// TestDriveDownloadDefaultOutputPathSanitizesSlashOnlyNames verifies slash-only names fall back safely.
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

// TestDriveDownloadDefaultOutputPathSanitizesWindowsReservedCharacters verifies reserved characters are sanitized.
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

// TestDriveDownloadDefaultOutputPathRejectsWindowsReservedDeviceNames verifies reserved device names are rejected.
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

// TestDriveDownloadDefaultOutputPathFallsBackWhenHeaderCandidateFailsPathValidation verifies invalid header names use a safe fallback.
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

// mustDriveDownloadDefaultOutputPath resolves a default download path or fails the test.
func mustDriveDownloadDefaultOutputPath(t *testing.T, header http.Header, title, fileToken string, validatePath driveDownloadOutputPathValidator) string {
	t.Helper()
	got, err := driveDownloadDefaultOutputPath(header, title, fileToken, validatePath)
	if err != nil {
		t.Fatalf("driveDownloadDefaultOutputPath() error = %v", err)
	}
	return got
}

// TestDriveDownloadDryRunPlansMetadataWhenOutputOmitted verifies default naming plans a metadata lookup.
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
	if len(apis) != 3 {
		t.Fatalf("api count = %d, want 3\nstdout=%s", len(apis), stdout.String())
	}
	first, _ := apis[0].(map[string]interface{})
	if first["method"] != "GET" || first["url"] != "/open-apis/drive/v1/permissions/file_dryrun/members/auth" {
		t.Fatalf("first api = %#v, want export permission auth", first)
	}
	firstParams, _ := first["params"].(map[string]interface{})
	if firstParams["type"] != "file" || firstParams["action"] != "export" {
		t.Fatalf("first params = %#v, want type=file action=export", firstParams)
	}
	second, _ := apis[1].(map[string]interface{})
	if second["method"] != "POST" || second["url"] != "/open-apis/drive/v1/metas/batch_query" {
		t.Fatalf("second api = %#v, want metadata batch_query", second)
	}
	third, _ := apis[2].(map[string]interface{})
	if third["method"] != "GET" || third["url"] != "/open-apis/drive/v1/files/file_dryrun/download" {
		t.Fatalf("third api = %#v, want file download", third)
	}
	if third["desc"] != "[3] Download file bytes; Content-Disposition filename wins over metadata title when present" {
		t.Fatalf("third desc = %#v, want metadata-aware step 3", third["desc"])
	}
}

// TestDriveDownloadDryRunExplicitOutputSkipsMetadata verifies explicit output avoids metadata lookup.
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
	if len(apis) != 2 {
		t.Fatalf("api count = %d, want 2\nstdout=%s", len(apis), stdout.String())
	}
	first, _ := apis[0].(map[string]interface{})
	if first["method"] != "GET" || first["url"] != "/open-apis/drive/v1/permissions/file_dryrun/members/auth" {
		t.Fatalf("first api = %#v, want export permission auth", first)
	}
	second, _ := apis[1].(map[string]interface{})
	if second["method"] != "GET" || second["url"] != "/open-apis/drive/v1/files/file_dryrun/download" {
		t.Fatalf("second api = %#v, want file download", second)
	}
	if second["desc"] != "[2] Download file bytes to the explicit output path" {
		t.Fatalf("api desc = %#v, want explicit-output step 2", second["desc"])
	}
	if data["output"] != "report.bin" {
		t.Fatalf("output = %#v, want report.bin", data["output"])
	}
}

// TestDriveDownloadOmittedOutputRequiresMetadataScope verifies default naming declares its metadata scope.
func TestDriveDownloadOmittedOutputRequiresMetadataScope(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	f.Credential = credential.NewCredentialProvider(nil, nil, &driveStatusScopedTokenResolver{scopes: "drive:file:download " + common.DrivePermissionMemberAuthScope}, nil)

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

func TestDriveDownloadTreatsPermissionMemberAuthScopeAsNonBlocking(t *testing.T) {
	for _, scope := range DriveDownload.Scopes {
		if scope == common.DrivePermissionMemberAuthScope {
			t.Fatalf("DriveDownload.Scopes = %v, permission auth scope must not be an unconditional preflight", DriveDownload.Scopes)
		}
	}
	if !slices.Contains(DriveDownload.ConditionalScopes, common.DrivePermissionMemberAuthScope) {
		t.Fatalf("DriveDownload.ConditionalScopes = %v, want best-effort scope %q", DriveDownload.ConditionalScopes, common.DrivePermissionMemberAuthScope)
	}
}

func TestDriveDownloadPermissionAuthScopeErrorsWarnAndContinue(t *testing.T) {
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
			f, _, stderr, reg := cmdutil.TestFactory(t, driveTestConfig())
			f.Credential = credential.NewCredentialProvider(nil, nil, &driveStatusScopedTokenResolver{scopes: "drive:file:download"}, nil)
			fileToken := "file_" + tt.name
			reg.Register(&httpmock.Stub{
				Method: http.MethodGet,
				URL:    "/open-apis/drive/v1/permissions/" + fileToken + "/members/auth",
				Body: map[string]interface{}{
					"code": tt.code,
					"msg":  tt.msg,
				},
			})
			reg.Register(&httpmock.Stub{
				Method:  http.MethodGet,
				URL:     "/open-apis/drive/v1/files/" + fileToken + "/download",
				Status:  http.StatusOK,
				RawBody: []byte("downloaded without permission auth scope"),
				Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
			})

			tmpDir := t.TempDir()
			withDriveWorkingDir(t, tmpDir)
			err := mountAndRunDrive(t, DriveDownload, []string{
				"+download",
				"--file-token", fileToken,
				"--output", "downloaded.bin",
				"--as", "bot",
			}, f, nil)
			if err != nil {
				t.Fatalf("download error = %v, want permission auth scope error %d to be non-blocking", err, tt.code)
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

// TestDriveDownloadRejectsInvalidFileToken verifies unsafe download tokens are rejected.
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

// TestDriveDownloadRejectsUnsafeExplicitOutput verifies unsafe output paths are rejected.
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

// TestDriveDownloadExplicitOutputSkipsMetadataScope verifies explicit output does not declare metadata scope.
func TestDriveDownloadExplicitOutputSkipsMetadataScope(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	f.Credential = credential.NewCredentialProvider(nil, nil, &driveStatusScopedTokenResolver{scopes: "drive:file:download " + common.DrivePermissionMemberAuthScope}, nil)
	registerDriveDownloadExportAuth(reg, "file_no_meta_scope", true)
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

// TestDriveDownloadRejectsExistingDefaultOutputWithoutOverwrite verifies default output also respects overwrite protection.
func TestDriveDownloadRejectsExistingDefaultOutputWithoutOverwrite(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	registerDriveDownloadExportAuth(reg, "file_existing_title", true)
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

// TestDriveDownloadUsesContentDispositionWhenOutputOmitted verifies response filenames take precedence.
func TestDriveDownloadUsesContentDispositionWhenOutputOmitted(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	authStub := registerDriveDownloadExportAuth(reg, "file_named", true)
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
			metadataSeenBeforeDownload = len(authStub.CapturedBodies) > 0 && len(metaStub.CapturedBody) > 0
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

// TestDriveDownloadFallsBackToMetadataTitleWhenOutputOmitted verifies metadata supplies the default filename.
func TestDriveDownloadFallsBackToMetadataTitleWhenOutputOmitted(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	registerDriveDownloadExportAuth(reg, "file_title", true)
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

// TestDriveDownloadFallsBackToTokenWhenOutputOmittedAndMetadataEmpty verifies the token is the final filename fallback.
func TestDriveDownloadFallsBackToTokenWhenOutputOmittedAndMetadataEmpty(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	registerDriveDownloadExportAuth(reg, "file_empty", true)
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

// TestDriveDownloadMetadataNonPermissionErrorContinuesWithTokenFallback verifies recoverable metadata failures use the token.
func TestDriveDownloadMetadataNonPermissionErrorContinuesWithTokenFallback(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, driveTestConfig())
	registerDriveDownloadExportAuth(reg, "file_rate_limited", true)
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

// TestDriveDownloadTypedMetadataTimeoutFallsBack verifies typed metadata timeouts use fallback naming.
func TestDriveDownloadTypedMetadataTimeoutFallsBack(t *testing.T) {
	err := errs.NewNetworkError(errs.SubtypeNetworkTimeout, "metadata lookup timed out")
	if driveDownloadShouldFailOnMetadataTitleError(context.Background(), err) {
		t.Fatal("typed metadata timeout should use warning fallback")
	}
}

// TestDriveDownloadMetadataContextErrorStopsBeforeDownload verifies command cancellation prevents the download request.
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
			permissionRequests := 0
			metadataRequests := 0
			downloadRequests := 0
			f.LarkClient = func() (*lark.Client, error) {
				return lark.NewClient(
					cfg.AppID,
					credential.RuntimeAppSecret(cfg.AppSecret),
					lark.WithEnableTokenCache(false),
					lark.WithLogLevel(larkcore.LogLevelError),
					lark.WithOpenBaseUrl(core.ResolveOpenBaseURL(cfg.Brand)),
					lark.WithHttpClient(&http.Client{Transport: driveRoundTripFunc(func(req *http.Request) (*http.Response, error) {
						if strings.Contains(req.URL.Path, "/permissions/") {
							permissionRequests++
							return &http.Response{
								StatusCode: http.StatusOK,
								Header:     http.Header{"Content-Type": []string{"application/json"}},
								Body:       io.NopCloser(strings.NewReader(`{"code":0,"data":{"auth_result":true}}`)),
								Request:    req,
							}, nil
						}
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
			if permissionRequests != 1 {
				t.Fatalf("permission requests = %d, want 1", permissionRequests)
			}
			if downloadRequests != 0 {
				t.Fatalf("download requests = %d, want 0", downloadRequests)
			}
		})
	}
}

// TestDriveDownloadMetadataErrorBeforeDownloadWhenOutputOmitted verifies permission failures stop before downloading.
func TestDriveDownloadMetadataErrorBeforeDownloadWhenOutputOmitted(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	registerDriveDownloadExportAuth(reg, "file_no_meta", true)
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

// decodeDriveMultipartBody decodes one captured multipart request for assertions.
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

const driveReportFileEventPath = "/open-apis/drive/v1/lark_cli_file_event/report"

// testDriveCapacityExpansionURL is a placeholder capacity-expansion URL used in
// tests. It intentionally uses example.com so no internal endpoint is embedded
// in the repository.
const testDriveCapacityExpansionURL = "https://example.com/space/upload/pay/prepare"

// registerDriveReportStub registers a successful report_file_event stub.
func registerDriveReportStub(t *testing.T, reg *httpmock.Registry) *httpmock.Stub {
	t.Helper()
	return registerDriveReportStubWithMsg(t, reg, "")
}

// registerDriveReportStubWithMsg registers a report_file_event stub returning
// code 0 and, when msg is non-empty, carrying it as data.msg.
func registerDriveReportStubWithMsg(t *testing.T, reg *httpmock.Registry, msg string) *httpmock.Stub {
	t.Helper()
	body := map[string]interface{}{"code": 0, "data": map[string]interface{}{}}
	if msg != "" {
		body["msg"] = "success"
		body["data"] = map[string]interface{}{"msg": msg}
	}
	stub := &httpmock.Stub{
		Method:   "POST",
		URL:      driveReportFileEventPath,
		Body:     body,
		Reusable: true,
	}
	reg.Register(stub)
	return stub
}

// decodeDriveReportTags verifies one captured Drive report and returns its tags.
func decodeDriveReportTags(t *testing.T, stub *httpmock.Stub) map[string]interface{} {
	t.Helper()
	if len(stub.CapturedBodies) != 1 {
		t.Fatalf("report call count = %d, want 1", len(stub.CapturedBodies))
	}
	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBodies[0], &body); err != nil {
		t.Fatalf("decode report body: %v", err)
	}
	if got := body["file_scene"]; got != "lark-cli" {
		t.Fatalf("file_scene = %v, want lark-cli", got)
	}
	if got := body["scene"]; got != "upload" {
		t.Fatalf("scene = %v, want upload", got)
	}
	if _, ok := body["user_id"]; ok {
		t.Fatalf("user_id must be omitted, got %v", body["user_id"])
	}
	if _, ok := body["tenant_id"]; ok {
		t.Fatalf("tenant_id must be omitted, got %v", body["tenant_id"])
	}
	tags, ok := body["tags"].(map[string]interface{})
	if !ok {
		t.Fatalf("tags = %#v, want object", body["tags"])
	}
	return tags
}

// TestDriveUploadSmallFileReportFileEventOnSuccess verifies reporting after a small Drive upload succeeds.
func TestDriveUploadSmallFileReportFileEventOnSuccess(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-report-small-ok", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)
	reportStub := registerDriveReportStub(t, reg)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"file_token": "file_report_ok"},
		},
	})

	withDriveWorkingDir(t, t.TempDir())
	if err := os.WriteFile("small.bin", make([]byte, 1024), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDrive(t, DriveUpload, []string{
		"+upload", "--file", "small.bin", "--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected upload to succeed, got error: %v", err)
	}

	tags := decodeDriveReportTags(t, reportStub)
	if got := tags["status"]; got != "success" {
		t.Fatalf("tags.status = %v, want success", got)
	}
	if got := tags["api_path"]; got != "/open-apis/drive/v1/files/upload_all" {
		t.Fatalf("tags.api_path = %v", got)
	}
	if _, ok := tags["upload_mode"]; ok {
		t.Fatal("tags.upload_mode must be omitted")
	}
	if got := tags["resource_type"]; got != "file" {
		t.Fatalf("tags.resource_type = %v, want file", got)
	}
	if got := tags["mount_point"]; got != driveUploadParentTypeExplorer {
		t.Fatalf("tags.mount_point = %v, want %s", got, driveUploadParentTypeExplorer)
	}
	if got := tags["file_token"]; got != "file_report_ok" {
		t.Fatalf("tags.file_token = %v, want file_report_ok", got)
	}
}

// TestDriveUploadSmallFileReportFileEventOnError verifies reporting after a small Drive upload fails.
func TestDriveUploadSmallFileReportFileEventOnError(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-report-small-err", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)
	reportStub := registerDriveReportStubWithMsg(t, reg, testDriveCapacityExpansionURL)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body:   map[string]interface{}{"code": 1061101, "msg": "tenant capacity exceeded"},
	})

	withDriveWorkingDir(t, t.TempDir())
	if err := os.WriteFile("small.bin", make([]byte, 1024), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDrive(t, DriveUpload, []string{
		"+upload", "--file", "small.bin", "--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T (%v)", err, err)
	}
	if p.Code != 1061101 {
		t.Fatalf("code = %d, want original 1061101", p.Code)
	}
	if !strings.Contains(p.Hint, testDriveCapacityExpansionURL) {
		t.Fatalf("hint = %q, want capacity expansion URL", p.Hint)
	}

	tags := decodeDriveReportTags(t, reportStub)
	if got := tags["status"]; got != "error" {
		t.Fatalf("tags.status = %v, want error", got)
	}
	if got := tags["code"]; got != "1061101" {
		t.Fatalf("tags.code = %v, want 1061101", got)
	}
}

// TestDriveUploadLargeFileReportFileEventOnPrepareError verifies reporting when multipart preparation fails.
func TestDriveUploadLargeFileReportFileEventOnPrepareError(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-report-large-prepare-err", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)
	reportStub := registerDriveReportStubWithMsg(t, reg, testDriveCapacityExpansionURL)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_prepare",
		Body:   map[string]interface{}{"code": 1061101, "msg": "tenant capacity exceeded"},
	})

	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
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
		"+upload", "--file", "large.bin", "--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Code != 1061101 {
		t.Fatalf("expected typed api error code 1061101, got %T (%v)", err, err)
	}
	if !strings.Contains(p.Hint, testDriveCapacityExpansionURL) {
		t.Fatalf("hint = %q, want capacity expansion URL", p.Hint)
	}

	tags := decodeDriveReportTags(t, reportStub)
	if got := tags["status"]; got != "error" {
		t.Fatalf("tags.status = %v, want error", got)
	}
	if _, ok := tags["upload_mode"]; ok {
		t.Fatal("tags.upload_mode must be omitted")
	}
	if got := tags["api_path"]; got != "/open-apis/drive/v1/files/upload_prepare" {
		t.Fatalf("tags.api_path = %v, want upload_prepare", got)
	}
}

// TestDriveUploadReportFileEventFailureKeepsUploadError verifies that reporting cannot replace the Drive upload error.
func TestDriveUploadReportFileEventFailureKeepsUploadError(t *testing.T) {
	uploadTestConfig := &core.CliConfig{
		AppID: "drive-upload-report-keeps-err", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, uploadTestConfig)
	reg.Register(&httpmock.Stub{
		Method:   "POST",
		URL:      driveReportFileEventPath,
		Body:     map[string]interface{}{"code": 500, "msg": "report rejected"},
		Reusable: true,
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body:   map[string]interface{}{"code": 1001, "msg": "quota exceeded"},
	})

	withDriveWorkingDir(t, t.TempDir())
	if err := os.WriteFile("small.bin", make([]byte, 1024), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDrive(t, DriveUpload, []string{
		"+upload", "--file", "small.bin", "--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T (%v)", err, err)
	}
	if p.Code != 1001 {
		t.Fatalf("code = %d, want original upload code 1001", p.Code)
	}
	if !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("error lost original message: %v", err)
	}
}
