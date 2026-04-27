// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
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
	if got := gjson.Get(out, "api.0.method").String(); got != "POST" {
		t.Fatalf("method=%q, want POST\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.url").String(); got != "/open-apis/drive/v1/export_tasks" {
		t.Fatalf("url=%q, want export_tasks\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.body.token").String(); got != "docxDryRunExport" {
		t.Fatalf("body.token=%q, want docxDryRunExport\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.body.type").String(); got != "docx" {
		t.Fatalf("body.type=%q, want docx\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.body.file_extension").String(); got != "pdf" {
		t.Fatalf("body.file_extension=%q, want pdf\nstdout:\n%s", got, out)
	}
	if gjson.Get(out, "api.0.body.file_name").Exists() {
		t.Fatalf("file_name should stay local metadata, not export_tasks body\nstdout:\n%s", out)
	}
	if got := gjson.Get(out, "file_name").String(); got != "custom-report.pdf" {
		t.Fatalf("file_name=%q, want custom-report.pdf\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "output_dir").String(); got != "./exports" {
		t.Fatalf("output_dir=%q, want ./exports\nstdout:\n%s", got, out)
	}
}
