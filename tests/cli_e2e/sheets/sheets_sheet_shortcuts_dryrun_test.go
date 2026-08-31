// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func setSheetsDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}

func TestSheets_SheetShortcutsDryRunRejectsURLAndTokenTogether(t *testing.T) {
	setSheetsDryRunEnv(t)

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "sheet-create",
			args: []string{
				"sheets", "+sheet-create",
				"--url", "https://example.feishu.cn/sheets/shtFromURL",
				"--spreadsheet-token", "shtTOKEN",
				"--title", "Data",
				"--dry-run",
			},
		},
		{
			name: "sheet-copy",
			args: []string{
				"sheets", "+sheet-copy",
				"--url", "https://example.feishu.cn/sheets/shtFromURL",
				"--spreadsheet-token", "shtTOKEN",
				"--sheet-id", "sheet1",
				"--title", "Copy",
				"--dry-run",
			},
		},
		{
			name: "sheet-delete",
			args: []string{
				"sheets", "+sheet-delete",
				"--url", "https://example.feishu.cn/sheets/shtFromURL",
				"--spreadsheet-token", "shtTOKEN",
				"--sheet-id", "sheet1",
				"--dry-run",
			},
		},
		{
			name: "sheet-rename",
			args: []string{
				"sheets", "+sheet-rename",
				"--url", "https://example.feishu.cn/sheets/shtFromURL",
				"--spreadsheet-token", "shtTOKEN",
				"--sheet-id", "sheet1",
				"--title", "Renamed",
				"--dry-run",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tt.args,
				DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 2)
			combined := result.Stdout + "\n" + result.Stderr
			if !strings.Contains(combined, "mutually exclusive") {
				t.Fatalf("expected mutual exclusivity error, got:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
			}
		})
	}
}

// +sheet-copy is deliberately absent: an omitted --title there means "let the
// server name the copy", so an empty one is valid input, not a rejection.
func TestSheets_SheetShortcutsDryRunRejectsEmptyTitle(t *testing.T) {
	setSheetsDryRunEnv(t)

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "sheet-create",
			args: []string{
				"sheets", "+sheet-create",
				"--spreadsheet-token", "shtDryRun",
				"--title", "",
				"--dry-run",
			},
		},
		{
			name: "sheet-rename",
			args: []string{
				"sheets", "+sheet-rename",
				"--spreadsheet-token", "shtDryRun",
				"--sheet-id", "sheet1",
				"--title", "",
				"--dry-run",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tt.args,
				DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 2)
			combined := result.Stdout + "\n" + result.Stderr
			if !strings.Contains(combined, "--title is required") {
				t.Fatalf("expected empty-title error, got:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
			}
		})
	}
}

