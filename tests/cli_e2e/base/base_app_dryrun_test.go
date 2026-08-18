// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseWorkspaceDryRun(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		result := runBaseDryRun(t, 0, "base", "+workspace-create", "--name", "Growth")
		output := strings.TrimSpace(result.Stdout)
		assert.Contains(t, output, "/open-apis/base/v3/workspaces")
		assert.Contains(t, output, `"method": "POST"`)
		assert.Contains(t, output, `"name": "Growth"`)
	})

	t.Run("create rejects unsupported icon", func(t *testing.T) {
		result := runBaseDryRun(t, 2, "base", "+workspace-create", "--name", "Growth", "--icon", "icon_1")
		assert.Contains(t, result.Stderr, "unknown flag")
	})

	t.Run("entity-list", func(t *testing.T) {
		result := runBaseDryRun(t, 0, "base", "+workspace-entity-list", "--workspace-token", "ws_x", "--type", "baseapp")
		output := strings.TrimSpace(result.Stdout)
		assert.Contains(t, output, "/open-apis/base/v3/workspaces/ws_x/entities")
		assert.Contains(t, output, `"method": "GET"`)
		assert.Contains(t, output, `"entity_type": "baseapp"`)
	})

	t.Run("move-in", func(t *testing.T) {
		result := runBaseDryRun(t, 0, "base", "+workspace-move-in",
			"--workspace-token", "ws_x", "--entity-token", "bascn_1")
		output := strings.TrimSpace(result.Stdout)
		assert.Contains(t, output, "/open-apis/base/v3/workspaces/ws_x/move_in")
		assert.Contains(t, output, `"entity_token": "bascn_1"`)
	})

}

func TestBaseappDryRun(t *testing.T) {
	t.Run("resolve app URL locally", func(t *testing.T) {
		result := runBaseDryRun(t, 0, "base", "+url-resolve", "--url",
			"https://example.larkoffice.com/app/app_x?pre_pathname=%2Fbase%2Fworkspace%2Fws_x&pageId=pg_1")
		output := strings.TrimSpace(result.Stdout)
		assert.Contains(t, output, `"resolution": "local"`)
		assert.Contains(t, output, `"url"`)
		assert.NotContains(t, output, `"/open-apis/`)
	})

	t.Run("create", func(t *testing.T) {
		result := runBaseDryRun(t, 0, "base", "+app-create",
			"--name", "Sales app", "--workspace-token", "ws_x", "--theme-style", "cloudBlue")
		output := strings.TrimSpace(result.Stdout)
		assert.Contains(t, output, "/open-apis/base/v3/base_apps")
		assert.Contains(t, output, `"method": "POST"`)
		assert.Contains(t, output, `"theme_style": "cloudBlue"`)
		assert.NotContains(t, output, "/open-apis/base/v3/bases")
		assert.NotContains(t, output, "/move_in")
	})

	t.Run("create requires workspace", func(t *testing.T) {
		result := runBaseDryRun(t, 2, "base", "+app-create", "--name", "Sales app")
		assert.Contains(t, result.Stderr, "workspace-token")
	})

	t.Run("get", func(t *testing.T) {
		result := runBaseDryRun(t, 0, "base", "+app-get", "--app-token", "app_x")
		output := strings.TrimSpace(result.Stdout)
		assert.Contains(t, output, "/open-apis/base/v3/base_apps/app_x")
		assert.NotContains(t, output, "with_pages")
		assert.NotContains(t, output, "with_components")
	})

}

