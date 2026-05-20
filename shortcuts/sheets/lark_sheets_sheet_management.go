// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var sheetProtectLockValues = []string{"LOCK", "UNLOCK"}

func sheetBatchUpdatePath(token string) string {
	return fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/sheets_batch_update", validate.EncodePathSegment(token))
}

func validateSheetManageToken(runtime *common.RuntimeContext) (string, error) {
	if err := common.ExactlyOne(runtime, "url", "spreadsheet-token"); err != nil {
		return "", err
	}
	if token := strings.TrimSpace(runtime.Str("spreadsheet-token")); token != "" {
		if err := validate.RejectControlChars(token, "spreadsheet-token"); err != nil {
			return "", common.FlagErrorf("%v", err)
		}
		return token, nil
	}

	url := strings.TrimSpace(runtime.Str("url"))
	if url == "" {
		return "", common.FlagErrorf("specify --url or --spreadsheet-token")
	}

	token := extractSpreadsheetToken(url)
	if token == "" || token == url {
		return "", common.FlagErrorf("--url must be a spreadsheet URL like https://.../sheets/<token>")
	}
	if err := validate.RejectControlChars(token, "url"); err != nil {
		return "", common.FlagErrorf("%v", err)
	}
	return token, nil
}

func validateSheetID(flagName, sheetID string) error {
	if strings.TrimSpace(sheetID) == "" {
		return common.FlagErrorf("specify --%s", flagName)
	}
	if err := validate.RejectControlChars(sheetID, flagName); err != nil {
		return common.FlagErrorf("%v", err)
	}
	return nil
}

