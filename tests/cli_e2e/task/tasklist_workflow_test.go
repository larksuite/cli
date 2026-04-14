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

func TestTask_TasklistWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	tasklistName := "lark-cli-e2e-tasklist-" + suffix
	taskSummary := "lark-cli-e2e-task-in-tasklist-" + suffix
	taskDescription := "created by tests/cli_e2e/task"

	var tasklistGUID string
	var taskGUID string

	t.Run("create tasklist with task", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"task", "+tasklist-create", "--name", tasklistName},
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
			deleteResult, deleteErr := clie2e.RunCmd(context.Background(), clie2e.Request{
				Args:   []string{"task", "tasks", "delete"},
				Params: map[string]any{"task_guid": taskGUID},
			})
			if deleteErr != nil {
				parentT.Errorf("delete task %s: %v", taskGUID, deleteErr)
				return
			}
			if deleteResult.ExitCode != 0 {
				parentT.Errorf("delete task %s failed: exit=%d stdout=%s stderr=%s", taskGUID, deleteResult.ExitCode, deleteResult.Stdout, deleteResult.Stderr)
			}
		})

		parentT.Cleanup(func() {
			deleteResult, deleteErr := clie2e.RunCmd(context.Background(), clie2e.Request{
				Args:   []string{"task", "tasklists", "delete"},
				Params: map[string]any{"tasklist_guid": tasklistGUID},
			})
			if deleteErr != nil {
				parentT.Errorf("delete tasklist %s: %v", tasklistGUID, deleteErr)
				return
			}
			if deleteResult.ExitCode != 0 {
				parentT.Errorf("delete tasklist %s failed: exit=%d stdout=%s stderr=%s", tasklistGUID, deleteResult.ExitCode, deleteResult.Stdout, deleteResult.Stderr)
			}
		})
	})

	t.Run("get tasklist", func(t *testing.T) {
		require.NotEmpty(t, tasklistGUID, "tasklist GUID should be created before get")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:   []string{"task", "tasklists", "get"},
			Params: map[string]any{"tasklist_guid": tasklistGUID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
		assert.Equal(t, tasklistGUID, gjson.Get(result.Stdout, "data.tasklist.guid").String())
		assert.Equal(t, tasklistName, gjson.Get(result.Stdout, "data.tasklist.name").String())
	})

	t.Run("list tasklist tasks", func(t *testing.T) {
		require.NotEmpty(t, tasklistGUID, "tasklist GUID should be created before listing tasks")
		require.NotEmpty(t, taskGUID, "task GUID should be created before listing tasks")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"task", "tasklists", "tasks"},
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

	t.Run("get task", func(t *testing.T) {
		require.NotEmpty(t, taskGUID, "task GUID should be created before get")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:   []string{"task", "tasks", "get"},
			Params: map[string]any{"task_guid": taskGUID},
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
