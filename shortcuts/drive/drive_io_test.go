// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bytes"
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
