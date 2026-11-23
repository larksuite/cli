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

func TestIMMessagesSearchDryRunSupportsUserAndBotIdentity(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	for _, identity := range []string{"user", "bot"} {
		t.Run(identity, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      []string{"im", "+messages-search", "--query", "incident", "--chat-id", "oc_dryrun", "--no-reactions", "--dry-run"},
				DefaultAs: identity,
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			require.Equal(t, identity, clie2e.DryRunGet(result.Stdout, "identity").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, http.MethodPost, clie2e.DryRunGet(result.Stdout, "api.0.method").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, "/open-apis/im/v1/messages/search", clie2e.DryRunGet(result.Stdout, "api.0.url").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, "incident", clie2e.DryRunGet(result.Stdout, "api.0.body.query").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, "oc_dryrun", clie2e.DryRunGet(result.Stdout, "api.0.body.filter.chat_ids.0").String(), "stdout:\n%s", result.Stdout)
		})
	}
}
