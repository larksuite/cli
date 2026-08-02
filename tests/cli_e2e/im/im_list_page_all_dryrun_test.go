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
)

func TestIM_ListPageAllDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	tests := []struct {
		name   string
		args   []string
		method string
		path   string
	}{
		{
			name:   "chat-messages-list",
			args:   []string{"im", "+chat-messages-list", "--chat-id", "oc_dryrun"},
			method: http.MethodGet,
			path:   "/open-apis/im/v1/messages",
		},
		{
			name:   "threads-messages-list",
			args:   []string{"im", "+threads-messages-list", "--thread", "omt_dryrun"},
			method: http.MethodGet,
			path:   "/open-apis/im/v1/messages",
		},
		{
			name:   "chat-list",
			args:   []string{"im", "+chat-list"},
			method: http.MethodGet,
			path:   "/open-apis/im/v1/chats",
		},
		{
			name:   "chat-search",
			args:   []string{"im", "+chat-search", "--query", "team"},
			method: http.MethodPost,
			path:   "/open-apis/im/v2/chats/search",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{}, tc.args...)
			args = append(args, "--page-all", "--page-limit", "3", "--dry-run")
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			require.Equal(t, tc.method, clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
			require.Equal(t, tc.path, clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
			require.Equal(t, "Auto-paginates until exhaustion or --page-limit is reached", clie2e.DryRunGet(out, "description").String(), "stdout:\n%s", out)
		})
	}
}
