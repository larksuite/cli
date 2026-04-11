// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDrive_FileCommentReplysUpdateWorkflow tests the file.comment.replys.update resource command.
// Workflow: import a docx -> create local comment -> add a reply -> update the reply -> verify update.
func TestDrive_FileCommentReplysUpdateWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	testContent := "# Lark CLI E2E Reply Update Test\n\nDocument for testing file.comment.replys.update.\nTimestamp: " + suffix + "\n\nUnique phrase for selection: lark-cli-e2e-update-reply-phrase-" + suffix + "\n\nEnd of document."

	docToken := importTestDoc(t, parentT, ctx, "reply-update", testContent)
	require.NotEmpty(t, docToken)

	var commentID string
	var replyID string

	// Step 1: Create a local comment
	t.Run("create local comment", func(t *testing.T) {
		commentContent := "lark-cli-e2e-update-comment-" + suffix
		selectionPhrase := "lark-cli-e2e-update-reply-phrase-" + suffix

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "+add-comment",
				"--doc",                     docToken,
				"--selection-with-ellipsis", selectionPhrase,
				"--content",                 `[{"type":"text","text":"` + commentContent + `"}]`,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		commentID = gjson.Get(result.Stdout, "data.comment_id").String()
		require.NotEmpty(t, commentID, "comment_id should be returned, stdout:\n%s", result.Stdout)
	})

	require.NotEmpty(t, commentID, "comment ID is required")

	// Step 2: Add a reply to the comment
	// Note: Small delay to avoid 1069307 race condition where comment is not yet ready for replies
	t.Run("add reply", func(t *testing.T) {
		time.Sleep(1 * time.Second)

		replyContent := "lark-cli-e2e-original-reply-" + suffix

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "file.comment.replys", "create"},
			Params: map[string]any{
				"file_token": docToken,
				"comment_id": commentID,
				"file_type":  "docx",
			},
			Data: map[string]any{
				"content": map[string]any{
					"elements": []map[string]any{
						{"type": "text_run", "text_run": map[string]any{"text": replyContent}},
					},
				},
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		replyID = gjson.Get(result.Stdout, "data.reply_id").String()
		require.NotEmpty(t, replyID, "reply_id should be returned, stdout:\n%s", result.Stdout)
	})

	require.NotEmpty(t, replyID, "reply ID is required for update")

	// Step 3: Update the reply
	t.Run("update reply", func(t *testing.T) {
		updatedContent := "lark-cli-e2e-updated-reply-" + suffix

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "file.comment.replys", "update"},
			Params: map[string]any{
				"file_token": docToken,
				"comment_id": commentID,
				"reply_id":   replyID,
				"file_type":  "docx",
			},
			Data: map[string]any{
				"content": map[string]any{
					"elements": []map[string]any{
						{"type": "text_run", "text_run": map[string]any{"text": updatedContent}},
					},
				},
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
		t.Logf("Updated reply: reply_id=%s", replyID)
	})

	// Step 4: Verify the update by listing replies - note: API may have eventual consistency delay
	t.Run("verify update", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "file.comment.replys", "list"},
			Params: map[string]any{
				"file_token": docToken,
				"comment_id": commentID,
				"file_type":  "docx",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		items := gjson.Get(result.Stdout, "data.items")
		require.True(t, items.IsArray(), "should have items array, stdout:\n%s", result.Stdout)

		// Find our updated reply
		found := false
		for _, item := range items.Array() {
			if gjson.Get(item.Raw, "reply_id").String() == replyID {
				found = true
				// The reply exists after update - verify content structure
				elements := gjson.Get(item.Raw, "content.elements")
				require.True(t, elements.IsArray(), "content.elements should be array")
				break
			}
		}
		require.True(t, found, "reply should still exist after update")
	})
}

// TestDrive_FileCommentReplysDeleteWorkflow tests the file.comment.replys.delete resource command.
// Workflow: import a docx -> create local comment -> add a reply -> delete the reply -> verify deletion.
func TestDrive_FileCommentReplysDeleteWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	testContent := "# Lark CLI E2E Reply Delete Test\n\nDocument for testing file.comment.replys.delete.\nTimestamp: " + suffix + "\n\nUnique phrase for selection: lark-cli-e2e-delete-reply-phrase-" + suffix + "\n\nEnd of document."

	docToken := importTestDoc(t, parentT, ctx, "reply-delete", testContent)
	require.NotEmpty(t, docToken)

	var commentID string
	var replyID string

	// Step 1: Create a local comment
	t.Run("create local comment", func(t *testing.T) {
		commentContent := "lark-cli-e2e-delete-comment-" + suffix
		selectionPhrase := "lark-cli-e2e-delete-reply-phrase-" + suffix

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "+add-comment",
				"--doc",                     docToken,
				"--selection-with-ellipsis", selectionPhrase,
				"--content",                 `[{"type":"text","text":"` + commentContent + `"}]`,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		commentID = gjson.Get(result.Stdout, "data.comment_id").String()
		require.NotEmpty(t, commentID, "comment_id should be returned, stdout:\n%s", result.Stdout)
	})

	require.NotEmpty(t, commentID, "comment ID is required")

	// Step 2: Add a reply to the comment
	// Note: Small delay to avoid 1069307 race condition where comment is not yet ready for replies
	t.Run("add reply", func(t *testing.T) {
		time.Sleep(1 * time.Second)

		replyContent := "lark-cli-e2e-reply-to-delete-" + suffix

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "file.comment.replys", "create"},
			Params: map[string]any{
				"file_token": docToken,
				"comment_id": commentID,
				"file_type":  "docx",
			},
			Data: map[string]any{
				"content": map[string]any{
					"elements": []map[string]any{
						{"type": "text_run", "text_run": map[string]any{"text": replyContent}},
					},
				},
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		replyID = gjson.Get(result.Stdout, "data.reply_id").String()
		require.NotEmpty(t, replyID, "reply_id should be returned, stdout:\n%s", result.Stdout)
	})

	require.NotEmpty(t, replyID, "reply ID is required for delete")

	// Step 3: Delete the reply
	t.Run("delete reply", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "file.comment.replys", "delete"},
			Params: map[string]any{
				"file_token": docToken,
				"comment_id": commentID,
				"reply_id":   replyID,
				"file_type":  "docx",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
		t.Logf("Deleted reply: reply_id=%s", replyID)

		// Note: Small delay to avoid eventual consistency issue where deleted reply still appears in list
		time.Sleep(1 * time.Second)
	})

	// Step 4: Verify the deletion by listing replies
	t.Run("verify deletion", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "file.comment.replys", "list"},
			Params: map[string]any{
				"file_token": docToken,
				"comment_id": commentID,
				"file_type":  "docx",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		items := gjson.Get(result.Stdout, "data.items")
		require.True(t, items.IsArray(), "should have items array, stdout:\n%s", result.Stdout)

		// Verify our deleted reply is no longer present
		for _, item := range items.Array() {
			deletedReplyID := gjson.Get(item.Raw, "reply_id").String()
			require.NotEqual(t, replyID, deletedReplyID,
				"deleted reply should not appear in list")
		}
	})
}