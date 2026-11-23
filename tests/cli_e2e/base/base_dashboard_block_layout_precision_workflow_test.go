// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestBaseDashboardBlockLayoutPrecisionWorkflow exercises the changed fields
// against the real API: create a statistics block with position and the server
// default number_format, update both fields, verify a precision-only update
// preserves formatName, and clean up the temporary dashboard/base. The test is
// skipped unless a tenant access token is present.
func TestBaseDashboardBlockLayoutPrecisionWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	baseToken := createBaseWithRetry(t, ctx, "lark-cli-e2e-dashboard-layout-"+clie2e.GenerateSuffix())
	tableName := "Dashboard Metrics " + clie2e.GenerateSuffix()
	createTableWithRetry(t, t, ctx, baseToken, tableName, `[{"name":"Name","type":"text"}]`, `{"name":"Main","type":"grid"}`)

	dashboardResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"base", "+dashboard-create", "--base-token", baseToken, "--name", "Layout Precision " + clie2e.GenerateSuffix()},
		DefaultAs: "bot",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	dashboardResult.AssertExitCode(t, 0)
	dashboardResult.AssertStdoutStatus(t, true)
	dashboardID := gjson.Get(dashboardResult.Stdout, "data.dashboard.dashboard_id").String()
	if dashboardID == "" {
		dashboardID = gjson.Get(dashboardResult.Stdout, "data.dashboard.id").String()
	}
	require.NotEmpty(t, dashboardID, "stdout:\n%s", dashboardResult.Stdout)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		result, cleanupErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"base", "+dashboard-delete", "--base-token", baseToken, "--dashboard-id", dashboardID, "--yes"},
			DefaultAs: "bot",
		})
		clie2e.ReportCleanupFailure(t, "delete dashboard "+dashboardID, result, cleanupErr)
	})

	createBlock, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{
			"base", "+dashboard-block-create",
			"--base-token", baseToken,
			"--dashboard-id", dashboardID,
			"--name", "Metric Precision",
			"--type", "statistics",
			"--data-config", `{"table_name":"` + tableName + `","count_all":true}`,
			"--position", `{"x":0,"y":0,"w":6,"h":4}`,
		},
		DefaultAs: "bot",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	createBlock.AssertExitCode(t, 0)
	createBlock.AssertStdoutStatus(t, true)
	blockID := gjson.Get(createBlock.Stdout, "data.block.block_id").String()
	if blockID == "" {
		blockID = gjson.Get(createBlock.Stdout, "data.block.id").String()
	}
	require.NotEmpty(t, blockID, "stdout:\n%s", createBlock.Stdout)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		result, cleanupErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"base", "+dashboard-block-delete", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", blockID, "--yes"},
			DefaultAs: "bot",
		})
		clie2e.ReportCleanupFailure(t, "delete dashboard block "+blockID, result, cleanupErr)
	})

	getNumberFormat := func(label string) gjson.Result {
		t.Helper()
		getBlock, getErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+dashboard-block-get", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", blockID},
			DefaultAs: "bot",
		})
		require.NoError(t, getErr)
		getBlock.AssertExitCode(t, 0)
		getBlock.AssertStdoutStatus(t, true)

		// Located by key rather than by a fixed path: the assertion is about
		// number_format, not about where the response nests data_config.
		numberFormats := gjson.Get(getBlock.Stdout, "@dig:number_format").Array()
		require.NotEmpty(t, numberFormats, "%s number_format missing from block read-back:\n%s", label, getBlock.Stdout)
		return numberFormats[0]
	}

	defaultFormat := getNumberFormat("default")
	require.Equal(t, "digital", defaultFormat.Get("formatName").String())
	require.False(t, defaultFormat.Get("precision").Exists())

	// Set a non-default format together with the new position, then update only
	// precision. The final read-back proves both the explicit format update and
	// the server's number_format subfield merge behavior.
	updateBlock, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+dashboard-block-update",
			"--base-token", baseToken,
			"--dashboard-id", dashboardID,
			"--block-id", blockID,
			"--data-config", `{"number_format":{"formatName":"dollar_rounded","precision":2}}`,
			"--position", `{"x":6,"y":0,"w":6,"h":4}`,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	updateBlock.AssertExitCode(t, 0)
	updateBlock.AssertStdoutStatus(t, true)

	partialUpdate, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+dashboard-block-update",
			"--base-token", baseToken,
			"--dashboard-id", dashboardID,
			"--block-id", blockID,
			"--data-config", `{"number_format":{"precision":0}}`,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	partialUpdate.AssertExitCode(t, 0)
	partialUpdate.AssertStdoutStatus(t, true)

	updated := getNumberFormat("updated")
	require.Equal(t, "dollar_rounded", updated.Get("formatName").String())
	require.True(t, updated.Get("precision").Exists())
	require.Equal(t, int64(0), updated.Get("precision").Int())
}
