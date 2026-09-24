// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"os"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// This workflow uses isolated resources and needs a deployment with countdown
// partial-update support. Dry-run alone cannot prove merge or readback behavior.
func TestDashboardCountdownWorkflow(t *testing.T) {
	if os.Getenv("LARK_CLI_E2E_DASHBOARD_COUNTDOWN_READY") != "1" {
		t.Skip("set LARK_CLI_E2E_DASHBOARD_COUNTDOWN_READY=1 after countdown partial updates are deployed")
	}
	clie2e.SkipWithoutTenantAccessToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	baseToken := createBaseWithRetry(t, ctx, "lark-cli-countdown-"+clie2e.GenerateSuffix())
	for _, name := range []string{"Tasks", "Milestones"} {
		createTableWithRetry(t, t, ctx, baseToken, name,
			`[{"name":"Name","type":"text"},{"name":"Deadline","type":"datetime"},{"name":"Planned","type":"datetime"},{"name":"Done","type":"checkbox"},{"name":"Cost","type":"number"}]`,
			`{"name":"Main","type":"grid"}`)
	}
	run := func(args ...string) *clie2e.Result {
		t.Helper()
		result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: append([]string{"base"}, args...), DefaultAs: "bot"})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		return result
	}
	dashboard := run("+dashboard-create", "--base-token", baseToken, "--name", "Countdown")
	dashboardID := gjson.Get(dashboard.Stdout, "data.dashboard.dashboard_id").String()
	require.NotEmpty(t, dashboardID)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		result, err := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args: []string{"base", "+dashboard-delete", "--base-token", baseToken, "--dashboard-id", dashboardID, "--yes"}, DefaultAs: "bot",
		})
		clie2e.ReportCleanupFailure(t, "delete countdown dashboard", result, err)
	})
	get := func(id string) gjson.Result {
		t.Helper()
		result := run("+dashboard-block-get", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", id)
		require.Equal(t, "countdown", gjson.Get(result.Stdout, "data.block.type").String())
		return gjson.Get(result.Stdout, "data.block.data_config")
	}
	create := func(config string) string {
		t.Helper()
		result := run("+dashboard-block-create", "--base-token", baseToken, "--dashboard-id", dashboardID, "--name", "Deadline", "--type", "countdown", "--data-config", config)
		id := gjson.Get(result.Stdout, "data.block.block_id").String()
		require.NotEmpty(t, id)
		require.JSONEq(t, gjson.Get(result.Stdout, "data.block.data_config").Raw, get(id).Raw)
		return id
	}
	update := func(id, patch string) gjson.Result {
		t.Helper()
		result := run("+dashboard-block-update", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", id, "--data-config", patch)
		readback := get(id)
		require.JSONEq(t, gjson.Get(result.Stdout, "data.block.data_config").Raw, readback.Raw)
		return readback
	}

	fixedID := create(`{"type":"fixed","targetTime":0}`)
	require.JSONEq(t, `{"type":"fixed","targetTime":1798732800000}`, update(fixedID, `{"targetTime":1798732800000}`).Raw)
	const field = `{"type":"field","aggregateType":"MIN","additionalDatasources":[{"tableName":"Tasks","countAll":true,"groupBy":[{"fieldName":"Deadline"}],"filter":{"conjunction":"and","conditions":[{"fieldName":"Done","operator":"is","value":false}]}}]}`
	fieldID := create(field)
	initial := get(fieldID)
	filterBefore := initial.Get("additionalDatasources.0.filter").Raw
	require.NotEmpty(t, filterBefore)
	changed := update(fieldID, `{"aggregateType":"MAX"}`)
	require.Equal(t, "MAX", changed.Get("aggregateType").String())
	require.JSONEq(t, filterBefore, changed.Get("additionalDatasources.0.filter").Raw)
	changed = update(fieldID, `{"additionalDatasources":[{"groupBy":[{"fieldName":"Planned"}]}]}`)
	require.Equal(t, "Planned", changed.Get("additionalDatasources.0.groupBy.0.fieldName").String())
	require.Equal(t, "MAX", changed.Get("aggregateType").String())
	require.JSONEq(t, filterBefore, changed.Get("additionalDatasources.0.filter").Raw)
	changed = update(fieldID, `{"additionalDatasources":[{"filter":{"conjunction":"and","conditions":[{"fieldName":"Cost","operator":"isGreater","value":0}]}}]}`)
	require.Len(t, changed.Get("additionalDatasources.0.filter.conditions").Array(), 1)
	require.Equal(t, "Cost", changed.Get("additionalDatasources.0.filter.conditions.0.fieldName").String())
	changed = update(fieldID, `{"additionalDatasources":[{"series":[{"fieldName":"Cost","rollup":"SUM"}]}]}`)
	require.False(t, changed.Get("additionalDatasources.0.countAll").Exists())
	require.Equal(t, "Cost", changed.Get("additionalDatasources.0.series.0.fieldName").String())
	changed = update(fieldID, `{"additionalDatasources":[{"countAll":true}]}`)
	require.False(t, changed.Get("additionalDatasources.0.series").Exists())
	require.True(t, changed.Get("additionalDatasources.0.countAll").Bool())
	changed = update(fieldID, `{"additionalDatasources":[{"filter":null}]}`)
	require.False(t, changed.Get("additionalDatasources.0.filter").Exists())
	require.JSONEq(t, changed.Raw, update(fieldID, `{}`).Raw)
	require.JSONEq(t, changed.Raw, update(fieldID, `{"additionalDatasources":[{}]}`).Raw)

	// Rejections must preserve the previous config and independent name update.
	for _, patch := range []string{`{"type":"fixed"}`, `{"additionalDatasources":[{"tableName":"Milestones"}]}`, `{"targetTime":0}`} {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"base", "+dashboard-block-update", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", fieldID, "--name", "Should not apply", "--data-config", patch}, DefaultAs: "bot",
		})
		require.NoError(t, err)
		require.NotZero(t, result.ExitCode)
		require.False(t, gjson.Get(result.Stderr, "ok").Bool())
		require.JSONEq(t, changed.Raw, get(fieldID).Raw)
		read := run("+dashboard-block-get", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", fieldID)
		require.Equal(t, "Deadline", gjson.Get(read.Stdout, "data.block.name").String())
	}

	update(fieldID, field) // Restore an explicit filter before changing tables.
	changed = update(fieldID, `{"aggregateType":"MAX","additionalDatasources":[{"tableName":"Milestones","countAll":true,"groupBy":[{"fieldName":"Deadline"}]}]}`)
	require.Equal(t, "Milestones", changed.Get("additionalDatasources.0.tableName").String())
	require.Equal(t, "MAX", changed.Get("aggregateType").String())
	require.False(t, changed.Get("additionalDatasources.0.filter").Exists())
	require.JSONEq(t, `{"type":"fixed","targetTime":0}`, update(fieldID, `{"type":"fixed","targetTime":0}`).Raw)
	changed = update(fieldID, field)
	require.Equal(t, "field", changed.Get("type").String())
	require.False(t, changed.Get("targetTime").Exists())
	require.Equal(t, "Tasks", changed.Get("additionalDatasources.0.tableName").String())
	changed = update(fixedID, `{"type":"field","aggregateType":"MIN","additionalDatasources":[{"tableName":"Tasks","countAll":true,"groupBy":[{"fieldName":"Deadline"}]}]}`)
	require.False(t, changed.Get("additionalDatasources.0.filter").Exists())
}
