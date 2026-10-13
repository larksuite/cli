// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// duplicate does not consume target_index. Validate the requested position
// before creating anything, then move only the new ID returned by duplicate.
func sheetCopyIndexPreflight(ctx context.Context, runtime *common.RuntimeContext, token string, target int) (map[string]bool, error) {
	out, err := callTool(ctx, runtime, token, ToolKindRead, "get_workbook_structure", map[string]interface{}{"excel_id": token})
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "cannot decode workbook sheets").WithCause(err)
	}
	var workbook struct {
		Sheets []struct {
			ID string `json:"sheet_id"`
		} `json:"sheets"`
	}
	if err := json.Unmarshal(b, &workbook); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "cannot decode workbook sheets").WithCause(err)
	}
	if len(workbook.Sheets) == 0 {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "workbook structure returned no sheets")
	}
	ids := make(map[string]bool, len(workbook.Sheets))
	for _, sheet := range workbook.Sheets {
		if sheet.ID == "" || ids[sheet.ID] {
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "workbook structure returned missing or duplicate sheet IDs")
		}
		ids[sheet.ID] = true
	}
	if target > len(workbook.Sheets) {
		return nil, sheetsValidationForFlag("index", "--index must be between 0 and %d for the new copy", len(workbook.Sheets))
	}
	return ids, nil
}

func placeSheetCopy(ctx context.Context, runtime *common.RuntimeContext, token string, target int, existingIDs map[string]bool, out interface{}) (interface{}, error) {
	result, ok := out.(map[string]interface{})
	if !ok {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "duplicate returned no copy metadata").
			WithHint("the copy may already exist; inspect +workbook-info and move that copy instead of copying again")
	}
	b, err := json.Marshal(result)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "cannot decode copy metadata").WithCause(err)
	}
	var copied struct {
		ID    string `json:"sheet_id"`
		Index *int   `json:"index"`
	}
	if err := json.Unmarshal(b, &copied); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "cannot decode copy metadata").WithCause(err).
			WithHint("the copy may already exist; inspect +workbook-info and move that copy instead of copying again")
	}
	if copied.ID == "" || existingIDs[copied.ID] || copied.Index == nil || *copied.Index < 0 {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "duplicate did not return a new sheet ID and valid index").
			WithHint("the copy may already exist; inspect +workbook-info and move that copy instead of copying again")
	}
	if *copied.Index == target {
		return result, nil
	}
	recovery := fmt.Sprintf("copy %q already exists; do not copy again. Read +workbook-info, then finish with +sheet-move --sheet-id %s --index %d", copied.ID, copied.ID, target)
	_, err = callTool(ctx, runtime, token, ToolKindWrite, "modify_workbook_structure", map[string]interface{}{
		"excel_id": token, "operation": "move", "sheet_id": copied.ID,
		"source_index": *copied.Index, "target_index": target,
	})
	if err == nil {
		_, index, readErr := lookupSheetIndex(ctx, runtime, token, copied.ID, "")
		err = readErr
		if err == nil && index != target {
			err = errs.NewInternalError(errs.SubtypeInvalidResponse, "copy %q is at index %d, expected %d", copied.ID, index, target)
		}
	}
	if err != nil {
		// Retrying the command would create another copy. The failing step
		// may be retryable in isolation, but this completed copy is not.
		if p, ok := errs.ProblemOf(err); ok && p != nil {
			p.Retryable = false
			if p.Hint != "" {
				recovery += "; " + p.Hint
			}
			p.Hint = recovery
			switch p.Category {
			case errs.CategoryAPI:
				var api *errs.APIError
				if errors.As(err, &api) && api != nil {
					api.WithRetryAfterSeconds(0)
				}
			case errs.CategoryNetwork:
				var network *errs.NetworkError
				if errors.As(err, &network) && network != nil {
					network.WithRetryAfterSeconds(0)
				}
			default:
				// Other typed errors do not carry a retry delay.
			}
		}
		return nil, err
	}
	result["index"] = target
	return result, nil
}
