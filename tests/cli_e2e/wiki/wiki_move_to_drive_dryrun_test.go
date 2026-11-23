// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setWikiMoveToDriveDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "wiki_move_to_drive_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "wiki_move_to_drive_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}

// TestWikiMoveToDriveDryRun pins both requests in the async orchestration
// without requiring credentials or calling a real tenant.
func TestWikiMoveToDriveDryRun(t *testing.T) {
	setWikiMoveToDriveDryRunEnv(t)

	t.Run("target folder", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"wiki", "+move-to-drive",
				"--node-token", "wikcnABC123",
				"--folder-token", "fldABC123",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/wiki/v2/nodes/wikcnABC123/move_wiki_to_docs", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.Equal(t, "fldABC123", clie2e.DryRunGet(result.Stdout, "api.0.body.folder_token").String())
		assert.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, "api.1.method").String())
		assert.Equal(t, "/open-apis/wiki/v2/tasks/%3Ctask_id%3E", clie2e.DryRunGet(result.Stdout, "api.1.url").String())
		assert.Equal(t, "move_wiki_to_docs", clie2e.DryRunGet(result.Stdout, "api.1.params.task_type").String())
	})

	t.Run("personal space root", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"wiki", "+move-to-drive",
				"--node-token", "wikcnABC123",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body.folder_token").Exists(),
			"folder_token must be omitted when targeting personal-space root; stdout:\n%s", result.Stdout)
	})

	t.Run("standalone continuation query", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+task_result",
				"--scenario", "wiki_move_to_drive",
				"--task-id", "task-raw-signature",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/wiki/v2/tasks/task-raw-signature", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.Equal(t, "move_wiki_to_docs", clie2e.DryRunGet(result.Stdout, "api.0.params.task_type").String())
	})
}
