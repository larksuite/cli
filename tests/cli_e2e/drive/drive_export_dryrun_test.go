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

func TestDriveExportDryRun_FileNameMetadata(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+export",
			"--token", "docxDryRunExport",
			"--doc-type", "docx",
			"--file-extension", "pdf",
			"--file-name", "custom-report",
			"--output-dir", "./exports",
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
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/drive/v1/export_tasks" {
		t.Fatalf("url=%q, want export_tasks\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.token").String(); got != "docxDryRunExport" {
		t.Fatalf("body.token=%q, want docxDryRunExport\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.type").String(); got != "docx" {
		t.Fatalf("body.type=%q, want docx\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.file_extension").String(); got != "pdf" {
		t.Fatalf("body.file_extension=%q, want pdf\nstdout:\n%s", got, out)
	}
	if clie2e.DryRunGet(out, "api.0.body.file_name").Exists() {
		t.Fatalf("file_name should stay local metadata, not export_tasks body\nstdout:\n%s", out)
	}
	if got := clie2e.DryRunGet(out, "file_name").String(); got != "custom-report.pdf" {
		t.Fatalf("file_name=%q, want custom-report.pdf\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "output_dir").String(); got != "./exports" {
		t.Fatalf("output_dir=%q, want ./exports\nstdout:\n%s", got, out)
	}
}

func TestDriveExportDryRun_WikiURLPlansResolveBeforeExportTask(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+export",
			"--url", "https://example.feishu.cn/wiki/wikiDryRunExport",
			"--file-extension", "pdf",
			"--file-name", "wiki-report",
			"--output-dir", "./exports",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.0.method").String(); got != "GET" {
		t.Fatalf("api.0.method=%q, want GET\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/wiki/v2/spaces/get_node" {
		t.Fatalf("api.0.url=%q, want wiki get_node\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.token").String(); got != "wikiDryRunExport" {
		t.Fatalf("api.0.params.token=%q, want wiki token\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.method").String(); got != "POST" {
		t.Fatalf("api.1.method=%q, want POST\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.url").String(); got != "/open-apis/drive/v1/export_tasks" {
		t.Fatalf("api.1.url=%q, want export_tasks\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.body.token").String(); got != "obj_token_from_step_0" {
		t.Fatalf("api.1.body.token=%q, want resolved token placeholder\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.body.type").String(); got != "obj_type_from_step_0" {
		t.Fatalf("api.1.body.type=%q, want resolved type placeholder\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "wiki_token").String(); got != "wikiDryRunExport" {
		t.Fatalf("wiki_token=%q, want source wiki token\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "file_name").String(); got != "wiki-report.pdf" {
		t.Fatalf("file_name=%q, want wiki-report.pdf\nstdout:\n%s", got, out)
	}
}

func TestDriveExportDryRun_WikiTokenTypePlansResolveBeforeExportTask(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+export",
			"--token", "wikiDryRunExport",
			"--doc-type", "wiki",
			"--file-extension", "pdf",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.0.method").String(); got != "GET" {
		t.Fatalf("api.0.method=%q, want GET\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/wiki/v2/spaces/get_node" {
		t.Fatalf("api.0.url=%q, want wiki get_node\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.token").String(); got != "wikiDryRunExport" {
		t.Fatalf("api.0.params.token=%q, want wiki token\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.body.token").String(); got != "obj_token_from_step_0" {
		t.Fatalf("api.1.body.token=%q, want resolved token placeholder\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.body.type").String(); got != "obj_type_from_step_0" {
		t.Fatalf("api.1.body.type=%q, want resolved type placeholder\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "wiki_token").String(); got != "wikiDryRunExport" {
		t.Fatalf("wiki_token=%q, want source wiki token\nstdout:\n%s", got, out)
	}
}

func TestDriveExportDryRun_MarkdownFetchAPI(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+export",
			"--token", "docxMdDryRun",
			"--doc-type", "docx",
			"--file-extension", "markdown",
			"--file-name", "my-notes",
			"--output-dir", "./md-exports",
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
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/docs_ai/v1/documents/docxMdDryRun/fetch" {
		t.Fatalf("url=%q, want docs_ai fetch\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.format").String(); got != "markdown" {
		t.Fatalf("body.format=%q, want markdown\nstdout:\n%s", got, out)
	}
	if clie2e.DryRunGet(out, "api.0.body.extra_param").Exists() {
		t.Fatalf("markdown drive export must not enable docs fetch extra_param\nstdout:\n%s", out)
	}
	if got := clie2e.DryRunGet(out, "file_name").String(); got != "my-notes.md" {
		t.Fatalf("file_name=%q, want my-notes.md\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "output_dir").String(); got != "./md-exports" {
		t.Fatalf("output_dir=%q, want ./md-exports\nstdout:\n%s", got, out)
	}
}

func TestDriveExportDryRun_BitableBaseOnlySchema(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+export",
			"--token", "bitableDryRunExport",
			"--doc-type", "bitable",
			"--file-extension", "base",
			"--only-schema",
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
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/drive/v1/export_tasks" {
		t.Fatalf("url=%q, want export_tasks\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.token").String(); got != "bitableDryRunExport" {
		t.Fatalf("body.token=%q, want bitableDryRunExport\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.type").String(); got != "bitable" {
		t.Fatalf("body.type=%q, want bitable\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.file_extension").String(); got != "base" {
		t.Fatalf("body.file_extension=%q, want base\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.only_schema").Bool(); !got {
		t.Fatalf("body.only_schema=%v, want true\nstdout:\n%s", got, out)
	}
}
