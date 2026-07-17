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

func TestDrive_DeleteAsyncWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	parentFolderToken := createDriveFolder(t, parentT, ctx, "lark-cli-e2e-drive-delete-"+suffix, "")

	t.Run("docx", func(t *testing.T) {
		docToken := createDeleteWorkflowDoc(t, ctx, parentFolderToken, "lark-cli-e2e-drive-delete-docx-"+suffix)

		taskID := deleteAsyncAndVerify(t, ctx, docToken, "docx")
		t.Logf("docx delete task_id=%s token=%s", taskID, docToken)
	})

	t.Run("empty folder", func(t *testing.T) {
		folderToken := createDriveFolder(t, parentT, ctx, "empty-"+suffix, parentFolderToken)

		taskID := deleteAsyncAndVerify(t, ctx, folderToken, "folder")
		t.Logf("empty folder delete task_id=%s token=%s", taskID, folderToken)
	})

	t.Run("nonempty folder", func(t *testing.T) {
		folderToken := createDriveFolder(t, parentT, ctx, "nonempty-"+suffix, parentFolderToken)
		_ = createDeleteWorkflowDoc(t, ctx, folderToken, "nested-doc-"+suffix)

		taskID := deleteAsyncAndVerify(t, ctx, folderToken, "folder")
		t.Logf("nonempty folder delete task_id=%s token=%s", taskID, folderToken)
	})
}

func createDeleteWorkflowDoc(t *testing.T, ctx context.Context, folderToken, title string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+create",
			"--parent-token", folderToken,
			"--doc-format", "markdown",
			"--content", "# " + title + "\n\nCreated by drive delete async workflow.",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	docToken := gjson.Get(result.Stdout, "data.document.document_id").String()
	require.NotEmpty(t, docToken, "stdout:\n%s", result.Stdout)
	return docToken
}

func deleteAsyncAndVerify(t *testing.T, ctx context.Context, token, docType string) string {
	t.Helper()

	result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"drive", "+delete", "--file-token", token, "--type", docType, "--yes"},
		DefaultAs: "bot",
	}, driveDeleteRetry)
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	taskID := gjson.Get(result.Stdout, "data.task_id").String()
	require.NotEmpty(t, taskID, "delete must return async task_id\nstdout:\n%s", result.Stdout)

	taskResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"drive", "+task_result", "--scenario", "task_check", "--task-id", taskID},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	taskResult.AssertExitCode(t, 0)
	taskResult.AssertStdoutStatus(t, true)
	require.Equal(t, taskID, gjson.Get(taskResult.Stdout, "data.task_id").String(), "stdout:\n%s", taskResult.Stdout)
	require.False(t, gjson.Get(taskResult.Stdout, "data.failed").Bool(), "stdout:\n%s", taskResult.Stdout)

	require.NoError(t, waitDriveResourceDeleted(ctx, token, docType, "bot", driveDeleteVisibilityWait))
	return taskID
}
