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

// TestIM_FeedShortcutWorkflowAsUser walks the full create → list → remove
// loop for a single CHAT-type feed shortcut, mirroring the +flag-* workflow
// test. The feed_shortcuts API is user-identity only and version-locked, so
// the assertion strategy uses RunCmdWithRetry against the list endpoint
// rather than assuming the index is updated synchronously.
func TestIM_FeedShortcutWorkflowAsUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	parentT := t
	suffix := clie2e.GenerateSuffix()
	chatName := "im-feed-shortcut-" + suffix
	var chatID string
	t.Cleanup(func() {
		cleanupFeedShortcuts(parentT, "user", chatID)
	})

	t.Run("create chat as user", func(t *testing.T) {
		chatID = createChatAs(t, parentT, ctx, chatName, "user")
	})

	t.Run("pin chat to feed as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+feed-shortcut-create",
				"--chat-id", chatID,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		// failed_shortcuts may be absent (server returns {} on full success)
		// or present-and-empty — either way it must not contain our chat.
		for _, item := range gjson.Get(result.Stdout, "data.failed_shortcuts").Array() {
			require.NotEqual(t, chatID, item.Get("shortcut.feed_card_id").String(),
				"create should not report our chat as failed")
		}
	})

	t.Run("list feed shortcuts as user with detail enrichment", func(t *testing.T) {
		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{
				"im", "+feed-shortcut-list",
			},
			DefaultAs: "user",
		}, clie2e.RetryOptions{
			ShouldRetry: func(result *clie2e.Result) bool {
				if result == nil || result.ExitCode != 0 {
					return true
				}
				for _, item := range gjson.Get(result.Stdout, "data.shortcuts").Array() {
					if item.Get("feed_card_id").String() == chatID {
						return false
					}
				}
				return true
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		var found bool
		for _, item := range gjson.Get(result.Stdout, "data.shortcuts").Array() {
			if item.Get("feed_card_id").String() != chatID {
				continue
			}
			found = true
			require.Equal(t, int64(1), item.Get("type").Int(), "type should be 1 (CHAT)")
			// detail enrichment is on by default — the chat we just created
			// must come back with the chat info object attached.
			require.True(t, item.Get("detail").Exists(),
				"detail field should be attached when enrichment is enabled")
			require.Equal(t, chatID, item.Get("detail.chat_id").String(),
				"detail.chat_id should echo feed_card_id")
			require.Equal(t, chatName, item.Get("detail.name").String(),
				"detail.name should carry the chat's group name")
			break
		}
		require.True(t, found, "expected chat %s in feed shortcut list", chatID)
	})

	t.Run("list feed shortcuts with --no-detail skips lookup", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+feed-shortcut-list",
				"--no-detail",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		var foundEntry gjson.Result
		for _, item := range gjson.Get(result.Stdout, "data.shortcuts").Array() {
			if item.Get("feed_card_id").String() == chatID {
				foundEntry = item
				break
			}
		}
		require.True(t, foundEntry.Exists(), "expected our chat in the bare list")
		require.False(t, foundEntry.Get("detail").Exists(),
			"detail field should NOT be present with --no-detail")
	})

	t.Run("unpin chat from feed as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+feed-shortcut-remove",
				"--chat-id", chatID,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		for _, item := range gjson.Get(result.Stdout, "data.failed_shortcuts").Array() {
			require.NotEqual(t, chatID, item.Get("shortcut.feed_card_id").String(),
				"remove should not report our chat as failed")
		}
	})

	t.Run("verify chat removed from feed", func(t *testing.T) {
		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{
				"im", "+feed-shortcut-list",
				"--no-detail",
			},
			DefaultAs: "user",
		}, clie2e.RetryOptions{
			ShouldRetry: func(result *clie2e.Result) bool {
				if result == nil || result.ExitCode != 0 {
					return true
				}
				for _, item := range gjson.Get(result.Stdout, "data.shortcuts").Array() {
					if item.Get("feed_card_id").String() == chatID {
						return true // still there, retry
					}
				}
				return false
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		for _, item := range gjson.Get(result.Stdout, "data.shortcuts").Array() {
			require.NotEqual(t, chatID, item.Get("feed_card_id").String(),
				"chat should not be in feed list after remove")
		}
	})
}

// TestIM_FeedShortcutBatchAsUser exercises batch create / remove with two
// chats in a single API call. The per-item failure path (failed_shortcuts)
// is checked by mixing a real chat with an obviously invalid id.
func TestIM_FeedShortcutBatchAsUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	parentT := t
	suffix := clie2e.GenerateSuffix()
	var chatA, chatB string
	t.Cleanup(func() {
		cleanupFeedShortcuts(parentT, "user", chatA, chatB)
	})

	t.Run("create two chats as user", func(t *testing.T) {
		chatA = createChatAs(t, parentT, ctx, "im-feed-batch-a-"+suffix, "user")
		chatB = createChatAs(t, parentT, ctx, "im-feed-batch-b-"+suffix, "user")
	})

	t.Run("batch pin both with --tail", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+feed-shortcut-create",
				"--chat-id", chatA + "," + chatB,
				"--tail",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("batch pin with one invalid id reports per-item failure", func(t *testing.T) {
		// Re-pinning chatA (already pinned) should still succeed without
		// noting it as failure; the synthetic invalid id should land in
		// failed_shortcuts with reason_label=invalid_item.
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+feed-shortcut-create",
				"--chat-id", chatA,
				"--chat-id", "oc_definitely_not_a_real_chat_id_" + suffix,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		// Partial failure exits with the generic API failure code while the
		// full ledger stays machine-readable on stdout.
		result.AssertExitCode(t, 1)
		result.AssertStdoutStatus(t, false)
		require.Equal(t, int64(2), gjson.Get(result.Stdout, "data.total").Int())
		require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.success_count").Int())
		require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.failure_count").Int())

		var sawInvalid bool
		for _, item := range gjson.Get(result.Stdout, "data.failed_shortcuts").Array() {
			require.NotEqual(t, chatA, item.Get("shortcut.feed_card_id").String(),
				"real chat should not appear in failed_shortcuts")
			if item.Get("shortcut.feed_card_id").String() == "oc_definitely_not_a_real_chat_id_"+suffix {
				sawInvalid = true
				require.Equal(t, "invalid_item", item.Get("reason_label").String(),
					"invalid id should be labeled invalid_item")
			}
		}
		require.True(t, sawInvalid, "expected the bogus chat id in failed_shortcuts")
		var sawSucceeded bool
		for _, item := range gjson.Get(result.Stdout, "data.succeeded_shortcuts").Array() {
			if item.Get("feed_card_id").String() == chatA {
				sawSucceeded = true
				require.Equal(t, int64(1), item.Get("type").Int(), "succeeded shortcut type should be CHAT")
			}
		}
		require.True(t, sawSucceeded, "expected the real chat id in succeeded_shortcuts")
	})

	t.Run("batch remove both", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+feed-shortcut-remove",
				"--chat-id", chatA,
				"--chat-id", chatB,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})
}

