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

// TestBaseDashboardLayoutDryRun proves the +dashboard-layout preview covers the
// whole pipeline: read the block list first, then one batch arrange POST that
// carries the submitted 48-col coordinates and declares fine_grid, then the tidy
// pass (arrange with tidy_only) that always runs afterwards.
func TestBaseDashboardLayoutDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+dashboard-layout",
			"--base-token", "app_x",
			"--dashboard-id", "dsh_1",
			"--layout", `[{"block_id":"blk_a","x":0,"y":0,"w":24,"h":8},{"block_id":"blk_b","x":24,"y":0,"w":24,"h":8},{"block_id":"blk_c","x":0,"y":8,"w":48,"h":16}]`,
			"--dry-run",
		},
		BinaryPath: "../../../lark-cli",
		DefaultAs:  "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	// Step 1: the block list is read whole (page_size 100), never a single page.
	require.Equal(t, "GET", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.Equal(t, "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks", clie2e.DryRunGet(out, "api.0.url").String(), out)
	require.Equal(t, int64(100), clie2e.DryRunGet(out, "api.0.params.page_size").Int(), out)
	// The read carries no grid parameter: the gateway drops it on this GET, and the
	// 48-col coordinates are converted by the CLI from standard_position instead.
	require.False(t, clie2e.DryRunGet(out, "api.0.params.grid_cols").Exists(), out)

	// Step 2: one atomic batch POST to arrange carrying every submitted block.
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.1.method").String(), out)
	require.Equal(t, "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/arrange", clie2e.DryRunGet(out, "api.1.url").String(), out)
	// fine_grid is a top-level sibling of positions; the numeric grid_cols is
	// rejected by the server and must not come back.
	require.True(t, clie2e.DryRunGet(out, "api.1.body.fine_grid").Bool(), out)
	require.False(t, clie2e.DryRunGet(out, "api.1.body.grid_cols").Exists(), out)

	require.Equal(t, "blk_a", clie2e.DryRunGet(out, "api.1.body.positions.0.block_id").String(), out)
	require.Equal(t, int64(0), clie2e.DryRunGet(out, "api.1.body.positions.0.x").Int(), out)
	require.Equal(t, int64(24), clie2e.DryRunGet(out, "api.1.body.positions.0.w").Int(), out)
	require.Equal(t, int64(8), clie2e.DryRunGet(out, "api.1.body.positions.0.h").Int(), out)
	require.Equal(t, "blk_b", clie2e.DryRunGet(out, "api.1.body.positions.1.block_id").String(), out)
	require.Equal(t, int64(24), clie2e.DryRunGet(out, "api.1.body.positions.1.x").Int(), out)
	require.Equal(t, "blk_c", clie2e.DryRunGet(out, "api.1.body.positions.2.block_id").String(), out)
	require.Equal(t, int64(48), clie2e.DryRunGet(out, "api.1.body.positions.2.w").Int(), out)
	require.Equal(t, int64(16), clie2e.DryRunGet(out, "api.1.body.positions.2.h").Int(), out)

	// Step 3: the tidy pass always runs; tidy_only cannot be combined with positions.
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.2.method").String(), out)
	require.Equal(t, "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/arrange", clie2e.DryRunGet(out, "api.2.url").String(), out)
	require.True(t, clie2e.DryRunGet(out, "api.2.body.tidy_only").Bool(), out)
	require.True(t, clie2e.DryRunGet(out, "api.2.body.fine_grid").Bool(), out)
	require.False(t, clie2e.DryRunGet(out, "api.2.body.positions").Exists(), out)

	// Nothing beyond the three requests: the preview mirrors exactly what is sent.
	require.False(t, clie2e.DryRunGet(out, "api.3").Exists(), out)
}

// TestBaseDashboardLayoutDryRun_RejectsPartialCoordinates proves the shape check
// fires before any request: a position is submitted whole, so an item missing a
// coordinate cannot express a placement.
func TestBaseDashboardLayoutDryRun_RejectsPartialCoordinates(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+dashboard-layout",
			"--base-token", "app_x",
			"--dashboard-id", "dsh_1",
			"--layout", `[{"block_id":"blk_a","x":0,"y":0,"w":24}]`,
			"--dry-run",
		},
		BinaryPath: "../../../lark-cli",
		DefaultAs:  "bot",
	})
	require.NoError(t, err)
	require.NotEqual(t, 0, result.ExitCode, result.Stdout+result.Stderr)
	require.Contains(t, result.Stderr, "validation", result.Stderr)
}
