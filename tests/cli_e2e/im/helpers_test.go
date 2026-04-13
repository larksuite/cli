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
		// No IM chat delete command is currently available in lark-cli,
		// so created chats are intentionally left in the test account.
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
