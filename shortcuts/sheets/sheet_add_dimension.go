// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var SheetAddDimension = common.Shortcut{
	Service:     "sheets",
	Command:     "+add-dimension",
	Description: "Add rows or columns at the end of a sheet",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "sheet-id", Desc: "worksheet ID", Required: true},
		{Name: "dimension", Desc: "ROWS or COLUMNS", Required: true, Enum: []string{"ROWS", "COLUMNS"}},
		{Name: "length", Type: "int", Desc: "number of rows/columns to add (1-5000)", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}
		if token == "" {
			return common.FlagErrorf("specify --url or --spreadsheet-token")
		}
		length := runtime.Int("length")
		if length < 1 || length > 5000 {
			return common.FlagErrorf("--length must be between 1 and 5000, got %d", length)
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}
		return common.NewDryRunAPI().
			POST("/open-apis/sheets/v2/spreadsheets/:token/dimension_range").
			Body(map[string]interface{}{
				"dimension": map[string]interface{}{
					"sheetId":        runtime.Str("sheet-id"),
					"majorDimension": runtime.Str("dimension"),
					"length":         runtime.Int("length"),
				},
			}).
			Set("token", token)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}

		data, err := runtime.CallAPI("POST",
			fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/dimension_range", validate.EncodePathSegment(token)),
			nil,
			map[string]interface{}{
				"dimension": map[string]interface{}{
					"sheetId":        runtime.Str("sheet-id"),
					"majorDimension": runtime.Str("dimension"),
					"length":         runtime.Int("length"),
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