// The sub-sheet lifecycle shortcuts all reach the backend through one tool
// (modify_workbook_structure); what distinguishes them is the `operation` and
// the fields packed beside it, which is what these dry-runs pin.
func TestSheets_SheetShortcutsDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)

	const toolURL = "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_write"

	tests := []struct {
		name    string
		args    []string
		wantURL string
		wantFn  func(t *testing.T, out string)
	}{
		{
			name: "sheet-create",
			args: []string{
				"sheets", "+sheet-create",
				"--spreadsheet-token", "shtDryRun",
				"--title", "Data",
				"--index", "0",
				"--dry-run",
			},
			wantURL: toolURL,
			wantFn: func(t *testing.T, out string) {
				require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
				require.Equal(t, "create", clie2e.DryRunGet(out, "tool_input.operation").String(), "stdout:\n%s", out)
				require.Equal(t, "Data", clie2e.DryRunGet(out, "tool_input.sheet_name").String(), "stdout:\n%s", out)
				require.Equal(t, int64(0), clie2e.DryRunGet(out, "tool_input.target_index").Int(), "stdout:\n%s", out)
			},
		},
		{
			name: "sheet-copy",
			args: []string{
				"sheets", "+sheet-copy",
				"--spreadsheet-token", "shtDryRun",
				"--sheet-id", "sheet1",
				"--title", "Copy",
				"--index", "2",
				"--dry-run",
			},
			wantURL: toolURL,
			wantFn: func(t *testing.T, out string) {
				require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
				require.Equal(t, "duplicate", clie2e.DryRunGet(out, "tool_input.operation").String(), "stdout:\n%s", out)
				require.Equal(t, "sheet1", clie2e.DryRunGet(out, "tool_input.sheet_id").String(), "stdout:\n%s", out)
				require.Equal(t, "Copy", clie2e.DryRunGet(out, "tool_input.new_name").String(), "stdout:\n%s", out)
				require.Equal(t, int64(2), clie2e.DryRunGet(out, "tool_input.target_index").Int(), "stdout:\n%s", out)
			},
		},
		{
			// The omitted --title path TestSheets_SheetShortcutsDryRunRejectsEmptyTitle
			// deliberately excludes: no --title means "let the server name the
			// copy", so new_name must be absent from the payload rather than
			// sent empty, and the call must not be rejected.
			name: "sheet-copy without a title",
			args: []string{
				"sheets", "+sheet-copy",
				"--spreadsheet-token", "shtDryRun",
				"--sheet-id", "sheet1",
				"--dry-run",
			},
			wantURL: toolURL,
			wantFn: func(t *testing.T, out string) {
				require.Equal(t, "duplicate", clie2e.DryRunGet(out, "tool_input.operation").String(), "stdout:\n%s", out)
				require.Equal(t, "sheet1", clie2e.DryRunGet(out, "tool_input.sheet_id").String(), "stdout:\n%s", out)
				require.False(t, clie2e.DryRunGet(out, "tool_input.new_name").Exists(),
					"an omitted --title must leave new_name out of the payload, not send it empty; stdout:\n%s", out)
			},
		},
		{
			name: "sheet-delete",
			args: []string{
				"sheets", "+sheet-delete",
				"--spreadsheet-token", "shtDryRun",
				"--sheet-id", "sheet1",
				"--dry-run",
			},
			wantURL: toolURL,
			wantFn: func(t *testing.T, out string) {
				require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
				require.Equal(t, "delete", clie2e.DryRunGet(out, "tool_input.operation").String(), "stdout:\n%s", out)
				require.Equal(t, "sheet1", clie2e.DryRunGet(out, "tool_input.sheet_id").String(), "stdout:\n%s", out)
			},
		},
		{
			name: "sheet-rename",
			args: []string{
				"sheets", "+sheet-rename",
				"--spreadsheet-token", "shtDryRun",
				"--sheet-id", "sheet1",
				"--title", "Renamed",
				"--dry-run",
			},
			wantURL: toolURL,
			wantFn: func(t *testing.T, out string) {
				require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
				require.Equal(t, "rename", clie2e.DryRunGet(out, "tool_input.operation").String(), "stdout:\n%s", out)
				require.Equal(t, "sheet1", clie2e.DryRunGet(out, "tool_input.sheet_id").String(), "stdout:\n%s", out)
				require.Equal(t, "Renamed", clie2e.DryRunGet(out, "tool_input.new_name").String(), "stdout:\n%s", out)
			},
		},
		{
			name: "sheet-move",
			args: []string{
				"sheets", "+sheet-move",
				"--spreadsheet-token", "shtDryRun",
				"--sheet-id", "sheet1",
				"--index", "2",
				"--dry-run",
			},
			wantURL: toolURL,
			wantFn: func(t *testing.T, out string) {
				require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
				require.Equal(t, "move", clie2e.DryRunGet(out, "tool_input.operation").String(), "stdout:\n%s", out)
				require.Equal(t, "sheet1", clie2e.DryRunGet(out, "tool_input.sheet_id").String(), "stdout:\n%s", out)
				require.Equal(t, int64(2), clie2e.DryRunGet(out, "tool_input.target_index").Int(), "stdout:\n%s", out)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tt.args,
				DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			require.Equal(t, tt.wantURL, clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
			// Asserted for every case, not just create: the operation and its
			// fields alone would still match if a case started selecting a
			// different tool on the same /tools/invoke_write endpoint.
			require.Equal(t, "modify_workbook_structure", clie2e.DryRunGet(out, "tool_name").String(), "stdout:\n%s", out)
			tt.wantFn(t, out)
		})
	}
}

func TestSheets_DimInsertDryRunInheritAfterKeepsBeforePosition(t *testing.T) {
	setSheetsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+dim-insert",
			"--spreadsheet-token", "shtDryRun",
			"--sheet-id", "sheet1",
			"--position", "D",
			"--count", "1",
			"--inherit-style", "after",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_write", clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
	require.Equal(t, "modify_sheet_structure", clie2e.DryRunGet(out, "tool_name").String(), "stdout:\n%s", out)
	require.Equal(t, "insert", clie2e.DryRunGet(out, "tool_input.operation").String(), "stdout:\n%s", out)
	// inherit-style=after copies the following column's style via a plain
	// before-insert at the same position (the backend anchors on the following
	// column), so position stays D with side=before — the blank lands before D.
	require.Equal(t, "D", clie2e.DryRunGet(out, "tool_input.position").String(), "stdout:\n%s", out)
	require.Equal(t, int64(1), clie2e.DryRunGet(out, "tool_input.count").Int(), "stdout:\n%s", out)
	require.Equal(t, "before", clie2e.DryRunGet(out, "tool_input.side").String(), "inherit-style=after copies the following-side style via side=before; stdout:\n%s", out)
}
