// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package demo

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDemo_TaskLifecycle(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	createdSummary := "lark-cli-e2e-create-" + suffix
	updatedSummary := "lark-cli-e2e-update-" + suffix
	createdDescription := "created by tests/cli_e2e/demo"
	updatedDescription := "updated by tests/cli_e2e/demo"

	var taskGUID string

	t.Run("create as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "+create"},
			DefaultAs: "bot",
			Data: map[string]any{
				"summary":     createdSummary,
				"description": createdDescription,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		taskGUID = gjson.Get(result.Stdout, "data.guid").String()
		require.NotEmpty(t, taskGUID, "stdout:\n%s", result.Stdout)

		parentT.Cleanup(func() {
			cleanupCtx, cancel := clie2e.CleanupContext()
			defer cancel()

			deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
				Args:      []string{"task", "tasks", "delete"},
				DefaultAs: "bot",
				Params:    map[string]any{"task_guid": taskGUID},
				Yes:       true,
			})
			clie2e.ReportCleanupFailure(parentT, "delete task "+taskGUID, deleteResult, deleteErr)
		})
	})

	t.Run("update as bot", func(t *testing.T) {
		require.NotEmpty(t, taskGUID, "task GUID should be created before update")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "+update", "--task-id", taskGUID},
			DefaultAs: "bot",
			Data: map[string]any{
				"summary":     updatedSummary,
				"description": updatedDescription,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("get as bot", func(t *testing.T) {
		require.NotEmpty(t, taskGUID, "task GUID should be created before get")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "tasks", "get"},
			DefaultAs: "bot",
			Params:    map[string]any{"task_guid": taskGUID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.task.guid").String())
		assert.Equal(t, updatedSummary, gjson.Get(result.Stdout, "data.task.summary").String())
		assert.Equal(t, updatedDescription, gjson.Get(result.Stdout, "data.task.description").String())
	})
}
