// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestDocsFetchDryRunIgnoresAPIVersionCompatFlag(t *testing.T) {
	setDocsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+fetch",
			"--doc", "doxcnDryRunCompat",
			"--api-version", "v1",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.0.method").String(); got != "POST" {
		t.Fatalf("method=%q, want POST\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/docs_ai/v1/documents/doxcnDryRunCompat/fetch" {
		t.Fatalf("url=%q, want docs fetch endpoint\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.format").String(); got != "xml" {
		t.Fatalf("format=%q, want xml\nstdout:\n%s", got, out)
	}
}

func TestDocsFetchDryRunRejectsInvalidPageSize(t *testing.T) {
	for _, value := range []string{"-1", "2147483648"} {
		t.Run(value, func(t *testing.T) {
			setDocsDryRunEnv(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{"docs", "+fetch", "--doc", "doxcnPage", "--doc-format", "markdown",
					"--page-size", value, "--dry-run"},
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 2)
			require.Contains(t, result.Stderr, "--page-size")
		})
	}
}

func TestDocsFetchDryRunMarkdownWholeDocForwardsPagination(t *testing.T) {
	setDocsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+fetch",
			"--doc", "doxcnPagination",
			"--doc-format", "markdown",
			"--page-token", "tok-1",
			"--page-size", "5",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/search/v2/knowledge_qa/fetch_doc_info" {
		t.Fatalf("url=%q, want paginated fetch endpoint\nstdout:\n%s", got, out)
	}
	if !clie2e.DryRunGet(out, "api.0.body.with_block_id").Bool() {
		t.Fatalf("with_block_id=false, want true\nstdout:\n%s", out)
	}
	if !clie2e.DryRunGet(out, "api.0.body.enable_pagination").Bool() {
		t.Fatalf("enable_pagination=false, want true\nstdout:\n%s", out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.page_token").String(); got != "tok-1" {
		t.Fatalf("page_token=%q, want tok-1\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.page_size").Int(); got != 5 {
		t.Fatalf("page_size=%d, want 5\nstdout:\n%s", got, out)
	}
}

func TestDocsFetchDryRunSelectionAnchorFragmentBecomesRangeStart(t *testing.T) {
	setDocsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+fetch",
			"--doc", "https://example.larksuite.com/wiki/wikcnDryRun#share-CUE3d6Ykno2fkexEvt8cGF8Wnse",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/docs_ai/v1/documents/wikcnDryRun/fetch" {
		t.Fatalf("url=%q, want docs fetch endpoint\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.read_option.read_mode").String(); got != "range" {
		t.Fatalf("read_mode=%q, want range\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.read_option.start_block_id").String(); got != "share-CUE3d6Ykno2fkexEvt8cGF8Wnse" {
		t.Fatalf("start_block_id=%q, want selection anchor\nstdout:\n%s", got, out)
	}
}

func TestDocsFetchDryRunUnsupportedSelectionAnchorFragmentStaysFull(t *testing.T) {
	setDocsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+fetch",
			"--doc", "https://example.larksuite.com/wiki/wikcnDryRun#part-CUE3d6Ykno2fkexEvt8cGF8Wnse",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.0.body.read_option").Raw; got != "" {
		t.Fatalf("read_option=%s, want omitted for unsupported selection anchor\nstdout:\n%s", got, out)
	}
}

func setDocsDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "docs_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "docs_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}
