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

// TestAppsAccessScopeSetDryRun pins the user-facing scope-string -> server-enum
// mapping (public->All, tenant->Tenant, specific->Range) and the three-way
// mutex between specific / public / tenant.
func TestAppsAccessScopeSetDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("SpecificMapsToRange", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+access-scope-set",
				"--app-id", "app_x",
				"--scope", "specific",
				"--targets", `[{"type":"user","id":"ou_x"},{"type":"chat","id":"oc_x"}]`,
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "PUT", gjson.Get(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x/access-scope", gjson.Get(result.Stdout, "api.0.url").String())
		assert.Equal(t, "Range", gjson.Get(result.Stdout, "api.0.body.scope").String())
		assert.Equal(t, "ou_x", gjson.Get(result.Stdout, "api.0.body.users.0").String())
		assert.Equal(t, "oc_x", gjson.Get(result.Stdout, "api.0.body.chats.0").String())
		assert.False(t, gjson.Get(result.Stdout, "api.0.body.departments").Exists(),
			"empty department list must be omitted")
		assert.False(t, gjson.Get(result.Stdout, "api.0.body.apply_config").Exists())
	})

	t.Run("SpecificWithApplyConfig", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+access-scope-set",
				"--app-id", "app_x",
				"--scope", "specific",
				"--targets", `[{"type":"user","id":"ou_x"}]`,
				"--apply-enabled",
				"--approver", "ou_y",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.True(t, gjson.Get(result.Stdout, "api.0.body.apply_config.enabled").Bool())
		assert.Equal(t, "ou_y", gjson.Get(result.Stdout, "api.0.body.apply_config.approvers.0").String())
	})

	t.Run("PublicMapsToAll", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+access-scope-set",
				"--app-id", "app_x",
				"--scope", "public",
				"--require-login=false",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "All", gjson.Get(result.Stdout, "api.0.body.scope").String())
		assert.False(t, gjson.Get(result.Stdout, "api.0.body.require_login").Bool())
		assert.False(t, gjson.Get(result.Stdout, "api.0.body.users").Exists())
		assert.False(t, gjson.Get(result.Stdout, "api.0.body.apply_config").Exists())
	})

	t.Run("TenantMapsToTenant", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+access-scope-set",
				"--app-id", "app_x",
				"--scope", "tenant",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "Tenant", gjson.Get(result.Stdout, "api.0.body.scope").String())
		// scope is the only body field in tenant mode.
		assert.False(t, gjson.Get(result.Stdout, "api.0.body.require_login").Exists())
		assert.False(t, gjson.Get(result.Stdout, "api.0.body.users").Exists())
	})

	t.Run("RejectsSpecificMissingTargets", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+access-scope-set",
				"--app-id", "app_x",
				"--scope", "specific",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		assert.NotEqual(t, 0, result.ExitCode)
		assert.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "--targets is required")
	})

	t.Run("RejectsTenantWithExtraFlags", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+access-scope-set",
				"--app-id", "app_x",
				"--scope", "tenant",
				"--targets", `[]`,
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		assert.NotEqual(t, 0, result.ExitCode)
		assert.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "no extra flags allowed")
	})

	t.Run("RejectsBadTargetType", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+access-scope-set",
				"--app-id", "app_x",
				"--scope", "specific",
				"--targets", `[{"type":"group","id":"oc_x"}]`,
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		assert.NotEqual(t, 0, result.ExitCode)
		assert.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "must be one of")
	})

	t.Run("RejectsApproverWithoutApplyEnabled", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+access-scope-set",
				"--app-id", "app_x",
				"--scope", "specific",
				"--targets", `[{"type":"user","id":"ou_x"}]`,
				"--approver", "ou_y",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		assert.NotEqual(t, 0, result.ExitCode)
		assert.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "--apply-enabled")
	})
}
