// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// This workflow needs the HTTP/Node linkage contract deployed. It creates all
// fixtures as bot and verifies persistence; page clicks require separate UAT.
func TestBaseDashboardLinkageWorkflow(t *testing.T) {
	if os.Getenv("LARK_CLI_E2E_LINKAGE_READY") != "1" {
		t.Skip("set LARK_CLI_E2E_LINKAGE_READY=1 after same-datasource linkage is deployed")
	}
	clie2e.SkipWithoutTenantAccessToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	t.Cleanup(cancel)
	call := func(t *testing.T, args ...string) string {
		t.Helper()
		result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		return result.Stdout
	}
	baseToken := createBaseWithRetry(t, ctx, "lark-cli-e2e-linkage-"+clie2e.GenerateSuffix())
	tableNames := []string{"Orders", "Budget"}
	for _, name := range tableNames {
		createTableWithRetry(t, t, ctx, baseToken, name, `[{"name":"Region","type":"text"}]`, `{"name":"Main","type":"grid"}`)
	}
	dashboard := call(t, "base", "+dashboard-create", "--base-token", baseToken, "--name", "Regional charts")
	dashboardID := gjson.Get(dashboard, "data.dashboard.dashboard_id").String()
	require.NotEmpty(t, dashboardID)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		result, err := clie2e.RunCmd(cleanupCtx, clie2e.Request{Args: []string{"base", "+dashboard-delete", "--base-token", baseToken, "--dashboard-id", dashboardID, "--yes"}, DefaultAs: "bot"})
		clie2e.ReportCleanupFailure(t, "delete linkage dashboard", result, err)
	})
	var ids []string
	for _, name := range []string{"Source", "Target B", "Target C", "Different table"} {
		table := tableNames[0]
		if name == "Different table" {
			table = tableNames[1]
		}
		config := `{"table_name":"` + table + `","count_all":true,"group_by":[{"field_name":"Region","mode":"integrated"}]}`
		result := call(t, "base", "+dashboard-block-create", "--base-token", baseToken, "--dashboard-id", dashboardID, "--name", name, "--type", "column", "--data-config", config)
		id := gjson.Get(result, "data.block.block_id").String()
		require.NotEmpty(t, id)
		ids = append(ids, id)
	}
	read := func(t *testing.T, id string) string {
		t.Helper()
		return call(t, "base", "+dashboard-block-get", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", id)
	}
	linkageJSON := func(t *testing.T, targets []string) string {
		t.Helper()
		encoded, err := json.Marshal(struct {
			Targets []string `json:"target_block_ids"`
		}{targets})
		require.NoError(t, err)
		return string(encoded)
	}
	update := func(t *testing.T, source string, targets []string) string {
		t.Helper()
		return call(t, "base", "+dashboard-block-update", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", source, "--linkage", linkageJSON(t, targets))
	}
	assertTargets := func(t *testing.T, output string, expected []string) {
		t.Helper()
		actual := gjson.Get(output, "data.block.linkage.target_block_ids")
		require.True(t, actual.IsArray())
		var values []string
		for _, value := range actual.Array() {
			values = append(values, value.String())
		}
		require.ElementsMatch(t, expected, values)
	}
	t.Run("set and read same-table targets", func(t *testing.T) {
		update(t, ids[2], []string{ids[0]})
		assertTargets(t, update(t, ids[0], ids[1:3]), ids[1:3])
		assertTargets(t, read(t, ids[0]), ids[1:3])
	})
	t.Run("replace preserves another source", func(t *testing.T) {
		assertTargets(t, update(t, ids[0], ids[1:2]), ids[1:2])
		assertTargets(t, read(t, ids[0]), ids[1:2])
		assertTargets(t, read(t, ids[2]), []string{ids[0]})
	})
	t.Run("reject different table and preserve previous state", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"base", "+dashboard-block-update", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", ids[0], "--linkage", linkageJSON(t, ids[3:])}, DefaultAs: "bot"})
		require.NoError(t, err)
		require.NotZero(t, result.ExitCode)
		require.False(t, gjson.Get(result.Stderr, "ok").Bool())
		require.NotZero(t, gjson.Get(result.Stderr, "error.code").Int())
		assertTargets(t, read(t, ids[0]), ids[1:2])
	})
	t.Run("omission preserves and empty list clears", func(t *testing.T) {
		call(t, "base", "+dashboard-block-update", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", ids[0], "--name", "Renamed source")
		assertTargets(t, read(t, ids[0]), ids[1:2])
		update(t, ids[0], []string{})
		require.False(t, gjson.Get(read(t, ids[0]), "data.block.linkage").Exists())
		assertTargets(t, read(t, ids[2]), []string{ids[0]})
	})
}
