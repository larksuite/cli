// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestSheets_SheetListDryRun pins the +sheet-list request shape: a single
// get_workbook_structure invoke_READ carrying just the workbook token. The
// endpoint half matters as much as the tool name — the gateway 403s a read tool
// sent to invoke_write, and +sheet-list sits in a file whose siblings are all
// write shortcuts. +sheet-list is added in this branch, so AGENTS.md requires
// the dry-run E2E.
//
// The command is hidden from `sheets --help`, which is exactly why it needs
// executable coverage: nothing on the help surface would reveal a regression.
func TestSheets_SheetListDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "by token",
			args: []string{"--spreadsheet-token", "shtDryRun"},
		},
		{
			name: "by url",
			args: []string{"--url", "https://example.feishu.cn/sheets/shtDryRun"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      append([]string{"sheets", "+sheet-list"}, append(tt.args, "--dry-run")...),
				DefaultAs: "user",
			})
			if result != nil {
				result.Stdout = clie2e.DryRunData(result.Stdout)
			}
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout

			require.Equal(t, "POST", gjson.Get(out, "api.0.method").String(), "stdout:\n%s", out)
			require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_read",
				gjson.Get(out, "api.0.url").String(), "stdout:\n%s", out)
			require.Equal(t, "get_workbook_structure",
				gjson.Get(out, "api.0.body.tool_name").String(), "stdout:\n%s", out)
			require.Equal(t, `{"excel_id":"shtDryRun"}`,
				gjson.Get(out, "api.0.body.input").String(), "stdout:\n%s", out)
		})
	}
}
