// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

// TestIM_ThreadsMessagesListWorkflow tests the +threads-messages-list shortcut.
func TestIM_ThreadsMessagesListWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := generateSuffix()
	chatName := "lark-cli-e2e-im-threads-" + suffix
	originalMessage := "Message for thread test"
	threadReplyText := "Reply in thread"

	chatID := createChat(t, parentT, ctx, chatName)
	messageID := sendMessage(t, parentT, ctx, chatID, originalMessage)

	t.Run("setup thread with reply", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+messages-reply",
				"--message-id", messageID,
				"--text", threadReplyText,
				"--reply-in-thread",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
	})

	t.Run("list thread messages", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+threads-messages-list",
				"--thread", messageID,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("list thread messages with asc sort", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+threads-messages-list",
				"--thread", messageID,
				"--sort", "asc",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("list thread messages with page size", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+threads-messages-list",
				"--thread", messageID,
				"--page-size", "10",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})
}