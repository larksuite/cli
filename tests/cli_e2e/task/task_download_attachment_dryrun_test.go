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

func TestTaskDownloadAttachmentDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "task_download_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "task_download_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"task", "+download-attachment",
			"--attachment-guid", "attachment-guid-1",
			"--output", "./downloads/",
			"--user-id-type", "union_id",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if count := clie2e.DryRunGet(out, "api.#").Int(); count != 2 {
		t.Fatalf("expected 2 download steps, got %d\nstdout:\n%s", count, out)
	}
	if method := clie2e.DryRunGet(out, "api.0.method").String(); method != "GET" {
		t.Fatalf("api[0].method = %q, want GET\nstdout:\n%s", method, out)
	}
	if url := clie2e.DryRunGet(out, "api.0.url").String(); url != "/open-apis/task/v2/attachments/attachment-guid-1" {
		t.Fatalf("api[0].url = %q\nstdout:\n%s", url, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.user_id_type").String(); got != "union_id" {
		t.Fatalf("api[0].params.user_id_type = %q, want union_id\nstdout:\n%s", got, out)
	}
	if url := clie2e.DryRunGet(out, "api.1.url").String(); url != "<temporary_attachment_url>" {
		t.Fatalf("api[1].url = %q\nstdout:\n%s", url, out)
	}
	if output := clie2e.DryRunGet(out, "output").String(); output != "./downloads/" {
		t.Fatalf("output = %q\nstdout:\n%s", output, out)
	}
}
