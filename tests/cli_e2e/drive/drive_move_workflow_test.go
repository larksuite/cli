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

// TestDrive_MoveFolderRetryWorkflow verifies taskless completion when a folder is moved to its current destination.
func TestDrive_MoveFolderRetryWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	parentFolderToken := os.Getenv("LARK_CLI_E2E_DRIVE_PARENT_FOLDER_TOKEN")
	// Register the destination first so cleanup deletes the source before its parent.
	destination := CreateDriveFolder(t, t, ctx, "lark-cli-e2e-move-destination-"+suffix, "bot", parentFolderToken)
	sourceName := "lark-cli-e2e-move-source-" + suffix
	source := CreateDriveFolder(t, t, ctx, sourceName, "bot", parentFolderToken)
	move := clie2e.Request{
		Args:      []string{"drive", "+move", "--file-token", source, "--type", "folder", "--folder-token", destination},
		DefaultAs: "bot",
	}

	waitForDestination := func(t *testing.T) {
		t.Helper()
		err := clie2e.WaitForCondition(ctx, clie2e.WaitOptions{
			Timeout:  30 * time.Second,
			Interval: 2 * time.Second,
		}, func() (bool, error) {
			files := listDriveFolderFilesByName(t, ctx, destination)
			return files[sourceName].Token == source, nil
		})
		require.NoError(t, err, "source folder must be listed in the destination")
	}

	if !t.Run("move to destination", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, move)
		require.NoError(t, err)
		require.Equal(t, 0, result.ExitCode, "stdout=%s stderr=%s", result.Stdout, result.Stderr)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, source, gjson.Get(result.Stdout, "data.file_token").String())
		require.Equal(t, destination, gjson.Get(result.Stdout, "data.folder_token").String())

		ready := gjson.Get(result.Stdout, "data.ready")
		taskID := gjson.Get(result.Stdout, "data.task_id").String()
		t.Logf("first move: ready=%s task_id=%q", ready.Raw, taskID)
		if ready.Type == gjson.True {
			require.Equal(t, "success", gjson.Get(result.Stdout, "data.status").String())
		} else {
			require.Equal(t, gjson.False, ready.Type, "move must report a boolean ready: %s", result.Stdout)
			require.NotEmpty(t, taskID, "pending move must return a task ID")
			err = clie2e.WaitForCondition(ctx, clie2e.WaitOptions{
				Timeout:  90 * time.Second,
				Interval: 2 * time.Second,
			}, func() (bool, error) {
				task, err := clie2e.RunCmd(ctx, clie2e.Request{
					Args:      []string{"drive", "+task_result", "--scenario", "task_check", "--task-id", taskID},
					DefaultAs: "bot",
				})
				if err != nil {
					return false, err
				}
				require.Equal(t, 0, task.ExitCode, "stdout=%s stderr=%s", task.Stdout, task.Stderr)
				task.AssertStdoutStatus(t, true)
				require.Equal(t, taskID, gjson.Get(task.Stdout, "data.task_id").String())
				require.Equal(t, gjson.False, gjson.Get(task.Stdout, "data.failed").Type, "task failed: %s", task.Stdout)
				taskReady := gjson.Get(task.Stdout, "data.ready")
				if taskReady.Type == gjson.True {
					require.Equal(t, "success", gjson.Get(task.Stdout, "data.status").String())
					return true, nil
				}
				require.Equal(t, gjson.False, taskReady.Type, "task must report a boolean ready: %s", task.Stdout)
				return false, nil
			})
			require.NoError(t, err, "first move must finish before the retry")
		}
		waitForDestination(t)
	}) {
		return
	}

	t.Run("retry completes without a task", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, move)
		require.NoError(t, err)
		require.Equal(t, 0, result.ExitCode, "stdout=%s stderr=%s", result.Stdout, result.Stderr)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, "bot", gjson.Get(result.Stdout, "identity").String())
		require.Equal(t, source, gjson.Get(result.Stdout, "data.file_token").String())
		require.Equal(t, destination, gjson.Get(result.Stdout, "data.folder_token").String())
		require.Equal(t, "success", gjson.Get(result.Stdout, "data.status").String())
		require.Equal(t, gjson.True, gjson.Get(result.Stdout, "data.ready").Type, "retry must complete synchronously: %s", result.Stdout)
		for _, field := range []string{"task_id", "timed_out", "next_command"} {
			require.False(t, gjson.Get(result.Stdout, "data."+field).Exists(), "same-destination fixture must exercise taskless completion, unexpected %s: %s", field, result.Stdout)
		}
		t.Log("repeated move completed synchronously without task or continuation fields")
		waitForDestination(t)
	})
}
