// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestDriveDeleteDryRunAsyncParams(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+delete",
			"--file-token", "docxDryRunDelete",
			"--type", "docx",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.#").Int(); got != 2 {
		t.Fatalf("api count=%d, want 2\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.method").String(); got != "DELETE" {
		t.Fatalf("api.0.method=%q, want DELETE\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/drive/v1/files/docxDryRunDelete" {
		t.Fatalf("api.0.url=%q, want delete files endpoint\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.type").String(); got != "docx" {
		t.Fatalf("api.0.params.type=%q, want docx\nstdout:\n%s", got, out)
	}
	async := clie2e.DryRunGet(out, "api.0.params.async")
	if !async.Exists() || !async.Bool() {
		t.Fatalf("api.0.params.async=%v, want true\nstdout:\n%s", async.Value(), out)
	}
	if got := clie2e.DryRunGet(out, "api.1.method").String(); got != "GET" {
		t.Fatalf("api.1.method=%q, want GET\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.url").String(); got != "/open-apis/drive/v1/files/task_check" {
		t.Fatalf("api.1.url=%q, want task_check endpoint\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.params.task_id").String(); got != "<task_id>" {
		t.Fatalf("api.1.params.task_id=%q, want placeholder\nstdout:\n%s", got, out)
	}
}
