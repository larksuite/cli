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

// TestAppsFileRemotePathDryRun covers the remote --path check on every storage
// command that takes one. Git Bash (MSYS) on Windows rewrites an argument that
// starts with "/" into a local path under its install root before lark-cli runs,
// so "/1858537546760216.png" arrives as "C:/Program Files/Git/1858537546760216.png".
// The rejection happens in Validate, so a dry run is enough to prove it, and the
// plain remote path must still produce a request plan.
func TestAppsFileRemotePathDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	const rewritten = "C:/Program Files/Git/1858537546760216.png"
	const remote = "/1858537546760216.png"
	commands := []struct {
		name string
		args func(path string) []string
		// plan locates the forwarded path in the dry-run output; list marks a
		// field that holds an array of paths.
		plan string
		list bool
	}{
		{"+file-get", func(p string) []string {
			return []string{"apps", "+file-get", "--app-id", "app_x", "--path", p, "--dry-run"}
		}, "data.api.0.params.path", false},
		{"+file-sign", func(p string) []string {
			return []string{"apps", "+file-sign", "--app-id", "app_x", "--path", p, "--dry-run"}
		}, "data.api.0.body.path", false},
		{"+file-download", func(p string) []string {
			return []string{"apps", "+file-download", "--app-id", "app_x", "--path", p, "--output", "out.png", "--dry-run"}
		}, "data.api.0.body.path", false},
		{"+file-delete", func(p string) []string {
			return []string{"apps", "+file-delete", "--app-id", "app_x", "--path", p, "--dry-run"}
		}, "data.api.0.body.paths", true},
		{"+file-list", func(p string) []string {
			return []string{"apps", "+file-list", "--app-id", "app_x", "--path", p, "--dry-run"}
		}, "data.api.0.params.path", false},
	}
	for _, c := range commands {
		t.Run(c.name+"/rewritten", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: c.args(rewritten), DefaultAs: "user"})
			require.NoError(t, err)
			assert.Equal(t, 2, result.ExitCode, "stderr:\n%s", result.Stderr)
			assert.Equal(t, "validation", errEnvelopeField(result, "error.type"), "stderr:\n%s", result.Stderr)
			assert.Equal(t, "invalid_argument", errEnvelopeField(result, "error.subtype"), "stderr:\n%s", result.Stderr)
			assert.Equal(t, "--path", errEnvelopeField(result, "error.param"), "stderr:\n%s", result.Stderr)
			assert.Contains(t, errEnvelopeField(result, "error.hint"), "MSYS_NO_PATHCONV=1", "stderr:\n%s", result.Stderr)
		})
		t.Run(c.name+"/remote", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: c.args(remote), DefaultAs: "user"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			field := gjson.Get(result.Stdout, c.plan)
			got := field.String()
			if c.list {
				require.True(t, field.IsArray(), "%s is not an array; stdout:\n%s", c.plan, result.Stdout)
				items := field.Array()
				require.Len(t, items, 1, "stdout:\n%s", result.Stdout)
				got = items[0].String()
			}
			require.Equal(t, remote, got, "the remote path must reach the request plan verbatim; stdout:\n%s", result.Stdout)
		})
	}
}
