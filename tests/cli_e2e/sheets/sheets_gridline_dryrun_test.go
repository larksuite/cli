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

// TestSheets_GridlineDryRun pins the +sheet-show-gridline / +sheet-hide-gridline
// dry-run shape: each emits a single modify_workbook_structure invoke_write with
// the correct operation name. These are the shortcuts added in this branch, so
// AGENTS.md requires a dry-run E2E to catch a request-shape regression early
// (before the live call hits a real spreadsheet).
func TestSheets_GridlineDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)

	tests := []struct {
		name       string
		shortcut   string
		wantOpName string
	}{
		{name: "show", shortcut: "+sheet-show-gridline", wantOpName: "show_gridline"},
		{name: "hide", shortcut: "+sheet-hide-gridline", wantOpName: "hide_gridline"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"sheets", tt.shortcut,
					"--spreadsheet-token", "shtDryRun",
					"--sheet-id", "sheet1",
					"--dry-run",
				},
				DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout

			require.Equal(t, "POST", gjson.Get(out, "api.0.method").String(), "stdout:\n%s", out)
			require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_write",
				gjson.Get(out, "api.0.url").String(), "stdout:\n%s", out)
			require.Equal(t, "modify_workbook_structure",
				gjson.Get(out, "api.0.body.tool_name").String(), "stdout:\n%s", out)
			input := gjson.Get(out, "api.0.body.input").String()
			require.Contains(t, input, `"operation":"`+tt.wantOpName+`"`, "stdout:\n%s", out)
			require.Contains(t, input, `"sheet_id":"sheet1"`, "stdout:\n%s", out)
		})
	}
}
