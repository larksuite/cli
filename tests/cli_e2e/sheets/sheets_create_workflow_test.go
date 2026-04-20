// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSheets_CreateWorkflowAsUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	parentT := t
	suffix := clie2e.GenerateSuffix()
	title := "lark-cli-e2e-user-sheets-" + suffix
	var spreadsheetToken string

	t.Run("create spreadsheet with +create as user", func(t *testing.T) {
		spreadsheetToken = createSpreadsheet(t, parentT, ctx, title, "user")
	})

	t.Run("get spreadsheet info with +info as user", func(t *testing.T) {
		require.NotEmpty(t, spreadsheetToken, "spreadsheet token is required")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"sheets", "+info", "--spreadsheet-token", spreadsheetToken},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		assert.Equal(t, spreadsheetToken, gjson.Get(result.Stdout, "data.spreadsheet.spreadsheet.token").String())
		require.NotEmpty(t, gjson.Get(result.Stdout, "data.sheets.sheets.0.sheet_id").String(), "stdout:\n%s", result.Stdout)
	})
}
