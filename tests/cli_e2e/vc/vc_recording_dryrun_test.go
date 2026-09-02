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

// TestVCRecordingDryRun_BotIdentity pins that `vc +recording --meeting-ids`
// accepts --as bot (GetRecordingByMeetingID accepts a tenant access token).
func TestVCRecordingDryRun_BotIdentity(t *testing.T) {
	setVCDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"vc", "+recording",
			"--meeting-ids", "7628568141510692381",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "GET", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/vc/v1/meetings/{meeting_id}/recording", clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)

	helpResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"vc", "+recording", "--help"},
	})
	require.NoError(t, err)
	helpResult.AssertExitCode(t, 0)
	require.Contains(t, helpResult.Stdout, "identity type: user | bot")
}

// TestVCRecordingDryRun_BotIdentity_CalendarEventIDs pins that the
// calendar-event-ids path also flows under --as bot: a bot has a primary
// calendar, so the primary -> mget_instance_relation_info -> recording chain
// is previewed without a validation error.
func TestVCRecordingDryRun_BotIdentity_CalendarEventIDs(t *testing.T) {
	setVCDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"vc", "+recording",
			"--calendar-event-ids", "evt_001",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Contains(t, out, "mget_instance_relation_info", "stdout:\n%s", out)
	require.Contains(t, out, "recording", "stdout:\n%s", out)
}
