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

func TestBaseDashboardBlockGetDataDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+dashboard-block-get-data",
			"--base-token", "app_x",
			"--block-id", "blk_chart",
			"--dry-run",
		},
		BinaryPath: "../../../lark-cli",
		DefaultAs:  "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := strings.TrimSpace(result.Stdout)
	assert.Contains(t, output, "/open-apis/base/v3/bases/app_x/dashboards/blocks/blk_chart/data")
	assert.Contains(t, output, `"method": "GET"`)
	assert.Contains(t, output, `"block_id": "blk_chart"`)
	assert.Contains(t, output, `"base_token": "app_x"`)
}

func TestBaseDashboardBlockGetDataDryRun_MissingRequiredFlags(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+dashboard-block-get-data",
			"--dry-run",
		},
		BinaryPath: "../../../lark-cli",
		DefaultAs:  "bot",
	})
	require.NoError(t, err)
	assert.NotEqual(t, 0, result.ExitCode)
	assert.Contains(t, result.Stderr, "base-token")
	assert.Contains(t, result.Stderr, "block-id")
}
