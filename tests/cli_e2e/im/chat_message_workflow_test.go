// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIM_ChatMessageWorkflowAsUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	parentT := t
	suffix := clie2e.GenerateSuffix()
	chatName := "im-chat-" + suffix
	messageText := "im-chat-msg-" + suffix
	var chatID string
	var messageID string

	t.Run("create chat as user", func(t *testing.T) {
		chatID = createChatAs(t, parentT, ctx, chatName, "user")
	})

	t.Run("send message as user", func(t *testing.T) {
		messageID = sendMessageAs(t, ctx, chatID, messageText, "user")
	})

	t.Run("list chat messages as user", func(t *testing.T) {
		startTime := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
		endTime := time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339)

		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{
				"im", "+chat-messages-list",
				"--chat-id", chatID,
				"--start", startTime,
				"--end", endTime,
			},
			DefaultAs: "user",
		}, clie2e.RetryOptions{
			ShouldRetry: func(result *clie2e.Result) bool {
				if result == nil || result.ExitCode != 0 {
					return true
				}
				for _, item := range gjson.Get(result.Stdout, "data.messages").Array() {
					if item.Get("message_id").String() == messageID && strings.Contains(item.Get("content").String(), messageText) {
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
		for _, item := range gjson.Get(result.Stdout, "data.messages").Array() {
			if item.Get("message_id").String() != messageID {
				continue
			}
			require.True(t, strings.Contains(item.Get("content").String(), messageText), "stdout:\n%s", result.Stdout)
			found = true
			break
		}
		require.True(t, found, "expected message %s in stdout:\n%s", messageID, result.Stdout)
	})
}
