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

// TestVCMeetingParticipantAudioLiveE2E verifies that the backend accepts mute
// and request-unmute calls for a participant in a provisioner-owned meeting.
// The provisioner cleanup ends the disposable meeting after the workflow.
func TestVCMeetingParticipantAudioLiveE2E(t *testing.T) {
	requireVCMeetingManagementDestructiveOptIn(t)
	fixture := requireVCKickoutProvisionFixture(t)
	require.EqualValues(t, vcMeetingManagementLarkUserType, fixture.hostTarget.userType)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	for _, tc := range []struct {
		command string
		action  string
		status  string
	}{
		{command: "+meeting-participant-mute", action: "mute", status: "completed"},
		{command: "+meeting-participant-unmute", action: "request_unmute", status: "request_sent"},
	} {
		if ok := t.Run(tc.action, func(t *testing.T) {
			result, err := runMeetingParticipantAudioLive(ctx, fixture, tc.command)
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			result.AssertStdoutStatus(t, true)
			require.Equal(t, "user", gjson.Get(result.Stdout, "identity").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, tc.action, gjson.Get(result.Stdout, "data.action").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, tc.status, gjson.Get(result.Stdout, "data.status").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, fixture.hostMeetingID, gjson.Get(result.Stdout, "data.meeting_id").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, fixture.hostTarget.id, gjson.Get(result.Stdout, "data.target_user_id").String(), "stdout:\n%s", result.Stdout)
		}); !ok {
			return
		}
	}
}

func runMeetingParticipantAudioLive(ctx context.Context, fixture vcMeetingParticipantKickoutLiveFixture, command string) (*clie2e.Result, error) {
	return clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{
			"vc", command,
			"--meeting-id", fixture.hostMeetingID,
			"--target-user-id", fixture.hostTarget.id,
			"--user-id-type", "open_id",
		},
		DefaultAs: "user",
		Env:       vcMeetingManagementUserEnv(fixture.appID, fixture.brand, fixture.isolatedConfigDir, fixture.hostToken),
	}, vcMeetingManagementNoRetry())
}
