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

type vcKickoutParticipantFixture struct {
	tuple    string
	id       string
	userType int64
}

type vcMeetingParticipantKickoutLiveFixture struct {
	appID             string
	brand             string
	hostToken         string
	cohostToken       string
	normalToken       string
	hostMeetingID     string
	hostTarget        vcKickoutParticipantFixture
	cohostMeetingID   string
	cohostTarget      vcKickoutParticipantFixture
	normalMeetingID   string
	normalTarget      vcKickoutParticipantFixture
	partialMeetingID  string
	partialSuccess    vcKickoutParticipantFixture
	partialFailure    vcKickoutParticipantFixture
	isolatedConfigDir string
}

// TestVCMeetingParticipantKickoutDryRunE2E proves repeated tuple flags reach
// the built command in their original order and that dry-run bypasses the
// high-risk confirmation without making a real request.
func TestVCMeetingParticipantKickoutDryRunE2E(t *testing.T) {
	setVCDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	helpResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"vc", "+meeting-participant-kickout", "--help"},
	})
	require.NoError(t, err)
	helpResult.AssertExitCode(t, 0)
	require.Contains(t, helpResult.Stdout, "identity type: user")
	require.NotContains(t, helpResult.Stdout, "identity type: user | bot")

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"vc", "+meeting-participant-kickout",
			"--meeting-id", "7651377260537433044",
			"--participant", "000123=1",
			"--participant", "000123=2",
			"--participant", "000123=1",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.True(t, gjson.Get(result.Stdout, "dry_run").Bool(), "stdout:\n%s", result.Stdout)
	require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String(), "stdout:\n%s", result.Stdout)
	require.Equal(t, "/open-apis/vc/v1/meetings/7651377260537433044/kickout", clie2e.DryRunGet(result.Stdout, "api.0.url").String(), "stdout:\n%s", result.Stdout)

	users := clie2e.DryRunGet(result.Stdout, "api.0.body.kickout_users").Array()
	require.Len(t, users, 3, "stdout:\n%s", result.Stdout)
	wantIDs := []string{"000123", "000123", "000123"}
	wantTypes := []int64{1, 2, 1}
	for index := range users {
		require.Equal(t, wantIDs[index], users[index].Get("id").String(), "stdout:\n%s", result.Stdout)
		require.Equal(t, wantTypes[index], users[index].Get("user_type").Int(), "stdout:\n%s", result.Stdout)
	}

	withoutConfirmation, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"vc", "+meeting-participant-kickout",
			"--meeting-id", "7651377260537433044",
			"--participant", "000123=1",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	withoutConfirmation.AssertExitCode(t, 10)
	require.Empty(t, withoutConfirmation.Stdout)
	require.Equal(t, "confirmation", gjson.Get(withoutConfirmation.Stderr, "error.type").String(), "stderr:\n%s", withoutConfirmation.Stderr)
	require.Equal(t, "confirmation_required", gjson.Get(withoutConfirmation.Stderr, "error.subtype").String(), "stderr:\n%s", withoutConfirmation.Stderr)
	require.Contains(t, gjson.Get(withoutConfirmation.Stderr, "error.hint").String(), "--yes")

	invalidParticipant, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"vc", "+meeting-participant-kickout",
			"--meeting-id", "7651377260537433044",
			"--participant", "0=1",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	invalidParticipant.AssertExitCode(t, 2)
	require.Empty(t, invalidParticipant.Stdout)
	require.Equal(t, "validation", gjson.Get(invalidParticipant.Stderr, "error.type").String(), "stderr:\n%s", invalidParticipant.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(invalidParticipant.Stderr, "error.subtype").String(), "stderr:\n%s", invalidParticipant.Stderr)
	require.Equal(t, "--participant", gjson.Get(invalidParticipant.Stderr, "error.param").String(), "stderr:\n%s", invalidParticipant.Stderr)

	botIdentity, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"vc", "+meeting-participant-kickout",
			"--meeting-id", "7651377260537433044",
			"--participant", "000123=1",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	botIdentity.AssertExitCode(t, 2)
	require.Empty(t, botIdentity.Stdout)
	require.Equal(t, "validation", gjson.Get(botIdentity.Stderr, "error.type").String(), "stderr:\n%s", botIdentity.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(botIdentity.Stderr, "error.subtype").String(), "stderr:\n%s", botIdentity.Stderr)
	require.Equal(t, "--as", gjson.Get(botIdentity.Stderr, "error.param").String(), "stderr:\n%s", botIdentity.Stderr)
}

