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
		if _, err := validateSheetManageToken(runtime); err != nil {
			return err
		}
		length := runtime.Int("length")
		if length < 1 || length > 5000 {
			return common.FlagErrorf("--length must be between 1 and 5000, got %d", length)
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateSheetManageToken(runtime)
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
		token, _ := validateSheetManageToken(runtime)

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

var SheetInsertDimension = common.Shortcut{
	Service:     "sheets",
	Command:     "+insert-dimension",
	Description: "Insert rows or columns at a specified position",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "sheet-id", Desc: "worksheet ID", Required: true},
		{Name: "dimension", Desc: "ROWS or COLUMNS", Required: true, Enum: []string{"ROWS", "COLUMNS"}},
		{Name: "start-index", Type: "int", Desc: "start position (0-indexed)", Required: true},
		{Name: "end-index", Type: "int", Desc: "end position (0-indexed, exclusive)", Required: true},
		{Name: "inherit-style", Desc: "style inheritance: BEFORE or AFTER", Enum: []string{"BEFORE", "AFTER"}},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateSheetManageToken(runtime); err != nil {
			return err
		}
		if runtime.Int("start-index") < 0 {
			return common.FlagErrorf("--start-index must be >= 0")
		}
		if runtime.Int("end-index") <= runtime.Int("start-index") {
			return common.FlagErrorf("--end-index must be greater than --start-index")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateSheetManageToken(runtime)
		body := map[string]interface{}{
			"dimension": map[string]interface{}{
				"sheetId":        runtime.Str("sheet-id"),
				"majorDimension": runtime.Str("dimension"),
				"startIndex":     runtime.Int("start-index"),
				"endIndex":       runtime.Int("end-index"),
			},
		}
		if s := runtime.Str("inherit-style"); s != "" {
			body["inheritStyle"] = s
		}
		return common.NewDryRunAPI().
			POST("/open-apis/sheets/v2/spreadsheets/:token/insert_dimension_range").
			Body(body).
			Set("token", token)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateSheetManageToken(runtime)

		body := map[string]interface{}{
			"dimension": map[string]interface{}{
				"sheetId":        runtime.Str("sheet-id"),
				"majorDimension": runtime.Str("dimension"),
				"startIndex":     runtime.Int("start-index"),
				"endIndex":       runtime.Int("end-index"),
			},
		}
		if s := runtime.Str("inherit-style"); s != "" {
			body["inheritStyle"] = s
		}

		data, err := runtime.CallAPI("POST",
			fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/insert_dimension_range", validate.EncodePathSegment(token)),
			nil, body,
		)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var SheetUpdateDimension = common.Shortcut{
	Service:     "sheets",
	Command:     "+update-dimension",
	Description: "Update row or column properties (visibility, size)",
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
		{Name: "visible", Type: "bool", Desc: "true to show, false to hide"},
		{Name: "fixed-size", Type: "int", Desc: "row height or column width in pixels"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateSheetManageToken(runtime); err != nil {
			return err
		}
		if runtime.Int("start-index") < 1 {
			return common.FlagErrorf("--start-index must be >= 1")
		}
		if runtime.Int("end-index") < runtime.Int("start-index") {
			return common.FlagErrorf("--end-index must be >= --start-index")
		}
		if !runtime.Cmd.Flags().Changed("visible") && !runtime.Cmd.Flags().Changed("fixed-size") {
			return common.FlagErrorf("specify at least one of --visible or --fixed-size")
		}
		if runtime.Cmd.Flags().Changed("fixed-size") && runtime.Int("fixed-size") < 1 {
			return common.FlagErrorf("--fixed-size must be >= 1")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateSheetManageToken(runtime)
		props := map[string]interface{}{}
		if runtime.Cmd.Flags().Changed("visible") {
			props["visible"] = runtime.Bool("visible")
		}
		if runtime.Cmd.Flags().Changed("fixed-size") {
			props["fixedSize"] = runtime.Int("fixed-size")
		}
		return common.NewDryRunAPI().
			PUT("/open-apis/sheets/v2/spreadsheets/:token/dimension_range").
			Body(map[string]interface{}{
				"dimension": map[string]interface{}{
					"sheetId":        runtime.Str("sheet-id"),
					"majorDimension": runtime.Str("dimension"),
					"startIndex":     runtime.Int("start-index"),
					"endIndex":       runtime.Int("end-index"),
				},
				"dimensionProperties": props,
			}).
			Set("token", token)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateSheetManageToken(runtime)

		props := map[string]interface{}{}
		if runtime.Cmd.Flags().Changed("visible") {
			props["visible"] = runtime.Bool("visible")
		}
		if runtime.Cmd.Flags().Changed("fixed-size") {
			props["fixedSize"] = runtime.Int("fixed-size")
		}

		data, err := runtime.CallAPI("PUT",
			fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/dimension_range", validate.EncodePathSegment(token)),
			nil,
			map[string]interface{}{
				"dimension": map[string]interface{}{
					"sheetId":        runtime.Str("sheet-id"),
					"majorDimension": runtime.Str("dimension"),
					"startIndex":     runtime.Int("start-index"),
					"endIndex":       runtime.Int("end-index"),
				},
				"dimensionProperties": props,
			},
		)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

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
		if _, err := validateSheetManageToken(runtime); err != nil {
			return err
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
		token, _ := validateSheetManageToken(runtime)
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
		token, _ := validateSheetManageToken(runtime)

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
		if _, err := validateSheetManageToken(runtime); err != nil {
			return err
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
		token, _ := validateSheetManageToken(runtime)
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
		token, _ := validateSheetManageToken(runtime)

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
