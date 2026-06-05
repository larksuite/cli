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

func TestIM_FlagWorkflowAsUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	parentT := t
	suffix := clie2e.GenerateSuffix()
	chatName := "im-flag-" + suffix
	messageText := "flag-test-msg-" + suffix
	var chatID string
	var messageID string

	t.Run("create chat as user", func(t *testing.T) {
		chatID = createChatAs(t, parentT, ctx, chatName, "user")
	})

	t.Run("send message as user", func(t *testing.T) {
		messageID = sendMessageAs(t, ctx, chatID, messageText, "user")
	})

	t.Run("create flag as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+flag-create",
				"--message-id", messageID,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("list flags as user", func(t *testing.T) {
		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{
				"im", "+flag-list",
				"--page-size", "10",
				"--page-all",
			},
			DefaultAs: "user",
		}, clie2e.RetryOptions{
			ShouldRetry: func(result *clie2e.Result) bool {
				if result == nil || result.ExitCode != 0 {
					return true
				}
				// Check if our message is in the list
				for _, item := range gjson.Get(result.Stdout, "data.flag_items").Array() {
					if item.Get("item_id").String() == messageID {
						return false
					}
				}
				return true
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		// Verify our flagged message is in the list
		var found bool
		for _, item := range gjson.Get(result.Stdout, "data.flag_items").Array() {
			if item.Get("item_id").String() == messageID {
				found = true
				// Verify it's a message-type flag (flag_type=2)
				require.Equal(t, "2", item.Get("flag_type").String(), "expected flag_type=2 (message)")
				break
			}
		}
		require.True(t, found, "expected message %s in flag list", messageID)
	})

	t.Run("cancel flag as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+flag-cancel",
				"--message-id", messageID,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("verify flag removed", func(t *testing.T) {
		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{
				"im", "+flag-list",
				"--page-size", "10",
				"--page-all",
			},
			DefaultAs: "user",
		}, clie2e.RetryOptions{
			ShouldRetry: func(result *clie2e.Result) bool {
				if result == nil || result.ExitCode != 0 {
					return true
				}
				// Check if our message is still in the list
				for _, item := range gjson.Get(result.Stdout, "data.flag_items").Array() {
					if item.Get("item_id").String() == messageID {
						return true // Still there, retry
					}
				}
				return false // Not found, success
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		// Verify our message is NOT in the list
		for _, item := range gjson.Get(result.Stdout, "data.flag_items").Array() {
			require.NotEqual(t, messageID, item.Get("item_id").String(), "message should not be in flag list after cancel")
		}
	})
}

func TestIM_FlagCreateWithExplicitTypeAsUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	parentT := t
	suffix := clie2e.GenerateSuffix()
	chatName := "im-flag-explicit-" + suffix
	messageText := "flag-explicit-msg-" + suffix
	var chatID string
	var messageID string

	t.Run("create chat as user", func(t *testing.T) {
		chatID = createChatAs(t, parentT, ctx, chatName, "user")
	})

	t.Run("send message as user", func(t *testing.T) {
		messageID = sendMessageAs(t, ctx, chatID, messageText, "user")
	})

	t.Run("create flag with explicit types as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+flag-create",
				"--message-id", messageID,
				"--item-type", "default",
				"--flag-type", "message",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("list flags to verify explicit types as user", func(t *testing.T) {
		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{
				"im", "+flag-list",
				"--page-size", "10",
				"--page-all",
			},
			DefaultAs: "user",
		}, clie2e.RetryOptions{
			ShouldRetry: func(result *clie2e.Result) bool {
				if result == nil || result.ExitCode != 0 {
					return true
				}
				for _, item := range gjson.Get(result.Stdout, "data.flag_items").Array() {
					if item.Get("item_id").String() == messageID {
						return false
					}
				}
				return true
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		// Verify explicit types were applied
		var found bool
		for _, item := range gjson.Get(result.Stdout, "data.flag_items").Array() {
			if item.Get("item_id").String() == messageID {
				found = true
				require.Equal(t, "0", item.Get("item_type").String(), "expected item_type=0 (default)")
				require.Equal(t, "2", item.Get("flag_type").String(), "expected flag_type=2 (message)")
				break
			}
		}
		require.True(t, found, "expected message %s in flag list", messageID)
	})

	t.Run("cancel flag with explicit types as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+flag-cancel",
				"--message-id", messageID,
				"--flag-type", "message",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})
}

func TestIM_FlagListPaginationAsUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	t.Run("list flags with page-all as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+flag-list",
				"--page-size", "5",
				"--page-all",
				"--page-limit", "3",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})
}

func TestIM_FlagDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_USER_ACCESS_TOKEN", "fake_user_token")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("create flag dry-run", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+flag-create",
				"--message-id", "om_test_dry_run",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "POST")
		require.Contains(t, result.Stdout, "/open-apis/im/v1/flags")
		require.Contains(t, result.Stdout, "flag_items")
		require.Contains(t, result.Stdout, "om_test_dry_run")
	})

	t.Run("cancel flag dry-run with om", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+flag-cancel",
				"--message-id", "om_test_dry_run",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "POST")
		require.Contains(t, result.Stdout, "/open-apis/im/v1/flags/cancel")
		require.Contains(t, result.Stdout, "flag_items")
		require.Contains(t, result.Stdout, "om_test_dry_run")
	})

	t.Run("list flag dry-run", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+flag-list",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "GET")
		require.Contains(t, result.Stdout, "/open-apis/im/v1/flags")
		require.Contains(t, result.Stdout, "page_size")
	})
}
