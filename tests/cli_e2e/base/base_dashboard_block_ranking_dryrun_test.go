// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestBaseDashboardBlockRankingCreateDryRun(t *testing.T) {
	result := runBaseDryRun(t, 0,
		"base", "+dashboard-block-create",
		"--base-token", "app_x",
		"--dashboard-id", "dsh_x",
		"--name", "Owner ranking",
		"--type", "ranking",
		"--data-config", `{"table_name":"Orders","count_all":true,"group_by":[{"field_name":"Owner","mode":"integrated"}]}`,
	)

	out := result.Stdout
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.Equal(t, "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks", clie2e.DryRunGet(out, "api.0.url").String(), out)
	require.Equal(t, "ranking", clie2e.DryRunGet(out, "api.0.body.type").String(), out)
	require.Equal(t, int64(10), clie2e.DryRunGet(out, "api.0.body.data_config.limit_size").Int(), out)
	require.Equal(t, "value", clie2e.DryRunGet(out, "api.0.body.data_config.group_by.0.sort.type").String(), out)
	require.Equal(t, "desc", clie2e.DryRunGet(out, "api.0.body.data_config.group_by.0.sort.order").String(), out)
}

func TestBaseDashboardBlockRankingUpdateDryRunPreservesPatch(t *testing.T) {
	result := runBaseDryRun(t, 0,
		"base", "+dashboard-block-update",
		"--base-token", "app_x",
		"--dashboard-id", "dsh_x",
		"--block-id", "blk_x",
		"--data-config", `{"limit_size":25}`,
	)

	out := result.Stdout
	require.Equal(t, "PATCH", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.Equal(t, int64(25), clie2e.DryRunGet(out, "api.0.body.data_config.limit_size").Int(), out)
	require.False(t, clie2e.DryRunGet(out, "api.0.body.data_config.group_by").Exists(), out)
}
