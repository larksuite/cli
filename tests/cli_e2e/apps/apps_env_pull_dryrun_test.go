// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppsEnvPullDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("DefaultPath", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+env-pull",
				"--app-id", "app_x",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x/env_vars", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.Equal(t, "dev", clie2e.DryRunGet(result.Stdout, "api.0.body.env").String())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body.include_values").Exists())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.params").Exists())
		assert.True(t, clie2e.DryRunGet(result.Stdout, "project_path").Exists())
		assert.Contains(t, clie2e.DryRunGet(result.Stdout, "env_file").String(), ".env.local")
		assert.False(t, clie2e.DryRunGet(result.Stdout, "env_keys").Exists())
	})

	t.Run("CustomProjectPath", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)
		projectDir := filepath.Join(t.TempDir(), "demo")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+env-pull",
				"--app-id", "app_x",
				"--project-path", projectDir,
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, projectDir, clie2e.DryRunGet(result.Stdout, "project_path").String())
		assert.Equal(t, filepath.Join(projectDir, ".env.local"), clie2e.DryRunGet(result.Stdout, "env_file").String())
	})

	t.Run("MissingAppID", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+env-pull",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		assert.Contains(t, result.Stdout+result.Stderr, `--app-id is required`)
	})
}
