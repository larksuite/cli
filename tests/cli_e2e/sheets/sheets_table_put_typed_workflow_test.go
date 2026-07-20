// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/larksuite/cli/tests/cli_e2e/drive"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestSheets_TablePutTypedWorkflow is the live regression for the typed
// +table-put write path added in this branch. AGENTS.md requires a live E2E
// for new flows; this one writes a small typed payload (date / int / string
// columns + number_format) to a real spreadsheet and verifies +table-get reads
// it back as the same typed shape, locking the dtype + format contract that
// makes round-trip (pipe +table-get into +table-put) work.
func TestSheets_TablePutTypedWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	spreadsheetToken := createSpreadsheet(t, parentT, ctx, "lark-cli-e2e-tableput-typed-"+suffix, "bot")

	// Write a 3-row typed table whose first column is a date, second is an
	// int64 numeric column with a custom number_format, third is a plain
	// string. The "Sheet1" name comes from createSpreadsheet's default sheet.
	putRes, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+table-put",
			"--spreadsheet-token", spreadsheetToken,
			"--sheets", `{"sheets":[{"name":"Sheet1","columns":["日期","数量","备注"],"dtypes":{"日期":"datetime64[ns]","数量":"int64","备注":"object"},"formats":{"数量":"#,##0","日期":"yyyy-mm-dd"},"data":[["2024-01-15",1500,"开张"],["2024-02-02",2300,"补货"],["2024-03-10",4200,"促销"]]}]}`,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	putRes.AssertExitCode(t, 0)
	putRes.AssertStdoutStatus(t, true)
	require.Equal(t, int64(3), gjson.Get(putRes.Stdout, "data.sheets.0.data_rows").Int(),
		"data_rows should reflect the 3-row payload; stdout:\n%s", putRes.Stdout)

	// Read it back via +table-get and confirm the typed contract held: the
	// date column became a real date (datetime64[ns]) with the format we
	// asked for, the numeric column kept its int64 dtype and #,##0 format,
	// and the string column landed as object. Numeric values must come back
	// as numbers (not the formatted display string).
	getRes, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+table-get",
			"--spreadsheet-token", spreadsheetToken,
			"--sheet-name", "Sheet1",
			"--range", "A1:C4",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	getRes.AssertExitCode(t, 0)

	out := getRes.Stdout
	require.Equal(t, "datetime64[ns]", gjson.Get(out, "data.sheets.0.dtypes.日期").String(),
		"date column should round-trip as datetime64[ns]; stdout:\n%s", out)
	// The backend does not distinguish int64 from float64 in the typed wire,
	// so an int column written as int64 reads back as float64. Both are
	// numeric and that's the only contract a CLI agent should rely on.
	numericDtype := gjson.Get(out, "data.sheets.0.dtypes.数量").String()
	require.Regexp(t, `^(int|float)\d+$|^Int64$`, numericDtype,
		"numeric column should round-trip with a numeric dtype (int*/float*/Int64), got %q; stdout:\n%s", numericDtype, out)
	require.Equal(t, "object", gjson.Get(out, "data.sheets.0.dtypes.备注").String(),
		"string column should round-trip as object; stdout:\n%s", out)
	require.Equal(t, "#,##0", gjson.Get(out, "data.sheets.0.formats.数量").String(),
		"numeric column's number_format should round-trip; stdout:\n%s", out)
	require.Equal(t, "2024-01-15", gjson.Get(out, "data.sheets.0.data.0.0").String(),
		"first date should come back as the ISO string written; stdout:\n%s", out)
	require.Equal(t, int64(1500), gjson.Get(out, "data.sheets.0.data.0.1").Int(),
		"numeric value should round-trip as a number, not the formatted display; stdout:\n%s", out)
}

// TestSheets_WorkbookCreateTypedWorkflow is the live regression for the typed
// +workbook-create --sheets path added in this branch: it bundles "create
// spreadsheet" + "write typed data" into one shortcut, adopting the new
// workbook's default sheet as the first payload sheet. The test confirms the
// adopted sheet carries the typed data we sent (no empty "Sheet1" remains)
// and that --sheets's typed contract holds end-to-end, not just on +table-put.
func TestSheets_WorkbookCreateTypedWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	title := "lark-cli-e2e-wb-create-typed-" + suffix

	// One-shot: create workbook + write typed payload (date + int + string).
	// --folder-token is optional; omit it so the test does not depend on drive:drive
	// (CreateDriveFolder) when validating the typed --sheets path.
	createRes, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+workbook-create",
			"--title", title,
			"--sheets", `{"sheets":[{"name":"销售","columns":["日期","金额","渠道"],"dtypes":{"日期":"datetime64[ns]","金额":"float64","渠道":"object"},"formats":{"金额":"$#,##0.00","日期":"yyyy-mm-dd"},"data":[["2024-01-15",1500.5,"门店"],["2024-02-02",2300.75,"线上"]]}]}`,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if createRes.ExitCode != 0 {
		combined := strings.ToLower(createRes.Stdout + "\n" + createRes.Stderr)
		if strings.Contains(combined, "app_scope_not_applied") ||
			strings.Contains(combined, "missing_scopes") ||
			strings.Contains(combined, "99991672") {
			t.Skipf("skip workbook-create typed workflow due to missing bot scope: %s", strings.TrimSpace(createRes.Stdout+"\n"+createRes.Stderr))
		}
	}
	createRes.AssertExitCode(t, 0)
	createRes.AssertStdoutStatus(t, true)

	spreadsheetToken := gjson.Get(createRes.Stdout, "data.spreadsheet.spreadsheet_token").String()
	require.NotEmpty(t, spreadsheetToken, "workbook-create should return a spreadsheet_token; stdout:\n%s", createRes.Stdout)

	parentT.Cleanup(func() {
		cleanupCtx, cancelCleanup := clie2e.CleanupContext()
		defer cancelCleanup()
		deleteResult, deleteErr := drive.DeleteDriveResourceAndVerify(cleanupCtx, spreadsheetToken, "sheet", "bot")
		clie2e.ReportCleanupFailure(parentT, "delete spreadsheet "+spreadsheetToken, deleteResult, deleteErr)
	})

	// Adopted sheet must be the one we named in the payload (NOT an empty
	// default Sheet1), and it must carry our 2 typed rows.
	require.Equal(t, "销售", gjson.Get(createRes.Stdout, "data.sheets.0.name").String(),
		"workbook-create should adopt the default sheet under the payload sheet name; stdout:\n%s", createRes.Stdout)

	// Round-trip read confirms the typed contract held through create+write.
	getRes, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+table-get",
			"--spreadsheet-token", spreadsheetToken,
			"--sheet-name", "销售",
			"--range", "A1:C3",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	getRes.AssertExitCode(t, 0)

	out := getRes.Stdout
	require.Equal(t, "datetime64[ns]", gjson.Get(out, "data.sheets.0.dtypes.日期").String(),
		"date dtype should survive workbook-create + read-back; stdout:\n%s", out)
	require.Equal(t, "float64", gjson.Get(out, "data.sheets.0.dtypes.金额").String(),
		"numeric dtype should survive workbook-create + read-back; stdout:\n%s", out)
	require.Equal(t, "$#,##0.00", gjson.Get(out, "data.sheets.0.formats.金额").String(),
		"currency format should survive workbook-create + read-back; stdout:\n%s", out)
	require.Equal(t, "2024-01-15", gjson.Get(out, "data.sheets.0.data.0.0").String(),
		"first date should come back as ISO string; stdout:\n%s", out)
}
