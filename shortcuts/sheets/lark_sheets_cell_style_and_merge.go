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

func validateBatchStyleData(raw string) error {
	var data interface{}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return common.FlagErrorf("--data must be valid JSON: %v", err)
	}
	arr, ok := data.([]interface{})
	if !ok || len(arr) == 0 {
		return common.FlagErrorf("--data must be a non-empty JSON array")
	}
	for i, item := range arr {
		entry, ok := item.(map[string]interface{})
		if !ok {
			return common.FlagErrorf("--data[%d] must be an object with ranges and style", i)
		}
		rangesRaw, ok := entry["ranges"]
		if !ok {
			return common.FlagErrorf("--data[%d].ranges is required", i)
		}
		ranges, ok := rangesRaw.([]interface{})
		if !ok || len(ranges) == 0 {
			return common.FlagErrorf("--data[%d].ranges must be a non-empty array of strings", i)
		}
		for j, r := range ranges {
			s, ok := r.(string)
			if !ok || s == "" {
				return common.FlagErrorf("--data[%d].ranges[%d] must be a non-empty string", i, j)
			}
			if _, _, ok := splitSheetRange(s); !ok {
				return common.FlagErrorf("--data[%d].ranges[%d] %q must include a sheetId! prefix", i, j, s)
			}
		}
		styleRaw, ok := entry["style"]
		if !ok {
			return common.FlagErrorf("--data[%d].style is required", i)
		}
		if _, ok := styleRaw.(map[string]interface{}); !ok {
			return common.FlagErrorf("--data[%d].style must be a JSON object", i)
		}
	}
	return nil
}

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
		r := normalizePointRange(runtime.Str("sheet-id"), runtime.Str("range"))
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

		r := normalizePointRange(runtime.Str("sheet-id"), runtime.Str("range"))
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

var SheetBatchSetStyle = common.Shortcut{
	Service:     "sheets",
	Command:     "+batch-set-style",
	Description: "Batch set cell styles for multiple ranges",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "data", Desc: "JSON array of {ranges, style} objects; each range must carry a sheetId! prefix (e.g. sheet1!A1)", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}
		if token == "" {
			return common.FlagErrorf("specify --url or --spreadsheet-token")
		}
		return validateBatchStyleData(runtime.Str("data"))
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}
		var data interface{}
		json.Unmarshal([]byte(runtime.Str("data")), &data)
		normalizeBatchStyleRanges(data)
		return common.NewDryRunAPI().
			PUT("/open-apis/sheets/v2/spreadsheets/:token/styles_batch_update").
			Body(map[string]interface{}{
				"data": data,
			}).
			Set("token", token)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}

		var data interface{}
		if err := json.Unmarshal([]byte(runtime.Str("data")), &data); err != nil {
			return common.FlagErrorf("--data must be valid JSON: %v", err)
		}
		normalizeBatchStyleRanges(data)

		result, err := runtime.CallAPI("PUT",
			fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/styles_batch_update", validate.EncodePathSegment(token)),
			nil,
			map[string]interface{}{
				"data": data,
			},
		)
		if err != nil {
			return err
		}
		runtime.Out(result, nil)
		return nil
	},
}

func normalizeBatchStyleRanges(data interface{}) {
	items, ok := data.([]interface{})
	if !ok {
		return
	}
	for _, item := range items {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		ranges, ok := entry["ranges"].([]interface{})
		if !ok {
			continue
		}
		for i, r := range ranges {
			if s, ok := r.(string); ok {
				ranges[i] = normalizePointRange("", s)
			}
		}
	}
}

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

var SheetUnmergeCells = common.Shortcut{
	Service:     "sheets",
	Command:     "+unmerge-cells",
	Description: "Unmerge (split) cells in a spreadsheet",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "range", Desc: "cell range (<sheetId>!A1:B2, or A1:B2 with --sheet-id)", Required: true},
		{Name: "sheet-id", Desc: "sheet ID (for relative range)"},
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
			POST("/open-apis/sheets/v2/spreadsheets/:token/unmerge_cells").
			Body(map[string]interface{}{
				"range": r,
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
			fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/unmerge_cells", validate.EncodePathSegment(token)),
			nil,
			map[string]interface{}{
				"range": r,
			},
		)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}
