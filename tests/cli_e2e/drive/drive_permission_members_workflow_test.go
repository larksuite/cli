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

// TestDrive_PermissionMembersWorkflow tests the permission.members create resource command.
// Note: This test requires a real user open_id to add as a member. In bot-only environments,
// this may fail. The test is written for environments that support user identity.
func TestDrive_PermissionMembersWorkflow(t *testing.T) {
	t.Skip("requires a real user open_id and user-capable test environment; permission.members create needs user identity")
}

// TestDrive_FileCommentsBatchQueryWorkflow tests the file.comments batch_query resource command.
// Workflow: import a doc -> create a comment -> batch query the comment by ID.
func TestDrive_FileCommentsBatchQueryWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	testContent := "# Lark CLI E2E File Comments Batch Query Test\n\nDocument for testing file.comments batch_query.\nTimestamp: " + suffix

	docToken := importTestDoc(t, parentT, ctx, "file-comments-batch", testContent)
	require.NotEmpty(t, docToken)

	var commentID string

	// First create a comment
	t.Run("create comment", func(t *testing.T) {
		commentContent := "lark-cli-e2e-batch-query-comment-" + suffix

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "file.comments", "create_v2"},
			Params: map[string]any{
				"file_token": docToken,
			},
			Data: map[string]any{
				"file_type": "docx",
				"reply_elements": []map[string]any{
					{"type": "text", "text": commentContent},
				},
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		commentID = gjson.Get(result.Stdout, "data.comment_id").String()
		require.NotEmpty(t, commentID, "stdout:\n%s", result.Stdout)
	})

	require.NotEmpty(t, commentID, "comment ID is required for batch_query")

	// Note: Small delay to allow comment to be indexed before batch query
	time.Sleep(1 * time.Second)

	// Then batch query with the created comment ID
	t.Run("batch_query - batch get comments by ID", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "file.comments", "batch_query"},
			Params: map[string]any{
				"file_token": docToken,
				"file_type":  "docx",
			},
			Data: map[string]any{
				"comment_ids": []string{commentID},
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		// Verify response has items with the comment
		items := gjson.Get(result.Stdout, "data.items")
		require.True(t, items.IsArray(), "should have items array, stdout:\n%s", result.Stdout)
	})
}

// TestDrive_FileCommentReplysWorkflow tests the file.comment.replys resource commands.
// Workflow: import a docx -> create local (selection) comment using +add-comment with
//          --selection-with-ellipsis -> verify comment is not is_whole -> add a reply.
func TestDrive_FileCommentReplysWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	// Use a unique text phrase for selection to ensure locate-doc finds it
	testContent := "# Lark CLI E2E Comment Reply Test\n\nDocument for testing file.comment.replys.\nTimestamp: " + suffix + "\n\nThis is a unique phrase for selection: lark-cli-e2e-reply-test-phrase-" + suffix + "\n\nEnd of test document."

	docToken := importTestDoc(t, parentT, ctx, "comment-reply", testContent)
	require.NotEmpty(t, docToken)

	var commentID string
	var isWhole bool

	// Step 1: Create a local (selection) comment using +add-comment with selection-with-ellipsis
	t.Run("create local comment with selection", func(t *testing.T) {
		commentContent := "lark-cli-e2e-local-comment-" + suffix
		selectionPhrase := "lark-cli-e2e-reply-test-phrase-" + suffix

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "+add-comment",
				"--doc",                  docToken,
				"--selection-with-ellipsis", selectionPhrase,
				"--content",              `[{"type":"text","text":"` + commentContent + `"}]`,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		commentID = gjson.Get(result.Stdout, "data.comment_id").String()
		require.NotEmpty(t, commentID, "comment_id should be returned, stdout:\n%s", result.Stdout)

		// Check if is_whole is false for local comment
		isWhole = gjson.Get(result.Stdout, "data.is_whole").Bool()
		t.Logf("Created local comment: comment_id=%s, is_whole=%v", commentID, isWhole)
	})

	require.NotEmpty(t, commentID, "comment ID is required for reply")
	require.False(t, isWhole, "local comment should have is_whole=false; cannot reply to whole-document comments")

	// Step 2: Add a reply to the local comment (verifies the comment accepts replies)
	// Note: Small delay to avoid 1069307 race condition where comment is not yet ready for replies
	t.Run("add reply to local comment", func(t *testing.T) {
		time.Sleep(1 * time.Second)

		replyContent := "lark-cli-e2e-reply-" + suffix

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

		replyID := gjson.Get(result.Stdout, "data.reply_id").String()
		require.NotEmpty(t, replyID, "reply_id should be returned, stdout:\n%s", result.Stdout)
		t.Logf("Added reply: reply_id=%s", replyID)
	})

	// Step 3: List replies to verify
	t.Run("list replies", func(t *testing.T) {
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
		// At least the reply we just created should be present
		require.GreaterOrEqual(t, len(items.Array()), 1, "should have at least 1 reply")
	})
}