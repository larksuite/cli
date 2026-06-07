// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// createChat creates a private chat with the given name and returns the chatID.
// The chat will be automatically cleaned up via parentT.Cleanup().
// Note: +chat-disband requires im:chat:delete, which is not guaranteed for all
// IM E2E credentials, so shared helpers avoid global chat cleanup side effects.
func createChat(t *testing.T, parentT *testing.T, ctx context.Context, name string) string {
	t.Helper()
	return createChatAs(t, parentT, ctx, name, "bot")
}

func createChatAs(t *testing.T, parentT *testing.T, ctx context.Context, name string, defaultAs string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"im", "+chat-create",
			"--name", name,
			"--type", "private",
		},
		DefaultAs: defaultAs,
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	chatID := gjson.Get(result.Stdout, "data.chat_id").String()
	require.NotEmpty(t, chatID, "chat_id should not be empty")

	parentT.Cleanup(func() {
		// Intentionally left blank. Tests that specifically cover chat disband
		// own their cleanup path and required scope.
	})

	return chatID
}

// sendMessage sends a text message to the specified chat and returns the messageID.
func sendMessage(t *testing.T, ctx context.Context, chatID string, text string) string {
	t.Helper()
	return sendMessageAs(t, ctx, chatID, text, "bot")
}

func sendMessageAs(t *testing.T, ctx context.Context, chatID string, text string, defaultAs string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"im", "+messages-send",
			"--chat-id", chatID,
			"--text", text,
		},
		DefaultAs: defaultAs,
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	messageID := gjson.Get(result.Stdout, "data.message_id").String()
	require.NotEmpty(t, messageID, "message_id should not be empty")

	return messageID
}
