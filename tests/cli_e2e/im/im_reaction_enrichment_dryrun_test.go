// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

const reactionsBatchQueryPath = "/open-apis/im/v1/messages/reactions/batch_query"

// TestIM_ReactionEnrichmentDryRun pins the observable half of the conditional
// reaction scope: enrichment is a second request, so --no-reactions must leave
// the plan with the message read alone. If the flag stopped suppressing the
// batch_query, requiring im:message.reactions:read only when enrichment runs
// would be wrong regardless of how the scope is declared.
func TestIM_ReactionEnrichmentDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	tests := []struct {
		name       string
		args       []string
		method     string
		path       string
		pathPrefix string
		defaultAs  string
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
			name:   "messages-mget",
			args:   []string{"im", "+messages-mget", "--message-ids", "om_dryrun"},
			method: http.MethodGet,
			// mget carries its query inline in the planned URL
			pathPrefix: "/open-apis/im/v1/messages/mget",
		},
		{
			name:      "messages-search",
			args:      []string{"im", "+messages-search", "--query", "release"},
			method:    http.MethodPost,
			path:      "/open-apis/im/v1/messages/search",
			defaultAs: "user",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			as := tc.defaultAs
			if as == "" {
				as = "bot"
			}

			t.Run("default enriches", func(t *testing.T) {
				args := append(append([]string{}, tc.args...), "--dry-run")
				result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: as})
				require.NoError(t, err)
				result.AssertExitCode(t, 0)

				out := result.Stdout
				assertPlannedRead(t, out, tc.method, tc.path, tc.pathPrefix)
				require.Equal(t, reactionsBatchQueryPath, clie2e.DryRunGet(out, "api.1.url").String(),
					"enrichment request missing from the default plan; stdout:\n%s", out)
				require.Equal(t, http.MethodPost, clie2e.DryRunGet(out, "api.1.method").String(), "stdout:\n%s", out)
			})

			t.Run("no-reactions skips enrichment", func(t *testing.T) {
				args := append(append([]string{}, tc.args...), "--no-reactions", "--dry-run")
				result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: as})
				require.NoError(t, err)
				result.AssertExitCode(t, 0)

				out := result.Stdout
				assertPlannedRead(t, out, tc.method, tc.path, tc.pathPrefix)
				require.False(t, clie2e.DryRunGet(out, "api.1").Exists(),
					"--no-reactions still planned a second request; stdout:\n%s", out)
			})
		})
	}
}

// assertPlannedRead checks the message read that every one of these shortcuts
// plans first. mget encodes its query into the planned URL, so it is matched by
// prefix instead of equality.
func assertPlannedRead(t *testing.T, stdout, method, path, pathPrefix string) {
	t.Helper()

	require.Equal(t, method, clie2e.DryRunGet(stdout, "api.0.method").String(), "stdout:\n%s", stdout)
	url := clie2e.DryRunGet(stdout, "api.0.url").String()
	if pathPrefix != "" {
		require.True(t, strings.HasPrefix(url, pathPrefix),
			"planned url %q does not start with %q; stdout:\n%s", url, pathPrefix, stdout)
		return
	}
	require.Equal(t, path, url, "stdout:\n%s", stdout)
}
