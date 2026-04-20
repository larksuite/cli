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

	if req.DefaultAs == "" {
		req.DefaultAs = "bot"
	}

	result, err := clie2e.RunCmd(ctx, req)
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	taskGUID := gjson.Get(result.Stdout, "data.guid").String()
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

	return taskGUID
}

func createTasklist(t *testing.T, parentT *testing.T, ctx context.Context, req clie2e.Request) string {
	t.Helper()

	if req.DefaultAs == "" {
		req.DefaultAs = "bot"
	}

	result, err := clie2e.RunCmd(ctx, req)
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	tasklistGUID := gjson.Get(result.Stdout, "data.guid").String()
	require.NotEmpty(t, tasklistGUID, "stdout:\n%s", result.Stdout)

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

	return tasklistGUID
}
