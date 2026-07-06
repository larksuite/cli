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

// TestAppsDBExecuteDryRun pins +db-execute 复用存量 URL，CLI 永远走 DBA 模式
// （?transactional=false），sql body 由 --sql 透传，默认不传 env（空值，由服务端按 workspace 定分支）。
func TestAppsDBExecuteDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("DefaultEnvUnsetAndTransactionalFalse", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+db-execute", "--app-id", "app_x", "--sql", "SELECT 1", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "POST", gjson.Get(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x/sql_commands", gjson.Get(result.Stdout, "api.0.url").String())
		assert.Equal(t, "SELECT 1", gjson.Get(result.Stdout, "api.0.body.sql").String())
		assert.Equal(t, "false", gjson.Get(result.Stdout, "api.0.params.transactional").String(),
			"CLI is DBA mode → must send transactional=false in query")
		assert.False(t, gjson.Get(result.Stdout, "api.0.body.transactional").Exists(),
			"transactional should be in query, not body")
		assert.False(t, gjson.Get(result.Stdout, "api.0.params.env").Exists(),
			"default: no --environment → env key must be omitted (server picks workspace default branch)")
	})

	t.Run("OnlineEnvSwitch", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+db-execute", "--app-id", "app_x", "--sql", "SELECT 1", "--environment", "online", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, "online", gjson.Get(result.Stdout, "api.0.params.env").String())
	})

	t.Run("RejectsEmptySQL", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+db-execute", "--app-id", "app_x", "--sql", "   ", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		assert.NotEqual(t, 0, result.ExitCode, "empty --sql must fail validation")
	})
}
