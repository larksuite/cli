// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var Undo = common.Shortcut{
	Service:     "sheets",
	Command:     "+undo",
	Description: "Undo the current user's latest spreadsheet write.",
    Risk:        "high-risk-write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	// History shortcuts keep locator flags hand-written because they share
	// revision/revert helpers; keep --count here in sync with sheet-skill-spec.
	Flags: append(historyLocatorFlags(),
		common.Flag{Name: "count", Type: "int", Default: "1", Desc: "Number of user undo stack entries to undo sequentially (1-20)."},
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if runtime.Int("count") < 1 || runtime.Int("count") > 20 {
			return sheetsValidationForFlag("count", "--count must be between 1 and 20")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		return invokeToolDryRun(token, ToolKindWrite, "undo_last", undoInput(runtime, token))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "undo_last", undoInput(runtime, token))
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

func undoInput(runtime *common.RuntimeContext, token string) map[string]interface{} {
	// Always send count, including the default 1, so dry-run mirrors the exact
	// request body sent by Execute.
	return map[string]interface{}{
		"excel_id": token,
		"count":    runtime.Int("count"),
	}
}
