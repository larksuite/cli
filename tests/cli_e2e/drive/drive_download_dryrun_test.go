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
	if got := clie2e.DryRunGet(out, "api.#").Int(); got != 3 {
		t.Fatalf("api count=%d, want 3\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.method").String(); got != "GET" {
		t.Fatalf("api.0.method=%q, want GET\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/drive/v1/permissions/fileDryRunDownload/members/auth" {
		t.Fatalf("api.0.url=%q, want permission auth\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.type").String(); got != "file" {
		t.Fatalf("api.0.params.type=%q, want file\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.action").String(); got != "export" {
		t.Fatalf("api.0.params.action=%q, want export\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.method").String(); got != "POST" {
		t.Fatalf("api.1.method=%q, want POST\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.url").String(); got != "/open-apis/drive/v1/metas/batch_query" {
		t.Fatalf("api.1.url=%q, want metas batch_query\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.body.request_docs.0.doc_token").String(); got != "fileDryRunDownload" {
		t.Fatalf("api.1.body.request_docs.0.doc_token=%q, want file token\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.body.request_docs.0.doc_type").String(); got != "file" {
		t.Fatalf("api.1.body.request_docs.0.doc_type=%q, want file\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.2.method").String(); got != "GET" {
		t.Fatalf("api.2.method=%q, want GET\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.2.url").String(); got != "/open-apis/drive/v1/files/fileDryRunDownload/download" {
		t.Fatalf("api.2.url=%q, want file download endpoint\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.2.desc").String(); got != "[3] Download file bytes; Content-Disposition filename wins over metadata title when present" {
		t.Fatalf("api.2.desc=%q, want metadata-aware step 3\nstdout:\n%s", got, out)
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
	if got := clie2e.DryRunGet(out, "api.#").Int(); got != 2 {
		t.Fatalf("api count=%d, want 2\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.method").String(); got != "GET" {
		t.Fatalf("api.0.method=%q, want GET\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/drive/v1/permissions/fileDryRunDownload/members/auth" {
		t.Fatalf("api.0.url=%q, want permission auth\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.url").String(); got != "/open-apis/drive/v1/files/fileDryRunDownload/download" {
		t.Fatalf("api.1.url=%q, want file download endpoint\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.desc").String(); got != "[2] Download file bytes to the explicit output path" {
		t.Fatalf("api.1.desc=%q, want explicit-output step 2\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "output").String(); got != "./artifacts/report.bin" {
		t.Fatalf("output=%q, want explicit output\nstdout:\n%s", got, out)
	}
}

// TestDriveDownloadDryRun_WikiURLResolvesBeforeDownload verifies a /wiki/ URL
// prepends a get_node resolution step ahead of the metadata + download steps.
func TestDriveDownloadDryRun_WikiURLResolvesBeforeDownload(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+download",
			"--url", "https://example.feishu.cn/wiki/wikiDryRunDownload",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.#").Int(); got != 4 {
		t.Fatalf("api count=%d, want 4 (get_node + permission auth + metadata + download)\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.method").String(); got != "GET" {
		t.Fatalf("api.0.method=%q, want GET\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/wiki/v2/spaces/get_node" {
		t.Fatalf("api.0.url=%q, want wiki get_node\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.token").String(); got != "wikiDryRunDownload" {
		t.Fatalf("api.0.params.token=%q, want wiki token\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "wiki_token").String(); got != "wikiDryRunDownload" {
		t.Fatalf("wiki_token=%q, want wiki token\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.url").String(); got != "/open-apis/drive/v1/permissions/obj_token_from_wiki_node/members/auth" {
		t.Fatalf("api.1.url=%q, want permission auth on resolved token\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.2.url").String(); got != "/open-apis/drive/v1/metas/batch_query" {
		t.Fatalf("api.2.url=%q, want metadata batch_query\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.3.url").String(); got != "/open-apis/drive/v1/files/obj_token_from_wiki_node/download" {
		t.Fatalf("api.3.url=%q, want download from resolved token\nstdout:\n%s", got, out)
	}
}

// TestDriveDownloadDryRun_WikiTokenExplicitOutput verifies --wiki-token with an
// explicit output plans get_node then the permission check and download,
// skipping the metadata lookup.
func TestDriveDownloadDryRun_WikiTokenExplicitOutput(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+download",
			"--wiki-token", "wikiDryRunDownload",
			"--output", "./artifacts/report.bin",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.#").Int(); got != 3 {
		t.Fatalf("api count=%d, want 3 (get_node + permission auth + download)\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/wiki/v2/spaces/get_node" {
		t.Fatalf("api.0.url=%q, want wiki get_node\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.url").String(); got != "/open-apis/drive/v1/permissions/obj_token_from_wiki_node/members/auth" {
		t.Fatalf("api.1.url=%q, want permission auth on resolved token\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.2.url").String(); got != "/open-apis/drive/v1/files/obj_token_from_wiki_node/download" {
		t.Fatalf("api.2.url=%q, want download from resolved token\nstdout:\n%s", got, out)
	}
}

// TestDriveDownloadDryRun_RejectsMutuallyExclusiveInputs verifies passing more
// than one source flag fails validation instead of silently picking one.
func TestDriveDownloadDryRun_RejectsMutuallyExclusiveInputs(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+download",
			"--file-token", "fileDryRunDownload",
			"--wiki-token", "wikiDryRunDownload",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	if result.Stdout != "" {
		t.Fatalf("stdout must stay empty on validation failure, got:\n%s", result.Stdout)
	}
	if got := clie2e.DryRunGet(result.Stderr, "error.type").String(); got != "validation" {
		t.Fatalf("error.type=%q, want validation\nstderr:\n%s", got, result.Stderr)
	}
	if got := clie2e.DryRunGet(result.Stderr, "error.subtype").String(); got != "invalid_argument" {
		t.Fatalf("error.subtype=%q, want invalid_argument\nstderr:\n%s", got, result.Stderr)
	}
	if got := clie2e.DryRunGet(result.Stderr, "error.param").String(); got != "--file-token" {
		t.Fatalf("error.param=%q, want --file-token\nstderr:\n%s", got, result.Stderr)
	}
	if got := clie2e.DryRunGet(result.Stderr, "error.message").String(); got == "" {
		t.Fatalf("error.message must be non-empty\nstderr:\n%s", result.Stderr)
	}
}
