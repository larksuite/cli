// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSlidesHistoryDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	tests := []struct {
		name      string
		args      []string
		wantURL   string
		wantVerb  string
		assertion func(t *testing.T, stdout string)
	}{
		{
			name: "list",
			args: []string{
				"slides", "+history-list",
				"--presentation", "presHistoryDryRun",
				"--page-size", "5",
				"--page-token", "page_token_1",
				"--dry-run",
			},
			wantURL:  "/open-apis/slides_ai/v1/xml_presentations/presHistoryDryRun/histories",
			wantVerb: "GET",
			assertion: func(t *testing.T, stdout string) {
				require.Equal(t, int64(5), gjson.Get(stdout, "data.api.0.params.page_size").Int(), stdout)
				require.Equal(t, "page_token_1", gjson.Get(stdout, "data.api.0.params.page_token").String(), stdout)
			},
		},
		{
			name: "revert",
			args: []string{
				"slides", "+history-revert",
				"--presentation", "presHistoryDryRun",
				"--history-version-id", "42",
				"--dry-run",
			},
			wantURL:  "/open-apis/slides_ai/v1/xml_presentations/presHistoryDryRun/history/revert",
			wantVerb: "POST",
			assertion: func(t *testing.T, stdout string) {
				require.Equal(t, "42", gjson.Get(stdout, "data.api.0.body.history_version_id").String(), stdout)
				require.False(t, gjson.Get(stdout, "data.api.0.body.wait_timeout_ms").Exists(), stdout)
			},
		},
		{
			name: "status",
			args: []string{
				"slides", "+history-revert-status",
				"--presentation", "presHistoryDryRun",
				"--task-id", "task_1",
				"--dry-run",
			},
			wantURL:  "/open-apis/slides_ai/v1/xml_presentations/presHistoryDryRun/history/revert_status",
			wantVerb: "GET",
			assertion: func(t *testing.T, stdout string) {
				require.Equal(t, "task_1", gjson.Get(stdout, "data.api.0.params.task_id").String(), stdout)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tt.args,
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			require.Equal(t, tt.wantVerb, gjson.Get(result.Stdout, "data.api.0.method").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, tt.wantURL, gjson.Get(result.Stdout, "data.api.0.url").String(), "stdout:\n%s", result.Stdout)
			tt.assertion(t, result.Stdout)
		})
	}
}

func setSlidesDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "slides_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "slides_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}
