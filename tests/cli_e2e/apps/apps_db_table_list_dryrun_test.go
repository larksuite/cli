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
	"github.com/tidwall/gjson"
)

// TestAppsDBTableListDryRun pins +db-table-list 复用存量 URL（/apps/{app_id}/tables，
// 不带 /db/），cursor 分页参数与 env 透传，且不发 include_stats query。
func TestAppsDBTableListDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("DefaultsToNoEnvAndPageSize20", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+db-table-list", "--app-id", "app_x", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "GET", gjson.Get(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x/tables", gjson.Get(result.Stdout, "api.0.url").String())
		assert.False(t, gjson.Get(result.Stdout, "api.0.params.env").Exists(),
			"default: no --environment → env key must be omitted (server picks workspace default branch)")
		assert.Equal(t, "20", gjson.Get(result.Stdout, "api.0.params.page_size").String())
		assert.False(t, gjson.Get(result.Stdout, "api.0.params.page_token").Exists(),
			"empty page_token must be omitted")
		assert.False(t, gjson.Get(result.Stdout, "api.0.params.include_stats").Exists(),
			"CLI should not send include_stats query (server returns stats by default)")
	})

	t.Run("CustomPaginationAndDevEnv", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"apps", "+db-table-list",
				"--app-id", "app_x", "--environment", "dev",
				"--page-size", "50", "--page-token", "cursor-abc",
				"--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "dev", gjson.Get(result.Stdout, "api.0.params.env").String())
		assert.Equal(t, "50", gjson.Get(result.Stdout, "api.0.params.page_size").String())
		assert.Equal(t, "cursor-abc", gjson.Get(result.Stdout, "api.0.params.page_token").String())
	})

	t.Run("RejectsBlankAppID", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+db-table-list", "--app-id", "   ", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		assert.NotEqual(t, 0, result.ExitCode, "blank app-id must fail validation")
		assert.Contains(t, validateErrorMessage(result), "app-id")
	})
}
