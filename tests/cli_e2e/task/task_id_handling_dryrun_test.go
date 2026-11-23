// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestTaskIDHandlingDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "task_id_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "task_id_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	run := func(t *testing.T, args []string) *clie2e.Result {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
		require.NoError(t, err)
		return result
	}

	t.Run("GUID and applink produce equivalent update requests", func(t *testing.T) {
		guidResult := run(t, []string{
			"task", "+update", "--task-id", "task-guid-123", "--summary", "updated", "--dry-run",
		})
		guidResult.AssertExitCode(t, 0)
		applinkResult := run(t, []string{
			"task", "+update", "--task-id", "https://applink.larksuite.com/client/todo/task?guid=task-guid-123", "--summary", "updated", "--dry-run",
		})
		applinkResult.AssertExitCode(t, 0)

		wantURL := "/open-apis/task/v2/tasks/task-guid-123"
		require.Equal(t, wantURL, clie2e.DryRunGet(guidResult.Stdout, "api.0.url").String())
		require.Equal(t, wantURL, clie2e.DryRunGet(applinkResult.Stdout, "api.0.url").String())
		require.Equal(t, clie2e.DryRunGet(guidResult.Stdout, "api.0.body").Raw, clie2e.DryRunGet(applinkResult.Stdout, "api.0.body").Raw)
	})

	t.Run("multi-ID update previews every mutation", func(t *testing.T) {
		result := run(t, []string{
			"task", "+update",
			"--task-id", "task-guid-1,https://applink.larksuite.com/client/todo/task?guid=task-guid-2",
			"--summary", "updated",
			"--dry-run",
		})
		result.AssertExitCode(t, 0)

		require.Equal(t, int64(2), clie2e.DryRunGet(result.Stdout, "api.#").Int())
		require.Equal(t, "PATCH", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		require.Equal(t, "/open-apis/task/v2/tasks/task-guid-1", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		require.Equal(t, "PATCH", clie2e.DryRunGet(result.Stdout, "api.1.method").String())
		require.Equal(t, "/open-apis/task/v2/tasks/task-guid-2", clie2e.DryRunGet(result.Stdout, "api.1.url").String())
		require.Equal(t, clie2e.DryRunGet(result.Stdout, "api.0.params").Raw, clie2e.DryRunGet(result.Stdout, "api.1.params").Raw)
		require.Equal(t, clie2e.DryRunGet(result.Stdout, "api.0.body").Raw, clie2e.DryRunGet(result.Stdout, "api.1.body").Raw)
	})

	t.Run("GUID and applink produce equivalent completion requests", func(t *testing.T) {
		guidResult := run(t, []string{
			"task", "+complete", "--task-id", "task-guid-456", "--dry-run",
		})
		guidResult.AssertExitCode(t, 0)
		applinkResult := run(t, []string{
			"task", "+complete", "--task-id", "https://applink.larksuite.com/client/todo/task?guid=task-guid-456", "--dry-run",
		})
		applinkResult.AssertExitCode(t, 0)

		wantURL := "/open-apis/task/v2/tasks/task-guid-456"
		for _, result := range []*clie2e.Result{guidResult, applinkResult} {
			require.Equal(t, int64(2), clie2e.DryRunGet(result.Stdout, "api.#").Int())
			require.Equal(t, wantURL, clie2e.DryRunGet(result.Stdout, "api.0.url").String())
			require.Equal(t, wantURL, clie2e.DryRunGet(result.Stdout, "api.1.url").String())
		}
	})

	for _, shortcut := range []string{"+update", "+complete"} {
		t.Run(shortcut+" rejects display numbers", func(t *testing.T) {
			args := []string{"task", shortcut, "--task-id", "t12345", "--dry-run"}
			if shortcut == "+update" {
				args = append(args, "--summary", "must not be written")
			}
			result := run(t, args)
			result.AssertExitCode(t, 2)

			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), "stderr:\n%s", result.Stderr)
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), "stderr:\n%s", result.Stderr)
			require.Equal(t, "--task-id", gjson.Get(result.Stderr, "error.param").String(), "stderr:\n%s", result.Stderr)
			require.Contains(t, gjson.Get(result.Stderr, "error.hint").String(), "guid=", "stderr:\n%s", result.Stderr)
			require.False(t, gjson.Get(result.Stdout, "data.api").Exists(), "invalid input must not emit a dry-run API request\nstdout:\n%s", result.Stdout)
		})
	}
}
