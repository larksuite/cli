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

// TestIM_MessagesMgetWorkflow tests the +messages-mget shortcut.
func TestIM_MessagesMgetWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := generateSuffix()
	chatName := "lark-cli-e2e-im-mget-" + suffix
	messageText := "Message for mget test"

	chatID := createChat(t, parentT, ctx, chatName)
	messageID := sendMessage(t, parentT, ctx, chatID, messageText)

	t.Run("batch get messages by ID", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+messages-mget",
				"--message-ids", messageID,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		messages := gjson.Get(result.Stdout, "data").Array()
		require.NotEmpty(t, messages, "should get at least one message")
	})
}

// TestIM_MessagesReplyWorkflow tests the +messages-reply shortcut.
func TestIM_MessagesReplyWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := generateSuffix()
	chatName := "lark-cli-e2e-im-reply-" + suffix
	originalMessage := "Original message for reply test"
	replyText := "This is a reply"

	chatID := createChat(t, parentT, ctx, chatName)
	messageID := sendMessage(t, parentT, ctx, chatID, originalMessage)

	t.Run("reply to message with text", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+messages-reply",
				"--message-id", messageID,
				"--text", replyText,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("reply to message with markdown", func(t *testing.T) {
		markdownReply := "**Bold** reply"
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+messages-reply",
				"--message-id", messageID,
				"--markdown", markdownReply,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})
}
