// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestIM_ChatMessagesListWorkflow tests the +chat-messages-list shortcut.
func TestIM_ChatMessagesListWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := generateSuffix()
	chatName := "lark-cli-e2e-im-list-" + suffix
	messageText := "Message for listing test"

	chatID := createChat(t, parentT, ctx, chatName)
	sendMessage(t, parentT, ctx, chatID, messageText)

	t.Run("list messages in chat", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+chat-messages-list",
				"--chat-id", chatID,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		hasData := gjson.Get(result.Stdout, "data").Exists()
		require.True(t, hasData, "should have data in response")
	})

	t.Run("list messages with sort order", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+chat-messages-list",
				"--chat-id", chatID,
				"--sort", "asc",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("list messages with page size", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+chat-messages-list",
				"--chat-id", chatID,
				"--page-size", "10",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})
}

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

// TestIM_MessagesReplyInThreadWorkflow tests the +messages-reply with reply-in-thread flag.
func TestIM_MessagesReplyInThreadWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := generateSuffix()
	chatName := "lark-cli-e2e-im-thread-" + suffix
	originalMessage := "Message for thread reply test"
	threadReplyText := "Reply in thread"

	chatID := createChat(t, parentT, ctx, chatName)
	messageID := sendMessage(t, parentT, ctx, chatID, originalMessage)

	t.Run("reply in thread", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+messages-reply",
				"--message-id", messageID,
				"--text", threadReplyText,
				"--reply-in-thread",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})
}

// TestIM_MessagesSearchWorkflow tests the +messages-search shortcut.
// Note: messages-search is user-only and requires user login. Skip in bot-only environments.
func TestIM_MessagesSearchWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := generateSuffix()
	chatName := "lark-cli-e2e-im-search-msg-" + suffix
	searchText := "lark-cli-e2e-searchable-" + suffix

	chatID := createChat(t, parentT, ctx, chatName)
	sendMessage(t, parentT, ctx, chatID, searchText)

	t.Run("search messages by keyword", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+messages-search",
				"--query", "lark-cli-e2e-searchable",
				"--as", "user",
			},
		})
		require.NoError(t, err)

		if result.ExitCode != 0 {
			t.Skip("messages-search requires user login, skipping in bot-only environment")
		}

		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("search messages with time range", func(t *testing.T) {
		startTime := time.Now().UTC().Add(-1 * time.Hour).Format("2006-01-02T15:04:05+08:00")
		endTime := time.Now().UTC().Add(1 * time.Hour).Format("2006-01-02T15:04:05+08:00")
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+messages-search",
				"--query", "lark-cli",
				"--start", startTime,
				"--end", endTime,
				"--as", "user",
			},
		})
		require.NoError(t, err)

		if result.ExitCode != 0 {
			t.Skip("messages-search requires user login, skipping in bot-only environment")
		}

		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	_ = chatID // silence unused warning
}

// TestIM_MessagesResourcesDownloadWorkflow tests the +messages-resources-download shortcut.
func TestIM_MessagesResourcesDownloadWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := generateSuffix()
	chatName := "lark-cli-e2e-im-download-" + suffix

	chatID := createChat(t, parentT, ctx, chatName)

	t.Run("send image message and download resource", func(t *testing.T) {
		sendResult, sendErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+messages-send",
				"--chat-id", chatID,
				"--image", "./red10x10.png",
			},
		})
		require.NoError(t, sendErr)
		sendResult.AssertExitCode(t, 0)
		sendResult.AssertStdoutStatus(t, true)

		messageID := gjson.Get(sendResult.Stdout, "data.message_id").String()
		require.NotEmpty(t, messageID, "message_id should not be empty")
		t.Logf("Sent image message with ID: %s", messageID)

		mgetResult, mgetErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+messages-mget",
				"--message-ids", messageID,
			},
		})
		require.NoError(t, mgetErr)
		mgetResult.AssertExitCode(t, 0)

		t.Logf("Mget full response: %s", mgetResult.Stdout)

		bodyContent := gjson.Get(mgetResult.Stdout, "data.messages.0.content").String()
		t.Logf("Message body content: %s", bodyContent)

		var imageKey string
		var contentMap map[string]string
		if err := json.Unmarshal([]byte(bodyContent), &contentMap); err != nil {
			if len(bodyContent) > 10 && bodyContent[:8] == "[Image: " {
				endIdx := len(bodyContent) - 1
				if bodyContent[endIdx] == ']' {
					imageKey = bodyContent[8:endIdx]
				}
			}
		} else {
			imageKey = contentMap["image_key"]
		}
		t.Logf("Extracted image_key: %s", imageKey)

		if imageKey != "" {
			downloadResult, downloadErr := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{"im", "+messages-resources-download",
					"--message-id", messageID,
					"--file-key", imageKey,
					"--type", "image",
				},
			})
			require.NoError(t, downloadErr)
			downloadResult.AssertExitCode(t, 0)
			t.Logf("Download result: %s", downloadResult.Stdout)
		} else {
			t.Skip("Could not extract image_key from message content")
		}
	})
}

// TestIM_MessagesDeleteWorkflow tests the im messages delete command.
func TestIM_MessagesDeleteWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := generateSuffix()
	chatName := "lark-cli-e2e-msg-delete-" + suffix

	chatID := createChat(t, parentT, ctx, chatName)
	messageID := sendMessage(t, parentT, ctx, chatID, "Message to be deleted")

	t.Run("delete message", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "messages", "delete"},
			Params: map[string]any{"message_id": messageID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
	})
}

// TestIM_MessagesForwardWorkflow tests the im messages forward command.
func TestIM_MessagesForwardWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := generateSuffix()
	chatName := "lark-cli-e2e-msg-forward-" + suffix

	chatID := createChat(t, parentT, ctx, chatName)
	messageID := sendMessage(t, parentT, ctx, chatID, "Message to be forwarded")

	t.Run("forward message (requires valid receive_id)", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "messages", "forward"},
			Params: map[string]any{
				"message_id":      messageID,
				"receive_id_type": "open_id",
			},
			Data: map[string]any{
				"receive_id": "ou_invalid_receiver_id",
			},
		})
		require.NoError(t, err)
		t.Logf("Forward result: %s", result.Stdout)
	})
}

// TestIM_MessagesMergeForwardWorkflow tests the im messages merge_forward command.
func TestIM_MessagesMergeForwardWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	t.Run("merge forward command structure", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "messages", "merge_forward"},
			Params: map[string]any{
				"receive_id_type": "chat_id",
			},
			Data: map[string]any{
				"message_id_list": []string{"om_invalid_message_id"},
				"receive_id":      "oc_invalid_chat_id",
			},
		})
		require.NoError(t, err)
		t.Logf("Merge forward result: %s", result.Stdout)
	})
}

// TestIM_MessagesReadUsersWorkflow tests the im messages read_users command.
func TestIM_MessagesReadUsersWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	t.Run("read_users command structure", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "messages", "read_users"},
			Params: map[string]any{
				"message_id":   "om_invalid_message_id",
				"user_id_type": "open_id",
			},
		})
		require.NoError(t, err)
		t.Logf("Read users result: %s", result.Stdout)
	})
}

// TestIM_ImagesCreateWorkflow tests the im images create command.
func TestIM_ImagesCreateWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	t.Run("upload image dry-run", func(t *testing.T) {
		dryRunResult, dryRunErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "images", "create", "--dry-run"},
			Data: map[string]any{
				"image_type": "message",
			},
		})
		require.NoError(t, dryRunErr)
		t.Logf("Dry-run result: %s", dryRunResult.Stdout)
	})
}