// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseDashboardPivotDryRun_CreateValuesOnlyMultipleMetrics(t *testing.T) {
	result := runBaseDryRun(t, 0,
		"base", "+dashboard-block-create",
		"--base-token", "app_x",
		"--dashboard-id", "dsh_1",
		"--name", "Values only",
		"--type", "pivotTable",
		"--data-config", `{"table_name":"Sales","values":[{"field_name":"Amount","rollup":"sum"},{"field_name":"Customer","rollup":"count_distinct"}]}`,
	)
	out := result.Stdout
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.Equal(t, "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks", clie2e.DryRunGet(out, "api.0.url").String(), out)
	require.Equal(t, "pivotTable", clie2e.DryRunGet(out, "api.0.body.type").String(), out)
	rows := clie2e.DryRunGet(out, "api.0.body.data_config.rows")
	columns := clie2e.DryRunGet(out, "api.0.body.data_config.columns")
	require.True(t, rows.IsArray(), out)
	require.True(t, columns.IsArray(), out)
	require.Empty(t, rows.Array(), out)
	require.Empty(t, columns.Array(), out)
	require.Equal(t, 2, len(clie2e.DryRunGet(out, "api.0.body.data_config.values").Array()), out)
	require.Equal(t, "SUM", clie2e.DryRunGet(out, "api.0.body.data_config.values.0.rollup").String(), out)
	require.Equal(t, "COUNT_DISTINCT", clie2e.DryRunGet(out, "api.0.body.data_config.values.1.rollup").String(), out)
	require.False(t, clie2e.DryRunGet(out, "api.0.body.data_config.sort").Exists(), out)
}

func TestBaseDashboardPivotDryRun_UpdateExplicitSortClear(t *testing.T) {
	result := runBaseDryRun(t, 0,
		"base", "+dashboard-block-update",
		"--base-token", "app_x",
		"--dashboard-id", "dsh_1",
		"--block-id", "blk_1",
		"--data-config", `{"sort":[]}`,
	)
	out := result.Stdout
	require.Equal(t, "PATCH", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.Equal(t, "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks/blk_1", clie2e.DryRunGet(out, "api.0.url").String(), out)
	sortConfig := clie2e.DryRunGet(out, "api.0.body.data_config.sort")
	require.True(t, sortConfig.IsArray(), out)
	require.Empty(t, sortConfig.Array(), out)
	require.False(t, clie2e.DryRunGet(out, "api.0.body.data_config.rows").Exists(), out)
	require.False(t, clie2e.DryRunGet(out, "api.0.body.data_config.columns").Exists(), out)
	require.False(t, clie2e.DryRunGet(out, "api.0.body.data_config.values").Exists(), out)
}

func TestBaseDashboardPivotDryRun_InvalidReferenceIsTyped(t *testing.T) {
	result := runBaseDryRun(t, 2,
		"base", "+dashboard-block-create",
		"--base-token", "app_x",
		"--dashboard-id", "dsh_1",
		"--name", "Bad pivot",
		"--type", "pivotTable",
		"--data-config", `{"table_name":"Sales","rows":[{"field_name":"Category"}],"values":[{"field_name":"Amount","rollup":"SUM"}],"sort":[{"sort_type":"FIELD","order":"asc","group_ref":{"area":"rows","index":1}}]}`,
	)
	require.True(t, gjson.Valid(result.Stderr), "stderr:\n%s", result.Stderr)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	require.Equal(t, "--data-config", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
	require.Contains(t, result.Stderr, "group_ref.index")
}

func TestBaseDashboardPivotDryRun_RejectsMissingAndUnsupportedConfig(t *testing.T) {
	tests := []struct {
		name       string
		extraArgs  []string
		wantDetail string
	}{
		{name: "missing data config", wantDetail: "pivotTable 类型组件必须提供 data-config"},
		{name: "ordinary chart fields", extraArgs: []string{"--data-config", `{"table_name":"Sales","series":[]}`}, wantDetail: "pivotTable 不支持字段 series"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{
				"base", "+dashboard-block-create",
				"--base-token", "app_x",
				"--dashboard-id", "dsh_1",
				"--name", "Invalid pivot",
				"--type", "pivotTable",
			}
			args = append(args, tt.extraArgs...)
			result := runBaseDryRun(t, 2, args...)
			require.True(t, gjson.Valid(result.Stderr), "stderr:\n%s", result.Stderr)
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
			require.Equal(t, "--data-config", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
			require.Contains(t, result.Stderr, tt.wantDetail)
		})
	}
}

func TestBaseDashboardPivotDryRun_UpdateRejectsInvalidReferenceInReplacedArea(t *testing.T) {
	result := runBaseDryRun(t, 2,
		"base", "+dashboard-block-update",
		"--base-token", "app_x",
		"--dashboard-id", "dsh_1",
		"--block-id", "blk_1",
		"--data-config", `{"rows":[{"field_name":"Region"}],"columns":[],"values":[{"field_name":"Amount","rollup":"SUM"}],"sort":[{"sort_type":"FIELD","order":"asc","group_ref":{"area":"rows","index":1}}]}`,
	)
	require.True(t, gjson.Valid(result.Stderr), "stderr:\n%s", result.Stderr)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "--data-config", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
	require.Contains(t, result.Stderr, "group_ref.index")
}

func TestBaseDashboardPivotDryRun_UpdateSortOnlyRejectsInvalidReferences(t *testing.T) {
	tests := []struct {
		name       string
		dataConfig string
		wantDetail string
	}{
		{
			name:       "missing group reference",
			dataConfig: `{"sort":[{"sort_type":"FIELD","order":"asc"}]}`,
			wantDetail: "sort 缺少 group_field_key 或 group_ref",
		},
		{
			name:       "numeric group key",
			dataConfig: `{"sort":[{"sort_type":"FIELD","order":"asc","group_field_key":123}]}`,
			wantDetail: "group_field_key 必须是非空字符串",
		},
		{
			name:       "field sort with value reference",
			dataConfig: `{"sort":[{"sort_type":"FIELD","order":"asc","group_field_key":"row_1","value_ref":{"area":"values","index":0}}]}`,
			wantDetail: "value_ref 仅支持 VALUE 排序",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runBaseDryRun(t, 2,
				"base", "+dashboard-block-update",
				"--base-token", "app_x",
				"--dashboard-id", "dsh_1",
				"--block-id", "blk_1",
				"--data-config", tt.dataConfig,
			)
			require.True(t, gjson.Valid(result.Stderr), "stderr:\n%s", result.Stderr)
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
			require.Equal(t, "--data-config", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
			require.Contains(t, result.Stderr, tt.wantDetail)
		})
	}
}
