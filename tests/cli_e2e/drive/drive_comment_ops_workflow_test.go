// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"os"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDriveCommentOpsWorkflow proves the comment-operation shortcuts
// (+batch-query-comments, +add-reply, +list-replies, +update-reply,
// +react-reply, +resolve-comment, +restore-comment, +delete-reply) against
// the live API in one self-contained flow, sharing the
// LARK_DRIVE_MD_COMMENT_E2E gate with the file-comment workflow in
// drive_add_comment_workflow_test.go (both write comments on a temporary
// supported file).
//
// Sequencing matters: the reply is created before resolving because solved
// comments reject replies, and state flips are separated by polling reads
// because back-to-back PATCHes on one comment can hit rate limiting.
func TestDriveCommentOpsWorkflow(t *testing.T) {
	if os.Getenv("LARK_DRIVE_MD_COMMENT_E2E") == "" {
		t.Skip("set LARK_DRIVE_MD_COMMENT_E2E=1 to run the comment operations workflow")
	}

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	fileName := "lark-cli-e2e-drive-comment-ops-" + suffix + ".md"

	// --- Create: fixture file + fixture comment ---
	createResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"markdown", "+create",
			"--name", fileName,
			"--content", "# Comment ops target\n\nbody\n",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	createResult.AssertExitCode(t, 0)
	fileToken := gjson.Get(createResult.Stdout, "data.file_token").String()
	require.NotEmpty(t, fileToken, "stdout:\n%s", createResult.Stdout)

	parentT.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args: []string{
				"drive", "+delete",
				"--file-token", fileToken,
				"--type", "file",
				"--yes",
			},
			DefaultAs: "bot",
		})
		clie2e.ReportCleanupFailure(parentT, "delete comment ops target "+fileToken, deleteResult, deleteErr)
	})

	commentResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{
			"drive", "+add-comment",
			"--doc", fileToken,
			"--type", "file",
			"--content", `[{"type":"text","text":"comment ops fixture"}]`,
		},
		DefaultAs: "bot",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	commentResult.AssertExitCode(t, 0)
	commentID := gjson.Get(commentResult.Stdout, "data.comment_id").String()
	require.NotEmpty(t, commentID, "stdout:\n%s", commentResult.Stdout)

	// --- Use: +batch-query-comments finds the fixture by ID ---
	batchArgs := []string{
		"drive", "+batch-query-comments",
		"--token", fileToken,
		"--type", "file",
		"--comment-ids", commentID,
	}
	batchResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      batchArgs,
		DefaultAs: "bot",
	}, clie2e.RetryOptions{
		ShouldRetry: func(result *clie2e.Result) bool {
			return result == nil || result.ExitCode != 0 || !driveCommentListContainsID(result.Stdout, commentID)
		},
	})
	require.NoError(t, err)
	batchResult.AssertExitCode(t, 0)
	require.True(t, driveCommentListContainsID(batchResult.Stdout, commentID), "stdout:\n%s", batchResult.Stdout)
	if got := gjson.Get(batchResult.Stdout, "data.file_type").String(); got != "file" {
		t.Fatalf("batch data.file_type=%q, want file\nstdout:\n%s", got, batchResult.Stdout)
	}
	fixture := driveCommentOpsItem(batchResult.Stdout, commentID)
	require.False(t, fixture.Get("is_solved").Bool(), "fixture must start unsolved\nstdout:\n%s", batchResult.Stdout)
	baseReplies := len(fixture.Get("reply_list.replies").Array())

	// --- Use: +add-reply attaches a reply under the fixture comment ---
	replyResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{
			"drive", "+add-reply",
			"--token", fileToken,
			"--type", "file",
			"--comment-id", commentID,
			"--content", `[{"type":"text","text":"comment ops reply"}]`,
		},
		DefaultAs: "bot",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	replyResult.AssertExitCode(t, 0)
	replyID := gjson.Get(replyResult.Stdout, "data.reply_id").String()
	require.NotEmpty(t, replyID, "stdout:\n%s", replyResult.Stdout)

	driveCommentOpsAwaitReplies(t, ctx, batchArgs, commentID, baseReplies+1)

	// --- Use: +list-replies surfaces the created reply ---
	listRepliesArgs := []string{
		"drive", "+list-replies",
		"--token", fileToken,
		"--type", "file",
		"--comment-id", commentID,
	}
	listResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      listRepliesArgs,
		DefaultAs: "bot",
	}, clie2e.RetryOptions{
		ShouldRetry: func(result *clie2e.Result) bool {
			return result == nil || result.ExitCode != 0 || !driveCommentOpsReplyItem(result.Stdout, replyID).Exists()
		},
	})
	require.NoError(t, err)
	listResult.AssertExitCode(t, 0)
	require.True(t, driveCommentOpsReplyItem(listResult.Stdout, replyID).Exists(), "stdout:\n%s", listResult.Stdout)
	if got := gjson.Get(listResult.Stdout, "data.comment_id").String(); got != commentID {
		t.Fatalf("list data.comment_id=%q, want %s\nstdout:\n%s", got, commentID, listResult.Stdout)
	}

	// --- Use: +update-reply rewrites the created reply's content ---
	updatedText := "comment ops reply updated " + suffix
	updateResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{
			"drive", "+update-reply",
			"--token", fileToken,
			"--type", "file",
			"--comment-id", commentID,
			"--reply-id", replyID,
			"--content", `[{"type":"text","text":"` + updatedText + `"}]`,
		},
		DefaultAs: "bot",
	}, clie2e.RetryOptions{
		ShouldRetry: func(result *clie2e.Result) bool {
			return result == nil || result.ExitCode != 0
		},
	})
	require.NoError(t, err)
	updateResult.AssertExitCode(t, 0)
	require.True(t, gjson.Get(updateResult.Stdout, "data.updated").Bool(), "stdout:\n%s", updateResult.Stdout)

	driveCommentOpsAwaitReplyText(t, ctx, listRepliesArgs, replyID, updatedText)

	// --- Use: +react-reply attaches and removes an emoji reaction ---
	driveCommentOpsReact(t, ctx, fileToken, replyID, "add")
	driveCommentOpsAwaitReaction(t, ctx, listRepliesArgs, replyID, "THUMBSUP", true)

	driveCommentOpsReact(t, ctx, fileToken, replyID, "delete")
	driveCommentOpsAwaitReaction(t, ctx, listRepliesArgs, replyID, "THUMBSUP", false)

	// --- Use: +resolve-comment flips is_solved both ways ---
	driveCommentOpsPatchSolved(t, ctx, fileToken, commentID, "resolve")
	driveCommentOpsAwaitSolved(t, ctx, batchArgs, commentID, true)

	driveCommentOpsPatchSolved(t, ctx, fileToken, commentID, "restore")
	driveCommentOpsAwaitSolved(t, ctx, batchArgs, commentID, false)

	// --- Use: +delete-reply removes exactly the created reply ---
	deleteReplyResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{
			"drive", "+delete-reply",
			"--token", fileToken,
			"--type", "file",
			"--comment-id", commentID,
			"--reply-id", replyID,
			"--yes",
		},
		DefaultAs: "bot",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	deleteReplyResult.AssertExitCode(t, 0)
	require.True(t, gjson.Get(deleteReplyResult.Stdout, "data.deleted").Bool(), "stdout:\n%s", deleteReplyResult.Stdout)

	driveCommentOpsAwaitReplies(t, ctx, batchArgs, commentID, baseReplies)
}

