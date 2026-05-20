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

// TestAppsAccessScopeGetDryRun pins URL shape and --app-id requirement for the
// read-side companion of +access-scope-set. Response passthrough (scope enum,
// split user/department/chat arrays) is covered by unit tests in shortcuts/apps.
func TestAppsAccessScopeGetDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("HappyPath", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+access-scope-get",
				"--app-id", "app_x",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "GET", gjson.Get(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x/access-scope", gjson.Get(result.Stdout, "api.0.url").String())
		// GET request: no body and no query params.
		assert.False(t, gjson.Get(result.Stdout, "api.0.body").Exists())
		assert.False(t, gjson.Get(result.Stdout, "api.0.params").Exists())
	})

	t.Run("RejectsMissingAppID", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+access-scope-get", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		assert.NotEqual(t, 0, result.ExitCode)
		assert.Contains(t, result.Stderr, `required flag(s) "app-id" not set`)
	})
}
