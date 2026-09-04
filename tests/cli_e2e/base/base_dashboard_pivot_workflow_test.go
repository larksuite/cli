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

// TestBaseDashboardPivotWorkflow covers create defaults, exact sort clearing,
// values-only multi-metric read-back, computed data, and fixture cleanup. It is
// skipped unless tenant credentials are available.
func TestBaseDashboardPivotWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	baseToken := createBaseWithRetry(t, ctx, "lark-cli-e2e-dashboard-pivot-"+suffix)
	tableName := "Pivot Metrics " + suffix
	createTableWithRetry(t, t, ctx, baseToken, tableName,
		`[{"name":"Category","type":"text"},{"name":"Amount","type":"number"}]`,
		`{"name":"Main","type":"grid"}`,
	)

	dashboardResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"base", "+dashboard-create", "--base-token", baseToken, "--name", "Pivot " + suffix},
		DefaultAs: "bot",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	dashboardResult.AssertExitCode(t, 0)
	dashboardResult.AssertStdoutStatus(t, true)
	dashboardID := firstNonEmpty(
		gjson.Get(dashboardResult.Stdout, "data.dashboard.dashboard_id").String(),
		gjson.Get(dashboardResult.Stdout, "data.dashboard.id").String(),
	)
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
			"--name", "Pivot values only",
			"--type", "pivotTable",
			"--data-config", `{"table_name":"` + tableName + `","rows":[{"field_name":"Category"}],"values":[{"field_name":"Amount","rollup":"SUM"},{"field_name":"Category","rollup":"COUNT_DISTINCT"}]}`,
		},
		DefaultAs: "bot",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	createBlock.AssertExitCode(t, 0)
	createBlock.AssertStdoutStatus(t, true)
	blockID := firstNonEmpty(
		gjson.Get(createBlock.Stdout, "data.block.block_id").String(),
		gjson.Get(createBlock.Stdout, "data.block.id").String(),
	)
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

	initialConfig := getPivotDataConfig(t, ctx, baseToken, dashboardID, blockID)
	require.Len(t, initialConfig.Get("rows").Array(), 1, initialConfig.Raw)
	require.Len(t, initialConfig.Get("values").Array(), 2, initialConfig.Raw)
	require.Len(t, initialConfig.Get("sort").Array(), 1, "create should initialize FIELD asc: %s", initialConfig.Raw)
	require.Equal(t, "FIELD", initialConfig.Get("sort.0.sort_type").String(), initialConfig.Raw)
	require.Equal(t, "asc", initialConfig.Get("sort.0.order").String(), initialConfig.Raw)

	updateBlock, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+dashboard-block-update",
			"--base-token", baseToken,
			"--dashboard-id", dashboardID,
			"--block-id", blockID,
			"--data-config", `{"rows":[],"columns":[],"sort":[]}`,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	updateBlock.AssertExitCode(t, 0)
	updateBlock.AssertStdoutStatus(t, true)

	valuesOnlyConfig := getPivotDataConfig(t, ctx, baseToken, dashboardID, blockID)
	require.Empty(t, valuesOnlyConfig.Get("rows").Array(), valuesOnlyConfig.Raw)
	require.Empty(t, valuesOnlyConfig.Get("columns").Array(), valuesOnlyConfig.Raw)
	require.Empty(t, valuesOnlyConfig.Get("sort").Array(), valuesOnlyConfig.Raw)
	require.Len(t, valuesOnlyConfig.Get("values").Array(), 2, valuesOnlyConfig.Raw)

	getData, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+dashboard-block-get-data", "--base-token", baseToken, "--block-id", blockID},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	getData.AssertExitCode(t, 0)
	getData.AssertStdoutStatus(t, true)
	mainDataMatches := gjson.Get(getData.Stdout, "@dig:main_data").Array()
	require.NotEmpty(t, mainDataMatches, "main_data missing from stdout:\n%s", getData.Stdout)
	require.LessOrEqual(t, len(mainDataMatches[0].Array()), 1, "values-only must return at most one row: %s", getData.Stdout)
	measureMatches := gjson.Get(getData.Stdout, "@dig:measures").Array()
	require.NotEmpty(t, measureMatches, "measures missing from stdout:\n%s", getData.Stdout)
	require.Len(t, measureMatches[0].Array(), 2, getData.Stdout)
}

func getPivotDataConfig(t *testing.T, ctx context.Context, baseToken, dashboardID, blockID string) gjson.Result {
	t.Helper()
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+dashboard-block-get", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", blockID},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	configs := gjson.Get(result.Stdout, "@dig:data_config").Array()
	require.NotEmpty(t, configs, "data_config missing from stdout:\n%s", result.Stdout)
	return configs[0]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
