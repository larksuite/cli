// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestIM_MessagesResourcesDownloadDryRun pins the request `im
// +messages-resources-download` would send. The existing
// im_download_resources_dryrun_test.go covers `+chat-messages-list
// --download-resources`, which is a different command, so this shortcut had no
// dry-run coverage of its own.
//
// It also pins the output-path validation, which decides where bytes land and is
// the one part of this command that rejects input before any request goes out.
func TestIM_MessagesResourcesDownloadDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	run := func(t *testing.T, args ...string) *clie2e.Result {
		t.Helper()
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      append([]string{"im", "+messages-resources-download"}, append(args, "--dry-run")...),
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		return result
	}

	t.Run("file resource request shape", func(t *testing.T) {
		result := run(t,
			"--message-id", "om_dryrun",
			"--file-key", "file_dryrun",
			"--type", "file",
			"--output", "./out.bin",
		)
		result.AssertExitCode(t, 0)
		out := result.Stdout

		require.Equal(t, "GET", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
		require.Equal(t, "/open-apis/im/v1/messages/om_dryrun/resources/file_dryrun",
			clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
		require.Equal(t, "file", clie2e.DryRunGet(out, "api.0.params.type").String(), "stdout:\n%s", out)
		require.Equal(t, "om_dryrun", clie2e.DryRunGet(out, "message_id").String(), "stdout:\n%s", out)
		require.Equal(t, "file_dryrun", clie2e.DryRunGet(out, "file_key").String(), "stdout:\n%s", out)
		require.Equal(t, "./out.bin", clie2e.DryRunGet(out, "output").String(), "stdout:\n%s", out)
	})

	t.Run("image resource keeps the same shape with type=image", func(t *testing.T) {
		result := run(t,
			"--message-id", "om_dryrun",
			"--file-key", "img_dryrun",
			"--type", "image",
		)
		result.AssertExitCode(t, 0)
		out := result.Stdout

		require.Equal(t, "/open-apis/im/v1/messages/om_dryrun/resources/img_dryrun",
			clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
		require.Equal(t, "image", clie2e.DryRunGet(out, "api.0.params.type").String(), "stdout:\n%s", out)
		// Without --output the file key is where the bytes would land.
		require.Equal(t, "img_dryrun", clie2e.DryRunGet(out, "output").String(), "stdout:\n%s", out)
	})

	// An absolute path inside an allowed root is the case agents kept failing
	// on: the command used to refuse the shape before the built-in path policy
	// could judge where it pointed.
	t.Run("accepts an absolute output path inside an allowed root", func(t *testing.T) {
		result := run(t,
			"--message-id", "om_dryrun",
			"--file-key", "file_dryrun",
			"--type", "file",
			"--output", "/tmp/out.bin",
		)
		result.AssertExitCode(t, 0)
		require.Equal(t, "/tmp/out.bin", clie2e.DryRunGet(result.Stdout, "output").String(),
			"stdout:\n%s", result.Stdout)
	})

	t.Run("rejects an output path the policy refuses", func(t *testing.T) {
		for _, tt := range []struct {
			name       string
			outputPath string
		}{
			// Neither resolves inside an allowed root from this working
			// directory, so the policy — not a shape check — turns them down.
			{name: "parent escape", outputPath: "../out.bin"},
			{name: "denylisted directory", outputPath: "/etc/out.bin"},
		} {
			t.Run(tt.name, func(t *testing.T) {
				result := run(t,
					"--message-id", "om_dryrun",
					"--file-key", "file_dryrun",
					"--type", "file",
					"--output", tt.outputPath,
				)
				require.Equal(t, 2, result.ExitCode, "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
				require.Empty(t, result.Stdout, "stdout must stay reserved for program data")
				require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(),
					"stderr:\n%s", result.Stderr)
				require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(),
					"stderr:\n%s", result.Stderr)
				require.Equal(t, "--output", gjson.Get(result.Stderr, "error.param").String(),
					"stderr:\n%s", result.Stderr)
				require.NotEmpty(t, gjson.Get(result.Stderr, "error.message").String(),
					"stderr:\n%s", result.Stderr)
			})
		}
	})
}
