// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

// TestVCDetailDryRun_BotIdentity pins that `vc +detail` accepts --as bot and
// previews the meeting.get + recording API round-trip (GetMeetingByID /
// GetRecordingByMeetingID both accept a tenant access token).
func TestVCDetailDryRun_BotIdentity(t *testing.T) {
	setVCDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"vc", "+detail",
			"--meeting-ids", "7628568141510692381",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Contains(t, result.Args, "--as")
	require.Contains(t, result.Args, "bot")

	out := result.Stdout
	require.Equal(t, int64(2), clie2e.DryRunGet(out, "api.#").Int(), "stdout:\n%s", out)
	require.Equal(t, "GET", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/vc/v1/meetings/{meeting_id}", clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/vc/v1/meetings/{meeting_id}/recording", clie2e.DryRunGet(out, "api.1.url").String(), "stdout:\n%s", out)
	require.Equal(t, "7628568141510692381", clie2e.DryRunGet(out, "meeting_ids.0").String(), "stdout:\n%s", out)

	helpResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"vc", "+detail", "--help"},
	})
	require.NoError(t, err)
	helpResult.AssertExitCode(t, 0)
	require.Contains(t, helpResult.Stdout, "identity type: user | bot")
}
