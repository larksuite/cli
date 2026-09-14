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

// TestBaseDashboardBlockNPSWorkflow creates an NPS block from a temporary
// Rating field, verifies the canonical type and server-defaulted group mode,
// and cleans up every resource it creates. The test is skipped unless a tenant
// access token is present.
func TestBaseDashboardBlockNPSWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	baseToken := createBaseWithRetry(t, ctx, "lark-cli-e2e-dashboard-nps-"+suffix)
	tableName := "NPS Survey " + suffix
	createTableWithRetry(
		t,
		t,
		ctx,
		baseToken,
		tableName,
		`[{"name":"Name","type":"text"},{"name":"Satisfaction","type":"rating"}]`,
		`{"name":"Main","type":"grid"}`,
	)

	dashboardResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"base", "+dashboard-create", "--base-token", baseToken, "--name", "NPS " + suffix},
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
			"--name", "Satisfaction NPS",
			"--type", " NpS ",
			"--data-config", `{"table_name":"` + tableName + `","group_by":[{"field_name":"Satisfaction"}]}`,
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
			Args: []string{
				"base", "+dashboard-block-delete",
				"--base-token", baseToken,
				"--dashboard-id", dashboardID,
				"--block-id", blockID,
				"--yes",
			},
			DefaultAs: "bot",
		})
		clie2e.ReportCleanupFailure(t, "delete dashboard block "+blockID, result, cleanupErr)
	})

	getBlock, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+dashboard-block-get", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", blockID},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	getBlock.AssertExitCode(t, 0)
	getBlock.AssertStdoutStatus(t, true)
	require.Equal(t, "nps", gjson.Get(getBlock.Stdout, "data.block.type").String(), "stdout:\n%s", getBlock.Stdout)
	require.Equal(
		t,
		"integrated",
		gjson.Get(getBlock.Stdout, "data.block.data_config.group_by.0.mode").String(),
		"stdout:\n%s",
		getBlock.Stdout,
	)
}
