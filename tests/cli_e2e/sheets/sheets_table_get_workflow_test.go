// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestSheets_TableGetUsedRangeWorkflow is the live regression for the pro016 /
// pro025 incident: data with an internal blank row (and a blank separator
// column) must read back in full when +table-get is run without --range. Before
// the fix, the default read used the A1 current region, which stopped at the
// first blank row/column and silently truncated everything past it.
//
// Layout written (header + 9 data rows, blank row at sheet row 6, blank column
// D between the A:C block and the E:F block):
//
//	row 1:  name  age  city  <blank>  x1  x2
//	rows 2-5: data
//	row 6:  <entirely blank>
//	rows 7-10: data
//
// The true used range is A1:F10. The default +table-get must return all 9 data
// rows and 6 columns and report a range covering row 10 / column F.
func TestSheets_TableGetUsedRangeWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	spreadsheetToken := createSpreadsheet(t, parentT, ctx, "lark-cli-e2e-tableget-"+suffix, "bot")

	infoRes, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"sheets", "+info", "--spreadsheet-token", spreadsheetToken},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	infoRes.AssertExitCode(t, 0)
	sheetID := gjson.Get(infoRes.Stdout, "data.sheets.sheets.0.sheet_id").String()
	require.NotEmpty(t, sheetID, "sheet_id should not be empty, stdout: %s", infoRes.Stdout)

	// Write the full A1:F10 block in one shot. Column D and row 6 are left blank
	// (empty strings) so the A1 current region would truncate at them.
	blank := ""
	values := [][]any{
		{"name", "age", "city", blank, "x1", "x2"},
		{"Alice", 30, "NY", blank, "p", "q"},
		{"Bob", 25, "LA", blank, "r", "s"},
		{"Carol", 40, "SF", blank, "t", "u"},
		{"Dave", 22, "TX", blank, "v", "w"},
		{blank, blank, blank, blank, blank, blank}, // row 6: entirely blank
		{"Eve", 33, "BOS", blank, "a", "b"},
		{"Frank", 28, "SEA", blank, "c", "d"},
		{"Grace", 45, "DEN", blank, "e", "f"},
		{"Hank", 50, "PHX", blank, "g", "h"},
	}
	valuesJSON, _ := json.Marshal(values)

	writeRes, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+write",
			"--spreadsheet-token", spreadsheetToken,
			"--sheet-id", sheetID,
			"--range", "A1:F10",
			"--values", string(valuesJSON),
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	writeRes.AssertExitCode(t, 0)
	writeRes.AssertStdoutStatus(t, true)

	t.Run("default table-get spans the internal blank row/column", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+table-get",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", sheetID,
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		sheet := gjson.Get(result.Stdout, "data.sheets.0")
		rows := sheet.Get("data").Array()
		require.Equal(t, 9, len(rows),
			"default table-get must return all 9 data rows (not truncate at the blank row 6); stdout:\n%s", result.Stdout)

		// The last data row must be present — the regression dropped everything
		// after the blank row.
		lastRow := rows[len(rows)-1].Array()
		assert.Equal(t, "Hank", lastRow[0].String(), "last data row should be Hank; stdout:\n%s", result.Stdout)

		// Columns must span past the blank separator column D to reach x1 / x2.
		cols := sheet.Get("columns").Array()
		require.Equal(t, 6, len(cols), "must read all 6 columns across the blank column D; stdout:\n%s", result.Stdout)
		assert.Equal(t, "x1", cols[4].String())
		assert.Equal(t, "x2", cols[5].String())

		// The reported range must cover the true used range (row 10, column F),
		// so a caller can detect truncation by inspecting it.
		rng := sheet.Get("range").String()
		require.NotEmpty(t, rng, "table-get output must report the range actually read; stdout:\n%s", result.Stdout)
		assert.Contains(t, rng, "10", "reported range should reach row 10; got %q", rng)
		assert.Contains(t, rng, "F", "reported range should reach column F; got %q", rng)
	})
}
