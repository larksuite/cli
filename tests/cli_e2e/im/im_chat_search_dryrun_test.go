// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestIM_ChatSearchDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"im", "+chat-search",
			"--query", "example-chat",
			"--search-types", "private,public_joined",
			"--chat-modes", "group",
			"--page-size", "25",
			"--page-token", "next_page",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String(), "stdout:\n%s", result.Stdout)
	require.Equal(t, "/open-apis/im/v2/chats/search", clie2e.DryRunGet(result.Stdout, "api.0.url").String(), "stdout:\n%s", result.Stdout)
	require.Equal(t, `"example-chat"`, clie2e.DryRunGet(result.Stdout, "api.0.body.query").String(), "stdout:\n%s", result.Stdout)
	require.Equal(t, "private", clie2e.DryRunGet(result.Stdout, "api.0.body.filter.search_types.0").String(), "stdout:\n%s", result.Stdout)
	require.Equal(t, "public_joined", clie2e.DryRunGet(result.Stdout, "api.0.body.filter.search_types.1").String(), "stdout:\n%s", result.Stdout)
	require.Equal(t, "default", clie2e.DryRunGet(result.Stdout, "api.0.body.filter.chat_modes.0").String(), "stdout:\n%s", result.Stdout)
	require.Equal(t, int64(25), clie2e.DryRunGet(result.Stdout, "api.0.params.page_size").Int(), "stdout:\n%s", result.Stdout)
	require.Equal(t, "next_page", clie2e.DryRunGet(result.Stdout, "api.0.params.page_token").String(), "stdout:\n%s", result.Stdout)
}
