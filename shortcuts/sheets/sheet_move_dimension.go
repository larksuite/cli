// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var SheetMoveDimension = common.Shortcut{
	Service:     "sheets",
	Command:     "+move-dimension",
	Description: "Move rows or columns to a new position",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "sheet-id", Desc: "worksheet ID", Required: true},
		{Name: "dimension", Desc: "ROWS or COLUMNS", Required: true, Enum: []string{"ROWS", "COLUMNS"}},
		{Name: "start-index", Type: "int", Desc: "source start position (0-indexed)", Required: true},
		{Name: "end-index", Type: "int", Desc: "source end position (0-indexed, inclusive)", Required: true},
		{Name: "destination-index", Type: "int", Desc: "target position to move to (0-indexed)", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}
		if token == "" {
			return common.FlagErrorf("specify --url or --spreadsheet-token")
		}
		if runtime.Int("start-index") < 0 {
			return common.FlagErrorf("--start-index must be >= 0")
		}
		if runtime.Int("end-index") < runtime.Int("start-index") {
			return common.FlagErrorf("--end-index must be >= --start-index")
		}
		if runtime.Int("destination-index") < 0 {
			return common.FlagErrorf("--destination-index must be >= 0")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}
		return common.NewDryRunAPI().
			POST("/open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/move_dimension").
			Body(map[string]interface{}{
				"source": map[string]interface{}{
					"major_dimension": runtime.Str("dimension"),
					"start_index":     runtime.Int("start-index"),
					"end_index":       runtime.Int("end-index"),
				},
				"destination_index": runtime.Int("destination-index"),
			}).
			Set("token", token).
			Set("sheet_id", runtime.Str("sheet-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}

		data, err := runtime.CallAPI("POST",
			fmt.Sprintf("/open-apis/sheets/v3/spreadsheets/%s/sheets/%s/move_dimension",
				validate.EncodePathSegment(token),
				validate.EncodePathSegment(runtime.Str("sheet-id")),
			),
			nil,
			map[string]interface{}{
				"source": map[string]interface{}{
					"major_dimension": runtime.Str("dimension"),
					"start_index":     runtime.Int("start-index"),
					"end_index":       runtime.Int("end-index"),
				},
				"destination_index": runtime.Int("destination-index"),
			},
		)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}
