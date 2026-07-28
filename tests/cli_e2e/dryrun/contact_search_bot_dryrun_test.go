// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package dryrun

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestContactSearchBotDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "contact_search_bot_dryrun")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "contact_search_bot_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"contact", "+search-bot",
			"--query", "助手",
			"--chat-ids", "oc_a,oc_b",
			"--has-chatted",
			"--page-size", "25",
			"--page-token", "cursor_in",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/bot/v4/bot/search", clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
	require.Equal(t, int64(25), clie2e.DryRunGet(out, "api.0.params.page_size").Int(), "stdout:\n%s", out)
	require.Equal(t, "cursor_in", clie2e.DryRunGet(out, "api.0.params.page_token").String(), "stdout:\n%s", out)
	require.Equal(t, "助手", clie2e.DryRunGet(out, "api.0.body.query").String(), "stdout:\n%s", out)
	require.Equal(t, []string{"oc_a", "oc_b"}, []string{
		clie2e.DryRunGet(out, "api.0.body.filter.chat_ids.0").String(),
		clie2e.DryRunGet(out, "api.0.body.filter.chat_ids.1").String(),
	}, "stdout:\n%s", out)
	require.True(t, clie2e.DryRunGet(out, "api.0.body.filter.has_chatter").Bool(), "stdout:\n%s", out)
}
