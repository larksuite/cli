// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// This workflow needs the new route and an authorized bot identity. It creates
// its own Base, records, dashboard, and chart; no existing user data is mutated.
func TestDashboardGlobalFilterWorkflow(t *testing.T) {
	if os.Getenv("LARK_CLI_E2E_DASHBOARD_GLOBAL_FILTER_READY") != "1" {
		t.Skip("set LARK_CLI_E2E_DASHBOARD_GLOBAL_FILTER_READY=1 after the global_filter API is deployed and bot access is enabled")
	}
	clie2e.SkipWithoutTenantAccessToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	baseToken := createBaseWithRetry(t, ctx, "lark-cli-e2e-global-filter-"+clie2e.GenerateSuffix())
	tableName := "Orders " + clie2e.GenerateSuffix()
	tableID, _, _ := createTableWithRetry(t, t, ctx, baseToken, tableName,
		`[{"name":"Name","type":"text"},{"name":"Amount","type":"number"}]`, `{"name":"Main","type":"grid"}`)
	records, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"base", "+record-batch-create", "--base-token", baseToken, "--table-id", tableID,
			"--json", `{"create_records":[{"Name":"Small","Amount":500},{"Name":"Medium","Amount":1500},{"Name":"Large","Amount":2500}]}`},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	records.AssertExitCode(t, 0)
	records.AssertStdoutStatus(t, true)
	require.Len(t, gjson.Get(records.Stdout, "data.record_id_list").Array(), 3)

	dashboard, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"base", "+dashboard-create", "--base-token", baseToken, "--name", "Global filter " + clie2e.GenerateSuffix()}, DefaultAs: "bot",
	})
	require.NoError(t, err)
	dashboard.AssertExitCode(t, 0)
	dashboard.AssertStdoutStatus(t, true)
	dashboardID := gjson.Get(dashboard.Stdout, "data.dashboard.dashboard_id").String()
	require.NotEmpty(t, dashboardID)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		result, cleanupErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args: []string{"base", "+dashboard-delete", "--base-token", baseToken, "--dashboard-id", dashboardID, "--yes"}, DefaultAs: "bot",
		})
		clie2e.ReportCleanupFailure(t, "delete global-filter dashboard", result, cleanupErr)
	})
	block, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"base", "+dashboard-block-create", "--base-token", baseToken, "--dashboard-id", dashboardID,
			"--name", "Order count", "--type", "statistics", "--data-config", fmt.Sprintf(`{"table_name":%q,"count_all":true}`, tableName)},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	block.AssertExitCode(t, 0)
	block.AssertStdoutStatus(t, true)
	blockID := gjson.Get(block.Stdout, "data.block.block_id").String()
	require.NotEmpty(t, blockID)
	path := fmt.Sprintf("/open-apis/base/v3/bases/%s/dashboards/%s/global_filter", baseToken, dashboardID)

	readFilters := func(t *testing.T) string {
		t.Helper()
		result, readErr := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"api", "GET", path}, DefaultAs: "bot"})
		require.NoError(t, readErr)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, dashboardID, gjson.Get(result.Stdout, "data.dashboard_id").String())
		filters := gjson.Get(result.Stdout, "data.global_filters")
		require.True(t, filters.IsArray(), result.Stdout)
		return filters.Raw
	}
	assertCount := func(t *testing.T, expected int64) {
		t.Helper()
		var last string
		waitErr := clie2e.WaitForCondition(ctx, clie2e.WaitOptions{Timeout: 30 * time.Second, Interval: time.Second}, func() (bool, error) {
			result, readErr := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{"base", "+dashboard-block-get-data", "--base-token", baseToken, "--block-id", blockID}, DefaultAs: "bot",
			})
			if readErr != nil {
				return false, readErr
			}
			last = result.Stdout
			if result.ExitCode != 0 {
				return false, fmt.Errorf("read chart failed: %s", result.Stderr)
			}
			measures := gjson.Get(last, "data.measures").Array()
			rows := gjson.Get(last, "data.main_data").Array()
			if len(measures) != 1 || len(rows) != 1 || measures[0].Get("aggregation").String() != "count_all" {
				return false, nil
			}
			alias := measures[0].Get("alias").String()
			value := rows[0].Map()[alias].Get("value")
			return value.Type == gjson.Number && value.Int() == expected, nil
		})
		require.NoError(t, waitErr, "expected count %d, last result: %s", expected, last)
	}

	if !t.Run("read initial configuration", func(t *testing.T) {
		require.JSONEq(t, `[]`, readFilters(t))
		assertCount(t, 3)
	}) {
		return
	}
	if !t.Run("set and read back normalized configuration", func(t *testing.T) {
		body := fmt.Sprintf(`{"global_filters":[{"table_name":%q,"filter":{"conjunction":"and","conditions":[{"field_name":"Amount","operator":"isGreater","value":1000}]}}]}`, tableName)
		result, setErr := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"api", "PATCH", path}, Data: json.RawMessage(body), DefaultAs: "bot"})
		require.NoError(t, setErr)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, dashboardID, gjson.Get(result.Stdout, "data.dashboard_id").String())
		filters := gjson.Get(result.Stdout, "data.global_filters")
		require.True(t, filters.IsArray())
		require.Len(t, filters.Array(), 1)
		require.JSONEq(t, filters.Raw, readFilters(t))
		assertCount(t, 2)
	}) {
		return
	}
	t.Run("clear and restore chart count", func(t *testing.T) {
		result, clearErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"api", "PATCH", path}, Data: json.RawMessage(`{"global_filters":[]}`), DefaultAs: "bot",
		})
		require.NoError(t, clearErr)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, dashboardID, gjson.Get(result.Stdout, "data.dashboard_id").String())
		require.JSONEq(t, `[]`, gjson.Get(result.Stdout, "data.global_filters").Raw)
		require.JSONEq(t, `[]`, readFilters(t))
		assertCount(t, 3)
	})
}
