// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
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

func TestBaseDashboardBlockRankingDryRunRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name       string
		dataConfig string
		want       string
	}{
		{
			name:       "series child",
			dataConfig: `{"table_name":"Orders","series":[{"field_name":"Amount","rollup":"SUM","formula":"x"}],"group_by":[{"field_name":"Owner"}]}`,
			want:       "series[0] 不支持字段 formula",
		},
		{
			name:       "group child",
			dataConfig: `{"table_name":"Orders","count_all":true,"group_by":[{"field_name":"Owner","table_id":"tbl_x"}]}`,
			want:       "group_by[0] 不支持字段 table_id",
		},
		{
			name:       "top-level field",
			dataConfig: `{"table_name":"Orders","count_all":true,"group_by":[{"field_name":"Owner"}],"limit_siz":10}`,
			want:       "ranking 不支持字段 limit_siz",
		},
		{
			name:       "filter type",
			dataConfig: `{"table_name":"Orders","count_all":true,"group_by":[{"field_name":"Owner"}],"filter":"all"}`,
			want:       "filter 必须是对象",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := runBaseDryRun(t, 2,
				"base", "+dashboard-block-create",
				"--base-token", "app_x",
				"--dashboard-id", "dsh_x",
				"--name", "Invalid ranking",
				"--type", "ranking",
				"--data-config", tc.dataConfig,
			)
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
			require.Equal(t, "--data-config", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
			require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), tc.want, result.Stderr)
			require.Empty(t, result.Stdout)
		})
	}
}
