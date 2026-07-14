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

// TestTask_GetMyTasksDryRun validates the request shape emitted by
// task +get-my-tasks under --dry-run. Fake credentials are sufficient because
// dry-run stops before any network call.
func TestTask_GetMyTasksDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "task_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "task_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"task", "+get-my-tasks",
			"--complete",
			"--page-token", "pt_001",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if count := clie2e.DryRunGet(out, "api.#").Int(); count != 1 {
		t.Fatalf("expected 1 API call, got %d\nstdout:\n%s", count, out)
	}
	if method := clie2e.DryRunGet(out, "api.0.method").String(); method != "GET" {
		t.Fatalf("api[0].method = %q, want GET\nstdout:\n%s", method, out)
	}
	if url := clie2e.DryRunGet(out, "api.0.url").String(); url != "/open-apis/task/v2/tasks" {
		t.Fatalf("api[0].url = %q, want /open-apis/task/v2/tasks\nstdout:\n%s", url, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.type").String(); got != "my_tasks" {
		t.Fatalf("api[0].params.type = %q, want my_tasks\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.user_id_type").String(); got != "open_id" {
		t.Fatalf("api[0].params.user_id_type = %q, want open_id\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.completed").Bool(); !got {
		t.Fatalf("api[0].params.completed = %v, want true\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.page_token").String(); got != "pt_001" {
		t.Fatalf("api[0].params.page_token = %q, want pt_001\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.page_size").Int(); got != 50 {
		t.Fatalf("api[0].params.page_size = %d, want 50\nstdout:\n%s", got, out)
	}
}
