// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

var driveTestConfigSeq atomic.Int64

func driveTestConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID: fmt.Sprintf("drive-test-app-%d", driveTestConfigSeq.Add(1)), AppSecret: "test-secret", Brand: core.BrandFeishu,
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

func registerDriveBotTokenStub(reg *httpmock.Registry) {
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/auth/v3/tenant_access_token/internal",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"tenant_access_token": "t-test-token", "expire": 7200,
		},
	})
}

func TestDriveUploadRejectsLargeFile(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	fh, err := os.Create("large.bin")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := fh.Truncate(maxDriveUploadFileSize + 1); err != nil {
		t.Fatalf("Truncate() error: %v", err)
	}
	if err := fh.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	err = mountAndRunDrive(t, DriveUpload, []string{
		"+upload",
		"--file", "large.bin",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected size limit error, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds 20MB limit") {
		t.Fatalf("unexpected error: %v", err)
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
	registerDriveBotTokenStub(reg)
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
