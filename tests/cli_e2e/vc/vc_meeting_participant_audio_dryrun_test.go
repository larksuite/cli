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

func TestVCMeetingParticipantAudioDryRun(t *testing.T) {
	setVCDryRunEnv(t)

	tests := []struct {
		command string
		path    string
	}{
		{command: "+meeting-participant-mute", path: "/open-apis/v1/bots/mute"},
		{command: "+meeting-participant-unmute", path: "/open-apis/v1/bots/unmute"},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"vc", tt.command,
					"--meeting-id", "7651377260537433044",
					"--target-user-id", "ou_target",
					"--user-id-type", "union_id",
					"--dry-run",
				},
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			require.Equal(t, int64(1), clie2e.DryRunGet(out, "api.#").Int(), "stdout:\n%s", out)
			require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
			require.Equal(t, tt.path, clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
			require.Equal(t, "union_id", clie2e.DryRunGet(out, "api.0.params.user_id_type").String(), "stdout:\n%s", out)
			require.Equal(t, "7651377260537433044", clie2e.DryRunGet(out, "api.0.body.meeting_id").String(), "stdout:\n%s", out)
			require.Equal(t, "ou_target", clie2e.DryRunGet(out, "api.0.body.target_user_id").String(), "stdout:\n%s", out)
			require.False(t, clie2e.DryRunGet(out, "api.0.body.device_id").Exists(), "device_id must not be sent; stdout:\n%s", out)
		})
	}
}
