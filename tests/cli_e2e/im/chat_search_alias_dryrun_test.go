// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIM_SearchChatAliasDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"im", "+search-chat",
			"--query", "aliassmoke",
			"--page-size", "10",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	require.Equal(t, "POST", gjson.Get(result.Stdout, "api.0.method").String(), "stdout:\n%s", result.Stdout)
	require.Equal(t, "/open-apis/im/v2/chats/search", gjson.Get(result.Stdout, "api.0.url").String(), "stdout:\n%s", result.Stdout)
	require.Equal(t, "aliassmoke", gjson.Get(result.Stdout, "api.0.body.query").String(), "stdout:\n%s", result.Stdout)
	require.Equal(t, int64(10), gjson.Get(result.Stdout, "api.0.params.page_size").Int(), "stdout:\n%s", result.Stdout)
}