// driveCommentOpsItem returns the batch-query item for commentID (zero Result
// if absent).
func driveCommentOpsItem(stdout, commentID string) gjson.Result {
	for _, item := range gjson.Get(stdout, "data.items").Array() {
		if item.Get("comment_id").String() == commentID {
			return item
		}
	}
	return gjson.Result{}
}

// driveCommentOpsReplyItem returns the +list-replies item for replyID (zero
// Result if absent).
func driveCommentOpsReplyItem(stdout, replyID string) gjson.Result {
	for _, item := range gjson.Get(stdout, "data.items").Array() {
		if item.Get("reply_id").String() == replyID {
			return item
		}
	}
	return gjson.Result{}
}

// driveCommentOpsReact runs +react-reply with the given action, retrying on
// non-zero exits (writes on one comment card can be rate limited).
func driveCommentOpsReact(t *testing.T, ctx context.Context, fileToken, replyID, action string) {
	t.Helper()
	result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{
			"drive", "+react-reply",
			"--token", fileToken,
			"--type", "file",
			"--reply-id", replyID,
			"--emoji", "THUMBSUP",
			"--action", action,
		},
		DefaultAs: "bot",
	}, clie2e.RetryOptions{
		ShouldRetry: func(result *clie2e.Result) bool {
			return result == nil || result.ExitCode != 0
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.True(t, gjson.Get(result.Stdout, "data.updated").Bool(), "stdout:\n%s", result.Stdout)
}

// driveCommentOpsAwaitReaction polls +list-replies --need-reaction until the
// reply's reaction of the given key is present (count>0) or gone. Entries
// with count=0 linger after deletion, so presence is judged by count.
func driveCommentOpsAwaitReaction(t *testing.T, ctx context.Context, listArgs []string, replyID, reactionKey string, want bool) {
	t.Helper()
	args := append(append([]string{}, listArgs...), "--need-reaction")
	hasReaction := func(stdout string) bool {
		for _, reaction := range driveCommentOpsReplyItem(stdout, replyID).Get("reactions").Array() {
			if reaction.Get("reaction_key").String() == reactionKey && reaction.Get("count").Int() > 0 {
				return true
			}
		}
		return false
	}
	result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      args,
		DefaultAs: "bot",
	}, clie2e.RetryOptions{
		ShouldRetry: func(result *clie2e.Result) bool {
			return result == nil || result.ExitCode != 0 || hasReaction(result.Stdout) != want
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Equal(t, want, hasReaction(result.Stdout), "stdout:\n%s", result.Stdout)
}

// driveCommentOpsAwaitReplyText polls +list-replies until replyID carries the
// wanted text_run text.
func driveCommentOpsAwaitReplyText(t *testing.T, ctx context.Context, listArgs []string, replyID, wantText string) {
	t.Helper()
	replyText := func(stdout string) string {
		return driveCommentOpsReplyItem(stdout, replyID).Get("content.elements.0.text_run.text").String()
	}
	result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      listArgs,
		DefaultAs: "bot",
	}, clie2e.RetryOptions{
		ShouldRetry: func(result *clie2e.Result) bool {
			return result == nil || result.ExitCode != 0 || replyText(result.Stdout) != wantText
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Equal(t, wantText, replyText(result.Stdout), "stdout:\n%s", result.Stdout)
}

// driveCommentOpsPatchSolved runs +resolve-comment or +restore-comment,
// retrying on non-zero exits (consecutive PATCHes on one comment can be rate
// limited).
func driveCommentOpsPatchSolved(t *testing.T, ctx context.Context, fileToken, commentID, action string) {
	t.Helper()
	command := "+resolve-comment"
	if action == "restore" {
		command = "+restore-comment"
	}
	result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{
			"drive", command,
			"--token", fileToken,
			"--type", "file",
			"--comment-id", commentID,
		},
		DefaultAs: "bot",
	}, clie2e.RetryOptions{
		ShouldRetry: func(result *clie2e.Result) bool {
			return result == nil || result.ExitCode != 0
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.True(t, gjson.Get(result.Stdout, "data.updated").Bool(), "stdout:\n%s", result.Stdout)
}

// driveCommentOpsAwaitSolved polls batch-query until the fixture comment
// reports the wanted is_solved state.
func driveCommentOpsAwaitSolved(t *testing.T, ctx context.Context, batchArgs []string, commentID string, want bool) {
	t.Helper()
	result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      batchArgs,
		DefaultAs: "bot",
	}, clie2e.RetryOptions{
		ShouldRetry: func(result *clie2e.Result) bool {
			if result == nil || result.ExitCode != 0 {
				return true
			}
			item := driveCommentOpsItem(result.Stdout, commentID)
			return !item.Exists() || item.Get("is_solved").Bool() != want
		},
	})
	require.NoError(t, err)
	item := driveCommentOpsItem(result.Stdout, commentID)
	require.True(t, item.Exists(), "stdout:\n%s", result.Stdout)
	require.Equal(t, want, item.Get("is_solved").Bool(), "stdout:\n%s", result.Stdout)
}

// driveCommentOpsAwaitReplies polls batch-query until the fixture comment
// carries the wanted reply count.
func driveCommentOpsAwaitReplies(t *testing.T, ctx context.Context, batchArgs []string, commentID string, want int) {
	t.Helper()
	result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      batchArgs,
		DefaultAs: "bot",
	}, clie2e.RetryOptions{
		ShouldRetry: func(result *clie2e.Result) bool {
			if result == nil || result.ExitCode != 0 {
				return true
			}
			item := driveCommentOpsItem(result.Stdout, commentID)
			return !item.Exists() || len(item.Get("reply_list.replies").Array()) != want
		},
	})
	require.NoError(t, err)
	item := driveCommentOpsItem(result.Stdout, commentID)
	require.True(t, item.Exists(), "stdout:\n%s", result.Stdout)
	require.Len(t, item.Get("reply_list.replies").Array(), want, "stdout:\n%s", result.Stdout)
}
