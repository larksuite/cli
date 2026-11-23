// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestBaseTableCopyDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("schema default with table name", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+table-copy",
				"--base-token", "app_x",
				"--table-id", "Source Tasks",
				"--name", "Copied Tasks",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		out := result.Stdout
		require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), out)
		require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/Source%20Tasks/copy", clie2e.DryRunGet(out, "api.0.url").String(), out)
		require.Equal(t, "Copied Tasks", clie2e.DryRunGet(out, "api.0.body.name").String(), out)
		require.Equal(t, "schema", clie2e.DryRunGet(out, "api.0.body.range").String(), out)
		require.False(t, clie2e.DryRunGet(out, "api.1").Exists(), out)
	})

	t.Run("all with wait", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+table-copy",
				"--base-token", "app_x",
				"--table-id", "tbl_source",
				"--name", "Copied Tasks",
				"--range", "all",
				"--wait",
				"--timeout", "300s",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		out := result.Stdout
		require.Equal(t, "all", clie2e.DryRunGet(out, "api.0.body.range").String(), out)
		require.Equal(t, "/open-apis/base/v3/bases/app_x/copy_table_state", clie2e.DryRunGet(out, "api.1.url").String(), out)
		require.Equal(t, "<task_id_from_step_1>", clie2e.DryRunGet(out, "api.1.body.task_id").String(), out)
		require.True(t, clie2e.DryRunGet(out, "wait").Bool(), out)
		require.Equal(t, "5m0s", clie2e.DryRunGet(out, "timeout").String(), out)
	})

	t.Run("status", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+table-copy-status",
				"--base-token", "app_x",
				"--task-id", "ct1.token",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		out := result.Stdout
		require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), out)
		require.Equal(t, "/open-apis/base/v3/bases/app_x/copy_table_state", clie2e.DryRunGet(out, "api.0.url").String(), out)
		require.Equal(t, "ct1.token", clie2e.DryRunGet(out, "api.0.body.task_id").String(), out)
	})
}
