// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/larksuite/cli/tests/cli_e2e/drive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestSheets_WorkbookImportWorkflow round-trips +workbook-import end to end:
// write a local CSV → import as a new Feishu spreadsheet → assert the import
// task finished (ready=true) with a sheet token → +info confirms the new
// workbook is reachable → cleanup deletes the spreadsheet.
//
// The dry-run E2E in sheets_workbook_import_dryrun_test.go pins the two-step
// request shape (media upload + import task with type=sheet); this live test
// validates the full flow including the async poll and that the resulting
// token is a usable sheet token.
func TestSheets_WorkbookImportWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	// CLI sandbox only accepts relative file paths under cwd; write the CSV
	// into a TempDir and hand RunCmd that as WorkDir so --file resolves.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.csv"),
		[]byte("Name,Age,City\nAlice,25,Beijing\nBob,30,Shanghai\n"), 0o644))

	suffix := clie2e.GenerateSuffix()
	title := "lark-cli-e2e-sheets-import-" + suffix
	folderToken := drive.CreateDriveFolder(t, parentT, ctx, title+"-folder", "bot", "")

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+workbook-import",
			"--file", "data.csv",
			"--name", title,
			"--folder-token", folderToken,
		},
		DefaultAs: "bot",
		WorkDir:   dir,
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	require.True(t, gjson.Get(result.Stdout, "data.ready").Bool(),
		"import task should be ready within poll window; stdout:\n%s", result.Stdout)
	assert.Equal(t, "sheet", gjson.Get(result.Stdout, "data.type").String(),
		"workbook-import hard-codes type=sheet; stdout:\n%s", result.Stdout)
	spreadsheetToken := gjson.Get(result.Stdout, "data.token").String()
	require.NotEmpty(t, spreadsheetToken, "spreadsheet token should not be empty; stdout:\n%s", result.Stdout)

	parentT.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		deleteResult, deleteErr := drive.DeleteDriveResourceAndVerify(cleanupCtx, spreadsheetToken, "sheet", "bot")
		clie2e.ReportCleanupFailure(parentT, "delete imported spreadsheet "+spreadsheetToken, deleteResult, deleteErr)
	})

	// Sanity: the imported token resolves through the sheets read path.
	infoResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"sheets", "+info", "--spreadsheet-token", spreadsheetToken},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	infoResult.AssertExitCode(t, 0)
	infoResult.AssertStdoutStatus(t, true)
	assert.True(t, gjson.Get(infoResult.Stdout, "data.sheets.sheets.0.sheet_id").Exists(),
		"imported workbook should expose at least one sub-sheet; stdout:\n%s", infoResult.Stdout)
}
