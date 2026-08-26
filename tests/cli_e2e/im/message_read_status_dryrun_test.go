// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"net/http"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIMMessagesReadStatusDryRun(t *testing.T) {
	setMessageReadStatusDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"im", "+messages-read-status",
			"--message-ids", "om_one,om_two",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Equal(t, "user", clie2e.DryRunGet(result.Stdout, "identity").String())
	require.Equal(t, http.MethodPost, clie2e.DryRunGet(result.Stdout, "api.0.method").String())
	require.Equal(t, "/open-apis/im/v1/messages/read_status", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
	require.Equal(t, "om_one", clie2e.DryRunGet(result.Stdout, "api.0.body.message_ids.0").String())
	require.Equal(t, "om_two", clie2e.DryRunGet(result.Stdout, "api.0.body.message_ids.1").String())
}

func TestIMMessagesReadStatusRejectsBotIdentity(t *testing.T) {
	setMessageReadStatusDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"im", "+messages-read-status",
			"--message-ids", "om_one",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Empty(t, result.Stdout)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
	require.Equal(t, "--as", gjson.Get(result.Stderr, "error.param").String())
}

func TestIMMessageReadUsersDryRunSupportsUserAndBot(t *testing.T) {
	setMessageReadStatusDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	for _, identity := range []string{"user", "bot"} {
		t.Run(identity, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"im", "+message-read-users",
					"--message-id", "om_test",
					"--dry-run",
				},
				DefaultAs: identity,
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			require.Equal(t, identity, clie2e.DryRunGet(result.Stdout, "identity").String())
			require.Equal(t, http.MethodGet, clie2e.DryRunGet(result.Stdout, "api.0.method").String())
			require.Equal(t, "/open-apis/im/v1/messages/om_test/read_users", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
			require.Equal(t, "open_id", clie2e.DryRunGet(result.Stdout, "api.0.params.user_id_type").String())
			require.Equal(t, int64(100), clie2e.DryRunGet(result.Stdout, "api.0.params.page_size").Int())
		})
	}
}

func setMessageReadStatusDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "im_read_status_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "im_read_status_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}