func TestBaseappPageDryRun(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		result := runBaseDryRun(t, 0, "base", "+app-page-list", "--app-token", "app_x")
		assert.Contains(t, result.Stdout, "/open-apis/base/v3/base_apps/app_x/pages")
	})

	t.Run("get", func(t *testing.T) {
		result := runBaseDryRun(t, 0, "base", "+app-page-get", "--app-token", "app_x", "--page-id", "pg_1")
		output := strings.TrimSpace(result.Stdout)
		assert.Contains(t, output, "/open-apis/base/v3/base_apps/app_x/pages/pg_1")
		assert.NotContains(t, output, "with_components")
	})

	t.Run("create", func(t *testing.T) {
		result := runBaseDryRun(t, 0, "base", "+app-page-create", "--app-token", "app_x", "--name", "Overview")
		output := strings.TrimSpace(result.Stdout)
		assert.Contains(t, output, "/open-apis/base/v3/base_apps/app_x/pages")
		assert.Contains(t, output, `"name": "Overview"`)
		assert.NotContains(t, output, "page_group_id")
	})

	t.Run("create rejects page group", func(t *testing.T) {
		result := runBaseDryRun(t, 2, "base", "+app-page-create",
			"--app-token", "app_x", "--name", "Overview", "--page-group-id", "pgrp_1")
		assert.Contains(t, result.Stderr, "unknown flag")
	})

	t.Run("rename", func(t *testing.T) {
		result := runBaseDryRun(t, 0, "base", "+app-page-update", "--app-token", "app_x", "--page-id", "pg_1", "--name", "Sales")
		output := strings.TrimSpace(result.Stdout)
		assert.Contains(t, output, "/open-apis/base/v3/base_apps/app_x/pages/pg_1")
		assert.Contains(t, output, `"method": "PATCH"`)
	})

	t.Run("delete", func(t *testing.T) {
		result := runBaseDryRun(t, 0, "base", "+app-page-delete", "--app-token", "app_x", "--page-id", "pg_1")
		output := strings.TrimSpace(result.Stdout)
		assert.Contains(t, output, "/open-apis/base/v3/base_apps/app_x/pages/pg_1")
		assert.Contains(t, output, `"method": "DELETE"`)
	})
}

func TestAppBlockDryRun(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		result := runBaseDryRun(t, 0, "base", "+app-block-list", "--app-token", "app_x", "--page-id", "pg_1")
		output := strings.TrimSpace(result.Stdout)
		assert.Contains(t, output, "/open-apis/base/v3/base_apps/app_x/pages/pg_1/blocks")
		assert.Contains(t, output, `"page_size": 20`)
	})

	t.Run("get", func(t *testing.T) {
		result := runBaseDryRun(t, 0, "base", "+app-block-get", "--app-token", "app_x", "--page-id", "pg_1", "--block-id", "wid_1")
		assert.Contains(t, result.Stdout, "/open-apis/base/v3/base_apps/app_x/pages/pg_1/blocks/wid_1")
	})

	t.Run("create chart", func(t *testing.T) {
		result := runBaseDryRun(t, 0, "base", "+app-block-create",
			"--app-token", "app_x", "--page-id", "pg_1",
			"--name", "Sales by month", "--type", "line",
			"--data-config", `{"base_token":"basx","data_sources":[{"table_name":"Orders","series":[{"field_name":"Amount","rollup":"sum"}]}]}`)
		output := strings.TrimSpace(result.Stdout)
		assert.Contains(t, output, "/open-apis/base/v3/base_apps/app_x/pages/pg_1/blocks")
		assert.Contains(t, output, `"method": "POST"`)
		assert.Contains(t, output, `"type": "line"`)
		// App 图表：顶层 base_token + 多数据源 data_sources
		assert.Contains(t, output, `"base_token": "basx"`)
		assert.Contains(t, output, "data_sources")
		// normalizeAppChartDataConfig 把每个数据源的 rollup 归一化为大写
		assert.Contains(t, output, "SUM")
	})

	t.Run("create chart rejects missing base_token", func(t *testing.T) {
		result := runBaseDryRun(t, 2, "base", "+app-block-create",
			"--app-token", "app_x", "--page-id", "pg_1",
			"--name", "Sales by month", "--type", "line",
			"--data-config", `{"data_sources":[{"table_name":"Orders","series":[{"field_name":"Amount","rollup":"SUM"}]}]}`)
		assert.Contains(t, result.Stderr, "base_token")
	})

	t.Run("create chart requires data_config", func(t *testing.T) {
		result := runBaseDryRun(t, 2, "base", "+app-block-create",
			"--app-token", "app_x", "--page-id", "pg_1",
			"--name", "Sales by month", "--type", "line")
		assert.Contains(t, result.Stderr, "data-config")
	})

	t.Run("create list", func(t *testing.T) {
		result := runBaseDryRun(t, 0, "base", "+app-block-create",
			"--app-token", "app_x", "--page-id", "pg_1",
			"--name", "Open orders", "--type", "list",
			"--data-config", `{"base_token":"basx","table_name":"Orders"}`)
		output := strings.TrimSpace(result.Stdout)
		assert.Contains(t, output, `"type": "list"`)
		assert.Contains(t, output, "basx")
		assert.NotContains(t, output, `"columns"`)
		assert.NotContains(t, output, `"sub_type"`)
	})

	t.Run("create list requires data_config", func(t *testing.T) {
		result := runBaseDryRun(t, 2, "base", "+app-block-create",
			"--app-token", "app_x", "--page-id", "pg_1",
			"--name", "Open orders", "--type", "list", "--sub-type", "standard")
		assert.Contains(t, result.Stderr, "data-config")
	})

	t.Run("create text block allows omitted data_config", func(t *testing.T) {
		result := runBaseDryRun(t, 0, "base", "+app-block-create",
			"--app-token", "app_x", "--page-id", "pg_1",
			"--name", "Notes", "--type", "text")
		assert.NotContains(t, result.Stdout, `"data_config"`)
	})

	t.Run("create rejects an unsupported type", func(t *testing.T) {
		result := runBaseDryRun(t, 2, "base", "+app-block-create",
			"--app-token", "app_x", "--page-id", "pg_1", "--name", "X", "--type", "gantt")
		assert.NotEqual(t, 0, result.ExitCode)
	})

	t.Run("create rejects an invalid chart data_config", func(t *testing.T) {
		result := runBaseDryRun(t, 2, "base", "+app-block-create",
			"--app-token", "app_x", "--page-id", "pg_1", "--name", "X", "--type", "line",
			"--data-config", `{"base_token":"basx","data_sources":[{"series":[{"field_name":"Amount","rollup":"SUM"}]}]}`)
		assert.Contains(t, result.Stderr, "table_name")
	})

	t.Run("create multi-datasource chart", func(t *testing.T) {
		result := runBaseDryRun(t, 0, "base", "+app-block-create",
			"--app-token", "app_x", "--page-id", "pg_1",
			"--name", "Sales vs cost", "--type", "combo",
			"--data-config", `{"base_token":"basx","data_source_mode":"compare","data_sources":[{"table_name":"Sales","series":[{"field_name":"Amount","rollup":"SUM"}]},{"table_name":"Cost","series":[{"field_name":"Cost","rollup":"SUM"}]}],"sort":{"type":"group","order":"asc"}}`)
		output := strings.TrimSpace(result.Stdout)
		assert.Contains(t, output, `"type": "combo"`)
		assert.Contains(t, output, `"compare"`)
		assert.Contains(t, output, "Sales")
		assert.Contains(t, output, "Cost")
	})

	t.Run("update", func(t *testing.T) {
		result := runBaseDryRun(t, 0, "base", "+app-block-update",
			"--app-token", "app_x", "--page-id", "pg_1", "--block-id", "wid_1", "--name", "Monthly sales")
		output := strings.TrimSpace(result.Stdout)
		assert.Contains(t, output, "/open-apis/base/v3/base_apps/app_x/pages/pg_1/blocks/wid_1")
		assert.Contains(t, output, `"method": "PATCH"`)
	})

	t.Run("update requires name or data_config", func(t *testing.T) {
		result := runBaseDryRun(t, 2, "base", "+app-block-update",
			"--app-token", "app_x", "--page-id", "pg_1", "--block-id", "wid_1")
		require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
		require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
		require.Equal(t, "--name", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
		require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "至少提供一个", result.Stderr)
	})

	t.Run("update rejects unknown data_config field", func(t *testing.T) {
		result := runBaseDryRun(t, 2, "base", "+app-block-update",
			"--app-token", "app_x", "--page-id", "pg_1", "--block-id", "wid_1",
			"--data-config", `{"bogus":1}`)
		require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
		require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
		require.Equal(t, "--data-config", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
		require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "bogus", result.Stderr)
	})
}

