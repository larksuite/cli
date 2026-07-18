// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"strings"
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

const deleteWorkflowMaxAttempts = 3

// deleteWorkflowRetryBackoff paces delete retries after a non-retryable
// failure whose target still exists. Unit tests shrink it.
var deleteWorkflowRetryBackoff = driveDeleteVisibilityPoll

// deleteAsyncAndVerify deletes token and converges every server outcome to the
// real postcondition: the resource is gone. Async deletes (non-empty task_id)
// additionally verify the task via drive +task_result; sync deletes (empty
// task_id) skip task polling; non-retryable delete failures (e.g. a transient
// "drive task failed") pass when the resource is already gone and are retried
// up to deleteWorkflowMaxAttempts times otherwise.
func deleteAsyncAndVerify(t *testing.T, ctx context.Context, token, docType string) string {
	t.Helper()

	var lastResult *clie2e.Result
	for attempt := 1; attempt <= deleteWorkflowMaxAttempts; attempt++ {
		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args:      []string{"drive", "+delete", "--file-token", token, "--type", docType, "--yes"},
			DefaultAs: "bot",
		}, driveDeleteRetry)
		require.NoError(t, err)
		lastResult = result

		if result.ExitCode == 0 {
			result.AssertStdoutStatus(t, true)
			taskID := gjson.Get(result.Stdout, "data.task_id").String()
			if taskID == "" {
				// Sync completion: the server deleted the resource inline and
				// returned no task to poll.
				require.True(t, gjson.Get(result.Stdout, "data.deleted").Bool(), "sync delete must report deleted=true\nstdout:\n%s", result.Stdout)
				t.Logf("drive +delete completed synchronously for %s %s (no task_id)", docType, token)
			} else {
				assertDriveDeleteTaskSucceeded(t, ctx, taskID)
			}
			require.NoError(t, waitDriveResourceDeleted(ctx, token, docType, "bot", driveDeleteVisibilityWait))
			return taskID
		}

		// Only the one verified backend transient may fall through to
		// terminal-state checking; any other failure is a real regression and
		// must not be rescued by the resource happening to be gone.
		if !isTransientDriveDeleteFailure(result) {
			t.Fatalf("drive +delete failed with an unexpected error on attempt %d\nstdout:\n%s\nstderr:\n%s",
				attempt, result.Stdout, result.Stderr)
		}

		// The failed delete task may still have removed the resource
		// server-side, so check the real terminal state before retrying.
		deleted, verifyErr := IsDriveResourceDeleted(ctx, token, docType, "bot")
		require.NoError(t, verifyErr, "verify %s %s after failed delete attempt %d", docType, token, attempt)
		if deleted {
			t.Logf("drive +delete attempt %d failed transiently but %s %s is gone: stderr=%s", attempt, docType, token, result.Stderr)
			return ""
		}
		if attempt < deleteWorkflowMaxAttempts {
			t.Logf("drive +delete attempt %d failed and %s %s still exists; retrying: stderr=%s", attempt, docType, token, result.Stderr)
			time.Sleep(deleteWorkflowRetryBackoff)
		}
	}

	t.Fatalf("drive +delete failed %d times and %s %s still exists\nstdout:\n%s\nstderr:\n%s",
		deleteWorkflowMaxAttempts, docType, token, lastResult.Stdout, lastResult.Stderr)
	return ""
}

func assertDriveDeleteTaskSucceeded(t *testing.T, ctx context.Context, taskID string) {
	t.Helper()

	taskResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"drive", "+task_result", "--scenario", "task_check", "--task-id", taskID},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	require.NotNil(t, taskResult)
	// Fatal exit-code gate first: the non-fatal assert flavor would cascade
	// into misleading empty-stdout failures, exactly what this fix removes.
	require.Equal(t, 0, taskResult.ExitCode, "drive +task_result failed\nstdout:\n%s\nstderr:\n%s", taskResult.Stdout, taskResult.Stderr)
	taskResult.AssertStdoutStatus(t, true)
	require.Equal(t, taskID, gjson.Get(taskResult.Stdout, "data.task_id").String(), "stdout:\n%s", taskResult.Stdout)
	// gjson returns false for an absent field too, so require presence or a
	// malformed task envelope would pass validation.
	failedField := gjson.Get(taskResult.Stdout, "data.failed")
	require.True(t, failedField.Exists(), "task result must report data.failed\nstdout:\n%s", taskResult.Stdout)
	require.False(t, failedField.Bool(), "stdout:\n%s", taskResult.Stdout)
}

// isTransientDriveDeleteFailure reports whether a failed drive +delete carries
// the one backend error this workflow tolerates: the async delete task
// transiently reporting a terminal "fail" state (observed as flake in CI; the
// resource is usually deleted regardless). Everything else — crashes, protocol
// regressions, auth or parameter errors — stays fatal.
func isTransientDriveDeleteFailure(result *clie2e.Result) bool {
	if result == nil {
		return false
	}
	for _, raw := range []string{result.Stderr, result.Stdout} {
		idx := strings.Index(raw, "{")
		if idx < 0 {
			continue
		}
		payload := raw[idx:]
		if !gjson.Valid(payload) {
			continue
		}
		errObj := gjson.Get(payload, "error")
		if errObj.Get("type").String() == "api" &&
			errObj.Get("subtype").String() == "server_error" &&
			errObj.Get("message").String() == "drive task failed" {
			return true
		}
	}
	return false
}
