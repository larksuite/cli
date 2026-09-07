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

func TestVCMeetingCountdownDryRun(t *testing.T) {
	setVCDryRunEnv(t)

	tests := []struct {
		name                string
		args                []string
		wantAction          string
		wantDuration        int64
		wantAudioAtEnd      *bool
		wantReminderBefore  int64
		wantDurationAbsent  bool
		wantReminderAbsent  bool
		wantAudioAtEndEmpty bool
	}{
		{
			name: "set",
			args: []string{
				"vc", "+meeting-countdown",
				"--meeting-id", "7651377260537433044",
				"--action", "set",
				"--duration", "5",
				"--need-play-audio-at-end",
				"--reminder-before-end", "1",
				"--dry-run",
			},
			wantAction:         "set",
			wantDuration:       5,
			wantAudioAtEnd:     ptrBool(true),
			wantReminderBefore: 1,
		},
		{
			name: "prolong",
			args: []string{
				"vc", "+meeting-countdown",
				"--meeting-id", "7651377260537433044",
				"--action", "prolong",
				"--duration", "2",
				"--dry-run",
			},
			wantAction:          "prolong",
			wantDuration:        2,
			wantReminderAbsent:  true,
			wantAudioAtEndEmpty: true,
		},
		{
			name: "end in advance",
			args: []string{
				"vc", "+meeting-countdown",
				"--meeting-id", "7651377260537433044",
				"--action", "end_in_advance",
				"--dry-run",
			},
			wantAction:          "end_in_advance",
			wantDurationAbsent:  true,
			wantReminderAbsent:  true,
			wantAudioAtEndEmpty: true,
		},
		{
			name: "close window",
			args: []string{
				"vc", "+meeting-countdown",
				"--meeting-id", "7651377260537433044",
				"--action", "close_window",
				"--dry-run",
			},
			wantAction:          "close_window",
			wantDurationAbsent:  true,
			wantReminderAbsent:  true,
			wantAudioAtEndEmpty: true,
		},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tt.args,
				DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			require.Equal(t, int64(1), clie2e.DryRunGet(out, "api.#").Int(), "stdout:\n%s", out)
			require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
			require.Equal(t, "/open-apis/vc/v1/bots/countdown", clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
			require.Equal(t, "7651377260537433044", clie2e.DryRunGet(out, "api.0.body.meeting_id").String(), "stdout:\n%s", out)
			require.Equal(t, tt.wantAction, clie2e.DryRunGet(out, "api.0.body.action").String(), "stdout:\n%s", out)

			duration := clie2e.DryRunGet(out, "api.0.body.duration")
			if tt.wantDurationAbsent {
				require.False(t, duration.Exists(), "stdout:\n%s", out)
			} else {
				require.Equal(t, tt.wantDuration, duration.Int(), "stdout:\n%s", out)
			}

			audioAtEnd := clie2e.DryRunGet(out, "api.0.body.need_play_audio_at_end")
			if tt.wantAudioAtEndEmpty {
				require.False(t, audioAtEnd.Exists(), "stdout:\n%s", out)
			} else {
				require.NotNil(t, tt.wantAudioAtEnd)
				require.Equal(t, *tt.wantAudioAtEnd, audioAtEnd.Bool(), "stdout:\n%s", out)
			}

			reminder := clie2e.DryRunGet(out, "api.0.body.reminder_before_end")
			if tt.wantReminderAbsent {
				require.False(t, reminder.Exists(), "stdout:\n%s", out)
			} else {
				require.Equal(t, tt.wantReminderBefore, reminder.Int(), "stdout:\n%s", out)
			}
		})
	}
}

func TestVCMeetingCountdownDryRunRejectsAudioAtEndOutsideSet(t *testing.T) {
	setVCDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"vc", "+meeting-countdown",
			"--meeting-id", "7651377260537433044",
			"--action", "prolong",
			"--duration", "5",
			"--need-play-audio-at-end=false",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), "stderr:\n%s", result.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), "stderr:\n%s", result.Stderr)
	require.Equal(t, "--need-play-audio-at-end", gjson.Get(result.Stderr, "error.param").String(), "stderr:\n%s", result.Stderr)
	require.Empty(t, result.Stdout)
}

func ptrBool(v bool) *bool {
	return &v
}
