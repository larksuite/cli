// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

// TestSheets_WorkbookExportDryRun pins the +workbook-export dry-run shape. It
// delegates to the shared drive export core but adds three sheet-specific
// guarantees that downstream agents rely on:
//
//  1. The doc type is hard-coded to "sheet" (drive +export would require
//     --doc-type sheet explicitly).
//  2. csv mode routes the --sheet-id flag onto the export_tasks body as
//     sub_id; xlsx mode omits sub_id.
//  3. The single --output-path flag collapses drive +export's --output-dir +
//     --file-name pair onto the dry-run plan's output_dir / file_name extras.
func TestSheets_WorkbookExportDryRun(t *testing.T) {
	t.Run("xlsx", func(t *testing.T) {
		setSheetsDryRunEnv(t)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+workbook-export",
				"--spreadsheet-token", "shtDryRunExport",
				"--file-extension", "xlsx",
				"--output-path", "./out.xlsx",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		out := result.Stdout
		require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
		require.Equal(t, "/open-apis/drive/v1/export_tasks",
			clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
		require.Equal(t, "shtDryRunExport", clie2e.DryRunGet(out, "api.0.body.token").String(), "stdout:\n%s", out)
		require.Equal(t, "sheet", clie2e.DryRunGet(out, "api.0.body.type").String(),
			"workbook-export must hard-code type=sheet; stdout:\n%s", out)
		require.Equal(t, "xlsx", clie2e.DryRunGet(out, "api.0.body.file_extension").String(), "stdout:\n%s", out)
		require.False(t, clie2e.DryRunGet(out, "api.0.body.sub_id").Exists(),
			"sub_id should be absent in xlsx mode; stdout:\n%s", out)
		require.Equal(t, "./out.xlsx", clie2e.DryRunGet(out, "output_dir").String(),
			"--output-path carries through to the dry-run plan's top-level output_dir; stdout:\n%s", out)
	})

	t.Run("csv requires sheet-id and emits sub_id", func(t *testing.T) {
		setSheetsDryRunEnv(t)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+workbook-export",
				"--spreadsheet-token", "shtDryRunExport",
				"--file-extension", "csv",
				"--sheet-id", "sheet1",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		out := result.Stdout
		require.Equal(t, "csv", clie2e.DryRunGet(out, "api.0.body.file_extension").String(), "stdout:\n%s", out)
		require.Equal(t, "sheet1", clie2e.DryRunGet(out, "api.0.body.sub_id").String(),
			"--sheet-id must reach sub_id in csv mode; stdout:\n%s", out)
	})

	t.Run("csv without sheet-id is rejected", func(t *testing.T) {
		setSheetsDryRunEnv(t)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+workbook-export",
				"--spreadsheet-token", "shtDryRunExport",
				"--file-extension", "csv",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		require.NotEqual(t, 0, result.ExitCode,
			"csv export without --sheet-id should surface a validation error; stdout:\n%s\nstderr:\n%s",
			result.Stdout, result.Stderr)
	})
}
