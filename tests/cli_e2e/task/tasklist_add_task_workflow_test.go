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

func TestTask_TasklistAddTaskWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	tasklistName := "lark-cli-e2e-tasklist-add-" + suffix
	taskSummary := "lark-cli-e2e-tasklist-add-task-" + suffix

	tasklistGUID := createTasklist(t, parentT, ctx, clie2e.Request{
		Args:      []string{"task", "+tasklist-create", "--name", tasklistName},
		DefaultAs: "bot",
	})
	taskGUID := createTask(t, parentT, ctx, clie2e.Request{
		Args:      []string{"task", "+create"},
		DefaultAs: "bot",
		Data: map[string]any{
			"summary":     taskSummary,
			"description": "created by tests/cli_e2e/task tasklist add workflow",
		},
	})

	t.Run("add task to tasklist as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "+tasklist-task-add", "--tasklist-id", tasklistGUID, "--task-id", taskGUID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		assert.Equal(t, tasklistGUID, gjson.Get(result.Stdout, "data.tasklist_guid").String())
		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.successful_tasks.0.guid").String())
		assert.False(t, gjson.Get(result.Stdout, "data.failed_tasks.0").Exists(), "stdout:\n%s", result.Stdout)
	})

	t.Run("list tasklist tasks as bot", func(t *testing.T) {
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

	t.Run("get task with tasklist link as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "tasks", "get"},
			DefaultAs: "bot",
			Params:    map[string]any{"task_guid": taskGUID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.task.guid").String())
		assert.Equal(t, tasklistGUID, gjson.Get(result.Stdout, "data.task.tasklists.0.tasklist_guid").String())
	})
}
