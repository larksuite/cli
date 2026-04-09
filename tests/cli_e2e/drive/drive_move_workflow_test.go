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

// TestDrive_MoveWorkflow tests the move shortcut method.
// Workflow: upload a file -> move to a folder (root by default) -> verify move completed.
func TestDrive_MoveWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")

	fileToken := uploadTestFile(t, parentT, ctx, "move-"+suffix)
	require.NotEmpty(t, fileToken)

	t.Run("move", func(t *testing.T) {
		require.NotEmpty(t, fileToken, "file token should be set from upload step")

		// Move to root folder (default folder-token is root)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "+move",
				"--file-token", fileToken,
				"--type",       "file",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		taskID := gjson.Get(result.Stdout, "data.task_id").String()
		if taskID != "" {
			// Poll for move task result
			taskResult, taskErr := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{"drive", "+task_result",
					"--task-id", taskID,
					"--scenario", "task_check",
				},
			})
			require.NoError(t, taskErr)
			taskResult.AssertExitCode(t, 0)
			taskResult.AssertStdoutStatus(t, true)
		} else {
			result.AssertStdoutStatus(t, true)
		}
	})
}