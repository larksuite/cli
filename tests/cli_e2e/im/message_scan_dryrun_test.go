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

func setupDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_USER_ACCESS_TOKEN", "fake_user_token")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}

func TestIM_ChatMessagesListDryRun(t *testing.T) {
	setupDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("basic dry-run with chat-id", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+chat-messages-list",
				"--chat-id", "oc_test_dry_run",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "GET")
		require.Contains(t, result.Stdout, "/open-apis/im/v1/messages")
		require.Contains(t, result.Stdout, "container_id")
		require.Contains(t, result.Stdout, "oc_test_dry_run")
	})

	t.Run("dry-run with time range and sort", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+chat-messages-list",
				"--chat-id", "oc_test_dry_run",
				"--start", "2025-01-01T00:00:00Z",
				"--end", "2025-12-31T23:59:59Z",
				"--sort", "asc",
				"--page-size", "20",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "GET")
		require.Contains(t, result.Stdout, "asc")
		require.Contains(t, result.Stdout, "20")
	})

	t.Run("dry-run with no-reactions skips batch_query", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+chat-messages-list",
				"--chat-id", "oc_test_dry_run",
				"--no-reactions",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "GET")
		require.NotContains(t, result.Stdout, "reactions/batch_query")
	})

	t.Run("dry-run with reactions includes batch_query", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+chat-messages-list",
				"--chat-id", "oc_test_dry_run",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "reactions/batch_query")
	})
}

func TestIM_MessagesSearchDryRun(t *testing.T) {
	setupDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("basic dry-run with query", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+messages-search",
				"--query", "test keyword",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "POST")
		require.Contains(t, result.Stdout, "/open-apis/im/v1/messages/search")
		require.Contains(t, result.Stdout, "test keyword")
	})

	t.Run("dry-run with chat-id filter", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+messages-search",
				"--query", "incident",
				"--chat-id", "oc_test_chat",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "POST")
		require.Contains(t, result.Stdout, "/open-apis/im/v1/messages/search")
	})
}

func TestIM_ThreadsMessagesListDryRun(t *testing.T) {
	setupDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("basic dry-run with omt_ thread id", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+threads-messages-list",
				"--thread", "omt_test_dry_run",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "GET")
		require.Contains(t, result.Stdout, "/open-apis/im/v1/messages")
		require.Contains(t, result.Stdout, "omt_test_dry_run")
	})

	t.Run("dry-run with om_ message id resolves thread", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+threads-messages-list",
				"--thread", "om_test_dry_run",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "GET")
		require.Contains(t, result.Stdout, "/open-apis/im/v1/messages")
	})

	t.Run("dry-run with sort desc", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+threads-messages-list",
				"--thread", "omt_test_dry_run",
				"--sort", "desc",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "ByCreateTimeDesc")
	})

	t.Run("dry-run with no-reactions skips batch_query", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+threads-messages-list",
				"--thread", "omt_test_dry_run",
				"--no-reactions",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.NotContains(t, result.Stdout, "reactions/batch_query")
	})
}

func TestIM_MessagesMgetDryRun(t *testing.T) {
	setupDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("basic dry-run with single message id", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+messages-mget",
				"--message-ids", "om_test_dry_run",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "GET")
		require.Contains(t, result.Stdout, "/open-apis/im/v1/messages/mget")
		require.Contains(t, result.Stdout, "om_test_dry_run")
	})

	t.Run("dry-run with multiple message ids", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+messages-mget",
				"--message-ids", "om_test_1,om_test_2,om_test_3",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "GET")
		require.Contains(t, result.Stdout, "/open-apis/im/v1/messages/mget")
		require.Contains(t, result.Stdout, "om_test_1")
		require.Contains(t, result.Stdout, "om_test_2")
		require.Contains(t, result.Stdout, "om_test_3")
	})

	t.Run("dry-run with no-reactions skips batch_query", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+messages-mget",
				"--message-ids", "om_test_dry_run",
				"--no-reactions",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.NotContains(t, result.Stdout, "reactions/batch_query")
	})

	t.Run("dry-run with reactions includes batch_query", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+messages-mget",
				"--message-ids", "om_test_dry_run",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "reactions/batch_query")
	})
}
