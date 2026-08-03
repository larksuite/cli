// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestIM_PageAllLiveWorkflow exercises the real multi-page pagination added to
// the im list shortcuts: pages are fetched until exhaustion or --page-limit,
// merged in order, and the merged result carries has_more plus the resume
// page_token from the last fetched page.
//
// Self-contained: creates its own chats and messages. Chat cleanup follows the
// repo-wide convention in createChat — lark-cli has no chat-delete command, so
// created chats are intentionally left in the test account.
//
// +chat-search pagination is intentionally not covered live: newly created
// chats are not immediately searchable (server-side indexing lag), which would
// make the assertion flaky. Its pagination loop is covered by unit and dry-run
// tests.
func TestIM_PageAllLiveWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	chatID := createChat(t, parentT, ctx, "lark-cli-e2e-page-all-"+suffix)
	// A second chat guarantees the bot is a member of at least two chats, so
	// +chat-list with --page-size 1 is guaranteed to have a second page.
	createChat(t, parentT, ctx, "lark-cli-e2e-page-all-b-"+suffix)

	texts := make([]string, 0, 3)
	var parentMessageID string
	for i := 1; i <= 3; i++ {
		text := fmt.Sprintf("lark-cli-e2e-page-all-msg-%d-%s", i, suffix)
		texts = append(texts, text)
		id := sendMessage(t, ctx, chatID, text)
		if i == 1 {
			parentMessageID = id
		}
	}

	t.Run("chat-messages-list stops at page limit with resume token", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+chat-messages-list", "--chat-id", chatID,
				"--page-size", "1", "--page-all", "--page-limit", "1"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.messages.#").Int())
		require.True(t, gjson.Get(result.Stdout, "data.has_more").Bool(),
			"3 messages at page-size 1 must not fit in one page")
		require.NotEmpty(t, gjson.Get(result.Stdout, "data.page_token").String(),
			"an incomplete merged result must carry the resume token")
		require.Contains(t, result.Stderr, "result is incomplete")
	})

	t.Run("chat-messages-list walks every page", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+chat-messages-list", "--chat-id", chatID,
				"--page-size", "1", "--page-all"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.GreaterOrEqual(t, gjson.Get(result.Stdout, "data.messages.#").Int(), int64(3))
		require.False(t, gjson.Get(result.Stdout, "data.has_more").Bool())
		require.Contains(t, result.Stderr, "page 2:", "expected a real second page fetch")
		for _, text := range texts {
			require.Contains(t, result.Stdout, text, "merged result must contain every sent message")
		}
	})

	t.Run("threads-messages-list walks a real thread", func(t *testing.T) {
		for i := 1; i <= 2; i++ {
			reply, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{"im", "+messages-reply",
					"--message-id", parentMessageID,
					"--text", fmt.Sprintf("lark-cli-e2e-page-all-reply-%d-%s", i, suffix),
					"--reply-in-thread",
				},
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			reply.AssertExitCode(t, 0)
			reply.AssertStdoutStatus(t, true)
		}

		// Thread replies replicate asynchronously; retry until both are visible.
		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{"im", "+threads-messages-list", "--thread", parentMessageID,
				"--page-size", "1", "--page-all"},
			DefaultAs: "bot",
		}, clie2e.RetryOptions{
			ShouldRetry: func(result *clie2e.Result) bool {
				if result == nil || result.ExitCode != 0 {
					return true
				}
				return strings.Count(result.Stdout, "lark-cli-e2e-page-all-reply-") < 2
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.GreaterOrEqual(t, gjson.Get(result.Stdout, "data.messages.#").Int(), int64(2))
		require.False(t, gjson.Get(result.Stdout, "data.has_more").Bool())
		require.Contains(t, result.Stderr, "page 2:", "expected a real second page fetch")
	})

	t.Run("chat-list paginates across chats", func(t *testing.T) {
		partial, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"im", "+chat-list", "--page-size", "1", "--page-all", "--page-limit", "1"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		partial.AssertExitCode(t, 0)
		partial.AssertStdoutStatus(t, true)
		require.Equal(t, int64(1), gjson.Get(partial.Stdout, "data.chats.#").Int())
		require.True(t, gjson.Get(partial.Stdout, "data.has_more").Bool(),
			"the bot is in at least two chats, so page 1 of size 1 must not be the end")
		require.NotEmpty(t, gjson.Get(partial.Stdout, "data.page_token").String())
		require.Contains(t, partial.Stderr, "result is incomplete")

		// The bot may be a member of many accumulated e2e chats, so a full walk
		// can legitimately end at the default --page-limit with has_more=true.
		// Assert the merge itself plus the resume contract instead of exhaustion.
		full, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"im", "+chat-list", "--page-size", "1", "--page-all"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		full.AssertExitCode(t, 0)
		full.AssertStdoutStatus(t, true)
		require.GreaterOrEqual(t, gjson.Get(full.Stdout, "data.chats.#").Int(), int64(2))
		require.Contains(t, full.Stderr, "page 2:", "expected a real second page fetch")
		if gjson.Get(full.Stdout, "data.has_more").Bool() {
			require.NotEmpty(t, gjson.Get(full.Stdout, "data.page_token").String(),
				"a truncated merged result must carry the resume token")
		}
	})
}
