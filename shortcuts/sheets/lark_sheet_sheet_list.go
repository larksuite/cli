// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// ─── lark_sheet_sheet_list ─────────────────────────────────────────────
//
// SheetList is a read-only derivative over get_workbook_structure that projects
// out only the sub-sheet array. +workbook-info is the documented way to read a
// workbook's structure; +sheet-list exists because callers reach for that name
// unprompted — the sheets surface has a whole +sheet-* family (+sheet-create /
// +sheet-copy / +sheet-delete / +sheet-info / ...), so "list the sheets" spells
// itself +sheet-list. The miss is not self-correcting either: internal/suggest
// ranks shared prefixes first, so the "did you mean" hint points at
// +sheet-create and its siblings, never at +workbook-info.
//
// Hidden on both surfaces on purpose: absent from `sheets --help` (the Hidden
// field below) and from the lark-sheets skill docs (doc_hidden_shortcuts in
// sheet-skill-spec's canonical-spec/surfaces/bundle.json). Callers who read
// either surface are never offered a second name for what +workbook-info
// already does; the command only ever answers someone who typed it anyway.
var SheetList = common.Shortcut{
	Service:     "sheets",
	Command:     "+sheet-list",
	Description: "List a spreadsheet's sub-sheets with their metadata (sheet_id, title, dimensions, freeze, hidden).",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Hidden:      true,
	Flags:       flagsFor("+sheet-list"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := resolveSpreadsheetToken(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		return invokeToolDryRun(token, ToolKindRead, "get_workbook_structure", map[string]interface{}{
			"excel_id": token,
		})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindRead, "get_workbook_structure", map[string]interface{}{
			"excel_id": token,
		})
		if err != nil {
			return err
		}
		sheets, err := projectSheets(out)
		if err != nil {
			return err
		}
		runtime.Out(sheets, nil)
		return nil
	},
	Tips: []string{
		"+workbook-info is the documented command for this: it returns these same sheets alongside the rest of the workbook structure.",
	},
}

// projectSheets narrows a get_workbook_structure response to its `sheets`
// array. Entries pass through exactly as the tool returned them, so the
// per-sheet shape is the one +workbook-info shows. Every workbook has at least
// one sub-sheet, so a missing or non-array `sheets` is a malformed response
// rather than an empty workbook — surface it as an error instead of emitting a
// silent empty list.
func projectSheets(out interface{}) (interface{}, error) {
	obj, ok := out.(map[string]interface{})
	if !ok {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"get_workbook_structure returned non-object output")
	}
	sheets, ok := obj["sheets"].([]interface{})
	if !ok {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"get_workbook_structure did not return a sheets array")
	}
	return sheets, nil
}
