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

func TestTask_TasklistWorkflowAsBot(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	tasklistName := "lark-cli-e2e-tasklist-" + suffix
	taskSummary := "lark-cli-e2e-task-in-tasklist-" + suffix
	taskDescription := "created by tests/cli_e2e/task"

	var tasklistGUID string
	var taskGUID string

	t.Run("create tasklist with task as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "+tasklist-create", "--name", tasklistName},
			DefaultAs: "bot",
			Data: []map[string]any{
				{
					"summary":     taskSummary,
					"description": taskDescription,
				},
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		tasklistGUID = gjson.Get(result.Stdout, "data.guid").String()
		taskGUID = gjson.Get(result.Stdout, "data.created_tasks.0.guid").String()
		require.NotEmpty(t, tasklistGUID, "stdout:\n%s", result.Stdout)
		require.NotEmpty(t, taskGUID, "stdout:\n%s", result.Stdout)

		parentT.Cleanup(func() {
			cleanupCtx, cancel := clie2e.CleanupContext()
			defer cancel()

			deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
				Args:      []string{"task", "tasks", "delete"},
				DefaultAs: "bot",
				Params:    map[string]any{"task_guid": taskGUID},
			})
			clie2e.ReportCleanupFailure(parentT, "delete task "+taskGUID, deleteResult, deleteErr)
		})

		parentT.Cleanup(func() {
			cleanupCtx, cancel := clie2e.CleanupContext()
			defer cancel()

			deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
				Args:      []string{"task", "tasklists", "delete"},
				DefaultAs: "bot",
				Params:    map[string]any{"tasklist_guid": tasklistGUID},
			})
			clie2e.ReportCleanupFailure(parentT, "delete tasklist "+tasklistGUID, deleteResult, deleteErr)
		})
	})

	t.Run("get tasklist as bot", func(t *testing.T) {
		require.NotEmpty(t, tasklistGUID, "tasklist GUID should be created before get")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "tasklists", "get"},
			DefaultAs: "bot",
			Params:    map[string]any{"tasklist_guid": tasklistGUID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
		assert.Equal(t, tasklistGUID, gjson.Get(result.Stdout, "data.tasklist.guid").String())
		assert.Equal(t, tasklistName, gjson.Get(result.Stdout, "data.tasklist.name").String())
	})

	t.Run("list tasklist tasks as bot", func(t *testing.T) {
		require.NotEmpty(t, tasklistGUID, "tasklist GUID should be created before listing tasks")
		require.NotEmpty(t, taskGUID, "task GUID should be created before listing tasks")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "tasklists", "tasks"},
			DefaultAs: "bot",
			Params: map[string]any{
				"tasklist_guid": tasklistGUID,
				"page_size":     50,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		taskItem := gjson.Get(result.Stdout, `data.items.#(guid=="`+taskGUID+`")`)
		assert.True(t, taskItem.Exists(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, taskSummary, taskItem.Get("summary").String())
	})

	t.Run("get task as bot", func(t *testing.T) {
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
		assert.Equal(t, taskSummary, gjson.Get(result.Stdout, "data.task.summary").String())
		assert.Equal(t, taskDescription, gjson.Get(result.Stdout, "data.task.description").String())
		assert.Equal(t, tasklistGUID, gjson.Get(result.Stdout, "data.task.tasklists.0.tasklist_guid").String())
	})
}

func TestTask_TasklistWorkflowAsUser(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	suffix := clie2e.GenerateSuffix()
	tasklistName := "lark-cli-e2e-user-tasklist-" + suffix
	patchedTasklistName := "lark-cli-e2e-user-tasklist-patched-" + suffix
	tasklistGUID := ""

	parentT.Cleanup(func() {
		if tasklistGUID == "" {
			return
		}

		cleanupCtx, cancel := clie2e.CleanupContext()
		defer cancel()

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"task", "tasklists", "delete"},
			DefaultAs: "user",
			Params:    map[string]any{"tasklist_guid": tasklistGUID},
		})
		clie2e.ReportCleanupFailure(parentT, "delete user tasklist "+tasklistGUID, deleteResult, deleteErr)
	})

	t.Run("create tasklist as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "+tasklist-create", "--name", tasklistName},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		tasklistGUID = gjson.Get(result.Stdout, "data.guid").String()
		require.NotEmpty(t, tasklistGUID, "stdout:\n%s", result.Stdout)
		assert.Equal(t, "user", gjson.Get(result.Stdout, "identity").String(), "stdout:\n%s", result.Stdout)
	})

	t.Run("patch tasklist as user", func(t *testing.T) {
		require.NotEmpty(t, tasklistGUID, "tasklist GUID should be created before patch")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "tasklists", "patch"},
			DefaultAs: "user",
			Params:    map[string]any{"tasklist_guid": tasklistGUID},
			Data: map[string]any{
				"tasklist":      map[string]any{"name": patchedTasklistName},
				"update_fields": []string{"name"},
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
		assert.Equal(t, tasklistGUID, gjson.Get(result.Stdout, "data.tasklist.guid").String(), "stdout:\n%s", result.Stdout)
	})

	t.Run("get patched tasklist as user", func(t *testing.T) {
		require.NotEmpty(t, tasklistGUID, "tasklist GUID should be patched before get")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "tasklists", "get"},
			DefaultAs: "user",
			Params:    map[string]any{"tasklist_guid": tasklistGUID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
		assert.Equal(t, tasklistGUID, gjson.Get(result.Stdout, "data.tasklist.guid").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, patchedTasklistName, gjson.Get(result.Stdout, "data.tasklist.name").String(), "stdout:\n%s", result.Stdout)
	})

	t.Run("list tasklists and find patched tasklist as user", func(t *testing.T) {
		require.NotEmpty(t, tasklistGUID, "tasklist GUID should be patched before list")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "tasklists", "list"},
			DefaultAs: "user",
			Params:    map[string]any{"page_size": 50},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		tasklistItem := gjson.Get(result.Stdout, `data.items.#(guid=="`+tasklistGUID+`")`)
		require.True(t, tasklistItem.Exists(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, patchedTasklistName, tasklistItem.Get("name").String(), "stdout:\n%s", result.Stdout)
	})
}
