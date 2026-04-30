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
	"github.com/tidwall/gjson"
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
			name: "create-sheet",
			args: []string{
				"sheets", "+create-sheet",
				"--url", "https://example.feishu.cn/sheets/shtFromURL",
				"--spreadsheet-token", "shtTOKEN",
				"--title", "Data",
				"--dry-run",
			},
		},
		{
			name: "copy-sheet",
			args: []string{
				"sheets", "+copy-sheet",
				"--url", "https://example.feishu.cn/sheets/shtFromURL",
				"--spreadsheet-token", "shtTOKEN",
				"--sheet-id", "sheet1",
				"--title", "Copy",
				"--dry-run",
			},
		},
		{
			name: "delete-sheet",
			args: []string{
				"sheets", "+delete-sheet",
				"--url", "https://example.feishu.cn/sheets/shtFromURL",
				"--spreadsheet-token", "shtTOKEN",
				"--sheet-id", "sheet1",
				"--dry-run",
			},
		},
		{
			name: "update-sheet",
			args: []string{
				"sheets", "+update-sheet",
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

func TestSheets_SheetShortcutsDryRunRejectsEmptyTitle(t *testing.T) {
	setSheetsDryRunEnv(t)

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "create-sheet",
			args: []string{
				"sheets", "+create-sheet",
				"--spreadsheet-token", "shtDryRun",
				"--title", "",
				"--dry-run",
			},
		},
		{
			name: "copy-sheet",
			args: []string{
				"sheets", "+copy-sheet",
				"--spreadsheet-token", "shtDryRun",
				"--sheet-id", "sheet1",
				"--title", "",
				"--dry-run",
			},
		},
		{
			name: "update-sheet",
			args: []string{
				"sheets", "+update-sheet",
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
			if !strings.Contains(combined, "must not be empty") {
				t.Fatalf("expected empty-title error, got:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
			}
		})
	}
}

func TestSheets_SheetShortcutsDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)

	tests := []struct {
		name    string
		args    []string
		wantURL string
		wantFn  func(t *testing.T, out string)
	}{
		{
			name: "create-sheet",
			args: []string{
				"sheets", "+create-sheet",
				"--spreadsheet-token", "shtDryRun",
				"--title", "Data",
				"--index", "0",
				"--dry-run",
			},
			wantURL: "/open-apis/sheets/v2/spreadsheets/shtDryRun/sheets_batch_update",
			wantFn: func(t *testing.T, out string) {
				require.Equal(t, "POST", gjson.Get(out, "api.0.method").String(), "stdout:\n%s", out)
				require.Equal(t, "Data", gjson.Get(out, "api.0.body.requests.0.addSheet.properties.title").String(), "stdout:\n%s", out)
				require.Equal(t, int64(0), gjson.Get(out, "api.0.body.requests.0.addSheet.properties.index").Int(), "stdout:\n%s", out)
			},
		},
		{
			name: "copy-sheet",
			args: []string{
				"sheets", "+copy-sheet",
				"--spreadsheet-token", "shtDryRun",
				"--sheet-id", "sheet1",
				"--title", "Copy",
				"--index", "2",
				"--dry-run",
			},
			wantURL: "/open-apis/sheets/v2/spreadsheets/shtDryRun/sheets_batch_update",
			wantFn: func(t *testing.T, out string) {
				require.Equal(t, "POST", gjson.Get(out, "api.0.method").String(), "stdout:\n%s", out)
				require.Equal(t, "sheet1", gjson.Get(out, "api.0.body.requests.0.copySheet.source.sheetId").String(), "stdout:\n%s", out)
				require.Equal(t, "Copy", gjson.Get(out, "api.0.body.requests.0.copySheet.destination.title").String(), "stdout:\n%s", out)
				require.Equal(t, "POST", gjson.Get(out, "api.1.method").String(), "stdout:\n%s", out)
				require.Equal(t, "<copied_sheet_id>", gjson.Get(out, "api.1.body.requests.0.updateSheet.properties.sheetId").String(), "stdout:\n%s", out)
				require.Equal(t, int64(2), gjson.Get(out, "api.1.body.requests.0.updateSheet.properties.index").Int(), "stdout:\n%s", out)
			},
		},
		{
			name: "delete-sheet",
			args: []string{
				"sheets", "+delete-sheet",
				"--spreadsheet-token", "shtDryRun",
				"--sheet-id", "sheet1",
				"--dry-run",
			},
			wantURL: "/open-apis/sheets/v2/spreadsheets/shtDryRun/sheets_batch_update",
			wantFn: func(t *testing.T, out string) {
				require.Equal(t, "POST", gjson.Get(out, "api.0.method").String(), "stdout:\n%s", out)
				require.Equal(t, "sheet1", gjson.Get(out, "api.0.body.requests.0.deleteSheet.sheetId").String(), "stdout:\n%s", out)
			},
		},
		{
			name: "update-sheet",
			args: []string{
				"sheets", "+update-sheet",
				"--spreadsheet-token", "shtDryRun",
				"--sheet-id", "sheet1",
				"--title", "Renamed",
				"--hidden=false",
				"--frozen-row-count", "2",
				"--frozen-col-count", "1",
				"--lock", "LOCK",
				"--lock-info", "private",
				"--user-ids", `["ou_1"]`,
				"--user-id-type", "open_id",
				"--dry-run",
			},
			wantURL: "/open-apis/sheets/v2/spreadsheets/shtDryRun/sheets_batch_update",
			wantFn: func(t *testing.T, out string) {
				require.Equal(t, "POST", gjson.Get(out, "api.0.method").String(), "stdout:\n%s", out)
				require.Equal(t, "open_id", gjson.Get(out, "api.0.params.user_id_type").String(), "stdout:\n%s", out)
				require.Equal(t, "sheet1", gjson.Get(out, "api.0.body.requests.0.updateSheet.properties.sheetId").String(), "stdout:\n%s", out)
				require.Equal(t, "Renamed", gjson.Get(out, "api.0.body.requests.0.updateSheet.properties.title").String(), "stdout:\n%s", out)
				require.Equal(t, false, gjson.Get(out, "api.0.body.requests.0.updateSheet.properties.hidden").Bool(), "stdout:\n%s", out)
				require.Equal(t, int64(2), gjson.Get(out, "api.0.body.requests.0.updateSheet.properties.frozenRowCount").Int(), "stdout:\n%s", out)
				require.Equal(t, int64(1), gjson.Get(out, "api.0.body.requests.0.updateSheet.properties.frozenColCount").Int(), "stdout:\n%s", out)
				require.Equal(t, "LOCK", gjson.Get(out, "api.0.body.requests.0.updateSheet.properties.protect.lock").String(), "stdout:\n%s", out)
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
			require.Equal(t, tt.wantURL, gjson.Get(out, "api.0.url").String(), "stdout:\n%s", out)
			tt.wantFn(t, out)
		})
	}
}
