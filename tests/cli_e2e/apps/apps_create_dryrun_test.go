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

// TestAppsCreateDryRun pins the request shape and Validate behavior for
// `apps +create`. The shortcut is UAT-only and posts to the registered
// /open-apis/spark/v1 namespace; both are checked here.
func TestAppsCreateDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("HappyPath_HTMLAppType", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+create",
				"--name", "Demo",
				"--app-type", "html",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "POST", gjson.Get(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps", gjson.Get(result.Stdout, "api.0.url").String())
		assert.Equal(t, "Demo", gjson.Get(result.Stdout, "api.0.body.name").String())
		assert.Equal(t, "html", gjson.Get(result.Stdout, "api.0.body.app_type").String())
		// Optional fields stay omitted when not provided.
		assert.False(t, gjson.Get(result.Stdout, "api.0.body.description").Exists())
		assert.False(t, gjson.Get(result.Stdout, "api.0.body.icon_url").Exists())
	})

	t.Run("AllFields", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+create",
				"--name", "Demo",
				"--app-type", "html",
				"--description", "survey app",
				"--icon-url", "https://example.com/icon.svg",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "Demo", gjson.Get(result.Stdout, "api.0.body.name").String())
		assert.Equal(t, "html", gjson.Get(result.Stdout, "api.0.body.app_type").String())
		assert.Equal(t, "survey app", gjson.Get(result.Stdout, "api.0.body.description").String())
		assert.Equal(t, "https://example.com/icon.svg", gjson.Get(result.Stdout, "api.0.body.icon_url").String())
	})

	t.Run("RejectsMissingName", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+create",
				"--app-type", "html",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		assert.Contains(t, validateErrorMessage(result), `required flag(s) "name" not set`)
	})

	t.Run("RejectsBlankName", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+create",
				"--name", "  ",
				"--app-type", "html",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		msg := validateErrorMessage(result)
		assert.Contains(t, msg, "--name is required")
	})

	t.Run("RejectsMissingAppType", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+create",
				"--name", "Demo",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		assert.Contains(t, validateErrorMessage(result), `required flag(s) "app-type" not set`)
	})

	t.Run("RejectsInvalidAppType", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+create",
				"--name", "Demo",
				"--app-type", "spa",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		msg := validateErrorMessage(result)
		assert.Contains(t, msg, "invalid value")
		assert.Contains(t, msg, "full_stack")
	})

	t.Run("RejectsLegacyUppercaseAppType", func(t *testing.T) {
		// --app-type is a strict lowercase enum (html / full_stack); the CLI does
		// not normalize case. Legacy uppercase "HTML" is rejected — backend
		// compatibility for legacy values is a server concern the client does not
		// surface.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+create",
				"--name", "Demo",
				"--app-type", "HTML",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		msg := validateErrorMessage(result)
		assert.Contains(t, msg, "invalid value")
		assert.Contains(t, msg, "HTML")
	})
}
