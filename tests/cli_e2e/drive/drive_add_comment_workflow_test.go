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

func TestDriveAddCommentMarkdownFileWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	if os.Getenv("LARK_DRIVE_MD_COMMENT_E2E") == "" {
		t.Skip("set LARK_DRIVE_MD_COMMENT_E2E=1 to run the supported file comment workflow")
	}

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	fileName := "lark-cli-e2e-drive-comment-" + suffix + ".md"

	createResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"markdown", "+create",
			"--name", fileName,
			"--content", "# Comment target\n\nbody\n",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	createResult.AssertExitCode(t, 0)
	createResult.AssertStdoutStatus(t, true)

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
		clie2e.ReportCleanupFailure(parentT, "delete file comment target "+fileToken, deleteResult, deleteErr)
	})

	commentResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{
			"drive", "+add-comment",
			"--doc", fileToken,
			"--type", "file",
			"--content", `[{"type":"text","text":"please update README"}]`,
		},
		DefaultAs: "bot",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	commentResult.AssertExitCode(t, 0)
	commentResult.AssertStdoutStatus(t, true)

	commentID := gjson.Get(commentResult.Stdout, "data.comment_id").String()
	require.NotEmpty(t, commentID, "stdout:\n%s", commentResult.Stdout)
	if got := gjson.Get(commentResult.Stdout, "data.file_type").String(); got != "file" {
		t.Fatalf("data.file_type=%q, want file\nstdout:\n%s", got, commentResult.Stdout)
	}
	if got := gjson.Get(commentResult.Stdout, "data.file_name").String(); got != fileName {
		t.Fatalf("data.file_name=%q, want %q\nstdout:\n%s", got, fileName, commentResult.Stdout)
	}
	if got := gjson.Get(commentResult.Stdout, "data.file_extension").String(); got != ".md" {
		t.Fatalf("data.file_extension=%q, want .md\nstdout:\n%s", got, commentResult.Stdout)
	}

	listResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{
			"drive", "+list-comments",
			"--token", fileToken,
			"--type", "file",
			"--solved-status", "all",
			"--page-size", "100",
		},
		DefaultAs: "bot",
	}, clie2e.RetryOptions{
		ShouldRetry: func(result *clie2e.Result) bool {
			return result == nil || result.ExitCode != 0 || !driveCommentListContainsID(result.Stdout, commentID)
		},
	})
	require.NoError(t, err)
	listResult.AssertExitCode(t, 0)
	listResult.AssertStdoutStatus(t, true)

	if got := gjson.Get(listResult.Stdout, "data.file_token").String(); got != fileToken {
		t.Fatalf("list data.file_token=%q, want %q\nstdout:\n%s", got, fileToken, listResult.Stdout)
	}
	if got := gjson.Get(listResult.Stdout, "data.file_type").String(); got != "file" {
		t.Fatalf("list data.file_type=%q, want file\nstdout:\n%s", got, listResult.Stdout)
	}
	if !driveCommentListContainsID(listResult.Stdout, commentID) {
		t.Fatalf("list comments did not include comment_id %q\nstdout:\n%s", commentID, listResult.Stdout)
	}
}

func driveCommentListContainsID(stdout, commentID string) bool {
	for _, item := range gjson.Get(stdout, "data.items").Array() {
		if item.Get("comment_id").String() == commentID {
			return true
		}
	}
	return false
}
