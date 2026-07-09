// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"os"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/larksuite/cli/tests/cli_e2e/drive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDocs_HistoryWorkflow(t *testing.T) {
	if os.Getenv("LARK_DOC_HISTORY_E2E") != "1" {
		t.Skip("set LARK_DOC_HISTORY_E2E=1 to run docs history live workflow")
	}
	clie2e.SkipWithoutUserToken(t)

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	folderName := "lark-cli-e2e-docs-history-folder-" + suffix
	docTitle := "lark-cli-e2e-docs-history-" + suffix
	originalMarker := "original history marker " + suffix
	updatedMarker := "updated history marker " + suffix
	const defaultAs = "user"

	folderToken := drive.CreateDriveFolder(t, parentT, ctx, folderName, defaultAs, "")
	docToken := createDocWithRetry(t, parentT, ctx, folderToken, docTitle, originalMarker, defaultAs)

	updateResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+update",
			"--doc", docToken,
			"--command", "overwrite",
			"--doc-format", "markdown",
			"--content", "# " + docTitle + "\n\n" + updatedMarker,
		},
		DefaultAs: defaultAs,
	})
	require.NoError(t, err)
	updateResult.AssertExitCode(t, 0)
	updateResult.AssertStdoutStatus(t, true)

	fetchUpdated, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"docs", "+fetch", "--doc", docToken, "--doc-format", "markdown"},
		DefaultAs: defaultAs,
	})
	require.NoError(t, err)
	fetchUpdated.AssertExitCode(t, 0)
	fetchUpdated.AssertStdoutStatus(t, true)
	updatedContent := gjson.Get(fetchUpdated.Stdout, "data.document.content").String()
	assert.Contains(t, updatedContent, updatedMarker)
	currentRevision := gjson.Get(fetchUpdated.Stdout, "data.document.revision_id").Int()
	require.Greater(t, currentRevision, int64(0), "stdout:\n%s", fetchUpdated.Stdout)

	var revertHistoryVersionID string
	require.Eventually(t, func() bool {
		listResult, listErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+history-list",
				"--doc", docToken,
				"--page-size", "20",
			},
			DefaultAs: defaultAs,
		})
		require.NoError(t, listErr)
		listResult.AssertExitCode(t, 0)
		listResult.AssertStdoutStatus(t, true)
		for _, entry := range gjson.Get(listResult.Stdout, "data.entries").Array() {
			revisionID := entry.Get("revision_id").Int()
			historyVersionID := entry.Get("history_version_id").String()
			if revisionID > 0 && revisionID < currentRevision && historyVersionID != "" {
				revertHistoryVersionID = historyVersionID
				return true
			}
		}
		return false
	}, 45*time.Second, 3*time.Second, "history list did not expose a prior revision")
	require.NotEmpty(t, revertHistoryVersionID)

	revertResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+history-revert",
			"--doc", docToken,
			"--history-version-id", revertHistoryVersionID,
			"--wait-timeout-ms", "30000",
		},
		DefaultAs: defaultAs,
	})
	require.NoError(t, err)
	revertResult.AssertExitCode(t, 0)
	revertResult.AssertStdoutStatus(t, true)

	status := gjson.Get(revertResult.Stdout, "data.status").String()
	taskID := gjson.Get(revertResult.Stdout, "data.task_id").String()
	statusStdout := revertResult.Stdout
	if status == "running" {
		require.NotEmpty(t, taskID, "stdout:\n%s", revertResult.Stdout)
		require.Eventually(t, func() bool {
			statusResult, statusErr := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"docs", "+history-revert-status",
					"--doc", docToken,
					"--task-id", taskID,
				},
				DefaultAs: defaultAs,
			})
			require.NoError(t, statusErr)
			statusResult.AssertExitCode(t, 0)
			statusResult.AssertStdoutStatus(t, true)
			statusStdout = statusResult.Stdout
			status = gjson.Get(statusResult.Stdout, "data.status").String()
			return status != "" && status != "running"
		}, 60*time.Second, 5*time.Second, "history revert task did not finish")
	}
	require.Equal(t, "done", status, "status stdout:\n%s", statusStdout)

	fetchReverted, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"docs", "+fetch", "--doc", docToken, "--doc-format", "markdown"},
		DefaultAs: defaultAs,
	})
	require.NoError(t, err)
	fetchReverted.AssertExitCode(t, 0)
	fetchReverted.AssertStdoutStatus(t, true)
	revertedContent := gjson.Get(fetchReverted.Stdout, "data.document.content").String()
	assert.Contains(t, revertedContent, originalMarker)
	assert.NotContains(t, revertedContent, updatedMarker)
}
