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

// TestBaseDashboardBlockCreateDryRun_PositionAndNumberFormat proves the
// create command dry-run surfaces the top-level position sibling and the
// statistics data_config.number_format in the request body preview.
func TestBaseDashboardBlockCreateDryRun_PositionAndNumberFormat(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+dashboard-block-create",
			"--base-token", "app_x",
			"--dashboard-id", "dsh_1",
			"--name", "Revenue",
			"--type", "statistics",
			"--data-config", `{"table_name":"Orders","series":[{"field_name":"Amount","rollup":"SUM"}],"number_format":{"formatName":"dollar_rounded","precision":2}}`,
			"--position", `{"x":0,"y":0,"w":6,"h":4}`,
			"--dry-run",
		},
		BinaryPath: "../../../lark-cli",
		DefaultAs:  "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks", clie2e.DryRunGet(out, "api.0.url").String(), out)
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), out)
	// position is a top-level sibling of name/type/data_config, not nested in it
	require.Equal(t, int64(0), clie2e.DryRunGet(out, "api.0.body.position.x").Int(), out)
	require.Equal(t, int64(6), clie2e.DryRunGet(out, "api.0.body.position.w").Int(), out)
	require.Equal(t, int64(4), clie2e.DryRunGet(out, "api.0.body.position.h").Int(), out)
	require.False(t, clie2e.DryRunGet(out, "api.0.body.data_config.position").Exists(), out)
	// number_format nested inside data_config
	require.Equal(t, "dollar_rounded", clie2e.DryRunGet(out, "api.0.body.data_config.number_format.formatName").String(), out)
	require.Equal(t, int64(2), clie2e.DryRunGet(out, "api.0.body.data_config.number_format.precision").Int(), out)
}

// TestBaseDashboardBlockUpdateDryRun_Position proves the update command
// dry-run surfaces the top-level position sibling in the request body preview.
func TestBaseDashboardBlockUpdateDryRun_Position(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+dashboard-block-update",
			"--base-token", "app_x",
			"--dashboard-id", "dsh_1",
			"--block-id", "blk_a",
			"--position", `{"x":6,"y":0,"w":6,"h":4}`,
			"--dry-run",
		},
		BinaryPath: "../../../lark-cli",
		DefaultAs:  "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks/blk_a", clie2e.DryRunGet(out, "api.0.url").String(), out)
	require.Equal(t, "PATCH", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.Equal(t, int64(6), clie2e.DryRunGet(out, "api.0.body.position.x").Int(), out)
	require.Equal(t, int64(6), clie2e.DryRunGet(out, "api.0.body.position.w").Int(), out)
	// update must not carry type; the API rejects a type change
	require.False(t, clie2e.DryRunGet(out, "api.0.body.type").Exists(), out)
}

// TestBaseDashboardBlockCreateDryRun_InvalidNumberFormat proves the statistics
// number_format validation rejects an out-of-enum formatName before any request.
func TestBaseDashboardBlockCreateDryRun_InvalidNumberFormat(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+dashboard-block-create",
			"--base-token", "app_x",
			"--dashboard-id", "dsh_1",
			"--name", "Revenue",
			"--type", "statistics",
			"--data-config", `{"table_name":"Orders","count_all":true,"number_format":{"formatName":"bogus","precision":2}}`,
			"--dry-run",
		},
		BinaryPath: "../../../lark-cli",
		DefaultAs:  "bot",
	})
	require.NoError(t, err)
	require.NotEqual(t, 0, result.ExitCode, "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	require.Contains(t, result.Stderr, "formatName", "validation errors must go to stderr, not stdout")
}
