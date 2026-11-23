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

// TestAppsFeaturesDryRun pins the read-only request shape of `apps +features`.
func TestAppsFeaturesDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("Default", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+features", "--app-id", "app_x", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x/features", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body").Exists())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.1").Exists())
	})

	t.Run("OperationRef", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+features", "--app-id", "app_x", "--operation-ref", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", clie2e.DryRunGet(result.Stdout, "api.0.params.operation_ref").String())
	})

	t.Run("RejectsPositionalArgument", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+features", "database", "--app-id", "app_x", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		// The shared positional rejection decides the exit code; pin only the
		// refusal and that no plan is printed.
		assert.NotEqual(t, 0, result.ExitCode)
		assert.Contains(t, result.Stderr, "positional arguments are not supported")
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0").Exists())
	})
}

// TestAppsFeatureEnableDryRun pins the plan of `apps +feature-enable`: a new
// request is a single POST whose body carries only the idempotent key (a
// placeholder in dry-run); an explicit --idempotent-key first reads the
// operation kept for that key, then posts the same key.
func TestAppsFeatureEnableDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("Plan", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+feature-enable", "--feature", "database", "--app-id", "app_x", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x/features/database/enable", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.Contains(t, clie2e.DryRunGet(result.Stdout, "api.0.body.idempotent_key").String(), "execution time")
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body.feature_protocol").Exists())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body.expected_config_revision").Exists())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.1").Exists())
	})

	t.Run("ExplicitIdempotentKey", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+feature-enable", "--feature", "database", "--app-id", "app_x", "--idempotent-key", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x/features", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.Equal(t, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", clie2e.DryRunGet(result.Stdout, "api.0.params.operation_ref").String())
		assert.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.1.method").String())
		assert.Equal(t, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", clie2e.DryRunGet(result.Stdout, "api.1.body.idempotent_key").String())
	})

	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"MissingFeature":     {[]string{"apps", "+feature-enable", "--app-id", "app_x", "--dry-run"}, `required flag(s) "feature" not set`},
		"UnsupportedFeature": {[]string{"apps", "+feature-enable", "--feature", "redis", "--app-id", "app_x", "--dry-run"}, "invalid value"},
		"EmptyFeature":       {[]string{"apps", "+feature-enable", "--feature", "", "--app-id", "app_x", "--dry-run"}, "--feature is required"},
		"MalformedKey":       {[]string{"apps", "+feature-enable", "--feature", "database", "--app-id", "app_x", "--idempotent-key", "retry-1", "--dry-run"}, "--idempotent-key"},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: tc.args, DefaultAs: "user"})
			require.NoError(t, err)
			result.AssertExitCode(t, 2)
			assert.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
			assert.Contains(t, gjson.Get(result.Stderr, "error.message").String(), tc.want)
			assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0").Exists())
		})
	}

	// Positional words, such as `+feature-enable database`,
	// go through the framework's existing positional rejection like every
	// other shortcut; this pins only that they are refused with that message
	// and no plan, not the exit code the shared root handler assigns.
	for name, args := range map[string][]string{
		"ExtraArgument":  {"apps", "+feature-enable", "--feature", "database", "redis", "--app-id", "app_x", "--dry-run"},
		"PositionalForm": {"apps", "+feature-enable", "database", "--app-id", "app_x", "--dry-run"},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "user"})
			require.NoError(t, err)
			assert.NotEqual(t, 0, result.ExitCode)
			assert.Contains(t, result.Stderr, "positional arguments are not supported")
			assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0").Exists())
		})
	}
}
