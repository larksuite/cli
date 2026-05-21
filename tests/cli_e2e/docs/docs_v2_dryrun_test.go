// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDocs_UpdateV2DryRun verifies the v2 (OpenAPI) update path constructs
// the correct request body with --api-version v2, --command append, and
// --doc-format defaults to xml.
func TestDocs_UpdateV2DryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+update",
			"--api-version", "v2",
			"--doc", "doxcnV2DryRunTest",
			"--command", "append",
			"--content", "<h2>Section</h2><p>text</p>",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := gjson.Get(out, "api.0.method").String(); got != "PUT" {
		t.Fatalf("method=%q, want PUT\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.url").String(); got != "/open-apis/docs_ai/v1/documents/doxcnV2DryRunTest" {
		t.Fatalf("url=%q, want /open-apis/docs_ai/v1/documents/doxcnV2DryRunTest\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.body.format").String(); got != "xml" {
		t.Fatalf("body.format=%q, want xml\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.body.command").String(); got != "block_insert_after" {
		t.Fatalf("body.command=%q, want block_insert_after (append is a shorthand)\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.body.block_id").String(); got != "-1" {
		t.Fatalf("body.block_id=%q, want -1 (append inserts at end)\nstdout:\n%s", got, out)
	}
}

// TestDocs_CreateV2DryRun verifies the v2 (OpenAPI) create path constructs
// the correct request body with --api-version v2 and --doc-format markdown.
func TestDocs_CreateV2DryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+create",
			"--api-version", "v2",
			"--content", "## Heading\n\nParagraph",
			"--doc-format", "markdown",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := gjson.Get(out, "api.0.method").String(); got != "POST" {
		t.Fatalf("method=%q, want POST\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.url").String(); got != "/open-apis/docs_ai/v1/documents" {
		t.Fatalf("url=%q, want /open-apis/docs_ai/v1/documents\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.body.format").String(); got != "markdown" {
		t.Fatalf("body.format=%q, want markdown\nstdout:\n%s", got, out)
	}
}
