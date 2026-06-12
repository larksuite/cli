// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package backward

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var sheetProtectLockValues = []string{"LOCK", "UNLOCK"}

func sheetBatchUpdatePath(token string) string {
	return fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/sheets_batch_update", validate.EncodePathSegment(token))
}

func validateSheetManageToken(runtime *common.RuntimeContext) (string, error) {
	if err := common.ExactlyOneTyped(runtime, "url", "spreadsheet-token"); err != nil {
		return "", err
	}
	if token := strings.TrimSpace(runtime.Str("spreadsheet-token")); token != "" {
		if err := validate.RejectControlChars(token, "spreadsheet-token"); err != nil {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "%v", err).WithParam("--spreadsheet-token").WithCause(err)
		}
		return token, nil
	}

	url := strings.TrimSpace(runtime.Str("url"))
	if url == "" {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "specify --url or --spreadsheet-token").WithParams(errs.InvalidParam{Name: "--url", Reason: "required; specify one"}, errs.InvalidParam{Name: "--spreadsheet-token", Reason: "required; specify one"})
	}

	token := extractSpreadsheetToken(url)
	if token == "" || token == url {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--url must be a spreadsheet URL like https://.../sheets/<token>").WithParam("--url")
	}
	if err := validate.RejectControlChars(token, "url"); err != nil {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "%v", err).WithParam("--url").WithCause(err)
	}
	return token, nil
}

func validateSheetID(flagName, sheetID string) error {
	if strings.TrimSpace(sheetID) == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "specify --%s", flagName).WithParam("--" + flagName)
	}
	if err := validate.RejectControlChars(sheetID, flagName); err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "%v", err).WithParam("--" + flagName).WithCause(err)
	}
	return nil
}

func validateSheetTitle(flagName, title string) error {
	if title == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--%s must not be empty", flagName).WithParam("--" + flagName)
	}
	if strings.ContainsAny(title, "\t\r\n") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--%s must not contain tabs or line breaks", flagName).WithParam("--" + flagName)
	}
	if err := validate.RejectControlChars(title, flagName); err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "%v", err).WithParam("--" + flagName).WithCause(err)
	}
	if len([]rune(title)) > 100 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--%s must be <= 100 characters", flagName).WithParam("--" + flagName)
	}
	if strings.ContainsAny(title, `/\?*[]:`) || strings.Contains(title, `\`) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--%s must not contain any of / \\ ? * [ ] :", flagName).WithParam("--" + flagName)
	}
	return nil
}

func validateNonNegativeInt(flagName string, value int) error {
	if value < 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--%s must be >= 0, got %d", flagName, value).WithParam("--" + flagName)
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

func copySheetMoveRetryCommand(token, sheetID string, index int) string {
	return fmt.Sprintf("lark-cli sheets +update-sheet --spreadsheet-token %s --sheet-id %s --index %d", token, sheetID, index)
}

// wrapCopySheetMoveError reports a +copy-sheet that created the new sheet but
// then failed to move it to the requested index. The copy already succeeded, so
// the recovery is to retry only the move (not the whole +copy-sheet, which would
// duplicate the sheet) — that guard and the exact retry command go into the
// hint. The underlying move error is already a typed errs.* error from
// CallAPITyped; its category/subtype/code/log_id are preserved in place
// (mirroring drive's enrichDriveSearchError) so the failure stays accurately
// classified, with only the partial-success context folded into message and hint.
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

	if p, ok := errs.ProblemOf(err); ok {
		if upstream := strings.TrimSpace(p.Message); upstream != "" {
			p.Message = fmt.Sprintf("%s: %s", msg, upstream)
		} else {
			p.Message = msg
		}
		if upstreamHint := strings.TrimSpace(p.Hint); upstreamHint != "" {
			p.Hint = upstreamHint + "\n" + hint
		} else {
			p.Hint = hint
		}
		return err
	}

	return errs.NewInternalError(errs.SubtypeSDKError, "%s: %v", msg, err).WithHint(hint).WithCause(err)
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
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%v", err).WithParam("--lock-info").WithCause(err)
		}
	}

	hasProtectConfig := runtime.Changed("lock") || runtime.Changed("lock-info") || runtime.Changed("user-ids")
	if hasProtectConfig {
		lock := runtime.Str("lock")
		if !runtime.Changed("lock") {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "specify --lock when updating protection settings").WithParam("--lock")
		}
		if runtime.Changed("lock-info") && lock != "LOCK" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--lock-info requires --lock LOCK").WithParam("--lock-info")
		}
		if runtime.Changed("user-ids") {
			if lock != "LOCK" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--user-ids requires --lock LOCK").WithParam("--user-ids")
			}
			if runtime.Str("user-id-type") == "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--user-ids requires --user-id-type").WithParam("--user-id-type")
			}
			userIDs, err := parseJSONStringArray("user-ids", runtime.Str("user-ids"))
			if err != nil {
				return err
			}
			if len(userIDs) == 0 {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--user-ids must not be empty").WithParam("--user-ids")
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
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "specify at least one of --title, --index, --hidden, --frozen-row-count, --frozen-col-count, --lock, --lock-info, or --user-ids").WithParams(errs.InvalidParam{Name: "--title", Reason: "required; specify at least one"}, errs.InvalidParam{Name: "--index", Reason: "required; specify at least one"}, errs.InvalidParam{Name: "--hidden", Reason: "required; specify at least one"}, errs.InvalidParam{Name: "--frozen-row-count", Reason: "required; specify at least one"}, errs.InvalidParam{Name: "--frozen-col-count", Reason: "required; specify at least one"}, errs.InvalidParam{Name: "--lock", Reason: "required; specify at least one"}, errs.InvalidParam{Name: "--lock-info", Reason: "required; specify at least one"}, errs.InvalidParam{Name: "--user-ids", Reason: "required; specify at least one"})
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
		data, err := runtime.CallAPITyped("POST", sheetBatchUpdatePath(token), nil, buildCreateSheetBody(runtime))
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
		data, err := runtime.CallAPITyped("POST", sheetBatchUpdatePath(token), nil, buildCopySheetBody(runtime))
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
			moveResp, err := runtime.CallAPITyped("POST", sheetBatchUpdatePath(token), nil, buildMoveCopiedSheetBody(copiedSheetID, runtime.Int("index")))
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
		data, err := runtime.CallAPITyped("POST", sheetBatchUpdatePath(token), nil, buildDeleteSheetBody(runtime.Str("sheet-id")))
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

		data, err := runtime.CallAPITyped("POST", sheetBatchUpdatePath(token), params, body)
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
