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

// TestIM_ReactionsWorkflow tests the im.reactions commands.
func TestIM_ReactionsWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := generateSuffix()
	chatName := "lark-cli-e2e-reactions-" + suffix

	chatID := createChat(t, parentT, ctx, chatName)
	messageID := sendMessage(t, parentT, ctx, chatID, "Message for reactions test")

	t.Run("list reactions for a message", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "reactions", "list"},
			Params: map[string]any{"message_id": messageID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		hasMore := gjson.Get(result.Stdout, "data.has_more").Exists()
		items := gjson.Get(result.Stdout, "data.items").Array()
		t.Logf("Found %d reactions, has_more: %v", len(items), hasMore)
	})

	t.Run("add a reaction to a message", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "reactions", "create"},
			Params: map[string]any{"message_id": messageID},
			Data: map[string]any{
				"reaction_type": map[string]any{
					"emoji_type": "SMILE",
				},
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		reactionID := gjson.Get(result.Stdout, "data.reaction_id").String()
		t.Logf("Created reaction: %s", reactionID)
	})

	t.Run("batch query reactions", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "reactions", "batch_query"},
			Data: map[string]any{
				"queries": []map[string]any{
					{"message_id": messageID},
				},
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		t.Logf("Batch query result: %s", result.Stdout)
	})

	t.Run("delete a reaction", func(t *testing.T) {
		listResult, listErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "reactions", "list"},
			Params: map[string]any{"message_id": messageID},
		})
		require.NoError(t, listErr)

		t.Logf("Reactions list for deletion: %s", listResult.Stdout)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "reactions", "delete"},
			Params: map[string]any{
				"message_id": messageID,
				"reaction_id": "invalid_reaction_id",
			},
		})
		require.NoError(t, err)
		t.Logf("Delete reaction result: %s", result.Stdout)
	})
}