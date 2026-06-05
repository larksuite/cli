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

func TestDriveVersionHistoryDryRun(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+version-history",
			"--file-token", "boxcnHistoryDryRun",
			"--limit", "5",
			"--cursor", "1777013761763",
			"--dry-run",
		},
		DefaultAs:  "bot",
		BinaryPath: "../../../lark-cli",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := strings.TrimSpace(result.Stdout)
	assert.Contains(t, output, "/open-apis/drive/v1/files/boxcnHistoryDryRun/history")
	assert.Contains(t, output, `"only_tag": true`)
	assert.Contains(t, output, `"page_size": 5`)
	assert.Contains(t, output, `"last_edit_time": "1777013761763"`)
}

func TestDriveVersionGetDryRun(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+version-get",
			"--file-token", "boxcnVersionDryRun",
			"--version", "7633658129540910621",
			"--output", "./artifact.bin",
			"--dry-run",
		},
		DefaultAs:  "bot",
		BinaryPath: "../../../lark-cli",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := strings.TrimSpace(result.Stdout)
	assert.Contains(t, output, "/open-apis/drive/v1/files/boxcnVersionDryRun/download")
	assert.Contains(t, output, `"version": "7633658129540910621"`)
	assert.Contains(t, output, `"output": "./artifact.bin"`)
}

func TestDriveVersionGetDryRunWithoutOutputUsesCurrentDirectory(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+version-get",
			"--file-token", "boxcnVersionDryRun",
			"--version", "7633658129540910621",
			"--dry-run",
		},
		DefaultAs:  "bot",
		BinaryPath: "../../../lark-cli",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := strings.TrimSpace(result.Stdout)
	assert.Contains(t, output, "/open-apis/drive/v1/files/boxcnVersionDryRun/download")
	assert.Contains(t, output, `"version": "7633658129540910621"`)
	assert.Contains(t, output, `"output": "."`)
}

func TestDriveVersionRevertDryRun(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+version-revert",
			"--file-token", "boxcnVersionDryRun",
			"--version", "7633658129540910621",
			"--dry-run",
		},
		DefaultAs:  "bot",
		BinaryPath: "../../../lark-cli",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := strings.TrimSpace(result.Stdout)
	assert.Contains(t, output, "/open-apis/drive/v1/files/boxcnVersionDryRun/revert")
	assert.Contains(t, output, `"version": "7633658129540910621"`)
}

func TestDriveVersionDeleteDryRun(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+version-delete",
			"--file-token", "boxcnVersionDryRun",
			"--version", "7633658129540910621",
			"--dry-run",
		},
		DefaultAs:  "bot",
		BinaryPath: "../../../lark-cli",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := strings.TrimSpace(result.Stdout)
	assert.Contains(t, output, "/open-apis/drive/v1/files/boxcnVersionDryRun/version_del")
	assert.Contains(t, output, `"version": "7633658129540910621"`)
}

func TestDriveVersionDryRunSupportsUser(t *testing.T) {
	clie2e.SkipWithoutUserToken(t)
	setDriveDryRunConfigEnv(t)

	tests := []struct {
		name         string
		args         []string
		wantContains []string
	}{
		{
			name: "history",
			args: []string{
				"drive", "+version-history",
				"--file-token", "boxcnHistoryDryRunUser",
				"--limit", "5",
				"--cursor", "1777013761763",
				"--dry-run",
			},
			wantContains: []string{
				"/open-apis/drive/v1/files/boxcnHistoryDryRunUser/history",
				`"only_tag": true`,
				`"page_size": 5`,
			},
		},
		{
			name: "get",
			args: []string{
				"drive", "+version-get",
				"--file-token", "boxcnVersionDryRunUser",
				"--version", "7633658129540910621",
				"--output", "./artifact-user.bin",
				"--dry-run",
			},
			wantContains: []string{
				"/open-apis/drive/v1/files/boxcnVersionDryRunUser/download",
				`"version": "7633658129540910621"`,
				`"output": "./artifact-user.bin"`,
			},
		},
		{
			name: "revert",
			args: []string{
				"drive", "+version-revert",
				"--file-token", "boxcnVersionDryRunUser",
				"--version", "7633658129540910621",
				"--dry-run",
			},
			wantContains: []string{
				"/open-apis/drive/v1/files/boxcnVersionDryRunUser/revert",
				`"version": "7633658129540910621"`,
			},
		},
		{
			name: "delete",
			args: []string{
				"drive", "+version-delete",
				"--file-token", "boxcnVersionDryRunUser",
				"--version", "7633658129540910621",
				"--dry-run",
			},
			wantContains: []string{
				"/open-apis/drive/v1/files/boxcnVersionDryRunUser/version_del",
				`"version": "7633658129540910621"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:       tt.args,
				DefaultAs:  "user",
				BinaryPath: "../../../lark-cli",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			output := strings.TrimSpace(result.Stdout)
			for _, needle := range tt.wantContains {
				assert.Contains(t, output, needle)
			}
		})
	}
}
