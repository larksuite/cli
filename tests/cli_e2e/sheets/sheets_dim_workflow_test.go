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

// TestSheets_DimShortcutsWorkflow is the live counterpart of the +dim-insert /
// +dim-delete dry-run pins: a column inserted with --inherit-style must land
// before --position (the flag only chooses which side's style is copied — the
// side-mapping regression made `after` shift the landing position), +dim-delete
// --range must remove it, and +dim-delete --ranges must delete scattered rows
// in one atomic call without index shift.
func TestSheets_DimShortcutsWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	spreadsheetToken := createSpreadsheet(t, parentT, ctx, "lark-cli-e2e-dim-"+suffix, "bot")

	infoRes, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"sheets", "+workbook-info", "--spreadsheet-token", spreadsheetToken},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	infoRes.AssertExitCode(t, 0)
	sheetID := gjson.Get(infoRes.Stdout, "data.sheets.0.sheet_id").String()
	require.NotEmpty(t, sheetID, "sheet_id should not be empty, stdout: %s", infoRes.Stdout)

	values := [][]any{
		{"r1c1", "r1c2", "r1c3"},
		{"r2c1", "r2c2", "r2c3"},
		{"r3c1", "r3c2", "r3c3"},
		{"r4c1", "r4c2", "r4c3"},
	}
	valuesJSON, _ := json.Marshal(values)
	writeRes, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+cells-set",
			"--spreadsheet-token", spreadsheetToken,
			"--sheet-id", sheetID,
			"--range", "A1:C4",
			"--cells", string(valuesJSON),
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	writeRes.AssertExitCode(t, 0)
	writeRes.AssertStdoutStatus(t, true)

	readRow1 := func(t *testing.T, rng string) string {
		t.Helper()
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+csv-get",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", sheetID,
				"--range", rng,
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		return result.Stdout
	}

	t.Run("dim-insert inherit-style before lands before position", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+dim-insert",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", sheetID,
				"--position", "B",
				"--count", "1",
				"--inherit-style", "before",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		// Old columns B/C shift right; the blank column sits at B.
		out := readRow1(t, "A1:D1")
		assert.Contains(t, out, "r1c1,,r1c2,r1c3",
			"inserted blank column must land before position B; stdout:\n%s", out)
	})

	t.Run("dim-delete --range removes the inserted column", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+dim-delete",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", sheetID,
				"--range", "B",
			},
			DefaultAs: "bot",
			Yes:       true,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		out := readRow1(t, "A1:C1")
		assert.Contains(t, out, "r1c1,r1c2,r1c3",
			"deleting column B must restore the original layout; stdout:\n%s", out)
	})

	t.Run("dim-insert inherit-style after also lands before position", func(t *testing.T) {
		// `after` was the regression direction: the side flag only picks which
		// neighbour's style is copied, never the landing side. A mapping
		// regression would put the blank after B instead.
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+dim-insert",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", sheetID,
				"--position", "B",
				"--count", "1",
				"--inherit-style", "after",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		out := readRow1(t, "A1:D1")
		assert.Contains(t, out, "r1c1,,r1c2,r1c3",
			"inherit-style=after must still land the blank before position B; stdout:\n%s", out)

		cleanup, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+dim-delete",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", sheetID,
				"--range", "B",
			},
			DefaultAs: "bot",
			Yes:       true,
		})
		require.NoError(t, err)
		cleanup.AssertExitCode(t, 0)
	})

	t.Run("dim-delete --ranges deletes scattered rows atomically", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+dim-delete",
				"--spreadsheet-token", spreadsheetToken,
				"--sheet-id", sheetID,
				"--ranges", `["2:2","4:4"]`,
			},
			DefaultAs: "bot",
			Yes:       true,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		// Rows 2 and 4 are gone; rows 1 and 3 close up as rows 1-2. If the
		// CLI deleted ascending without re-ordering, row 4's index would have
		// shifted and the wrong row would remain.
		out := readRow1(t, "A1:C2")
		assert.Contains(t, out, "r1c1,r1c2,r1c3", "row 1 must survive; stdout:\n%s", out)
		assert.Contains(t, out, "r3c1,r3c2,r3c3", "row 3 must survive as the new row 2; stdout:\n%s", out)
	})
}
