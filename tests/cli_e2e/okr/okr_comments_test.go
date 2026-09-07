// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"fmt"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func runCommentDryRun(t *testing.T, args ...string) *clie2e.Result {
	t.Helper()
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	commandArgs := append([]string{"okr"}, args...)
	commandArgs = append(commandArgs, "--as", "user")
	result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: commandArgs})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	return result
}

func assertCommentDryRunOmitsDepartmentIDType(t *testing.T, result *clie2e.Result) {
	t.Helper()
	require.False(t, clie2e.DryRunGet(result.Stdout, "api.0.params.department_id_type").Exists(), result.Stdout)
	require.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body.department_id_type").Exists(), result.Stdout)
}

func TestOKR_CommentListDryRun(t *testing.T) {
	result := runCommentDryRun(t, "+comment-list", "--target-id", "123456", "--target-type", "objective", "--page-size", "25", "--page-token", "next_page", "--user-id-type", "union_id", "--dry-run")
	require.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
	require.Equal(t, "/open-apis/okr/v2/comments", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
	require.Equal(t, "123456", clie2e.DryRunGet(result.Stdout, "api.0.params.target_id").String())
	require.Equal(t, "objective", clie2e.DryRunGet(result.Stdout, "api.0.params.target_type").String())
	require.Equal(t, "25", clie2e.DryRunGet(result.Stdout, "api.0.params.page_size").String())
	require.Equal(t, "next_page", clie2e.DryRunGet(result.Stdout, "api.0.params.page_token").String())
	require.Equal(t, "union_id", clie2e.DryRunGet(result.Stdout, "api.0.params.user_id_type").String())
	assertCommentDryRunOmitsDepartmentIDType(t, result)
}

func TestOKR_CommentGetDryRun(t *testing.T) {
	result := runCommentDryRun(t, "+comment-get", "--comment-id", "987654", "--user-id-type", "user_id", "--dry-run")
	require.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
	require.Equal(t, "/open-apis/okr/v2/comments/987654", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
	require.Equal(t, "user_id", clie2e.DryRunGet(result.Stdout, "api.0.params.user_id_type").String())
	assertCommentDryRunOmitsDepartmentIDType(t, result)
}

func TestOKR_CommentCreateDryRun_Entity(t *testing.T) {
	result := runCommentDryRun(t, "+comment-create", "--target-id", "123456", "--target-type", "cycle", "--content", "{\"text\":\"Cycle comment\"}", "--dry-run")
	require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
	require.Equal(t, "/open-apis/okr/v2/comments", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
	require.Equal(t, "123456", clie2e.DryRunGet(result.Stdout, "api.0.body.target.target_id").String())
	require.Equal(t, "cycle", clie2e.DryRunGet(result.Stdout, "api.0.body.target.target_type").String())
	require.Equal(t, "Cycle comment", clie2e.DryRunGet(result.Stdout, "api.0.body.content.blocks.0.paragraph.elements.0.text_run.text").String())
	require.Equal(t, "open_id", clie2e.DryRunGet(result.Stdout, "api.0.params.user_id_type").String())
	assertCommentDryRunOmitsDepartmentIDType(t, result)
}

func TestOKR_CommentCreateDryRun_SelectedText(t *testing.T) {
	result := runCommentDryRun(t, "+comment-create", "--target-id", "234567", "--target-type", "key_result", "--content", "{\"text\":\"Selected comment\"}", "--selected-text", "important text", "--dry-run")
	require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
	require.Equal(t, "important text", clie2e.DryRunGet(result.Stdout, "api.0.body.selected_text").String())
	require.Equal(t, "key_result", clie2e.DryRunGet(result.Stdout, "api.0.body.target.target_type").String())
	require.Equal(t, "open_id", clie2e.DryRunGet(result.Stdout, "api.0.params.user_id_type").String())
	assertCommentDryRunOmitsDepartmentIDType(t, result)
}

func TestOKR_CommentCreateDryRun_SelectAll(t *testing.T) {
	result := runCommentDryRun(t, "+comment-create", "--target-id", "345678", "--target-type", "objective", "--content", "{\"text\":\"whole objective\"}", "--select-all", "--dry-run")
	require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
	require.Equal(t, "***************", clie2e.DryRunGet(result.Stdout, "api.0.body.selected_text").String())
	assertCommentDryRunOmitsDepartmentIDType(t, result)
}

func TestOKR_CommentCreateDryRun_RefComment(t *testing.T) {
	result := runCommentDryRun(t, "+comment-create", "--target-id", "456789", "--target-type", "progress", "--content", "{\"text\":\"Reply\"}", "--ref-comment-id", "111222", "--dry-run")
	require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
	require.Equal(t, "111222", clie2e.DryRunGet(result.Stdout, "api.0.body.ref_comment_id").String())
	require.Equal(t, "progress", clie2e.DryRunGet(result.Stdout, "api.0.body.target.target_type").String())
	assertCommentDryRunOmitsDepartmentIDType(t, result)
}

func TestOKR_CommentPatchDryRun(t *testing.T) {
	result := runCommentDryRun(t, "+comment-patch", "--comment-id", "567890", "--content", "{\"text\":\"Updated comment\"}", "--dry-run")
	require.Equal(t, "PATCH", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
	require.Equal(t, "/open-apis/okr/v2/comments/567890", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
	require.Equal(t, "Updated comment", clie2e.DryRunGet(result.Stdout, "api.0.body.content.blocks.0.paragraph.elements.0.text_run.text").String())
	require.Equal(t, "open_id", clie2e.DryRunGet(result.Stdout, "api.0.params.user_id_type").String())
	assertCommentDryRunOmitsDepartmentIDType(t, result)
}

func TestOKR_CommentDeleteDryRun(t *testing.T) {
	result := runCommentDryRun(t, "+comment-delete", "--comment-id", "678901", "--dry-run")
	require.Equal(t, "DELETE", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
	require.Equal(t, "/open-apis/okr/v2/comments/678901", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
	assertCommentDryRunOmitsDepartmentIDType(t, result)
}

func TestOKR_CommentSolveDryRun(t *testing.T) {
	result := runCommentDryRun(t, "+comment-solve", "--comment-id", "789012", "--dry-run")
	require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
	require.Equal(t, "/open-apis/okr/v2/comments/789012/solve", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
	require.Equal(t, "open_id", clie2e.DryRunGet(result.Stdout, "api.0.params.user_id_type").String())
	assertCommentDryRunOmitsDepartmentIDType(t, result)
}

func TestOKR_CommentReopenDryRun(t *testing.T) {
	result := runCommentDryRun(t, "+comment-reopen", "--comment-id", "890123", "--dry-run")
	require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
	require.Equal(t, "/open-apis/okr/v2/comments/890123/reopen", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
	require.Equal(t, "open_id", clie2e.DryRunGet(result.Stdout, "api.0.params.user_id_type").String())
	assertCommentDryRunOmitsDepartmentIDType(t, result)
}

// TestOKR_CommentLifecycleLive exercises every comment shortcut against one
// cycle-level comment. It is opt-in because it writes and permanently deletes
// tenant data; the created comment is cleaned up if any later step fails.
func TestOKR_CommentLifecycleLive(t *testing.T) {
	clie2e.SkipWithoutUserToken(t)
	cycleID := getTestCycleID(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	commentIDs := []string{}
	cleanup := func() {
		if len(commentIDs) == 0 {
			return
		}
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		for i := len(commentIDs) - 1; i >= 0; i-- {
			commentID := commentIDs[i]
			result, err := clie2e.RunCmd(cleanupCtx, clie2e.Request{
				Args:      []string{"okr", "+comment-delete", "--comment-id", commentID},
				DefaultAs: "user",
				Yes:       true,
			})
			clie2e.ReportCleanupFailure(t, "delete live comment "+commentID, result, err)
		}
	}
	t.Cleanup(cleanup)

	create := func(content string) *clie2e.Result {
		t.Helper()
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"okr", "+comment-create",
				"--target-type", "cycle",
				"--target-id", cycleID,
				"--content", fmt.Sprintf("{\"text\":%q}", content),
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		return result
	}

	created := create("comment shortcut lifecycle")
	commentID := gjson.Get(created.Stdout, "data.comment_id").String()
	require.NotEmpty(t, commentID, "create should return data.comment_id")
	commentIDs = append(commentIDs, commentID)

	reply, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"okr", "+comment-create", "--target-type", "cycle", "--target-id", cycleID, "--content", `{"text":"comment shortcut reply"}`, "--ref-comment-id", commentID},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	reply.AssertExitCode(t, 0)
	replyID := gjson.Get(reply.Stdout, "data.comment_id").String()
	require.NotEmpty(t, replyID, "reply should return data.comment_id")
	commentIDs = append(commentIDs, replyID)

	list, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"okr", "+comment-list", "--target-type", "cycle", "--target-id", cycleID, "--user-id-type", "union_id"},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	list.AssertExitCode(t, 0)
	require.Contains(t, list.Stdout, commentID)
	require.Contains(t, list.Stdout, replyID)

	detail, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"okr", "+comment-detail", "--cycle-id", cycleID},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	detail.AssertExitCode(t, 0)
	require.Equal(t, cycleID, gjson.Get(detail.Stdout, "data.cycle_id").String())

	get, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"okr", "+comment-get", "--comment-id", commentID, "--user-id-type", "union_id"},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	get.AssertExitCode(t, 0)
	require.Equal(t, commentID, gjson.Get(get.Stdout, "data.comment.id").String())

	patchResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"okr", "+comment-patch", "--comment-id", commentID, "--content", "{\"text\":\"patched comment\"}", "--user-id-type", "union_id"},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	patchResult.AssertExitCode(t, 0)
	require.Equal(t, commentID, gjson.Get(patchResult.Stdout, "data.comment.id").String())

	solve, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"okr", "+comment-solve", "--comment-id", commentID, "--user-id-type", "union_id"},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	solve.AssertExitCode(t, 0)
	require.True(t, gjson.Get(solve.Stdout, "data.affected_comments").Exists())

	reopen, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"okr", "+comment-reopen", "--comment-id", commentID, "--user-id-type", "union_id"},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	reopen.AssertExitCode(t, 0)
	require.True(t, gjson.Get(reopen.Stdout, "data.affected_comments").Exists())

	deleted, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"okr", "+comment-delete", "--comment-id", replyID},
		DefaultAs: "user",
		Yes:       true,
	})
	require.NoError(t, err)
	deleted.AssertExitCode(t, 0)
	require.True(t, gjson.Get(deleted.Stdout, "data.deleted").Bool())
	commentIDs = []string{commentID}
}
