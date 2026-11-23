// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

// TestIM_DownloadResourcesDryRun verifies the --download-resources flag is wired
// into +chat-messages-list without breaking the existing request structure: the
// underlying GET /open-apis/im/v1/messages call is identical with or without the
// flag (AC4), and the flag only adds a resource-download declaration to dry-run.
func TestIM_DownloadResourcesDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	run := func(t *testing.T, extraArgs ...string) string {
		t.Helper()
		args := append([]string{
			"im", "+chat-messages-list",
			"--chat-id", "oc_dryrun",
			"--no-reactions",
		}, extraArgs...)
		args = append(args, "--dry-run")
		result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		return result.Stdout
	}

	t.Run("default off: no resources declaration, request unchanged", func(t *testing.T) {
		out := run(t)
		require.Equal(t, "GET", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
		require.Equal(t, "/open-apis/im/v1/messages", clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
		require.Equal(t, "oc_dryrun", clie2e.DryRunGet(out, "api.0.params.container_id").String(), "stdout:\n%s", out)
		require.NotContains(t, strings.ToLower(out), "lark-im-resources", "default must not declare resource download:\n%s", out)
	})

	t.Run("with --download-resources: request unchanged, declares download", func(t *testing.T) {
		out := run(t, "--download-resources")
		require.Equal(t, "GET", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
		require.Equal(t, "/open-apis/im/v1/messages", clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
		require.Equal(t, "oc_dryrun", clie2e.DryRunGet(out, "api.0.params.container_id").String(), "stdout:\n%s", out)
		require.Contains(t, strings.ToLower(out), "lark-im-resources", "flag must declare resource download:\n%s", out)
	})
}