func validateSheetTitle(flagName, title string) error {
	if title == "" {
		return common.FlagErrorf("--%s must not be empty", flagName)
	}
	if strings.ContainsAny(title, "\t\r\n") {
		return common.FlagErrorf("--%s must not contain tabs or line breaks", flagName)
	}
	if err := validate.RejectControlChars(title, flagName); err != nil {
		return common.FlagErrorf("%v", err)
	}
	if len([]rune(title)) > 100 {
		return common.FlagErrorf("--%s must be <= 100 characters", flagName)
	}
	if strings.ContainsAny(title, `/\?*[]:`) || strings.Contains(title, `\`) {
		return common.FlagErrorf("--%s must not contain any of / \\ ? * [ ] :", flagName)
	}
	return nil
}

func validateNonNegativeInt(flagName string, value int) error {
	if value < 0 {
		return common.FlagErrorf("--%s must be >= 0, got %d", flagName, value)
	}
	return nil
}

func buildSheetCreateProperties(runtime *common.RuntimeContext) map[string]interface{} {
	properties := map[string]interface{}{}
	if runtime.Changed("title") {
		properties["title"] = runtime.Str("title")
	}
	if runtime.Changed("index") {
		properties["index"] = runtime.Int("index")
	}
	return properties
}

func buildCreateSheetBody(runtime *common.RuntimeContext) map[string]interface{} {
	return map[string]interface{}{
		"requests": []interface{}{
			map[string]interface{}{
				"addSheet": map[string]interface{}{
					"properties": buildSheetCreateProperties(runtime),
				},
			},
		},
	}
}

func buildCopySheetBody(runtime *common.RuntimeContext) map[string]interface{} {
	copySheet := map[string]interface{}{
		"source": map[string]interface{}{
			"sheetId": runtime.Str("sheet-id"),
		},
	}
	if runtime.Changed("title") {
		copySheet["destination"] = map[string]interface{}{
			"title": runtime.Str("title"),
		}
	}
	return map[string]interface{}{
		"requests": []interface{}{
			map[string]interface{}{
				"copySheet": copySheet,
			},
		},
	}
}

func buildDeleteSheetBody(sheetID string) map[string]interface{} {
	return map[string]interface{}{
		"requests": []interface{}{
			map[string]interface{}{
				"deleteSheet": map[string]interface{}{
					"sheetId": sheetID,
				},
			},
		},
	}
}

func buildMoveCopiedSheetBody(sheetID string, index int) map[string]interface{} {
	return map[string]interface{}{
		"requests": []interface{}{
			map[string]interface{}{
				"updateSheet": map[string]interface{}{
					"properties": map[string]interface{}{
						"sheetId": sheetID,
						"index":   index,
					},
				},
			},
		},
	}
}

func normalizeSheetProperties(properties map[string]interface{}, titleChanged bool) map[string]interface{} {
	sheet := map[string]interface{}{}
	if v, ok := properties["sheetId"]; ok {
		sheet["sheet_id"] = v
	}
	if v, ok := properties["title"]; ok {
		if title, ok := v.(string); !ok || title != "" || titleChanged {
			sheet["title"] = v
		}
	}
	if v, ok := properties["index"]; ok {
		sheet["index"] = v
	}
	if v, ok := properties["hidden"]; ok {
		sheet["hidden"] = v
	}

	grid := map[string]interface{}{}
	if v, ok := properties["frozenRowCount"]; ok {
		grid["frozen_row_count"] = v
	}
	if v, ok := properties["frozenColCount"]; ok {
		grid["frozen_column_count"] = v
	}
	if len(grid) > 0 {
		sheet["grid_properties"] = grid
	}

	if protect, ok := properties["protect"].(map[string]interface{}); ok {
		outProtect := map[string]interface{}{}
		if v, ok := protect["lock"]; ok {
			outProtect["lock"] = v
		}
		if v, ok := protect["lockInfo"]; ok {
			outProtect["lock_info"] = v
		}
		if v, ok := protect["userIDs"]; ok {
			outProtect["user_ids"] = v
		}
		if len(outProtect) > 0 {
			sheet["protect"] = outProtect
		}
	}
	return sheet
}

func firstReply(data map[string]interface{}) (map[string]interface{}, bool) {
	replies, ok := data["replies"].([]interface{})
	if !ok || len(replies) == 0 {
		return nil, false
	}
	reply, ok := replies[0].(map[string]interface{})
	if !ok {
		return nil, false
	}
	return reply, true
}

func buildOperateSheetOutput(token string, data map[string]interface{}, opKey string, titleChanged bool) (map[string]interface{}, bool) {
	reply, ok := firstReply(data)
	if !ok {
		return nil, false
	}
	op, ok := reply[opKey].(map[string]interface{})
	if !ok {
		return nil, false
	}
	properties, ok := op["properties"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	sheet := normalizeSheetProperties(properties, titleChanged)
	out := map[string]interface{}{
		"spreadsheet_token": token,
		"sheet":             sheet,
	}
	if sheetID, ok := sheet["sheet_id"].(string); ok && sheetID != "" {
		out["sheet_id"] = sheetID
	}
	return out, true
}

func buildDeleteSheetOutput(token string, sheetID string, data map[string]interface{}) (map[string]interface{}, bool) {
	reply, ok := firstReply(data)
	if !ok {
		return nil, false
	}
	del, ok := reply["deleteSheet"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	out := map[string]interface{}{
		"spreadsheet_token": token,
		"sheet_id":          sheetID,
		"deleted":           true,
	}
	if v, ok := del["sheetId"].(string); ok && v != "" {
		out["sheet_id"] = v
	}
	if v, ok := del["result"].(bool); ok {
		out["deleted"] = v
	}
	return out, true
}

func mergeSheetOutputs(base, overlay map[string]interface{}) map[string]interface{} {
	if base == nil {
		return overlay
	}
	if overlay == nil {
		return base
	}
	out := map[string]interface{}{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		if k == "sheet" {
			baseSheet, _ := out["sheet"].(map[string]interface{})
			overlaySheet, _ := v.(map[string]interface{})
			mergedSheet := map[string]interface{}{}
			for sk, sv := range baseSheet {
				mergedSheet[sk] = sv
			}
			for sk, sv := range overlaySheet {
				mergedSheet[sk] = sv
			}
			out["sheet"] = mergedSheet
			continue
		}
		out[k] = v
	}
	return out
}

func mergeSheetErrorDetail(detail interface{}, overlay map[string]interface{}) interface{} {
	if len(overlay) == 0 {
		return detail
	}
	if detail == nil {
		return overlay
	}
	if existing, ok := detail.(map[string]interface{}); ok {
		merged := map[string]interface{}{}
		for k, v := range existing {
			merged[k] = v
		}
		for k, v := range overlay {
			merged[k] = v
		}
		return merged
	}

	merged := map[string]interface{}{}
	for k, v := range overlay {
		merged[k] = v
	}
	merged["cause_detail"] = detail
	return merged
}

func copySheetMoveRetryCommand(token, sheetID string, index int) string {
	return fmt.Sprintf("lark-cli sheets +update-sheet --spreadsheet-token %s --sheet-id %s --index %d", token, sheetID, index)
}

func wrapCopySheetMoveError(err error, token, sheetID string, index int) error {
	if strings.TrimSpace(sheetID) == "" {
		return err
	}

	retryCommand := copySheetMoveRetryCommand(token, sheetID, index)
	msg := fmt.Sprintf("sheet copied successfully as %q, but moving it to index %d failed", sheetID, index)
	hint := fmt.Sprintf(
		"do not retry +copy-sheet: the new sheet already exists as %s\nretry only the move with: %s",
		sheetID,
		retryCommand,
	)
	detail := map[string]interface{}{
		"partial_success":   true,
		"failed_step":       "move_copied_sheet",
		"spreadsheet_token": token,
		"sheet_id":          sheetID,
		"requested_index":   index,
		"retry_command":     retryCommand,
	}

	var exitErr *output.ExitError
	if errors.As(err, &exitErr) && exitErr.Detail != nil {
		if upstreamHint := strings.TrimSpace(exitErr.Detail.Hint); upstreamHint != "" {
			hint = upstreamHint + "\n" + hint
		}
		return &output.ExitError{
			Code: exitErr.Code,
			Detail: &output.ErrDetail{
				Type:       exitErr.Detail.Type,
				Code:       exitErr.Detail.Code,
				Message:    fmt.Sprintf("%s: %s", msg, exitErr.Detail.Message),
				Hint:       hint,
				ConsoleURL: exitErr.Detail.ConsoleURL,
				Risk:       exitErr.Detail.Risk,
				Detail:     mergeSheetErrorDetail(exitErr.Detail.Detail, detail),
			},
			Err: err,
			Raw: exitErr.Raw,
		}
	}

	return &output.ExitError{
		Code: output.ExitAPI,
		Detail: &output.ErrDetail{
			Type:    "api_error",
			Message: fmt.Sprintf("%s: %v", msg, err),
			Hint:    hint,
			Detail:  detail,
		},
		Err: err,
	}
}

func validateUpdateSheetFlags(runtime *common.RuntimeContext) error {
	if err := validateSheetID("sheet-id", runtime.Str("sheet-id")); err != nil {
		return err
	}
	if runtime.Changed("title") {
		if err := validateSheetTitle("title", runtime.Str("title")); err != nil {
			return err
		}
	}
	if runtime.Changed("index") {
		if err := validateNonNegativeInt("index", runtime.Int("index")); err != nil {
			return err
		}
	}
	if runtime.Changed("frozen-row-count") {
		if err := validateNonNegativeInt("frozen-row-count", runtime.Int("frozen-row-count")); err != nil {
			return err
		}
	}
	if runtime.Changed("frozen-col-count") {
		if err := validateNonNegativeInt("frozen-col-count", runtime.Int("frozen-col-count")); err != nil {
			return err
		}
	}
	if runtime.Changed("lock-info") {
		if err := validate.RejectControlChars(runtime.Str("lock-info"), "lock-info"); err != nil {
			return common.FlagErrorf("%v", err)
		}
	}

	hasProtectConfig := runtime.Changed("lock") || runtime.Changed("lock-info") || runtime.Changed("user-ids")
	if hasProtectConfig {
		lock := runtime.Str("lock")
		if !runtime.Changed("lock") {
			return common.FlagErrorf("specify --lock when updating protection settings")
		}
		if runtime.Changed("lock-info") && lock != "LOCK" {
			return common.FlagErrorf("--lock-info requires --lock LOCK")
		}
		if runtime.Changed("user-ids") {
			if lock != "LOCK" {
				return common.FlagErrorf("--user-ids requires --lock LOCK")
			}
			if runtime.Str("user-id-type") == "" {
				return common.FlagErrorf("--user-ids requires --user-id-type")
			}
			userIDs, err := parseJSONStringArray("user-ids", runtime.Str("user-ids"))
			if err != nil {
				return err
			}
			if len(userIDs) == 0 {
				return common.FlagErrorf("--user-ids must not be empty")
			}
		}
	}

	hasUpdate := runtime.Changed("title") ||
		runtime.Changed("index") ||
		runtime.Changed("hidden") ||
		runtime.Changed("frozen-row-count") ||
		runtime.Changed("frozen-col-count") ||
		hasProtectConfig
	if !hasUpdate {
		return common.FlagErrorf("specify at least one of --title, --index, --hidden, --frozen-row-count, --frozen-col-count, --lock, --lock-info, or --user-ids")
	}

	return nil
}

func buildUpdateSheetBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	properties := map[string]interface{}{
		"sheetId": runtime.Str("sheet-id"),
	}

	if runtime.Changed("title") {
		properties["title"] = runtime.Str("title")
	}
	if runtime.Changed("index") {
		properties["index"] = runtime.Int("index")
	}
	if runtime.Changed("hidden") {
		properties["hidden"] = runtime.Bool("hidden")
	}
	if runtime.Changed("frozen-row-count") {
		properties["frozenRowCount"] = runtime.Int("frozen-row-count")
	}
	if runtime.Changed("frozen-col-count") {
		properties["frozenColCount"] = runtime.Int("frozen-col-count")
	}
	if runtime.Changed("lock") || runtime.Changed("lock-info") || runtime.Changed("user-ids") {
		protect := map[string]interface{}{
			"lock": runtime.Str("lock"),
		}
		if runtime.Changed("lock-info") {
			protect["lockInfo"] = runtime.Str("lock-info")
		}
		if runtime.Changed("user-ids") {
			userIDs, err := parseJSONStringArray("user-ids", runtime.Str("user-ids"))
			if err != nil {
				return nil, err
			}
			protect["userIDs"] = userIDs
		}
		properties["protect"] = protect
	}

	return map[string]interface{}{
		"requests": []interface{}{
			map[string]interface{}{
				"updateSheet": map[string]interface{}{
					"properties": properties,
				},
			},
		},
	}, nil
}

func buildUpdateSheetOutput(token string, data map[string]interface{}, titleChanged bool) (map[string]interface{}, bool) {
	return buildOperateSheetOutput(token, data, "updateSheet", titleChanged)
}

var SheetCreateSheet = common.Shortcut{
	Service:     "sheets",
	Command:     "+create-sheet",
	Description: "Create a sheet in an existing spreadsheet",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "title", Desc: "sheet title"},
		{Name: "index", Type: "int", Desc: "sheet index (0-based)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateSheetManageToken(runtime); err != nil {
			return err
		}
		if runtime.Changed("title") {
			if err := validateSheetTitle("title", runtime.Str("title")); err != nil {
				return err
			}
		}
		if runtime.Changed("index") {
			if err := validateNonNegativeInt("index", runtime.Int("index")); err != nil {
				return err
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateSheetManageToken(runtime)
		return common.NewDryRunAPI().
			POST("/open-apis/sheets/v2/spreadsheets/:token/sheets_batch_update").
			Body(buildCreateSheetBody(runtime)).
			Set("token", token)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateSheetManageToken(runtime)
		data, err := runtime.CallAPI("POST", sheetBatchUpdatePath(token), nil, buildCreateSheetBody(runtime))
		if err != nil {
			return err
		}
		if out, ok := buildOperateSheetOutput(token, data, "addSheet", runtime.Changed("title")); ok {
			runtime.Out(out, nil)
			return nil
		}
		runtime.Out(data, nil)
		return nil
	},
}

var SheetCopySheet = common.Shortcut{
	Service:     "sheets",
	Command:     "+copy-sheet",
	Description: "Copy a sheet within a spreadsheet",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "sheet-id", Desc: "source sheet ID", Required: true},
		{Name: "title", Desc: "new sheet title"},
		{Name: "index", Type: "int", Desc: "new sheet index (0-based)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateSheetManageToken(runtime); err != nil {
			return err
		}
		if err := validateSheetID("sheet-id", runtime.Str("sheet-id")); err != nil {
			return err
		}
		if runtime.Changed("title") {
			if err := validateSheetTitle("title", runtime.Str("title")); err != nil {
				return err
			}
		}
		if runtime.Changed("index") {
			if err := validateNonNegativeInt("index", runtime.Int("index")); err != nil {
				return err
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateSheetManageToken(runtime)
		dry := common.NewDryRunAPI().
			POST("/open-apis/sheets/v2/spreadsheets/:token/sheets_batch_update").
			Desc("[1] Copy sheet").
			Body(buildCopySheetBody(runtime)).
			Set("token", token)
		if runtime.Changed("index") {
			dry.POST("/open-apis/sheets/v2/spreadsheets/:token/sheets_batch_update").
				Desc("[2] Move copied sheet to requested index").
				Body(buildMoveCopiedSheetBody("<copied_sheet_id>", runtime.Int("index"))).
				Set("token", token)
		}
		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateSheetManageToken(runtime)
		data, err := runtime.CallAPI("POST", sheetBatchUpdatePath(token), nil, buildCopySheetBody(runtime))
		if err != nil {
			return err
		}
		out, ok := buildOperateSheetOutput(token, data, "copySheet", runtime.Changed("title"))
		if !ok {
			runtime.Out(data, nil)
			return nil
		}
		if runtime.Changed("index") {
			copiedSheetID, _ := out["sheet_id"].(string)
			moveResp, err := runtime.CallAPI("POST", sheetBatchUpdatePath(token), nil, buildMoveCopiedSheetBody(copiedSheetID, runtime.Int("index")))
			if err != nil {
				return wrapCopySheetMoveError(err, token, copiedSheetID, runtime.Int("index"))
			}
			if moveOut, ok := buildUpdateSheetOutput(token, moveResp, false); ok {
				out = mergeSheetOutputs(out, moveOut)
			}
		}
		runtime.Out(out, nil)
		return nil
	},
}

var SheetDeleteSheet = common.Shortcut{
	Service:     "sheets",
	Command:     "+delete-sheet",
	Description: "Delete a sheet from a spreadsheet",
	Risk:        "high-risk-write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "sheet-id", Desc: "sheet ID to delete", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateSheetManageToken(runtime); err != nil {
			return err
		}
		return validateSheetID("sheet-id", runtime.Str("sheet-id"))
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateSheetManageToken(runtime)
		return common.NewDryRunAPI().
			POST("/open-apis/sheets/v2/spreadsheets/:token/sheets_batch_update").
			Body(buildDeleteSheetBody(runtime.Str("sheet-id"))).
			Set("token", token)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateSheetManageToken(runtime)
		data, err := runtime.CallAPI("POST", sheetBatchUpdatePath(token), nil, buildDeleteSheetBody(runtime.Str("sheet-id")))
		if err != nil {
			return err
		}
		if out, ok := buildDeleteSheetOutput(token, runtime.Str("sheet-id"), data); ok {
			runtime.Out(out, nil)
			return nil
		}
		runtime.Out(data, nil)
		return nil
	},
}

var SheetUpdateSheet = common.Shortcut{
	Service:     "sheets",
	Command:     "+update-sheet",
	Description: "Update sheet properties",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "sheet-id", Desc: "sheet ID", Required: true},
		{Name: "title", Desc: "sheet title"},
		{Name: "index", Type: "int", Desc: "sheet index (0-based)"},
		{Name: "hidden", Type: "bool", Desc: "set true to hide or false to unhide"},
		{Name: "frozen-row-count", Type: "int", Desc: "freeze rows through this count (0 unfreezes)"},
		{Name: "frozen-col-count", Type: "int", Desc: "freeze columns through this count (0 unfreezes)"},
		{Name: "lock", Desc: "sheet protection mode", Enum: sheetProtectLockValues},
		{Name: "lock-info", Desc: "protection remark"},
		{Name: "user-ids", Desc: `extra editor IDs for protected sheet as JSON array (e.g. '["ou_xxx"]')`},
		{Name: "user-id-type", Desc: "user ID type for --user-ids", Enum: []string{"open_id", "union_id", "lark_id", "user_id"}},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateSheetManageToken(runtime); err != nil {
			return err
		}
		return validateUpdateSheetFlags(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateSheetManageToken(runtime)
		body, _ := buildUpdateSheetBody(runtime)
		dry := common.NewDryRunAPI().
			POST("/open-apis/sheets/v2/spreadsheets/:token/sheets_batch_update").
			Body(body).
			Set("token", token)
		if userIDType := runtime.Str("user-id-type"); userIDType != "" {
			dry.Params(map[string]interface{}{"user_id_type": userIDType})
		}
		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateSheetManageToken(runtime)
		body, err := buildUpdateSheetBody(runtime)
		if err != nil {
			return err
		}
		var params map[string]interface{}
		if userIDType := runtime.Str("user-id-type"); userIDType != "" {
			params = map[string]interface{}{"user_id_type": userIDType}
		}

		data, err := runtime.CallAPI("POST", sheetBatchUpdatePath(token), params, body)
		if err != nil {
			return err
		}
		if out, ok := buildUpdateSheetOutput(token, data, runtime.Changed("title")); ok {
			runtime.Out(out, nil)
			return nil
		}
		runtime.Out(data, nil)
		return nil
	},
}
