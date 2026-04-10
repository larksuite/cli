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

// createChat creates a private chat with the given name and returns the chatID.
// The chat will be automatically cleaned up via parentT.Cleanup().
// Note: Chat deletion is not available via lark-cli im command.
func createChat(t *testing.T, parentT *testing.T, ctx context.Context, name string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"im", "+chat-create",
			"--name", name,
			"--type", "private",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	chatID := gjson.Get(result.Stdout, "data.chat_id").String()
	require.NotEmpty(t, chatID, "chat_id should not be empty")

	parentT.Cleanup(func() {
		// Best-effort cleanup - chat will be automatically orphaned
		// since im chats delete command is not available
	})

	return chatID
}

// createChatWithBotManager creates a private chat with bot as manager and returns the chatID.
func createChatWithBotManager(t *testing.T, parentT *testing.T, ctx context.Context, name string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"im", "+chat-create",
			"--name", name,
			"--type", "private",
			"--set-bot-manager",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	chatID := gjson.Get(result.Stdout, "data.chat_id").String()
	require.NotEmpty(t, chatID, "chat_id should not be empty")

	parentT.Cleanup(func() {
		// Best-effort cleanup - chat will be automatically orphaned
	})

	return chatID
}

// sendMessage sends a text message to the specified chat and returns the messageID.
func sendMessage(t *testing.T, parentT *testing.T, ctx context.Context, chatID string, text string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"im", "+messages-send",
			"--chat-id", chatID,
			"--text", text,
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	messageID := gjson.Get(result.Stdout, "data.message_id").String()
	require.NotEmpty(t, messageID, "message_id should not be empty")

	return messageID
}

// sendMarkdown sends a markdown message to the specified chat and returns the messageID.
func sendMarkdown(t *testing.T, parentT *testing.T, ctx context.Context, chatID string, markdown string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"im", "+messages-send",
			"--chat-id", chatID,
			"--markdown", markdown,
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	messageID := gjson.Get(result.Stdout, "data.message_id").String()
	require.NotEmpty(t, messageID, "message_id should not be empty")

	return messageID
}

// sendImage sends an image message to the specified chat and returns the messageID.
func sendImage(t *testing.T, parentT *testing.T, ctx context.Context, chatID string, imagePath string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"im", "+messages-send",
			"--chat-id", chatID,
			"--image", imagePath,
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	messageID := gjson.Get(result.Stdout, "data.message_id").String()
	require.NotEmpty(t, messageID, "message_id should not be empty")

	return messageID
}

// replyMessage sends a reply to a message and returns the reply messageID.
func replyMessage(t *testing.T, parentT *testing.T, ctx context.Context, messageID string, text string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"im", "+messages-reply",
			"--message-id", messageID,
			"--text", text,
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	replyMessageID := gjson.Get(result.Stdout, "data.message_id").String()
	require.NotEmpty(t, replyMessageID, "reply message_id should not be empty")

	return replyMessageID
}

// replyInThread sends a reply in thread to a message and returns the reply messageID.
func replyInThread(t *testing.T, parentT *testing.T, ctx context.Context, messageID string, text string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"im", "+messages-reply",
			"--message-id", messageID,
			"--text", text,
			"--reply-in-thread",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	replyMessageID := gjson.Get(result.Stdout, "data.message_id").String()
	require.NotEmpty(t, replyMessageID, "reply message_id should not be empty")

	return replyMessageID
}

// generateSuffix generates a unique suffix based on current timestamp.
func generateSuffix() string {
	return time.Now().UTC().Format("20060102-150405")
}