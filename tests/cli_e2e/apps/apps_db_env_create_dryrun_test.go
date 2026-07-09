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

// TestAppsDBEnvCreateDryRun pins +db-env-create URL `/apps/{app_id}/db_dev_init` 和 sync_data body 透传。
// Risk: high-risk-write 在 dry-run 下不需要 --yes 确认。
func TestAppsDBEnvCreateDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("DefaultSyncDataFalse", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+db-env-create", "--app-id", "app_x", "--environment", "dev", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "POST", gjson.Get(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x/db_dev_init", gjson.Get(result.Stdout, "api.0.url").String())
		assert.Equal(t, "false", gjson.Get(result.Stdout, "api.0.body.sync_data").String())
	})

	t.Run("SyncDataTrue", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+db-env-create", "--app-id", "app_x", "--environment", "dev", "--sync-data", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, "true", gjson.Get(result.Stdout, "api.0.body.sync_data").String())
	})
}
