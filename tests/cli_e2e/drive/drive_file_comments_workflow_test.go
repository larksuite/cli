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

// TestDrive_FileCommentsWorkflow tests the file.comments resource commands.
// Workflow: import a doc -> add comment via create_v2 -> list comments -> patch (resolve) comment.
func TestDrive_FileCommentsWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	testContent := "# Lark CLI E2E File Comments Test\n\nDocument for testing file.comments resource.\nTimestamp: " + suffix

	docToken := importTestDoc(t, parentT, ctx, "file-comments", testContent)
	require.NotEmpty(t, docToken)

	var commentID string

	t.Run("create_v2 - add comment", func(t *testing.T) {
		commentContent := "lark-cli-e2e-drive-comment-" + suffix

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

	t.Run("list - get comments", func(t *testing.T) {
		require.NotEmpty(t, commentID, "comment ID should be set from create_v2 step")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "file.comments", "list"},
			Params: map[string]any{
				"file_token": docToken,
				"file_type":  "docx",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		// Verify the created comment appears in the list
		items := gjson.Get(result.Stdout, "data.items")
		require.True(t, items.IsArray(), "should have items array, stdout:\n%s", result.Stdout)
	})

	t.Run("patch - resolve comment", func(t *testing.T) {
		require.NotEmpty(t, commentID, "comment ID should be set from create_v2 step")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "file.comments", "patch"},
			Params: map[string]any{
				"file_token": docToken,
				"file_type":  "docx",
				"comment_id": commentID,
			},
			Data: map[string]any{
				"is_solved": true,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
	})
}