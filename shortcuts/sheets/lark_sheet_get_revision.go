// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// ─── lark_sheet_get_revision ───────────────────────────────────────────
//
// GetRevision is a read-only derivative over get_workbook_structure that
// projects out only the document revision (version number). The backend
// surfaces `revision` on every read/write tool response, so this shortcut
// needs no dedicated backend tool — it issues the lightest existing read
// (no range, just the workbook token) and narrows the payload to the single
// field callers want.
//
// The revision is the anchor for recover / undo. Callers that have just run a
// write already have it in that write's response; +get-revision is the
// explicit, zero-side-effect way to fetch the current value on its own.
var GetRevision = common.Shortcut{
	Service:     "sheets",
	Command:     "+get-revision",
	Description: "Get the spreadsheet's current document revision (version number).",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+get-revision"),
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
		rev, err := projectRevision(out)
		if err != nil {
			return err
		}
		runtime.Out(map[string]interface{}{"revision": rev}, nil)
		return nil
	},
	Tips: []string{
		"The revision is the version anchor for recover / undo; every read and write tool response already carries it.",
	},
}

// projectRevision narrows a get_workbook_structure response to its `revision`
// field. An absent revision means the backend predates revision injection on
// read responses; surface that as an explicit error rather than emitting a
// silent null.
func projectRevision(out interface{}) (interface{}, error) {
	obj, ok := out.(map[string]interface{})
	if !ok {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"get_workbook_structure returned non-object output")
	}
	rev, ok := obj["revision"]
	if !ok {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"get_workbook_structure did not return a revision (backend may not support it yet)")
	}
	return rev, nil
}