func TestAppBlockGetDataDryRun(t *testing.T) {
	result := runBaseDryRun(t, 0, "base", "+app-block-get-data",
		"--app-token", "app_x", "--base-token", "bas_x", "--block-id", "cht_chart")
	output := strings.TrimSpace(result.Stdout)
	assert.Contains(t, output, "/open-apis/base/v3/base_apps/app_x/blocks/cht_chart/data")
	assert.Contains(t, output, `"method": "GET"`)

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "app token", args: []string{"--base-token", "bas_x", "--block-id", "cht_chart"}, want: "app-token"},
		{name: "base token", args: []string{"--app-token", "app_x", "--block-id", "cht_chart"}, want: "base-token"},
		{name: "block id", args: []string{"--app-token", "app_x", "--base-token", "bas_x"}, want: "block-id"},
	} {
		t.Run("missing "+tc.name, func(t *testing.T) {
			args := append([]string{"base", "+app-block-get-data"}, tc.args...)
			missing := runBaseDryRun(t, 2, args...)
			assert.Contains(t, missing.Stderr, tc.want)
		})
	}

	unknownPage := runBaseDryRun(t, 2, "base", "+app-block-get-data",
		"--app-token", "app_x", "--base-token", "bas_x", "--block-id", "cht_chart", "--page-id", "pg_x")
	assert.Contains(t, unknownPage.Stderr, "unknown flag")
}
