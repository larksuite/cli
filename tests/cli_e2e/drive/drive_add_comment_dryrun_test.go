// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestDriveAddCommentDryRun_DocxBlockIDWithoutMCP(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+add-comment",
			"--doc", "https://example.larksuite.com/docx/docxDryRunComment",
			"--content", `[{"type":"text","text":"review this paragraph"}]`,
			"--block-id", "blk_target",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.#").Int(); got != 1 {
		t.Fatalf("data.api count=%d, want one direct OpenAPI request\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/drive/v1/files/docxDryRunComment/new_comments" {
		t.Fatalf("data.api.0.url=%q, want new_comments\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.anchor.block_id").String(); got != "blk_target" {
		t.Fatalf("data.api.0.body.anchor.block_id=%q, want blk_target\nstdout:\n%s", got, out)
	}
	if strings.Contains(out, "/mcp") || strings.Contains(out, "locate-doc") {
		t.Fatalf("dry-run must not contain an MCP locator step\nstdout:\n%s", out)
	}
}

func TestDriveAddCommentRemovedSelectionFlagRejected(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+add-comment",
			"--doc", "https://example.larksuite.com/docx/docxDryRunComment",
			"--content", `[{"type":"text","text":"review this paragraph"}]`,
			"--selection-with-ellipsis", "target text",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Contains(t, result.Stderr, "unknown flag", result.Stderr)
}

func TestDriveAddCommentDryRun_File(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+add-comment",
			"--doc", "https://example.larksuite.com/file/fileDryRunComment",
			"--content", `[{"type":"text","text":"please update README"}]`,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/drive/v1/metas/batch_query" {
		t.Fatalf("data.api.0.url=%q, want metas/batch_query\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.request_docs.0.doc_type").String(); got != "file" {
		t.Fatalf("data.api.0.body.request_docs.0.doc_type=%q, want file\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.url").String(); got != "/open-apis/drive/v1/files/fileDryRunComment/new_comments" {
		t.Fatalf("data.api.1.url=%q, want new_comments\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.body.file_type").String(); got != "file" {
		t.Fatalf("data.api.1.body.file_type=%q, want file\nstdout:\n%s", got, out)
	}
	if !clie2e.DryRunGet(out, "api.1.body.anchor.block_id").Exists() {
		t.Fatalf("data.api.1.body.anchor.block_id should exist for file comment\nstdout:\n%s", out)
	}
	if got := clie2e.DryRunGet(out, "api.1.body.anchor.block_id").String(); got != "test" {
		t.Fatalf("data.api.1.body.anchor.block_id=%q, want test\nstdout:\n%s", got, out)
	}
}

func TestDriveAddCommentDryRun_Base(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+add-comment",
			"--doc", "https://example.larksuite.com/base/baseDryRunComment",
			"--content", `[{"type":"text","text":"please check this record"}]`,
			"--block-id", "tbl9mp6fj9kDKHQV!recBIBgGmb!vewc46MG1R",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/drive/v1/files/baseDryRunComment/new_comments" {
		t.Fatalf("data.api.0.url=%q, want new_comments\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.file_type").String(); got != "bitable" {
		t.Fatalf("data.api.0.body.file_type=%q, want bitable\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.anchor.block_id").String(); got != "tbl9mp6fj9kDKHQV" {
		t.Fatalf("data.api.0.body.anchor.block_id=%q, want tbl9mp6fj9kDKHQV\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.anchor.base_record_id").String(); got != "recBIBgGmb" {
		t.Fatalf("data.api.0.body.anchor.base_record_id=%q, want recBIBgGmb\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.anchor.base_view_id").String(); got != "vewc46MG1R" {
		t.Fatalf("data.api.0.body.anchor.base_view_id=%q, want vewc46MG1R\nstdout:\n%s", got, out)
	}
}
