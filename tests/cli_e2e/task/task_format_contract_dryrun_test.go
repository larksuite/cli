// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func setTaskFormatDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "task_format_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "task_format_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}

func TestTaskDryRunMixedCasePrettyUsesPlainTextPreview(t *testing.T) {
	setTaskFormatDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"task", "+get-my-tasks",
			"--dry-run",
			"--format", "Pretty",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	require.NoError(t, result.RunErr, "stderr:\n%s", result.Stderr)
	result.AssertExitCode(t, 0)
	require.True(t, strings.HasPrefix(result.Stdout, "# dry-run: request not sent\n"), "stdout:\n%s", result.Stdout)
	require.Contains(t, result.Stdout, "/open-apis/task/v2/tasks", "stdout:\n%s", result.Stdout)
	require.False(t, strings.HasPrefix(strings.TrimSpace(result.Stdout), "{"), "stdout must be plain text:\n%s", result.Stdout)
}

func TestTaskDryRunUnknownFormatReturnsTypedValidationBeforePreview(t *testing.T) {
	setTaskFormatDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"task", "+get-my-tasks",
			"--dry-run",
			"--format", "yaml",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	require.Error(t, result.RunErr)
	result.AssertExitCode(t, 2)
	require.Empty(t, strings.TrimSpace(result.Stdout), "request preview must not be emitted:\n%s", result.Stdout)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), "stderr:\n%s", result.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), "stderr:\n%s", result.Stderr)
	require.Equal(t, "--format", gjson.Get(result.Stderr, "error.param").String(), "stderr:\n%s", result.Stderr)
}
