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

// TestSheets_GridlineWorkflow round-trips +sheet-show-gridline and
// +sheet-hide-gridline against a real spreadsheet. The dry-run E2E pins the
// wire shape; this live test validates the backend accepts both operations
// end-to-end (the gridline state itself is write-only — there is no read
// field exposed via +sheet-info / +workbook-info — so success here is the
// ok=true envelope, not a value comparison).
func TestSheets_GridlineWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	spreadsheetToken := createSpreadsheet(t, parentT, ctx, "lark-cli-e2e-sheets-gridline-"+suffix, "bot")

	infoResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"sheets", "+info", "--spreadsheet-token", spreadsheetToken},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	infoResult.AssertExitCode(t, 0)
	infoResult.AssertStdoutStatus(t, true)
	sheetID := gjson.Get(infoResult.Stdout, "data.sheets.sheets.0.sheet_id").String()
	require.NotEmpty(t, sheetID, "sheet_id should not be empty, stdout: %s", infoResult.Stdout)

	for _, shortcut := range []string{"+sheet-hide-gridline", "+sheet-show-gridline"} {
		t.Run(shortcut+" as bot", func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"sheets", shortcut,
					"--spreadsheet-token", spreadsheetToken,
					"--sheet-id", sheetID,
				},
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			result.AssertStdoutStatus(t, true)
		})
	}
}
