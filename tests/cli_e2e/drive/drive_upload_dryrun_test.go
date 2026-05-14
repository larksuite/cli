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
	assert.Contains(t, output, "parent_type")
	assert.Contains(t, output, "parent_node")
	assert.Contains(t, output, "wikcnDryRunUploadTarget")
	assert.Contains(t, output, `"parent_type": "wiki"`)
}

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
	assert.Contains(t, output, `"parent_node": "fldDryRunUploadTarget"`)
	assert.Contains(t, output, `"file_token": "boxcnDryRunOverwriteTarget"`)
}

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

func setDriveDryRunConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "drive_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "drive_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}
