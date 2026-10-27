// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestMail_ReadSharedCardDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "cli_test_app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "cli_test_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"mail", "+read-shared-card",
			"--owner-id", "12345",
			"--card-id", "card_1",
			"--message-format", "full",
			"--include-attachment-download-urls",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Equal(t, int64(3), clie2e.DryRunGet(result.Stdout, "api.#").Int(), "stdout:\n%s", result.Stdout)
	require.Equal(t, "/open-apis/mail/v1/user_mailboxes/me/messages/get_by_card", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
	require.Equal(t, "/open-apis/mail/v1/messages/by_card", clie2e.DryRunGet(result.Stdout, "api.1.url").String())
	require.Equal(t, "/open-apis/mail/v1/messages/attachments/download_url_by_card", clie2e.DryRunGet(result.Stdout, "api.2.url").String())
	require.Equal(t, "12345", clie2e.DryRunGet(result.Stdout, "api.0.params.owner_id").String())
	require.False(t, clie2e.DryRunGet(result.Stdout, "api.0.params.user_id_type").Exists())
	require.Equal(t, "12345", clie2e.DryRunGet(result.Stdout, "api.1.params.owner_id").String())
	require.Equal(t, "card_1", clie2e.DryRunGet(result.Stdout, "api.1.params.card_id").String())
	require.Equal(t, "12345", clie2e.DryRunGet(result.Stdout, "api.2.params.owner_id").String())
	require.Equal(t, "<card_authorized_message_id>", clie2e.DryRunGet(result.Stdout, "api.2.params.message_id").String())
}
