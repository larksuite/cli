// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestTask_UpdateWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	taskSummary := "lark-cli-e2e-user-my-task-" + suffix
	taskDescription := "created by tests/cli_e2e/task user workflow"
	updatedTaskSummary := "lark-cli-e2e-user-my-task-updated-" + suffix
	updatedTaskDescription := "updated by task +update user workflow"
	patchedTaskSummary := "lark-cli-e2e-user-my-task-patched-" + suffix
	patchedTaskDescription := "patched by task tasks patch user workflow"
	taskGUID := ""

	clie2e.SkipWithoutUserToken(t)

	parentT.Cleanup(func() {
		if taskGUID == "" {
			return
		}

		cleanupCtx, cancel := clie2e.CleanupContext()
		defer cancel()

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"task", "tasks", "delete"},
			DefaultAs: "user",
			Params:    map[string]any{"task_guid": taskGUID},
			Yes:       true,
		})
		clie2e.ReportCleanupFailure(parentT, "delete user task "+taskGUID, deleteResult, deleteErr)
	})

	t.Run("create task as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "+create"},
			DefaultAs: "user",
			Data: map[string]any{
				"summary":     taskSummary,
				"description": taskDescription,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		taskGUID = gjson.Get(result.Stdout, "data.guid").String()
		require.NotEmpty(t, taskGUID, "stdout:\n%s", result.Stdout)
		assert.Equal(t, "user", gjson.Get(result.Stdout, "identity").String(), "stdout:\n%s", result.Stdout)
	})

	t.Run("get created task as user", func(t *testing.T) {
		require.NotEmpty(t, taskGUID, "task GUID should be created before get")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "tasks", "get"},
			DefaultAs: "user",
			Params:    map[string]any{"task_guid": taskGUID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.task.guid").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, taskSummary, gjson.Get(result.Stdout, "data.task.summary").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, taskDescription, gjson.Get(result.Stdout, "data.task.description").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, "todo", gjson.Get(result.Stdout, "data.task.status").String(), "stdout:\n%s", result.Stdout)
	})

	t.Run("update task with shortcut as user", func(t *testing.T) {
		require.NotEmpty(t, taskGUID, "task GUID should be created before update")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"task", "+update",
				"--task-id", taskGUID,
				"--summary", updatedTaskSummary,
				"--description", updatedTaskDescription,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.tasks.0.guid").String(), "stdout:\n%s", result.Stdout)
	})

	t.Run("get task updated by shortcut as user", func(t *testing.T) {
		require.NotEmpty(t, taskGUID, "task GUID should be updated before get")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "tasks", "get"},
			DefaultAs: "user",
			Params:    map[string]any{"task_guid": taskGUID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		assert.Equal(t, updatedTaskSummary, gjson.Get(result.Stdout, "data.task.summary").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, updatedTaskDescription, gjson.Get(result.Stdout, "data.task.description").String(), "stdout:\n%s", result.Stdout)
	})

	t.Run("patch task with api as user", func(t *testing.T) {
		require.NotEmpty(t, taskGUID, "task GUID should be updated before patch")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "tasks", "patch"},
			DefaultAs: "user",
			Params:    map[string]any{"task_guid": taskGUID},
			Data: map[string]any{
				"task": map[string]any{
					"summary":     patchedTaskSummary,
					"description": patchedTaskDescription,
				},
				"update_fields": []string{"summary", "description"},
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.task.guid").String(), "stdout:\n%s", result.Stdout)
	})

	t.Run("get task patched by api as user", func(t *testing.T) {
		require.NotEmpty(t, taskGUID, "task GUID should be patched before get")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "tasks", "get"},
			DefaultAs: "user",
			Params:    map[string]any{"task_guid": taskGUID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.task.guid").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, patchedTaskSummary, gjson.Get(result.Stdout, "data.task.summary").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, patchedTaskDescription, gjson.Get(result.Stdout, "data.task.description").String(), "stdout:\n%s", result.Stdout)
	})

	t.Run("complete task as user", func(t *testing.T) {
		require.NotEmpty(t, taskGUID, "task GUID should be created before complete")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "+complete", "--task-id", taskGUID},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.guid").String(), "stdout:\n%s", result.Stdout)
	})

	t.Run("get completed task as user", func(t *testing.T) {
		require.NotEmpty(t, taskGUID, "task GUID should be completed before get")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "tasks", "get"},
			DefaultAs: "user",
			Params:    map[string]any{"task_guid": taskGUID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.task.guid").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, patchedTaskSummary, gjson.Get(result.Stdout, "data.task.summary").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, "done", gjson.Get(result.Stdout, "data.task.status").String(), "stdout:\n%s", result.Stdout)
		assert.NotZero(t, gjson.Get(result.Stdout, "data.task.completed_at").Int(), "stdout:\n%s", result.Stdout)
	})
}
