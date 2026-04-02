// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func createTask(t *testing.T, parentT *testing.T, ctx context.Context, req clie2e.Request) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, req)
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	taskGUID := gjson.Get(result.Stdout, "data.guid").String()
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

	return taskGUID
}

func createTasklist(t *testing.T, parentT *testing.T, ctx context.Context, req clie2e.Request) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, req)
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	tasklistGUID := gjson.Get(result.Stdout, "data.guid").String()
	require.NotEmpty(t, tasklistGUID, "stdout:\n%s", result.Stdout)

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

	return tasklistGUID
}
