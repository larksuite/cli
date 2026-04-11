// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var SheetMergeCells = common.Shortcut{
	Service:     "sheets",
	Command:     "+merge-cells",
	Description: "Merge cells in a spreadsheet",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "range", Desc: "cell range (<sheetId>!A1:B2, or A1:B2 with --sheet-id)", Required: true},
		{Name: "sheet-id", Desc: "sheet ID (for relative range)"},
		{Name: "merge-type", Desc: "merge method", Required: true, Enum: []string{"MERGE_ALL", "MERGE_ROWS", "MERGE_COLUMNS"}},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}
		if token == "" {
			return common.FlagErrorf("specify --url or --spreadsheet-token")
		}
		if err := validateSheetRangeInput(runtime.Str("sheet-id"), runtime.Str("range")); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}
		r := normalizeSheetRange(runtime.Str("sheet-id"), runtime.Str("range"))
		return common.NewDryRunAPI().
			POST("/open-apis/sheets/v2/spreadsheets/:token/merge_cells").
			Body(map[string]interface{}{
				"range":     r,
				"mergeType": runtime.Str("merge-type"),
			}).
			Set("token", token)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}

		r := normalizeSheetRange(runtime.Str("sheet-id"), runtime.Str("range"))

		data, err := runtime.CallAPI("POST",
			fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/merge_cells", validate.EncodePathSegment(token)),
			nil,
			map[string]interface{}{
				"range":     r,
				"mergeType": runtime.Str("merge-type"),
			},
		)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}
