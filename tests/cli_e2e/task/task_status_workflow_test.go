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

func TestTask_StatusWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	taskGUID := createTask(t, parentT, ctx, clie2e.Request{
		Args: []string{"task", "+create"},
		Data: map[string]any{
			"summary":     "lark-cli-e2e-summary-" + suffix,
			"description": "created by tests/cli_e2e/task status workflow",
		},
	})

	t.Run("complete", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"task", "+complete", "--task-id", taskGUID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.guid").String())
	})

	t.Run("get completed task", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:   []string{"task", "tasks", "get"},
			Params: map[string]any{"task_guid": taskGUID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.task.guid").String())
		assert.Equal(t, "done", gjson.Get(result.Stdout, "data.task.status").String())
		assert.NotZero(t, gjson.Get(result.Stdout, "data.task.completed_at").Int(), "stdout:\n%s", result.Stdout)
	})

	t.Run("reopen", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"task", "+reopen", "--task-id", taskGUID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.guid").String())
	})

	t.Run("get reopened task", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:   []string{"task", "tasks", "get"},
			Params: map[string]any{"task_guid": taskGUID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.task.guid").String())
		assert.Equal(t, "todo", gjson.Get(result.Stdout, "data.task.status").String())
		assert.Equal(t, "0", gjson.Get(result.Stdout, "data.task.completed_at").String())
	})
}
