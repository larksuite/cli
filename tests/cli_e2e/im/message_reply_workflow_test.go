// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIM_MessageReplyWorkflowAsBot(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	chatName := "lark-cli-e2e-im-reply-" + suffix
	originalMessage := "lark-cli-e2e-original-message-" + suffix
	replyText := "lark-cli-e2e-reply-text-" + suffix

	chatID := createChat(t, parentT, ctx, chatName)
	messageID := sendMessage(t, ctx, chatID, originalMessage)

	t.Run("reply to message in thread as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+messages-reply",
				"--message-id", messageID,
				"--text", replyText,
				"--reply-in-thread",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.NotEmpty(t, gjson.Get(result.Stdout, "data.message_id").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, chatID, gjson.Get(result.Stdout, "data.chat_id").String(), "stdout:\n%s", result.Stdout)
	})

	t.Run("list thread replies as bot", func(t *testing.T) {
		listResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{
				"im", "+chat-messages-list",
				"--chat-id", chatID,
				"--start", time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
				"--end", time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339),
			},
			DefaultAs: "bot",
		}, clie2e.RetryOptions{
			ShouldRetry: func(result *clie2e.Result) bool {
				if result == nil || result.ExitCode != 0 {
					return true
				}
				for _, item := range gjson.Get(result.Stdout, "data.messages").Array() {
					if item.Get("message_id").String() == messageID && item.Get("thread_id").String() != "" {
						return false
					}
				}
				return true
			},
		})
		require.NoError(t, err)
		listResult.AssertExitCode(t, 0)
		listResult.AssertStdoutStatus(t, true)

		var threadID string
		for _, item := range gjson.Get(listResult.Stdout, "data.messages").Array() {
			if item.Get("message_id").String() == messageID {
				threadID = item.Get("thread_id").String()
				break
			}
		}
		require.NotEmpty(t, threadID, "expected thread_id for message %s in stdout:\n%s", messageID, listResult.Stdout)

		threadResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args:      []string{"im", "+threads-messages-list", "--thread", threadID},
			DefaultAs: "bot",
		}, clie2e.RetryOptions{
			ShouldRetry: func(result *clie2e.Result) bool {
				if result == nil || result.ExitCode != 0 {
					return true
				}
				for _, item := range gjson.Get(result.Stdout, "data.messages").Array() {
					if strings.Contains(item.Get("content").String(), replyText) {
						return false
					}
				}
				return true
			},
		})
		require.NoError(t, err)
		threadResult.AssertExitCode(t, 0)
		threadResult.AssertStdoutStatus(t, true)

		var found bool
		for _, item := range gjson.Get(threadResult.Stdout, "data.messages").Array() {
			if strings.Contains(item.Get("content").String(), replyText) {
				found = true
				break
			}
		}
		require.True(t, found, "expected reply content in stdout:\n%s", threadResult.Stdout)
	})
}
