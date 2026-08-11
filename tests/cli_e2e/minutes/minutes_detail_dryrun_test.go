// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

// TestMinutesDetailDryRun_BotIdentity pins that `minutes +detail` accepts
// --as bot for both the metadata (GetMinuteArtifacts) and transcript
// (GetMinuteTranscript) paths, which accept a tenant access token.
func TestMinutesDetailDryRun_BotIdentity(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	setDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"minutes", "+detail",
			"--minute-tokens", "obcnexampleminute",
			"--transcript",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Contains(t, result.Args, "--as")
	require.Contains(t, result.Args, "bot")

	out := result.Stdout
	require.Equal(t, "GET", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/minutes/v1/minutes/{minute_token}", clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/minutes/v1/minutes/{minute_token}/artifacts", clie2e.DryRunGet(out, "api.1.url").String(), "stdout:\n%s", out)

	helpResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"minutes", "+detail", "--help"},
	})
	require.NoError(t, err)
	helpResult.AssertExitCode(t, 0)
	require.Contains(t, helpResult.Stdout, "identity type: user | bot")
}
