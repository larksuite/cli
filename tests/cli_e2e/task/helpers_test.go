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
			Yes:       true,
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
			Yes:       true,
		})
		clie2e.ReportCleanupFailure(parentT, "delete tasklist "+tasklistGUID, deleteResult, deleteErr)
	})

	return tasklistGUID
}

func findTaskInTasklist(t *testing.T, ctx context.Context, tasklistGUID string, taskGUID string) gjson.Result {
	t.Helper()

	require.NotEmpty(t, tasklistGUID, "tasklist GUID is required")
	require.NotEmpty(t, taskGUID, "task GUID is required")

	pageToken := ""
	seenPageTokens := map[string]struct{}{}
	for {
		params := map[string]any{
			"tasklist_guid": tasklistGUID,
			"page_size":     50,
		}
		if pageToken != "" {
			if _, seen := seenPageTokens[pageToken]; seen {
				t.Fatalf("tasklist task pagination loop detected for tasklist %q, repeated page_token %q", tasklistGUID, pageToken)
			}
			seenPageTokens[pageToken] = struct{}{}
			params["page_token"] = pageToken
		}

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"task", "tasklists", "tasks"},
			DefaultAs: "bot",
			Params:    params,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		taskItem := gjson.Get(result.Stdout, `data.items.#(guid=="`+taskGUID+`")`)
		if taskItem.Exists() {
			return taskItem
		}

		hasMore := gjson.Get(result.Stdout, "data.has_more").Bool()
		pageToken = gjson.Get(result.Stdout, "data.page_token").String()
		if !hasMore || pageToken == "" {
			t.Fatalf("task %q not found in tasklist %q pages, last stdout:\n%s", taskGUID, tasklistGUID, result.Stdout)
		}
	}
}
