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

// TestIM_PinsWorkflow tests the im.pins commands.
func TestIM_PinsWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := generateSuffix()
	chatName := "lark-cli-e2e-pins-" + suffix

	chatID := createChat(t, parentT, ctx, chatName)
	messageID := sendMessage(t, parentT, ctx, chatID, "Message to be pinned")

	t.Run("pin a message", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "pins", "create"},
			Data: map[string]any{
				"message_id": messageID,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
	})

	t.Run("list pinned messages", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "pins", "list"},
			Params: map[string]any{"chat_id": chatID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		hasMore := gjson.Get(result.Stdout, "data.has_more").Exists()
		items := gjson.Get(result.Stdout, "data.items").Array()
		t.Logf("Found %d pinned messages, has_more: %v", len(items), hasMore)
	})

	t.Run("unpin a message", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "pins", "delete"},
			Params: map[string]any{"message_id": messageID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
	})
}