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

// TestAppsDeleteDryRun pins the two-step plan: GET the app to check
// --confirm-name, then DELETE the same path; both --app-id and --confirm-name
// are required.
func TestAppsDeleteDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("GetThenDelete", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+delete",
				"--app-id", "app_x",
				"--confirm-name", "demo",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.Equal(t, "DELETE", clie2e.DryRunGet(result.Stdout, "api.1.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x", clie2e.DryRunGet(result.Stdout, "api.1.url").String())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.2").Exists(), "dry-run must plan exactly two requests")
	})

	t.Run("MetaTokenMarksDeleteTargetAsResolved", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+delete",
				"--app-id", "meta_token_x",
				"--confirm-name", "demo",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/meta_token_x", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.Equal(t, "DELETE", clie2e.DryRunGet(result.Stdout, "api.1.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/<resolved_app_id>", clie2e.DryRunGet(result.Stdout, "api.1.url").String())
		assert.Contains(t, clie2e.DryRunGet(result.Stdout, "api.1.desc").String(), "app_id returned by step 1")
	})

	t.Run("RejectsMissingAppID", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+delete",
				"--confirm-name", "demo",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		assert.Contains(t, validateErrorMessage(result), `required flag(s) "app-id" not set`)
	})

	t.Run("RejectsMissingConfirmName", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+delete",
				"--app-id", "app_x",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		assert.Contains(t, validateErrorMessage(result), `required flag(s) "confirm-name" not set`)
	})

	t.Run("RejectsBlankConfirmName", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+delete",
				"--app-id", "app_x",
				"--confirm-name", "  ",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		assert.Contains(t, validateErrorMessage(result), "--confirm-name is required")
	})
}
