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
// against the real API: create a statistics block with position and
// number_format, update both fields, read it back, and clean up the temporary
// dashboard/base. The test is skipped unless a tenant access token is present.
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
		reportCleanupFailure(t, "delete dashboard "+dashboardID, result, cleanupErr)
	})

	createBlock, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{
			"base", "+dashboard-block-create",
			"--base-token", baseToken,
			"--dashboard-id", dashboardID,
			"--name", "Metric Precision",
			"--type", "statistics",
			"--data-config", `{"table_name":"` + tableName + `","count_all":true,"number_format":{"formatName":"digital","precision":2}}`,
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
		reportCleanupFailure(t, "delete dashboard block "+blockID, result, cleanupErr)
	})

	// Update both fields to values distinct from the create call, so the
	// read-back below proves the update landed rather than re-reading the
	// created state. number_format is replaced as a whole (like every
	// data_config top-level key), so formatName is carried back explicitly.
	updateBlock, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+dashboard-block-update",
			"--base-token", baseToken,
			"--dashboard-id", dashboardID,
			"--block-id", blockID,
			"--data-config", `{"number_format":{"formatName":"dollar_rounded","precision":0}}`,
			"--position", `{"x":6,"y":0,"w":6,"h":4}`,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	updateBlock.AssertExitCode(t, 0)
	updateBlock.AssertStdoutStatus(t, true)

	getBlock, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+dashboard-block-get", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", blockID},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	getBlock.AssertExitCode(t, 0)
	getBlock.AssertStdoutStatus(t, true)

	// Located by key rather than by a fixed path: the assertion is about the
	// updated values, not about where the response nests data_config.
	// Coordinate read-back (x/y/w/h) is not asserted — the get response is not
	// contracted to echo position in this iteration.
	numberFormats := gjson.Get(getBlock.Stdout, "@dig:number_format").Array()
	require.NotEmpty(t, numberFormats, "number_format missing from block read-back:\n%s", getBlock.Stdout)
	updated := numberFormats[0]
	require.Equal(t, "dollar_rounded", updated.Get("formatName").String(), "stdout:\n%s", getBlock.Stdout)
	require.True(t, updated.Get("precision").Exists(), "precision missing from block read-back:\n%s", getBlock.Stdout)
	require.Equal(t, int64(0), updated.Get("precision").Int(), "stdout:\n%s", getBlock.Stdout)
}
