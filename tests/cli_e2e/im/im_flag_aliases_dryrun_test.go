// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIMFlagAliasesDryRun(t *testing.T) {
	setFlagAliasDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	tests := []struct {
		name          string
		aliasArgs     []string
		canonicalArgs []string
		defaultAs     string
		notes         []string
	}{
		{
			name: "chat messages",
			aliasArgs: []string{
				"im", "+chat-messages-list", "--chat-id", "oc_dryrun",
				"--start-time", "2026-07-27 00:00:00 +08:00",
				"--end-time", "1785254400",
				"--sort-order", "asc", "--limit", "25", "--no-reactions", "--dry-run",
			},
			canonicalArgs: []string{
				"im", "+chat-messages-list", "--chat-id", "oc_dryrun",
				"--start", "2026-07-27 00:00:00 +08:00",
				"--end", "1785254400",
				"--order", "asc", "--page-size", "25", "--no-reactions", "--dry-run",
			},
			defaultAs: "bot",
			notes: []string{
				"note: --start-time is an alias for --start",
				"note: --end-time is an alias for --end",
				"note: --sort-order is an alias for --order",
				"note: --limit is an alias for --page-size",
			},
		},
		{
			name:          "thread id",
			aliasArgs:     []string{"im", "+threads-messages-list", "--thread-id", "omt_dryrun", "--no-reactions", "--dry-run"},
			canonicalArgs: []string{"im", "+threads-messages-list", "--thread", "omt_dryrun", "--no-reactions", "--dry-run"},
			defaultAs:     "bot",
			notes:         []string{"note: --thread-id is an alias for --thread"},
		},
		{
			name:          "message id",
			aliasArgs:     []string{"im", "+messages-mget", "--message-id", "om_dryrun", "--no-reactions", "--dry-run"},
			canonicalArgs: []string{"im", "+messages-mget", "--message-ids", "om_dryrun", "--no-reactions", "--dry-run"},
			defaultAs:     "bot",
			notes:         []string{"note: --message-id is an alias for --message-ids"},
		},
		{
			name:          "message search",
			aliasArgs:     []string{"im", "+messages-search", "--keyword", "project", "--limit", "30", "--no-reactions", "--dry-run"},
			canonicalArgs: []string{"im", "+messages-search", "--query", "project", "--page-size", "30", "--no-reactions", "--dry-run"},
			defaultAs:     "user",
			notes: []string{
				"note: --keyword is an alias for --query",
				"note: --limit is an alias for --page-size",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aliasResult, err := clie2e.RunCmd(ctx, clie2e.Request{Args: tt.aliasArgs, DefaultAs: tt.defaultAs})
			require.NoError(t, err)
			aliasResult.AssertExitCode(t, 0)

			canonicalResult, err := clie2e.RunCmd(ctx, clie2e.Request{Args: tt.canonicalArgs, DefaultAs: tt.defaultAs})
			require.NoError(t, err)
			canonicalResult.AssertExitCode(t, 0)

			require.JSONEq(t, canonicalResult.Stdout, aliasResult.Stdout)
			require.NotContains(t, aliasResult.Stdout, "is an alias for")
			for _, note := range tt.notes {
				require.Equal(t, 1, strings.Count(aliasResult.Stderr, note), "stderr:\n%s", aliasResult.Stderr)
			}
		})
	}
}

func TestIMFlagAliasesHiddenFromHelp(t *testing.T) {
	setFlagAliasDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	tests := []struct {
		command string
		aliases []string
	}{
		{"+chat-messages-list", []string{"start-time", "end-time", "sort-order", "limit"}},
		{"+threads-messages-list", []string{"thread-id"}},
		{"+messages-mget", []string{"message-id"}},
		{"+messages-search", []string{"keyword", "limit"}},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"im", tt.command, "--help"}})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			for _, alias := range tt.aliases {
				pattern := regexp.MustCompile(`(?m)^\s+--` + regexp.QuoteMeta(alias) + `(?:\s|$)`)
				require.False(t, pattern.MatchString(result.Stdout), "--%s leaked into help:\n%s", alias, result.Stdout)
			}
		})
	}
}

func setFlagAliasDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "alias_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "alias_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
	t.Setenv("LARKSUITE_CLI_NO_UPDATE_NOTIFIER", "1")
	t.Setenv("LARKSUITE_CLI_NO_SKILLS_NOTIFIER", "1")
}

// A hidden alias with an invalid value must not fail the command when the
// canonical flag is present — the canonical flag wins and the alias is
// ignored entirely, including its value. This exercises the full runner path
// (declared enums are framework-validated before command Validate runs, so
// alias value sets must not be declared as enums).
func TestIMChatMessagesListCanonicalOrderIgnoresInvalidAliasValue(t *testing.T) {
	setFlagAliasDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"im", "+chat-messages-list", "--chat-id", "oc_dryrun",
			"--order", "asc", "--sort-order", "unexpected", "--dry-run"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Equal(t, "ByCreateTimeAsc", clie2e.DryRunGet(result.Stdout, "api.0.params.sort_type").String())
	require.NotContains(t, result.Stderr, "alias")

	// Alias in effect on its own: the value set is enforced and the error is
	// attributed to the alias flag.
	rejected, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"im", "+chat-messages-list", "--chat-id", "oc_dryrun",
			"--sort-order", "unexpected", "--dry-run"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	rejected.AssertExitCode(t, 2)
	require.Empty(t, rejected.Stdout)
	require.Equal(t, `invalid value "unexpected" for --sort-order, allowed: asc, desc`, gjson.Get(rejected.Stderr, "error.message").String())
	require.Equal(t, "--sort-order", gjson.Get(rejected.Stderr, "error.param").String())
}

// Alias-supplied invalid values must be attributed to the flag the caller
// actually typed — never to the canonical flag it maps to.
func TestIMAliasErrorsNameTheTypedFlag(t *testing.T) {
	setFlagAliasDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	cases := []struct {
		name      string
		args      []string
		wantParam string
		wantMsg   string
	}{
		{
			name:      "start-time",
			args:      []string{"im", "+chat-messages-list", "--chat-id", "oc_dryrun", "--start-time", "bad-time", "--dry-run"},
			wantParam: "--start-time",
			wantMsg:   "--start-time: cannot parse time",
		},
		{
			name:      "thread-id",
			args:      []string{"im", "+threads-messages-list", "--thread-id", "not-a-thread", "--dry-run"},
			wantParam: "--thread-id",
			wantMsg:   `invalid --thread-id "not-a-thread"`,
		},
		{
			name:      "message-id",
			args:      []string{"im", "+messages-mget", "--message-id", "not-om", "--dry-run"},
			wantParam: "--message-id",
			wantMsg:   `invalid message ID "not-om"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: tc.args, DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 2)
			require.Empty(t, result.Stdout)
			require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), tc.wantMsg)
			require.Equal(t, tc.wantParam, gjson.Get(result.Stderr, "error.param").String())
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
		})
	}
}
