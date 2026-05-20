// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

func parseValues2DJSON(raw string) ([][]interface{}, error) {
	var rows [][]interface{}
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil, common.FlagErrorf("--values invalid JSON, must be a 2D array")
	}
	if rows == nil {
		return nil, common.FlagErrorf("--values invalid JSON, must be a 2D array")
	}
	return rows, nil
}

var SheetRead = common.Shortcut{
	Service:     "sheets",
	Command:     "+read",
	Description: "Read spreadsheet cell values",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "range", Desc: "read range (<sheetId>!A1:D10, A1:D10 with --sheet-id, or a single cell like C2)"},
		{Name: "sheet-id", Desc: "sheet ID"},
		{Name: "value-render-option", Desc: "render option: ToString|FormattedValue|Formula|UnformattedValue"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateSheetManageToken(runtime); err != nil {
			return err
		}
		if err := validateSheetRangeInput(runtime.Str("sheet-id"), runtime.Str("range")); err != nil {
			return err
		}
		if r := runtime.Str("range"); r != "" {
			if rangeSheetID, _, ok := splitSheetRange(r); ok && runtime.Str("sheet-id") != "" && rangeSheetID != runtime.Str("sheet-id") {
				return common.FlagErrorf("--range sheet ID %q does not match --sheet-id %q", rangeSheetID, runtime.Str("sheet-id"))
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateSheetManageToken(runtime)
		readRange := runtime.Str("range")
		if readRange == "" && runtime.Str("sheet-id") != "" {
			readRange = runtime.Str("sheet-id")
		}
		readRange = normalizePointRange(runtime.Str("sheet-id"), readRange)
		return common.NewDryRunAPI().
			GET("/open-apis/sheets/v2/spreadsheets/:token/values/:range").
			Set("token", token).Set("range", readRange)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateSheetManageToken(runtime)

		readRange := runtime.Str("range")
		if readRange == "" && runtime.Str("sheet-id") != "" {
			readRange = runtime.Str("sheet-id")
		}

		if readRange == "" {
			var err error
			readRange, err = getFirstSheetID(runtime, token)
			if err != nil {
				return err
			}
		}
		readRange = normalizePointRange(runtime.Str("sheet-id"), readRange)

		params := map[string]interface{}{}
		renderOption := runtime.Str("value-render-option")
		if renderOption != "" {
			params["valueRenderOption"] = renderOption
		}

		data, err := runtime.CallAPI("GET", fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/values/%s", validate.EncodePathSegment(token), validate.EncodePathSegment(readRange)), params, nil)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var SheetWrite = common.Shortcut{
	Service:     "sheets",
	Command:     "+write",
	Description: "Write to spreadsheet cells (overwrite mode)",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "range", Desc: "write range (<sheetId>!A1:D10, A1:D10 with --sheet-id, or a single cell like C2)"},
		{Name: "sheet-id", Desc: "sheet ID"},
		{Name: "values", Desc: "2D array JSON", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateSheetManageToken(runtime); err != nil {
			return err
		}

		if _, err := parseValues2DJSON(runtime.Str("values")); err != nil {
			return err
		}
		if err := validateSheetRangeInput(runtime.Str("sheet-id"), runtime.Str("range")); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateSheetManageToken(runtime)
		writeRange := runtime.Str("range")
		if writeRange == "" && runtime.Str("sheet-id") != "" {
			writeRange = runtime.Str("sheet-id")
		}
		values, _ := parseValues2DJSON(runtime.Str("values"))
		writeRange = normalizeWriteRange(runtime.Str("sheet-id"), writeRange, values)
		return common.NewDryRunAPI().
			PUT("/open-apis/sheets/v2/spreadsheets/:token/values").
			Body(map[string]interface{}{"valueRange": map[string]interface{}{"range": writeRange, "values": values}}).
			Set("token", token)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateSheetManageToken(runtime)

		values, err := parseValues2DJSON(runtime.Str("values"))
		if err != nil {
			return err
		}

		writeRange := runtime.Str("range")
		if writeRange == "" && runtime.Str("sheet-id") != "" {
			writeRange = runtime.Str("sheet-id")
		}

		if writeRange == "" {
			var err error
			writeRange, err = getFirstSheetID(runtime, token)
			if err != nil {
				return err
			}
		}
		writeRange = normalizeWriteRange(runtime.Str("sheet-id"), writeRange, values)

		data, err := runtime.CallAPI("PUT", fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/values", validate.EncodePathSegment(token)), nil, map[string]interface{}{
			"valueRange": map[string]interface{}{
				"range":  writeRange,
				"values": values,
			},
		})
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var SheetAppend = common.Shortcut{
	Service:     "sheets",
	Command:     "+append",
	Description: "Append rows to a spreadsheet",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "range", Desc: "append range (<sheetId>!A1:D10, A1:D10 with --sheet-id, or a single cell like C2)"},
		{Name: "sheet-id", Desc: "sheet ID"},
		{Name: "values", Desc: "2D array JSON", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateSheetManageToken(runtime); err != nil {
			return err
		}

		if _, err := parseValues2DJSON(runtime.Str("values")); err != nil {
			return err
		}
		if err := validateSheetRangeInput(runtime.Str("sheet-id"), runtime.Str("range")); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateSheetManageToken(runtime)
		appendRange := runtime.Str("range")
		if appendRange == "" && runtime.Str("sheet-id") != "" {
			appendRange = runtime.Str("sheet-id")
		}
		values, _ := parseValues2DJSON(runtime.Str("values"))
		appendRange = normalizePointRange(runtime.Str("sheet-id"), appendRange)
		return common.NewDryRunAPI().
			POST("/open-apis/sheets/v2/spreadsheets/:token/values_append").
			Body(map[string]interface{}{"valueRange": map[string]interface{}{"range": appendRange, "values": values}}).
			Set("token", token)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateSheetManageToken(runtime)

		values, err := parseValues2DJSON(runtime.Str("values"))
		if err != nil {
			return err
		}

		appendRange := runtime.Str("range")
		if appendRange == "" && runtime.Str("sheet-id") != "" {
			appendRange = runtime.Str("sheet-id")
		}

		if appendRange == "" {
			var err error
			appendRange, err = getFirstSheetID(runtime, token)
			if err != nil {
				return err
			}
		}
		appendRange = normalizePointRange(runtime.Str("sheet-id"), appendRange)

		data, err := runtime.CallAPI("POST", fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/values_append", validate.EncodePathSegment(token)), nil, map[string]interface{}{
			"valueRange": map[string]interface{}{
				"range":  appendRange,
				"values": values,
			},
		})
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var SheetFind = common.Shortcut{
	Service:     "sheets",
	Command:     "+find",
	Description: "Find cells in a spreadsheet",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "sheet-id", Desc: "sheet ID", Required: true},
		{Name: "find", Desc: "search text", Required: true},
		{Name: "range", Desc: "search range (<sheetId>!A1:D10, or A1:D10 / C2 with --sheet-id)"},
		{Name: "ignore-case", Type: "bool", Desc: "case-insensitive search"},
		{Name: "match-entire-cell", Type: "bool", Desc: "match entire cell"},
		{Name: "search-by-regex", Type: "bool", Desc: "regex search"},
		{Name: "include-formulas", Type: "bool", Desc: "search formulas"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateSheetManageToken(runtime); err != nil {
			return err
		}
		if err := validateSheetRangeInput(runtime.Str("sheet-id"), runtime.Str("range")); err != nil {
			return err
		}
		if r := runtime.Str("range"); r != "" {
			if rangeSheetID, _, ok := splitSheetRange(r); ok && runtime.Str("sheet-id") != "" && rangeSheetID != runtime.Str("sheet-id") {
				return common.FlagErrorf("--range sheet ID %q does not match --sheet-id %q", rangeSheetID, runtime.Str("sheet-id"))
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateSheetManageToken(runtime)
		sheetID := runtime.Str("sheet-id")
		findCondition := map[string]interface{}{
			"range":             sheetID,
			"match_case":        !runtime.Bool("ignore-case"),
			"match_entire_cell": runtime.Bool("match-entire-cell"),
			"search_by_regex":   runtime.Bool("search-by-regex"),
			"include_formulas":  runtime.Bool("include-formulas"),
		}
		if runtime.Str("range") != "" {
			findCondition["range"] = normalizePointRange(sheetID, runtime.Str("range"))
		}
		return common.NewDryRunAPI().
			POST("/open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/find").
			Body(map[string]interface{}{
				"find":           runtime.Str("find"),
				"find_condition": findCondition,
			}).
			Set("token", token).Set("sheet_id", sheetID).Set("find", runtime.Str("find"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateSheetManageToken(runtime)

		sheetID := runtime.Str("sheet-id")
		findText := runtime.Str("find")

		findCondition := map[string]interface{}{
			"range":             sheetID,
			"match_case":        !runtime.Bool("ignore-case"),
			"match_entire_cell": runtime.Bool("match-entire-cell"),
			"search_by_regex":   runtime.Bool("search-by-regex"),
			"include_formulas":  runtime.Bool("include-formulas"),
		}
		if runtime.Str("range") != "" {
			findCondition["range"] = normalizePointRange(sheetID, runtime.Str("range"))
		}

		reqData := map[string]interface{}{
			"find_condition": findCondition,
			"find":           findText,
		}

		data, err := runtime.CallAPI("POST", fmt.Sprintf("/open-apis/sheets/v3/spreadsheets/%s/sheets/%s/find", validate.EncodePathSegment(token), validate.EncodePathSegment(sheetID)), nil, reqData)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var SheetReplace = common.Shortcut{
	Service:     "sheets",
	Command:     "+replace",
	Description: "Find and replace cell values in a spreadsheet",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "sheet-id", Desc: "sheet ID", Required: true},
		{Name: "find", Desc: "search text or regex pattern", Required: true},
		{Name: "replacement", Desc: "replacement text", Required: true},
		{Name: "range", Desc: "search range (<sheetId>!A1:D10, or A1:D10 with --sheet-id)"},
		{Name: "match-case", Type: "bool", Desc: "case-sensitive search"},
		{Name: "match-entire-cell", Type: "bool", Desc: "match entire cell content"},
		{Name: "search-by-regex", Type: "bool", Desc: "use regex search"},
		{Name: "include-formulas", Type: "bool", Desc: "search in formulas"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateSheetManageToken(runtime); err != nil {
			return err
		}
		if err := validateSheetRangeInput(runtime.Str("sheet-id"), runtime.Str("range")); err != nil {
			return err
		}
		if r := runtime.Str("range"); r != "" {
			if rangeSheetID, _, ok := splitSheetRange(r); ok && runtime.Str("sheet-id") != "" && rangeSheetID != runtime.Str("sheet-id") {
				return common.FlagErrorf("--range sheet ID %q does not match --sheet-id %q", rangeSheetID, runtime.Str("sheet-id"))
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateSheetManageToken(runtime)
		sheetID := runtime.Str("sheet-id")
		findCondition := map[string]interface{}{
			"range":             sheetID,
			"match_case":        runtime.Bool("match-case"),
			"match_entire_cell": runtime.Bool("match-entire-cell"),
			"search_by_regex":   runtime.Bool("search-by-regex"),
			"include_formulas":  runtime.Bool("include-formulas"),
		}
		if runtime.Str("range") != "" {
			findCondition["range"] = normalizeSheetRange(sheetID, runtime.Str("range"))
		}
		return common.NewDryRunAPI().
			POST("/open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/replace").
			Body(map[string]interface{}{
				"find_condition": findCondition,
				"find":           runtime.Str("find"),
				"replacement":    runtime.Str("replacement"),
			}).
			Set("token", token).Set("sheet_id", sheetID)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateSheetManageToken(runtime)

		sheetID := runtime.Str("sheet-id")
		findCondition := map[string]interface{}{
			"range":             sheetID,
			"match_case":        runtime.Bool("match-case"),
			"match_entire_cell": runtime.Bool("match-entire-cell"),
			"search_by_regex":   runtime.Bool("search-by-regex"),
			"include_formulas":  runtime.Bool("include-formulas"),
		}
		if runtime.Str("range") != "" {
			findCondition["range"] = normalizeSheetRange(sheetID, runtime.Str("range"))
		}

		data, err := runtime.CallAPI("POST",
			fmt.Sprintf("/open-apis/sheets/v3/spreadsheets/%s/sheets/%s/replace",
				validate.EncodePathSegment(token),
				validate.EncodePathSegment(sheetID),
			),
			nil,
			map[string]interface{}{
				"find_condition": findCondition,
				"find":           runtime.Str("find"),
				"replacement":    runtime.Str("replacement"),
			},
		)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}
