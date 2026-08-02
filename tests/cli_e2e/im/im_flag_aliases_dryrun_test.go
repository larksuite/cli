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

func TestIMFlagAliasesDryRun(t *testing.T) {
	setFlagAliasDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	tests := []struct {
		name          string
		aliasArgs     []string
		canonicalArgs []string
		defaultAs     string
	}{
		{
			name: "chat messages",
			aliasArgs: []string{
				"im", "+chat-messages-list", "--chat-id", "oc_dryrun",
				"--start-time", "2026-07-27T00:00:00+08:00",
				"--end-time", "1785254400",
				"--sort-order", "asc", "--limit", "25", "--no-reactions", "--dry-run",
			},
			canonicalArgs: []string{
				"im", "+chat-messages-list", "--chat-id", "oc_dryrun",
				"--start", "2026-07-27T00:00:00+08:00",
				"--end", "1785254400",
				"--order", "asc", "--page-size", "25", "--no-reactions", "--dry-run",
			},
			defaultAs: "bot",
		},
		{
			name:          "chat members page size",
			aliasArgs:     []string{"im", "+chat-members-list", "--chat-id", "oc_dryrun", "--limit", "25", "--page-all", "--dry-run"},
			canonicalArgs: []string{"im", "+chat-members-list", "--chat-id", "oc_dryrun", "--page-size", "25", "--page-all", "--dry-run"},
			defaultAs:     "bot",
		},
		{
			name:          "thread id",
			aliasArgs:     []string{"im", "+threads-messages-list", "--thread-id", "omt_dryrun", "--no-reactions", "--dry-run"},
			canonicalArgs: []string{"im", "+threads-messages-list", "--thread", "omt_dryrun", "--no-reactions", "--dry-run"},
			defaultAs:     "bot",
		},
		{
			name:          "message id",
			aliasArgs:     []string{"im", "+messages-mget", "--message-id", "om_dryrun", "--no-reactions", "--dry-run"},
			canonicalArgs: []string{"im", "+messages-mget", "--message-ids", "om_dryrun", "--no-reactions", "--dry-run"},
			defaultAs:     "bot",
		},
		{
			name:          "message search",
			aliasArgs:     []string{"im", "+messages-search", "--keyword", "project", "--limit", "30", "--no-reactions", "--dry-run"},
			canonicalArgs: []string{"im", "+messages-search", "--query", "project", "--page-size", "30", "--no-reactions", "--dry-run"},
			defaultAs:     "user",
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
			require.Equal(t, canonicalResult.Stderr, aliasResult.Stderr)
		})
	}
}

func TestIMLegacySortInputsNormalizeBeforeDryRun(t *testing.T) {
	setFlagAliasDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	tests := []struct {
		name       string
		args       []string
		resultPath string
		want       string
	}{
		{
			name:       "chat list upstream vocabulary",
			args:       []string{"im", "+chat-list", "--sort-type", "ByActiveTimeDesc", "--dry-run"},
			resultPath: "api.0.params.sort_type",
			want:       "ByActiveTimeDesc",
		},
		{
			name:       "chat search upstream vocabulary",
			args:       []string{"im", "+chat-search", "--query", "team", "--sort-by", "update_time_desc", "--dry-run"},
			resultPath: "api.0.body.sorter",
			want:       "update_time_desc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: tt.args, DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			require.Equal(t, tt.want, clie2e.DryRunGet(result.Stdout, tt.resultPath).String(), result.Stdout)
			require.Empty(t, result.Stderr)
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
