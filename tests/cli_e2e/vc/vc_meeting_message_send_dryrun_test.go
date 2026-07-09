// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestVCMeetingMessageSendDryRun(t *testing.T) {
	setVCDryRunEnv(t)

	tests := []struct {
		name        string
		args        []string
		wantMsgType string
		wantContent string
		wantUUID    string
	}{
		{
			name: "text",
			args: []string{
				"vc", "+meeting-message-send",
				"--meeting-id", "7651377260537433044",
				"--text", "hello from dry-run",
				"--uuid", "cid-dryrun-text",
				"--dry-run",
			},
			wantMsgType: "text",
			wantContent: "hello from dry-run",
			wantUUID:    "cid-dryrun-text",
		},
		{
			name: "reaction",
			args: []string{
				"vc", "+meeting-message-send",
				"--meeting-id", "7651377260537433044",
				"--msg-type", "reaction",
				"--emoji-type", "VC_NoSound",
				"--dry-run",
			},
			wantMsgType: "reaction",
			wantContent: "VC_NoSound",
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
			require.Equal(t, "/open-apis/vc/v1/bots/message", clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
			require.Equal(t, "7651377260537433044", clie2e.DryRunGet(out, "api.0.body.meeting_id").String(), "stdout:\n%s", out)
			require.Equal(t, tt.wantMsgType, clie2e.DryRunGet(out, "api.0.body.msg_type").String(), "stdout:\n%s", out)
			require.Equal(t, tt.wantContent, clie2e.DryRunGet(out, "api.0.body.content").String(), "stdout:\n%s", out)
			if tt.wantUUID == "" {
				require.False(t, clie2e.DryRunGet(out, "api.0.body.uuid").Exists(), "stdout:\n%s", out)
			} else {
				require.Equal(t, tt.wantUUID, clie2e.DryRunGet(out, "api.0.body.uuid").String(), "stdout:\n%s", out)
			}
		})
	}
}

func TestVCMeetingMessageSendDryRunRejectsLongUUID(t *testing.T) {
	setVCDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"vc", "+meeting-message-send",
			"--meeting-id", "7651377260537433044",
			"--text", "hello from dry-run",
			"--uuid", strings.Repeat("u", 129),
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), "stderr:\n%s", result.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), "stderr:\n%s", result.Stderr)
	require.Equal(t, "--uuid", gjson.Get(result.Stderr, "error.param").String(), "stderr:\n%s", result.Stderr)
	require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "--uuid is too long", "stderr:\n%s", result.Stderr)
	require.Empty(t, result.Stdout)
}

func setVCDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "vc_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "vc_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}
