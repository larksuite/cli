// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	drivee2e "github.com/larksuite/cli/tests/cli_e2e/drive"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func createSpreadsheet(t *testing.T, parentT *testing.T, ctx context.Context, title string, defaultAs string) string {
	t.Helper()

	folderToken := drivee2e.CreateDriveFolder(t, parentT, ctx, title+"-folder", defaultAs, "")

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+create",
			"--title", title,
			"--folder-token", folderToken,
		},
		DefaultAs: defaultAs,
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	spreadsheetToken := gjson.Get(result.Stdout, "data.spreadsheet_token").String()
	require.NotEmpty(t, spreadsheetToken, "stdout:\n%s", result.Stdout)

	parentT.Cleanup(func() {
		cleanupCtx, cancel := clie2e.CleanupContext()
		defer cancel()

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args: []string{
				"drive", "+delete",
				"--file-token", spreadsheetToken,
				"--type", "sheet",
				"--yes",
			},
			DefaultAs: defaultAs,
		})
		clie2e.ReportCleanupFailure(parentT, "delete spreadsheet "+spreadsheetToken, deleteResult, deleteErr)
	})

	return spreadsheetToken
}
