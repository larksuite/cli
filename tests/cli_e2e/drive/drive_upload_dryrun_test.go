// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDriveUploadDryRun_WikiTarget verifies upload-report planning for a wiki target.
func TestDriveUploadDryRun_WikiTarget(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+upload",
			"--file", "./report.pdf",
			"--wiki-token", "wikcnDryRunUploadTarget",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := strings.TrimSpace(result.Stdout)
	assert.Contains(t, output, "/open-apis/drive/v1/files/upload_all")
	assert.Contains(t, output, "/open-apis/drive/v1/metas/batch_query")
	assert.Contains(t, output, `"with_url": true`)
	assert.Contains(t, output, "parent_type")
	assert.Contains(t, output, "parent_node")
	assert.Contains(t, output, "wikcnDryRunUploadTarget")
	assert.Contains(t, output, `"parent_type": "wiki"`)
	assertDriveUploadReportDryRun(t, result.Stdout, "wiki")
}

// TestDriveUploadDryRun_WithFileToken verifies upload-report planning for overwrite uploads.
func TestDriveUploadDryRun_WithFileToken(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+upload",
			"--file", "./report.pdf",
			"--folder-token", "fldDryRunUploadTarget",
			"--file-token", "boxcnDryRunOverwriteTarget",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := strings.TrimSpace(result.Stdout)
	assert.Contains(t, output, "/open-apis/drive/v1/files/upload_all")
	assert.Contains(t, output, "/open-apis/drive/v1/metas/batch_query")
	assert.Contains(t, output, `"with_url": true`)
	assert.Contains(t, output, `"parent_node": "fldDryRunUploadTarget"`)
	assert.Equal(t, "boxcnDryRunOverwriteTarget", clie2e.DryRunGet(output, "api.0.body.file_token").String())
	assertDriveUploadReportDryRun(t, result.Stdout, "explorer")
}

// TestDriveUploadDryRunRejectsEmptyWikiToken verifies validation of explicitly empty wiki targets.
func TestDriveUploadDryRunRejectsEmptyWikiToken(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+upload",
			"--file", "./report.pdf",
			"--wiki-token", "",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	assert.Contains(t, result.Stderr, "--wiki-token cannot be empty")
}

// setDriveDryRunConfigEnv configures deterministic credentials for Drive dry-run tests.
func setDriveDryRunConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "drive_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "drive_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}

// assertDriveUploadReportDryRun verifies the upload report request in a dry-run
// plan for the expected Drive mount point.
func assertDriveUploadReportDryRun(t *testing.T, out, mountPoint string) {
	t.Helper()
	if got := clie2e.DryRunGet(out, "api.1.method").String(); got != "POST" {
		t.Fatalf("data.api.1.method = %q, want POST\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.url").String(); got != "/open-apis/drive/v1/lark_cli_file_event/report" {
		t.Fatalf("data.api.1.url = %q, want report_file_event\nstdout:\n%s", got, out)
	}
	checks := map[string]string{
		"api.1.body.file_scene":         "lark-cli",
		"api.1.body.scene":              "upload",
		"api.1.body.operation":          "upload",
		"api.1.body.tags.api_path":      "/open-apis/drive/v1/files/upload_all",
		"api.1.body.tags.command":       "drive +upload",
		"api.1.body.tags.resource_type": "file",
		"api.1.body.tags.status":        "success",
		"api.1.body.tags.mount_point":   mountPoint,
		"api.1.body.tags.file_token":    "<file_token from upload response>",
	}
	for path, want := range checks {
		if got := clie2e.DryRunGet(out, path).String(); got != want {
			t.Fatalf("data.%s = %q, want %q\nstdout:\n%s", path, got, want, out)
		}
	}
	if clie2e.DryRunGet(out, "api.1.body.tags.upload_mode").Exists() {
		t.Fatalf("data.api.1.body.tags.upload_mode must be omitted\nstdout:\n%s", out)
	}
}
