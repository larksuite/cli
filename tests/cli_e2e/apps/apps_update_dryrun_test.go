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

// TestAppsUpdateDryRun pins partial-update semantics: PATCH with only the
// fields the user supplied; --app-id and at-least-one-field are both required.
func TestAppsUpdateDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("PartialFieldsName", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+update",
				"--app-id", "app_x",
				"--name", "v2",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "PATCH", gjson.Get(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x", gjson.Get(result.Stdout, "api.0.url").String())
		assert.Equal(t, "v2", gjson.Get(result.Stdout, "api.0.body.name").String())
		assert.False(t, gjson.Get(result.Stdout, "api.0.body.description").Exists(),
			"description must be omitted when not provided")
	})

	t.Run("WithDescription", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+update",
				"--app-id", "app_x",
				"--name", "v2",
				"--description", "updated",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "v2", gjson.Get(result.Stdout, "api.0.body.name").String())
		assert.Equal(t, "updated", gjson.Get(result.Stdout, "api.0.body.description").String())
	})

	t.Run("RejectsMissingAppID", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+update",
				"--name", "v2",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 1)
		assert.Contains(t, result.Stdout+result.Stderr, `required flag(s) "app-id" not set`)
	})

	t.Run("RejectsNoFields", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+update",
				"--app-id", "app_x",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		msg := validateErrorMessage(result)
		assert.Contains(t, msg, "at least one")
	})
}
