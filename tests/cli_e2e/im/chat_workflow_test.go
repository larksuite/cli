// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestIM_ChatUpdateWorkflow tests the +chat-update shortcut.
func TestIM_ChatUpdateWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
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

	t.Run("get updated chat", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:   []string{"im", "chats", "get"},
			Params: map[string]any{"chat_id": chatID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		assert.Equal(t, updatedName, gjson.Get(result.Stdout, "data.name").String())
		assert.Equal(t, updatedDescription, gjson.Get(result.Stdout, "data.description").String())
	})
}

// TestIM_ChatsGetWorkflow tests the im chats get command.
func TestIM_ChatsGetWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	chatName := "lark-cli-e2e-chats-get-" + suffix

	chatID := createChat(t, parentT, ctx, chatName)

	t.Run("get chat info", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:   []string{"im", "chats", "get"},
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

// TestIM_ChatsLinkWorkflow tests the im chats link command.
func TestIM_ChatsLinkWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	chatName := "lark-cli-e2e-chats-link-" + suffix

	chatID := createChat(t, parentT, ctx, chatName)

	t.Run("get chat share link", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:   []string{"im", "chats", "link"},
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
