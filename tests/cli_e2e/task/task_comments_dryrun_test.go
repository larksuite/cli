// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func setTaskCommentsDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "task_comments_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "task_comments_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_TENANT_ACCESS_TOKEN", "")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}

func TestTaskCommentsDryRun(t *testing.T) {
	setTaskCommentsDryRunEnv(t)
	tests := []struct {
		name       string
		params     map[string]any
		data       any
		method     string
		url        string
		wantParams string
		wantBody   string
	}{
		{
			name: "create", method: "POST", url: "/open-apis/task/v2/comments",
			params: map[string]any{"user_id_type": "open_id"},
			data: map[string]any{
				"content": "Reply with quoted text\nsecond line", "resource_type": "task",
				"resource_id": "task-guid-001", "reply_to_comment_id": "7198104824246747156",
			},
			wantParams: `{"user_id_type":"open_id"}`,
			wantBody:   `{"content":"Reply with quoted text\nsecond line","resource_type":"task","resource_id":"task-guid-001","reply_to_comment_id":"7198104824246747156"}`,
		},
		{
			name: "delete", method: "DELETE", url: "/open-apis/task/v2/comments/7198104824246747156",
			params: map[string]any{"comment_id": "7198104824246747156"},
		},
		{
			name: "get", method: "GET", url: "/open-apis/task/v2/comments/7198104824246747156",
			params:     map[string]any{"comment_id": "7198104824246747156", "user_id_type": "union_id"},
			wantParams: `{"user_id_type":"union_id"}`,
		},
		{
			name: "list", method: "GET", url: "/open-apis/task/v2/comments",
			params: map[string]any{
				"resource_type": "task", "resource_id": "task-guid-001", "direction": "desc",
				"page_size": 25, "page_token": "next-page-token", "user_id_type": "open_id",
			},
			wantParams: `{"resource_type":"task","resource_id":"task-guid-001","direction":"desc","page_size":25,"page_token":"next-page-token","user_id_type":"open_id"}`,
		},
		{
			name: "patch", method: "PATCH", url: "/open-apis/task/v2/comments/7198104824246747156",
			params:     map[string]any{"comment_id": "7198104824246747156", "user_id_type": "open_id"},
			data:       map[string]any{"comment": map[string]any{"content": "Updated comment"}, "update_fields": []string{"content"}},
			wantParams: `{"user_id_type":"open_id"}`,
			wantBody:   `{"comment":{"content":"Updated comment"},"update_fields":["content"]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:   []string{"task", "comments", tt.name, "--dry-run"},
				Params: tt.params, Data: tt.data, DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			require.True(t, gjson.Get(result.Stdout, "ok").Bool(), result.Stdout)
			require.True(t, gjson.Get(result.Stdout, "dry_run").Bool(), result.Stdout)
			require.Equal(t, int64(1), clie2e.DryRunGet(result.Stdout, "api.#").Int(), result.Stdout)
			require.Equal(t, tt.method, clie2e.DryRunGet(result.Stdout, "api.0.method").String(), result.Stdout)
			require.Equal(t, tt.url, clie2e.DryRunGet(result.Stdout, "api.0.url").String(), result.Stdout)
			if tt.wantParams == "" {
				require.False(t, clie2e.DryRunGet(result.Stdout, "api.0.params").Exists(), result.Stdout)
			} else {
				require.JSONEq(t, tt.wantParams, clie2e.DryRunGet(result.Stdout, "api.0.params").Raw, result.Stdout)
			}
			if tt.wantBody == "" {
				require.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body").Exists(), result.Stdout)
			} else {
				require.JSONEq(t, tt.wantBody, clie2e.DryRunGet(result.Stdout, "api.0.body").Raw, result.Stdout)
			}
		})
	}
}

func TestTaskCommentsDeleteWithoutYesRequiresConfirmation(t *testing.T) {
	setTaskCommentsDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"task", "comments", "delete"},
		Params:    map[string]any{"comment_id": "7198104824246747156"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 10)
	require.Equal(t, "confirmation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "confirmation_required", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	require.Equal(t, "high-risk-write", gjson.Get(result.Stderr, "error.risk").String(), result.Stderr)
	require.Equal(t, "task.comments.delete", gjson.Get(result.Stderr, "error.action").String(), result.Stderr)
	require.Empty(t, result.Stdout)
}
