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

func TestDriveDownloadDryRun_DefaultNamePlansMetadataBeforeDownload(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+download",
			"--file-token", "fileDryRunDownload",
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
	if got := clie2e.DryRunGet(out, "api.0.method").String(); got != "POST" {
		t.Fatalf("api.0.method=%q, want POST\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/drive/v1/metas/batch_query" {
		t.Fatalf("api.0.url=%q, want metas batch_query\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.request_docs.0.doc_token").String(); got != "fileDryRunDownload" {
		t.Fatalf("api.0.body.request_docs.0.doc_token=%q, want file token\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.request_docs.0.doc_type").String(); got != "file" {
		t.Fatalf("api.0.body.request_docs.0.doc_type=%q, want file\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.method").String(); got != "GET" {
		t.Fatalf("api.1.method=%q, want GET\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.url").String(); got != "/open-apis/drive/v1/files/fileDryRunDownload/download" {
		t.Fatalf("api.1.url=%q, want file download endpoint\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.desc").String(); got != "[2] Download file bytes; Content-Disposition filename wins over metadata title when present" {
		t.Fatalf("api.1.desc=%q, want metadata-aware step 2\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "output").String(); got != "<Content-Disposition filename | metadata title | token>" {
		t.Fatalf("output=%q, want filename priority placeholder\nstdout:\n%s", got, out)
	}
}

func TestDriveDownloadDryRun_ExplicitOutputSkipsMetadata(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+download",
			"--file-token", "fileDryRunDownload",
			"--output", "./artifacts/report.bin",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.#").Int(); got != 1 {
		t.Fatalf("api count=%d, want 1\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.method").String(); got != "GET" {
		t.Fatalf("api.0.method=%q, want GET\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/drive/v1/files/fileDryRunDownload/download" {
		t.Fatalf("api.0.url=%q, want file download endpoint\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.desc").String(); got != "[1] Download file bytes to the explicit output path" {
		t.Fatalf("api.0.desc=%q, want explicit-output step 1\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "output").String(); got != "./artifacts/report.bin" {
		t.Fatalf("output=%q, want explicit output\nstdout:\n%s", got, out)
	}
}
