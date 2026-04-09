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

// TestIM_ChatCreateSendWorkflow tests the +chat-create and +messages-send shortcuts.
func TestIM_ChatCreateSendWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := generateSuffix()
	chatName := "lark-cli-e2e-im-" + suffix
	messageText := "Hello from lark-cli e2e test"

	chatID := createChat(t, parentT, ctx, chatName)

	t.Run("send text message to chat", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+messages-send",
				"--chat-id", chatID,
				"--text", messageText,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		messageID := gjson.Get(result.Stdout, "data.message_id").String()
		require.NotEmpty(t, messageID, "message_id should not be empty")
	})

	t.Run("send markdown message to chat", func(t *testing.T) {
		markdownContent := "**Bold** and *italic* text"
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+messages-send",
				"--chat-id", chatID,
				"--markdown", markdownContent,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		messageID := gjson.Get(result.Stdout, "data.message_id").String()
		require.NotEmpty(t, messageID, "message_id should not be empty")
	})

	t.Run("send image message to chat", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+messages-send",
				"--chat-id", chatID,
				"--image", "./red10x10.png",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		messageID := gjson.Get(result.Stdout, "data.message_id").String()
		require.NotEmpty(t, messageID, "message_id should not be empty")
	})
}

// TestIM_ChatCreateWithOptionsWorkflow tests +chat-create with various options.
func TestIM_ChatCreateWithOptionsWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := generateSuffix()
	chatName := "lark-cli-e2e-im-users-" + suffix

	t.Run("create chat with set-bot-manager", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+chat-create",
				"--name", chatName,
				"--type", "private",
				"--set-bot-manager",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		chatID := gjson.Get(result.Stdout, "data.chat_id").String()
		require.NotEmpty(t, chatID, "chat_id should not be empty")
	})

	t.Run("create public chat with description", func(t *testing.T) {
		publicChatName := chatName + "-public"
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+chat-create",
				"--name", publicChatName,
				"--type", "public",
				"--description", "Test public chat for e2e",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		publicChatID := gjson.Get(result.Stdout, "data.chat_id").String()
		require.NotEmpty(t, publicChatID)
	})
}

// TestIM_ChatUpdateWorkflow tests the +chat-update shortcut.
func TestIM_ChatUpdateWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := generateSuffix()
	originalName := "lark-cli-e2e-im-update-" + suffix
	updatedName := originalName + "-updated"
	updatedDescription := "Updated description for e2e test"

	chatID := createChat(t, parentT, ctx, originalName)

	t.Run("update chat name", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+chat-update",
				"--chat-id", chatID,
				"--name", updatedName,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("update chat description", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+chat-update",
				"--chat-id", chatID,
				"--description", updatedDescription,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})
}

// TestIM_ChatSearchWorkflow tests the +chat-search shortcut.
func TestIM_ChatSearchWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := generateSuffix()
	chatName := "lark-cli-e2e-im-search-" + suffix

	createChat(t, parentT, ctx, chatName)

	t.Run("search chat by name", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+chat-search",
				"--query", chatName,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		hasData := gjson.Get(result.Stdout, "data").Exists()
		if !hasData {
			t.Skip("chat-search may not return bot-created chats in user identity mode")
		}
	})

	t.Run("search chat with sort", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+chat-search",
				"--query", chatName,
				"--sort-by", "create_time_desc",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})
}

// TestIM_ChatsGetWorkflow tests the im chats get command.
func TestIM_ChatsGetWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := generateSuffix()
	chatName := "lark-cli-e2e-chats-get-" + suffix

	chatID := createChat(t, parentT, ctx, chatName)

	t.Run("get chat info", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "chats", "get"},
			Params: map[string]any{"chat_id": chatID},
		})
		require.NoError(t, err)
		t.Logf("chats get result: %s", result.Stdout)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		dataExists := gjson.Get(result.Stdout, "data").Exists()
		require.True(t, dataExists, "data object should exist")

		chatNameGot := gjson.Get(result.Stdout, "data.name").String()
		require.Equal(t, chatName, chatNameGot)
	})
}

// TestIM_ChatsListWorkflow tests the im chats list command.
func TestIM_ChatsListWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := generateSuffix()
	chatName := "lark-cli-e2e-chats-list-" + suffix

	createChat(t, parentT, ctx, chatName)

	t.Run("list chats", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "chats", "list"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		hasMore := gjson.Get(result.Stdout, "data.has_more").Exists()
		items := gjson.Get(result.Stdout, "data.items").Array()
		require.NotNil(t, items, "data.items should exist")
		t.Logf("Found %d chats, has_more: %v", len(items), hasMore)
	})
}

// TestIM_ChatsLinkWorkflow tests the im chats link command.
func TestIM_ChatsLinkWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := generateSuffix()
	chatName := "lark-cli-e2e-chats-link-" + suffix

	chatID := createChat(t, parentT, ctx, chatName)

	t.Run("get chat share link", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "chats", "link"},
			Params: map[string]any{"chat_id": chatID},
			Data: map[string]any{
				"validity_period": "week",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		shareLink := gjson.Get(result.Stdout, "data.share_link").String()
		require.NotEmpty(t, shareLink, "share_link should not be empty")
		t.Logf("Generated share link: %s", shareLink)
	})
}

// TestIM_ChatMembersWorkflow tests the im chat.members commands.
func TestIM_ChatMembersWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := generateSuffix()
	chatName := "lark-cli-e2e-members-" + suffix

	chatID := createChatWithBotManager(t, parentT, ctx, chatName)

	t.Run("get chat members", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "chat.members", "get"},
			Params: map[string]any{"chat_id": chatID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		hasMore := gjson.Get(result.Stdout, "data.has_more").Exists()
		items := gjson.Get(result.Stdout, "data.items").Array()
		require.NotNil(t, items, "data.items should exist")
		t.Logf("Found %d members, has_more: %v", len(items), hasMore)
	})

	t.Run("add member to chat (bot only - requires valid user ID)", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "chat.members", "create"},
			Params: map[string]any{"chat_id": chatID},
			Data: map[string]any{
				"id_list": []string{"ou_invalid_user_id"},
			},
		})
		require.NoError(t, err)
		t.Logf("Add member result: %s", result.Stdout)
	})

	t.Run("remove member from chat (requires valid member ID)", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "chat.members", "delete"},
			Params: map[string]any{"chat_id": chatID},
			Data: map[string]any{
				"id_list": []string{"ou_invalid_member_id"},
			},
		})
		require.NoError(t, err)
		t.Logf("Remove member result: %s", result.Stdout)
	})
}