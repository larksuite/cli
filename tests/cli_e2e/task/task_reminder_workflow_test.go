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

func TestTask_ReminderWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	taskGUID := createTask(t, parentT, ctx, clie2e.Request{
		Args:      []string{"task", "+create"},
		DefaultAs: "bot",
		Data: map[string]any{
			"summary":     "lark-cli-e2e-reminder-" + suffix,
			"description": "created by tests/cli_e2e/task reminder workflow",
			"due": map[string]any{
				"timestamp":  time.Now().Add(48 * time.Hour).UnixMilli(),
				"is_all_day": false,
			},
		},
	})

	t.Run("set reminder as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "+reminder", "--task-id", taskGUID, "--set", "30m"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.guid").String())
	})

	t.Run("get task with reminder as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "tasks", "get"},
			DefaultAs: "bot",
			Params:    map[string]any{"task_guid": taskGUID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.task.guid").String())
		assert.Equal(t, int64(30), gjson.Get(result.Stdout, "data.task.reminders.0.relative_fire_minute").Int())
		assert.NotEmpty(t, gjson.Get(result.Stdout, "data.task.reminders.0.id").String(), "stdout:\n%s", result.Stdout)
	})

	t.Run("remove reminder as bot", func(t *testing.T) {
		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args:      []string{"task", "+reminder", "--task-id", taskGUID, "--remove"},
			DefaultAs: "bot",
		}, clie2e.RetryOptions{})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.guid").String())
	})

	t.Run("get task without reminder as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "tasks", "get"},
			DefaultAs: "bot",
			Params:    map[string]any{"task_guid": taskGUID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.task.guid").String())
		assert.False(t, gjson.Get(result.Stdout, "data.task.reminders.0").Exists(), "stdout:\n%s", result.Stdout)
	})
}
