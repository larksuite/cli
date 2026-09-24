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

// The workflow owns every resource it touches and verifies persisted config
// after each write. Enable it after deploying progress create and patch support.
func TestBaseDashboardProgressWorkflow(t *testing.T) {
	if os.Getenv("LARK_CLI_E2E_PROGRESS_READY") != "1" {
		t.Skip("set LARK_CLI_E2E_PROGRESS_READY=1 after progress create and partial update are deployed")
	}
	clie2e.SkipWithoutTenantAccessToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	t.Cleanup(cancel)

	baseToken := createBaseWithRetry(t, ctx, "lark-cli-e2e-progress-"+clie2e.GenerateSuffix())
	tableName := "Progress Metrics " + clie2e.GenerateSuffix()
	goalTable := "Progress Goals " + clie2e.GenerateSuffix()
	for _, name := range []string{tableName, goalTable} {
		createTableWithRetry(t, t, ctx, baseToken, name, `[{"name":"Name","type":"text"},{"name":"Amount","type":"number"}]`, `{"name":"Main","type":"grid"}`)
	}
	created, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"base", "+dashboard-create", "--base-token", baseToken, "--name", "Progress workflow " + clie2e.GenerateSuffix()}, DefaultAs: "bot",
	})
	require.NoError(t, err)
	created.AssertExitCode(t, 0)
	created.AssertStdoutStatus(t, true)
	dashboardID := gjson.Get(created.Stdout, "data.dashboard.dashboard_id").String()
	require.NotEmpty(t, dashboardID, created.Stdout)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		result, cleanupErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args: []string{"base", "+dashboard-delete", "--base-token", baseToken, "--dashboard-id", dashboardID, "--yes"}, DefaultAs: "bot",
		})
		clie2e.ReportCleanupFailure(t, "delete progress dashboard", result, cleanupErr)
	})

	config := `{"progress":{"type":"value","current":{"type":"field","table_name":"` + tableName + `","count_all":true,"filter":{"conjunction":"and","conditions":[{"field_name":"Name","operator":"is","value":["kept"]}]}},"target":{"type":"custom","value":100}}}`
	created, err = clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"base", "+dashboard-block-create", "--base-token", baseToken, "--dashboard-id", dashboardID, "--name", "Progress", "--type", "progress", "--data-config", config}, DefaultAs: "bot",
	})
	require.NoError(t, err)
	created.AssertExitCode(t, 0)
	created.AssertStdoutStatus(t, true)
	blockID := gjson.Get(created.Stdout, "data.block.block_id").String()
	require.NotEmpty(t, blockID, created.Stdout)
	deleted := false
	t.Cleanup(func() {
		if deleted {
			return
		}
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		result, cleanupErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args: []string{"base", "+dashboard-block-delete", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", blockID, "--yes"}, DefaultAs: "bot",
		})
		clie2e.ReportCleanupFailure(t, "delete progress block", result, cleanupErr)
	})

	read := func(t *testing.T) gjson.Result {
		t.Helper()
		result, readErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"base", "+dashboard-block-get", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", blockID}, DefaultAs: "bot",
		})
		require.NoError(t, readErr)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, blockID, gjson.Get(result.Stdout, "data.block.block_id").String())
		require.Equal(t, "progress", gjson.Get(result.Stdout, "data.block.type").String())
		progress := gjson.Get(result.Stdout, "data.block.data_config.progress")
		require.True(t, progress.IsObject(), result.Stdout)
		return progress
	}
	update := func(t *testing.T, patch string) gjson.Result {
		t.Helper()
		result, updateErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"base", "+dashboard-block-update", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", blockID, "--data-config", patch}, DefaultAs: "bot",
		})
		require.NoError(t, updateErr)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		persisted := read(t)
		require.JSONEq(t, persisted.Raw, gjson.Get(result.Stdout, "data.block.data_config.progress").Raw)
		return persisted
	}
	initial := read(t)
	require.Equal(t, "value", initial.Get("type").String())
	require.Equal(t, tableName, initial.Get("current.table_name").String())
	require.Equal(t, "kept", initial.Get("current.filter.conditions.0.value.0").String())

	if !t.Run("target-only preserves current and filter", func(t *testing.T) {
		next := update(t, `{"progress":{"target":{"value":200}}}`)
		require.Equal(t, float64(200), next.Get("target.value").Float())
		require.JSONEq(t, initial.Get("current").Raw, next.Get("current").Raw)
	}) {
		return
	}
	if !t.Run("clear and replace filter", func(t *testing.T) {
		next := update(t, `{"progress":{"current":{"filter":null}}}`)
		require.False(t, next.Get("current.filter").Exists())
		require.Equal(t, float64(200), next.Get("target.value").Float())
		next = update(t, `{"progress":{"current":{"filter":{"conjunction":"or","conditions":[{"field_name":"Name","operator":"isNot","value":["excluded"]}]}}}}`)
		require.Equal(t, "or", next.Get("current.filter.conjunction").String())
		require.Equal(t, "excluded", next.Get("current.filter.conditions.0.value.0").String())
	}) {
		return
	}
	if !t.Run("switch one table and one source", func(t *testing.T) {
		next := update(t, `{"progress":{"current":{"table_name":"`+goalTable+`","count_all":true}}}`)
		require.Equal(t, goalTable, next.Get("current.table_name").String())
		require.False(t, next.Get("current.filter").Exists())
		next = update(t, `{"progress":{"target":{"type":"field","table_name":"`+tableName+`","series":[{"field_name":"Amount","rollup":"SUM"}]}}}`)
		require.Equal(t, goalTable, next.Get("current.table_name").String())
		require.Equal(t, tableName, next.Get("target.table_name").String())
		require.Equal(t, "Amount", next.Get("target.series.0.field_name").String())
	}) {
		return
	}
	if !t.Run("switch to date and preserve date format", func(t *testing.T) {
		next := update(t, `{"progress":{"type":"date","current":{"type":"relative","value":"yesterday"},"target":{"type":"relative","value":"quarter"},"dateFormat":"YYYY/MM/DD"}}`)
		require.Equal(t, "date", next.Get("type").String())
		require.False(t, next.Get("current.table_name").Exists())
		next = update(t, `{"progress":{"target":{"value":"month"}}}`)
		require.Equal(t, "month", next.Get("target.value").String())
		require.Equal(t, "yesterday", next.Get("current.value").String())
		require.Equal(t, "YYYY/MM/DD", next.Get("dateFormat").String())
		next = update(t, `{"progress":{"target":{"type":"custom","range":[0,1]}}}`)
		require.JSONEq(t, `[0,1]`, next.Get("target.range").Raw)
		require.Equal(t, "YYYY/MM/DD", next.Get("dateFormat").String())
	}) {
		return
	}
	if !t.Run("switch to value and replay full config", func(t *testing.T) {
		next := update(t, `{"progress":{"type":"value","current":{"type":"custom","value":0},"target":{"type":"custom","value":100}}}`)
		require.Equal(t, float64(0), next.Get("current.value").Float())
		require.False(t, next.Get("dateFormat").Exists())
		replayed := update(t, `{"progress":`+next.Raw+`}`)
		require.JSONEq(t, next.Raw, replayed.Raw)
	}) {
		return
	}
	t.Run("delete block", func(t *testing.T) {
		result, deleteErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"base", "+dashboard-block-delete", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", blockID, "--yes"}, DefaultAs: "bot",
		})
		require.NoError(t, deleteErr)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.True(t, gjson.Get(result.Stdout, "data.deleted").Bool(), result.Stdout)
		deleted = true
	})
}
