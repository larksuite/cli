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
		var data interface{}
		if err := json.Unmarshal([]byte(runtime.Str("data")), &data); err != nil {
			return common.FlagErrorf("--data must be valid JSON: %v", err)
		}
		arr, ok := data.([]interface{})
		if !ok || len(arr) == 0 {
			return common.FlagErrorf("--data must be a non-empty JSON array")
		}
		return nil
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

// normalizeBatchStyleRanges mutates each string entry in data[].ranges in place
// so the /styles_batch_update endpoint accepts single-cell shorthand.
// Entries carrying a sheetId! prefix (e.g. "sheet1!A1") are expanded to
// "sheet1!A1:A1"; multi-cell spans pass through unchanged.
// A bare single cell without the sheetId! prefix (e.g. "A1") cannot be
// expanded because the helper has no sheet-id context (the shortcut exposes
// no --sheet-id flag), and the backend would reject the payload anyway —
// such entries pass through unchanged. Non-string entries, missing
// ranges keys, and non-array top-level inputs are ignored silently.
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
