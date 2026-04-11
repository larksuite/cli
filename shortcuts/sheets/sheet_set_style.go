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

var SheetSetStyle = common.Shortcut{
	Service:     "sheets",
	Command:     "+set-style",
	Description: "Set cell style for a range",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "range", Desc: "cell range (<sheetId>!A1:B2, or A1:B2 with --sheet-id)", Required: true},
		{Name: "sheet-id", Desc: "sheet ID (for relative range)"},
		{Name: "style", Desc: "style JSON object (e.g. {\"font\":{\"bold\":true},\"backColor\":\"#ff0000\"})", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}
		if token == "" {
			return common.FlagErrorf("specify --url or --spreadsheet-token")
		}
		var style interface{}
		if err := json.Unmarshal([]byte(runtime.Str("style")), &style); err != nil {
			return common.FlagErrorf("--style must be valid JSON: %v", err)
		}
		if _, ok := style.(map[string]interface{}); !ok {
			return common.FlagErrorf("--style must be a JSON object, got %T", style)
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
		var style interface{}
		json.Unmarshal([]byte(runtime.Str("style")), &style)
		return common.NewDryRunAPI().
			PUT("/open-apis/sheets/v2/spreadsheets/:token/style").
			Body(map[string]interface{}{
				"appendStyle": map[string]interface{}{
					"range": r,
					"style": style,
				},
			}).
			Set("token", token)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}

		r := normalizeSheetRange(runtime.Str("sheet-id"), runtime.Str("range"))
		var style interface{}
		if err := json.Unmarshal([]byte(runtime.Str("style")), &style); err != nil {
			return common.FlagErrorf("--style must be valid JSON: %v", err)
		}

		data, err := runtime.CallAPI("PUT",
			fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/style", validate.EncodePathSegment(token)),
			nil,
			map[string]interface{}{
				"appendStyle": map[string]interface{}{
					"range": r,
					"style": style,
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