func cleanupFeedShortcuts(parentT *testing.T, defaultAs string, chatIDs ...string) {
	parentT.Helper()
	var ids []string
	for _, id := range chatIDs {
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}

	cleanupCtx, cancel := clie2e.CleanupContext()
	defer cancel()
	listResult, listErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
		Args:      []string{"im", "+feed-shortcut-list", "--no-detail"},
		DefaultAs: defaultAs,
	})
	clie2e.ReportCleanupFailure(parentT, "cleanup feed shortcuts list", listResult, listErr)
	if listErr != nil || listResult == nil || listResult.ExitCode != 0 {
		return
	}

	present := make(map[string]bool, len(ids))
	for _, id := range ids {
		present[id] = false
	}
	for _, item := range gjson.Get(listResult.Stdout, "data.shortcuts").Array() {
		id := item.Get("feed_card_id").String()
		if _, ok := present[id]; ok {
			present[id] = true
		}
	}

	args := []string{"im", "+feed-shortcut-remove"}
	for _, id := range ids {
		if present[id] {
			args = append(args, "--chat-id", id)
		}
	}
	if len(args) == 2 {
		return
	}
	result, err := clie2e.RunCmd(cleanupCtx, clie2e.Request{
		Args:      args,
		DefaultAs: defaultAs,
	})
	clie2e.ReportCleanupFailure(parentT, "cleanup feed shortcuts", result, err)
}

// TestIM_FeedShortcutDryRun covers all three shortcuts in --dry-run mode
// using env-only identity hints. strict_mode/default_as lock the command to
// user identity without injecting a fake user token that would trigger
// user_info verification during startup.
func TestIM_FeedShortcutDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
	t.Setenv("LARKSUITE_CLI_STRICT_MODE", "user")
	t.Setenv("LARKSUITE_CLI_DEFAULT_AS", "user")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("create dry-run", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+feed-shortcut-create",
				"--chat-id", "oc_test_dry_run",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "POST")
		require.Contains(t, result.Stdout, "/open-apis/im/v2/feed_shortcuts")
		require.Contains(t, result.Stdout, "oc_test_dry_run")
		// --head is the default so the body should be is_header=true
		require.Contains(t, result.Stdout, `"is_header"`)
	})

	t.Run("create dry-run with --tail flips is_header to false", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+feed-shortcut-create",
				"--chat-id", "oc_test_dry_run",
				"--tail",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, `"is_header": false`)
	})

	t.Run("create dry-run rejects --head + --tail", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+feed-shortcut-create",
				"--chat-id", "oc_test_dry_run",
				"--head",
				"--tail",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		// Validation errors exit 2 with the structured error envelope;
		// assert against combined output to stay channel-agnostic.
		result.AssertExitCode(t, 2)
		combined := result.Stdout + "\n" + result.Stderr
		require.Contains(t, combined, "mutually exclusive")
	})

	t.Run("create dry-run rejects non-oc chat ids", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+feed-shortcut-create",
				"--chat-id", "om_not_a_chat",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		combined := result.Stdout + "\n" + result.Stderr
		require.Contains(t, combined, "must be an open_chat_id")
	})

	t.Run("remove dry-run hits /remove endpoint", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+feed-shortcut-remove",
				"--chat-id", "oc_a,oc_b",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "POST")
		require.Contains(t, result.Stdout, "/open-apis/im/v2/feed_shortcuts/remove")
		require.Contains(t, result.Stdout, "oc_a")
		require.Contains(t, result.Stdout, "oc_b")
		require.NotContains(t, result.Stdout, "is_header", "remove must not send is_header")
	})

	t.Run("list dry-run mentions detail enrichment path", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+feed-shortcut-list",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "GET")
		require.Contains(t, result.Stdout, "/open-apis/im/v2/feed_shortcuts")
		// Enrichment is on by default → DryRun adds a desc about the extra
		// chats.batch_query call and the conditional scope.
		require.Contains(t, result.Stdout, "im:chat:read")
		require.Contains(t, result.Stdout, "batch_query")
	})

	t.Run("list dry-run with --no-detail omits the extra-scope note", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"im", "+feed-shortcut-list",
				"--no-detail",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.NotContains(t, result.Stdout, "im:chat:read",
			"with --no-detail, dry-run must not mention im:chat:read")
	})
}