// TestVCMeetingParticipantKickoutLiveE2E is destructive and default-off. The
// dedicated remote lane must explicitly opt in; after that, missing or invalid
// provisioner data is fatal. It proves tuple-correlated command results against
// pre-mutation fixture readback, not participant removal at the final sink.
func TestVCMeetingParticipantKickoutLiveE2E(t *testing.T) {
	requireVCMeetingManagementDestructiveOptIn(t)
	fixture := requireVCKickoutProvisionFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	t.Run("host kicks ordinary participant", func(t *testing.T) {
		result, err := runMeetingParticipantKickoutLive(ctx, fixture, fixture.hostToken, fixture.hostMeetingID, fixture.hostTarget)
		require.NoError(t, err)
		assertKickoutLiveResults(t, result, []vcExpectedKickoutResult{{participant: fixture.hostTarget, result: 1}})
	})

	t.Run("cohost kicks ordinary participant", func(t *testing.T) {
		result, err := runMeetingParticipantKickoutLive(ctx, fixture, fixture.cohostToken, fixture.cohostMeetingID, fixture.cohostTarget)
		require.NoError(t, err)
		assertKickoutLiveResults(t, result, []vcExpectedKickoutResult{{participant: fixture.cohostTarget, result: 1}})
	})

	t.Run("normal user is denied", func(t *testing.T) {
		result, err := runMeetingParticipantKickoutLive(ctx, fixture, fixture.normalToken, fixture.normalMeetingID, fixture.normalTarget)
		require.NoError(t, err)
		assertVCMeetingManagementDenied(t, result)
	})

	t.Run("host receives tuple-correlated partial results", func(t *testing.T) {
		result, err := runMeetingParticipantKickoutLive(
			ctx,
			fixture,
			fixture.hostToken,
			fixture.partialMeetingID,
			fixture.partialSuccess,
			fixture.partialFailure,
		)
		require.NoError(t, err)
		assertKickoutLiveResults(t, result, []vcExpectedKickoutResult{
			{participant: fixture.partialSuccess, result: 1},
			{participant: fixture.partialFailure, result: 2},
		})
	})
}

type vcExpectedKickoutResult struct {
	participant vcKickoutParticipantFixture
	result      int64
}

type vcKickoutParticipantResultKey struct {
	id       string
	userType int64
}

func assertKickoutLiveResults(t *testing.T, result *clie2e.Result, want []vcExpectedKickoutResult) {
	t.Helper()
	require.NotNil(t, result)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	require.Equal(t, "user", gjson.Get(result.Stdout, "identity").String(), "stdout:\n%s", result.Stdout)

	got := gjson.Get(result.Stdout, "data.data.kickout_results").Array()
	require.Len(t, got, len(want), "stdout:\n%s", result.Stdout)

	remainingByParticipant := make(map[vcKickoutParticipantResultKey]map[int64]int, len(want))
	for _, expected := range want {
		key := vcKickoutParticipantResultKey{id: expected.participant.id, userType: expected.participant.userType}
		if remainingByParticipant[key] == nil {
			remainingByParticipant[key] = make(map[int64]int)
		}
		remainingByParticipant[key][expected.result]++
	}

	for index, item := range got {
		id := item.Get("id")
		userType := item.Get("user_type")
		resultCode := item.Get("result")
		require.True(t, id.Exists() && id.String() != "" && userType.Exists() && resultCode.Exists(), "participant result %d is missing id, user_type, or result; stdout:\n%s", index, result.Stdout)

		key := vcKickoutParticipantResultKey{id: id.String(), userType: userType.Int()}
		remainingResults, ok := remainingByParticipant[key]
		require.True(t, ok, "unexpected or duplicate participant result %s/%d at index %d; stdout:\n%s", key.id, key.userType, index, result.Stdout)
		remaining := remainingResults[resultCode.Int()]
		require.Greater(t, remaining, 0, "unexpected result %d for participant %s/%d at index %d; stdout:\n%s", resultCode.Int(), key.id, key.userType, index, result.Stdout)

		if remaining == 1 {
			delete(remainingResults, resultCode.Int())
		} else {
			remainingResults[resultCode.Int()] = remaining - 1
		}
		if len(remainingResults) == 0 {
			delete(remainingByParticipant, key)
		}
	}
	require.Empty(t, remainingByParticipant, "missing participant result tuple(s); stdout:\n%s", result.Stdout)
}

func runMeetingParticipantKickoutLive(ctx context.Context, fixture vcMeetingParticipantKickoutLiveFixture, token, meetingID string, participants ...vcKickoutParticipantFixture) (*clie2e.Result, error) {
	args := []string{"vc", "+meeting-participant-kickout", "--meeting-id", meetingID}
	for _, participant := range participants {
		args = append(args, "--participant", participant.tuple)
	}
	return clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      args,
		DefaultAs: "user",
		Yes:       true,
		Env:       vcMeetingManagementUserEnv(fixture.appID, fixture.brand, fixture.isolatedConfigDir, token),
	}, vcMeetingManagementNoRetry())
}
