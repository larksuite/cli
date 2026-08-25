// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type vcMeetingEndLiveFixture struct {
	appID             string
	brand             string
	hostToken         string
	cohostToken       string
	normalToken       string
	hostMeetingID     string
	cohostMeetingID   string
	normalMeetingID   string
	isolatedConfigDir string
}

// TestVCMeetingEndDryRunE2E proves the built binary exposes the dual-identity
// high-risk command, emits each identity's exact request, and stops at
// confirmation when --yes is absent.
func TestVCMeetingEndDryRunE2E(t *testing.T) {
	setVCDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	helpResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"vc", "+meeting-end", "--help"},
	})
	require.NoError(t, err)
	helpResult.AssertExitCode(t, 0)
	require.Contains(t, helpResult.Stdout, "identity type: user | bot")

	dryRun, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"vc", "+meeting-end",
			"--meeting-id", " 123456789 ",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	dryRun.AssertExitCode(t, 0)
	require.True(t, gjson.Get(dryRun.Stdout, "dry_run").Bool(), "stdout:\n%s", dryRun.Stdout)
	require.Equal(t, "user", gjson.Get(dryRun.Stdout, "identity").String(), "stdout:\n%s", dryRun.Stdout)
	require.Equal(t, int64(1), clie2e.DryRunGet(dryRun.Stdout, "api.#").Int(), "stdout:\n%s", dryRun.Stdout)
	require.Equal(t, "PATCH", clie2e.DryRunGet(dryRun.Stdout, "api.0.method").String(), "stdout:\n%s", dryRun.Stdout)
	require.Equal(t, "/open-apis/vc/v1/meetings/123456789/end", clie2e.DryRunGet(dryRun.Stdout, "api.0.url").String(), "stdout:\n%s", dryRun.Stdout)
	require.False(t, clie2e.DryRunGet(dryRun.Stdout, "api.0.body").Exists(), "stdout:\n%s", dryRun.Stdout)

	withoutConfirmation, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"vc", "+meeting-end", "--meeting-id", "123456789"},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	withoutConfirmation.AssertExitCode(t, 10)
	require.Empty(t, withoutConfirmation.Stdout)
	require.Equal(t, "confirmation", gjson.Get(withoutConfirmation.Stderr, "error.type").String(), "stderr:\n%s", withoutConfirmation.Stderr)
	require.Equal(t, "confirmation_required", gjson.Get(withoutConfirmation.Stderr, "error.subtype").String(), "stderr:\n%s", withoutConfirmation.Stderr)
	require.Contains(t, gjson.Get(withoutConfirmation.Stderr, "error.hint").String(), "--yes")

	invalidMeetingID, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"vc", "+meeting-end",
			"--meeting-id", "0",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	invalidMeetingID.AssertExitCode(t, 2)
	require.Empty(t, invalidMeetingID.Stdout)
	require.Equal(t, "validation", gjson.Get(invalidMeetingID.Stderr, "error.type").String(), "stderr:\n%s", invalidMeetingID.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(invalidMeetingID.Stderr, "error.subtype").String(), "stderr:\n%s", invalidMeetingID.Stderr)
	require.Equal(t, "--meeting-id must be a positive base-10 int64", gjson.Get(invalidMeetingID.Stderr, "error.message").String(), "stderr:\n%s", invalidMeetingID.Stderr)
	require.Equal(t, "--meeting-id", gjson.Get(invalidMeetingID.Stderr, "error.param").String(), "stderr:\n%s", invalidMeetingID.Stderr)

	botDryRun, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"vc", "+meeting-end",
			"--meeting-id", "7628568141510692381",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	botDryRun.AssertExitCode(t, 0)
	require.Equal(t, "bot", gjson.Get(botDryRun.Stdout, "identity").String(), "stdout:\n%s", botDryRun.Stdout)
	require.Equal(t, "POST", clie2e.DryRunGet(botDryRun.Stdout, "api.0.method").String(), "stdout:\n%s", botDryRun.Stdout)
	require.Equal(t, "/open-apis/vc/v1/bots/end", clie2e.DryRunGet(botDryRun.Stdout, "api.0.url").String(), "stdout:\n%s", botDryRun.Stdout)
	require.Equal(t, "7628568141510692381", clie2e.DryRunGet(botDryRun.Stdout, "api.0.body.meeting_id").String(), "stdout:\n%s", botDryRun.Stdout)

	for _, args := range [][]string{
		{"vc", "+meeting-end", "--meeting-id", "123456789", "--dry-run"},
		{"vc", "+meeting-end", "--meeting-id", "123456789", "--dry-run", "--as", "auto"},
	} {
		result, runErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: args,
			Env: map[string]string{
				"LARKSUITE_CLI_APP_ID":     "",
				"LARKSUITE_CLI_APP_SECRET": "",
				"LARKSUITE_CLI_BRAND":      "",
			},
		})
		require.NoError(t, runErr)
		result.AssertExitCode(t, 2)
		require.Empty(t, result.Stdout)
		require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), "stderr:\n%s", result.Stderr)
		require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), "stderr:\n%s", result.Stderr)
		require.Equal(t, "--dry-run for +meeting-end requires explicit --as user or --as bot because offline preflight cannot resolve default or automatic identity", gjson.Get(result.Stderr, "error.message").String(), "stderr:\n%s", result.Stderr)
		require.Equal(t, "--as", gjson.Get(result.Stderr, "error.param").String(), "stderr:\n%s", result.Stderr)
	}
}

// TestVCMeetingEndLiveE2E is destructive and default-off. The dedicated remote
// lane must explicitly opt in; after that, missing or invalid provisioner data
// is fatal. This test proves the command response and pre-mutation fixture
// roles, but does not claim a post-end final-state readback.
func TestVCMeetingEndLiveE2E(t *testing.T) {
	requireVCMeetingManagementDestructiveOptIn(t)
	fixture := requireVCMeetingEndProvisionFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	for _, tc := range []struct {
		name      string
		token     string
		meetingID string
	}{
		{name: "normal user denied", token: fixture.normalToken, meetingID: fixture.normalMeetingID},
		{name: "cohost denied", token: fixture.cohostToken, meetingID: fixture.cohostMeetingID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := runMeetingEndLive(ctx, fixture, tc.token, tc.meetingID)
			require.NoError(t, err)
			assertVCMeetingManagementDenied(t, result)
		})
	}

	t.Run("host succeeds", func(t *testing.T) {
		result, err := runMeetingEndLive(ctx, fixture, fixture.hostToken, fixture.hostMeetingID)
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, "user", gjson.Get(result.Stdout, "identity").String(), "stdout:\n%s", result.Stdout)
		require.True(t, gjson.Get(result.Stdout, "data").IsObject(), "stdout:\n%s", result.Stdout)
	})
}

func runMeetingEndLive(ctx context.Context, fixture vcMeetingEndLiveFixture, token, meetingID string) (*clie2e.Result, error) {
	return clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"vc", "+meeting-end", "--meeting-id", meetingID},
		DefaultAs: "user",
		Yes:       true,
		Env:       vcMeetingManagementUserEnv(fixture.appID, fixture.brand, fixture.isolatedConfigDir, token),
	}, vcMeetingManagementNoRetry())
}
