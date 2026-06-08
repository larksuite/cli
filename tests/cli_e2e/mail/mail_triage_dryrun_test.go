// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMail_TriageDryRunPreservesMailboxInRequestChain(t *testing.T) {
	setMailTriageDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"mail", "+triage",
			"--mailbox", "alias@example.com",
			"--filter", `{"folder_id":"INBOX"}`,
			"--max", "3",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	require.Equal(t, int64(2), gjson.Get(result.Stdout, "api.#").Int(), "stdout:\n%s", result.Stdout)
	require.Equal(t, "GET", gjson.Get(result.Stdout, "api.0.method").String(), "stdout:\n%s", result.Stdout)
	require.Equal(t, "/open-apis/mail/v1/user_mailboxes/alias@example.com/messages", gjson.Get(result.Stdout, "api.0.url").String(), "stdout:\n%s", result.Stdout)
	require.Equal(t, int64(3), gjson.Get(result.Stdout, "api.0.params.page_size").Int(), "stdout:\n%s", result.Stdout)
	require.Equal(t, "INBOX", gjson.Get(result.Stdout, "api.0.params.folder_id").String(), "stdout:\n%s", result.Stdout)

	require.Equal(t, "POST", gjson.Get(result.Stdout, "api.1.method").String(), "stdout:\n%s", result.Stdout)
	require.Equal(t, "/open-apis/mail/v1/user_mailboxes/alias@example.com/messages/batch_get", gjson.Get(result.Stdout, "api.1.url").String(), "stdout:\n%s", result.Stdout)
	require.Equal(t, "metadata", gjson.Get(result.Stdout, "api.1.body.format").String(), "stdout:\n%s", result.Stdout)
}

func setMailTriageDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "mail_triage_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "mail_triage_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}
