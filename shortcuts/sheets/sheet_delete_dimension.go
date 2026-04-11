// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var SheetDeleteDimension = common.Shortcut{
	Service:     "sheets",
	Command:     "+delete-dimension",
	Description: "Delete rows or columns",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "sheet-id", Desc: "worksheet ID", Required: true},
		{Name: "dimension", Desc: "ROWS or COLUMNS", Required: true, Enum: []string{"ROWS", "COLUMNS"}},
		{Name: "start-index", Type: "int", Desc: "start position (1-indexed, inclusive)", Required: true},
		{Name: "end-index", Type: "int", Desc: "end position (1-indexed, inclusive)", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}
		if token == "" {
			return common.FlagErrorf("specify --url or --spreadsheet-token")
		}
		if runtime.Int("start-index") < 1 {
			return common.FlagErrorf("--start-index must be >= 1")
		}
		if runtime.Int("end-index") < runtime.Int("start-index") {
			return common.FlagErrorf("--end-index must be >= --start-index")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}
		return common.NewDryRunAPI().
			DELETE("/open-apis/sheets/v2/spreadsheets/:token/dimension_range").
			Body(map[string]interface{}{
				"dimension": map[string]interface{}{
					"sheetId":        runtime.Str("sheet-id"),
					"majorDimension": runtime.Str("dimension"),
					"startIndex":     runtime.Int("start-index"),
					"endIndex":       runtime.Int("end-index"),
				},
			}).
			Set("token", token)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}

		data, err := runtime.CallAPI("DELETE",
			fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/dimension_range", validate.EncodePathSegment(token)),
			nil,
			map[string]interface{}{
				"dimension": map[string]interface{}{
					"sheetId":        runtime.Str("sheet-id"),
					"majorDimension": runtime.Str("dimension"),
					"startIndex":     runtime.Int("start-index"),
					"endIndex":       runtime.Int("end-index"),
				},
			},
		)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}
