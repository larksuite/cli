// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

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
		if r := runtime.Str("range"); r != "" {
			if rangeSheetID, _, ok := splitSheetRange(r); ok && runtime.Str("sheet-id") != "" && rangeSheetID != runtime.Str("sheet-id") {
				return common.FlagErrorf("--range sheet ID %q does not match --sheet-id %q", rangeSheetID, runtime.Str("sheet-id"))
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}
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
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}

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
