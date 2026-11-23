// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestTask_SearchPaginationDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "task_search_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "task_search_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	tests := []struct {
		name    string
		command string
		url     string
	}{
		{
			name:    "tasks",
			command: "+search",
			url:     "/open-apis/task/v2/tasks/search",
		},
		{
			name:    "tasklists",
			command: "+tasklist-search",
			url:     "/open-apis/task/v2/tasklists/search",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"task", tt.command,
					"--query", "pagination",
					"--page-token", "initial_pt",
					"--dry-run",
				},
				DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), out)
			require.Equal(t, tt.url, clie2e.DryRunGet(out, "api.0.url").String(), out)
			require.Equal(t, "initial_pt", clie2e.DryRunGet(out, "api.0.params.page_token").String(), out)
			require.Equal(t, "pagination", clie2e.DryRunGet(out, "api.0.body.query").String(), out)
			require.False(t, clie2e.DryRunGet(out, "api.0.body.page_token").Exists(), out)
		})
	}
}
