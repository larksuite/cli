// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"net/url"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestTaskIDHandlingWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	originalSummary := "lark-cli-e2e-task-id-original-" + suffix
	updatedSummary := "lark-cli-e2e-task-id-updated-" + suffix
	taskGUID := createTask(t, parentT, ctx, clie2e.Request{
		Args:      []string{"task", "+create"},
		DefaultAs: "bot",
		Data: map[string]any{
			"summary":     originalSummary,
			"description": "created by task ID handling workflow",
		},
	})
	taskApplink := "https://applink.larksuite.com/client/todo/task?guid=" + url.QueryEscape(taskGUID)

	t.Run("update accepts task applink", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "+update", "--task-id", taskApplink, "--summary", updatedSummary},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, taskGUID, gjson.Get(result.Stdout, "data.tasks.0.guid").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, updatedSummary, gjson.Get(result.Stdout, "data.tasks.0.confirmed.summary").String(), "stdout:\n%s", result.Stdout)
	})

	t.Run("display number is rejected without modifying task", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "+update", "--task-id", "t12345", "--summary", "must-not-be-written-" + suffix},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		assert.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), "stderr:\n%s", result.Stderr)
		assert.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), "stderr:\n%s", result.Stderr)
		assert.Equal(t, "--task-id", gjson.Get(result.Stderr, "error.param").String(), "stderr:\n%s", result.Stderr)

		getResult, getErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "tasks", "get"},
			DefaultAs: "bot",
			Params:    map[string]any{"task_guid": taskGUID},
		})
		require.NoError(t, getErr)
		getResult.AssertExitCode(t, 0)
		getResult.AssertStdoutStatus(t, true)
		assert.Equal(t, updatedSummary, gjson.Get(getResult.Stdout, "data.task.summary").String(), "stdout:\n%s", getResult.Stdout)
	})
}
