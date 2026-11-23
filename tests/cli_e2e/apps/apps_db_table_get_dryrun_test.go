// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAppsDBTableGetDryRun pins +db-table-get 复用存量 URL。
// 没有独立 --ddl flag —— 由 --format 同时驱动 CLI 渲染和 server 请求形态：
//
//	--format pretty → CLI 给 server 带 ?format=ddl
//	--format json / table / ndjson / csv（含默认）→ CLI 不传 format query
func TestAppsDBTableGetDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("DefaultFormatJSONOmitsFormatQuery", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+db-table-get", "--app-id", "app_x", "--table", "orders", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x/tables/orders", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.params.format").Exists(),
			"default (json) should omit format query")
	})

	t.Run("PrettyFormatSendsFormatDDL", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+db-table-get", "--app-id", "app_x", "--table", "orders", "--format", "pretty", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		// pretty 模式 dry-run 输出是 plain text 列表（非 JSON envelope），用 substring 校验 query。
		assert.Contains(t, result.Stdout, "format=ddl",
			"--format pretty must trigger ?format=ddl")
	})

	t.Run("TableFormatOmitsFormatQuery", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+db-table-get", "--app-id", "app_x", "--table", "orders", "--format", "table", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.params.format").Exists())
	})

	t.Run("RequiresTableFlag", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+db-table-get", "--app-id", "app_x", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		assert.NotEqual(t, 0, result.ExitCode, "missing --table must fail")
	})
}
