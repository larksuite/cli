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

	suffix := clie2e.GenerateSuffix()
	taskGUID := createTask(t, parentT, ctx, clie2e.Request{
		Args:      []string{"task", "+create"},
		DefaultAs: "bot",
		Data: map[string]any{
			"summary":     "lark-cli-e2e-summary-" + suffix,
			"description": "created by tests/cli_e2e/task status workflow",
		},
	})

	t.Run("complete as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "+complete", "--task-id", taskGUID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.guid").String())
	})

	t.Run("get completed task as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "tasks", "get"},
			DefaultAs: "bot",
			Params:    map[string]any{"task_guid": taskGUID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.task.guid").String())
		assert.Equal(t, "done", gjson.Get(result.Stdout, "data.task.status").String())
		assert.NotZero(t, gjson.Get(result.Stdout, "data.task.completed_at").Int(), "stdout:\n%s", result.Stdout)
	})

	t.Run("reopen as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "+reopen", "--task-id", taskGUID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.guid").String())
	})

	t.Run("get reopened task as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "tasks", "get"},
			DefaultAs: "bot",
			Params:    map[string]any{"task_guid": taskGUID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.task.guid").String())
		assert.Equal(t, "todo", gjson.Get(result.Stdout, "data.task.status").String())
		assert.Equal(t, "0", gjson.Get(result.Stdout, "data.task.completed_at").String())
	})
}
