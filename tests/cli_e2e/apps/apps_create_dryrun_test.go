// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestAppsCreateDryRun pins the request shape and Validate behavior for
// `apps +create`. The shortcut is UAT-only and currently posts to the BOE
// /spark/v1 namespace; both are checked here.
func TestAppsCreateDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("HappyPath_HTMLAppType", func(t *testing.T) {
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
		result.AssertExitCode(t, 0)

		assert.Equal(t, "POST", gjson.Get(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps", gjson.Get(result.Stdout, "api.0.url").String())
		assert.Equal(t, "Demo", gjson.Get(result.Stdout, "api.0.body.name").String())
		assert.Equal(t, "HTML", gjson.Get(result.Stdout, "api.0.body.app_type").String())
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
				"--app-type", "HTML",
				"--description", "survey app",
				"--icon-url", "https://example.com/icon.svg",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "Demo", gjson.Get(result.Stdout, "api.0.body.name").String())
		assert.Equal(t, "HTML", gjson.Get(result.Stdout, "api.0.body.app_type").String())
		assert.Equal(t, "survey app", gjson.Get(result.Stdout, "api.0.body.description").String())
		assert.Equal(t, "https://example.com/icon.svg", gjson.Get(result.Stdout, "api.0.body.icon_url").String())
	})

	t.Run("RejectsMissingName", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+create",
				"--app-type", "HTML",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		// cobra Required flag — exits with non-zero and writes the message to stderr.
		assert.NotEqual(t, 0, result.ExitCode, "expected non-zero exit, got stdout=%s stderr=%s", result.Stdout, result.Stderr)
		assert.Contains(t, result.Stderr, `required flag(s) "name" not set`)
	})

	t.Run("RejectsBlankName", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+create",
				"--name", "  ",
				"--app-type", "HTML",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		assert.NotEqual(t, 0, result.ExitCode)
		msg := gjson.Get(result.Stderr, "error.message").String()
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
		assert.NotEqual(t, 0, result.ExitCode, "stdout=%s stderr=%s", result.Stdout, result.Stderr)
		assert.Contains(t, result.Stderr, `required flag(s) "app-type" not set`)
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
		assert.NotEqual(t, 0, result.ExitCode)
		msg := gjson.Get(result.Stderr, "error.message").String()
		assert.Contains(t, msg, "not supported")
		assert.Contains(t, msg, "HTML")
	})

	t.Run("RejectsLowercaseAppType", func(t *testing.T) {
		// app-type is case-sensitive; lowercase "html" must be rejected even though
		// it differs from the allowed "HTML" by case alone.
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
		assert.NotEqual(t, 0, result.ExitCode)
		msg := gjson.Get(result.Stderr, "error.message").String()
		assert.True(t, strings.Contains(msg, `"html"`) && strings.Contains(msg, "not supported"),
			"expected case-sensitive rejection, got: %s", msg)
	})
}
