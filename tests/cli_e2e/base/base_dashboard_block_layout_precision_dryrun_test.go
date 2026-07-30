// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
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

	output := strings.TrimSpace(result.Stdout)
	assert.Contains(t, output, "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks")
	assert.Contains(t, output, `"method": "POST"`)
	// top-level position sibling of name/type/data_config
	assert.Contains(t, output, `"position"`)
	assert.Contains(t, output, `"w": 6`)
	// number_format nested inside data_config
	assert.Contains(t, output, `"number_format"`)
	assert.Contains(t, output, `"dollar_rounded"`)
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

	output := strings.TrimSpace(result.Stdout)
	assert.Contains(t, output, "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks/blk_a")
	assert.Contains(t, output, `"method": "PATCH"`)
	assert.Contains(t, output, `"position"`)
	assert.Contains(t, output, `"x": 6`)
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
	assert.NotEqual(t, 0, result.ExitCode)
	assert.Contains(t, result.Stderr, "formatName")
}
