// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

// ─── lark_sheet_history (BE-1: +history-list) ─────────────────────────
//
// Wraps the facade-agg `history_list` tool (read) behind the One-OpenAPI
// invoke_read endpoint. The tool returns a sheet's version history. The
// facade-agg tool already performs the response transform (minor_histories
// trim / id → history_version_id / 4-field projection / RFC3339 create_time),
// so the CLI passes the tool output straight through and does NOT re-implement
// the transform client-side.
//
// History is workbook-level (no sheet selector), mirroring +workbook-info:
// the only locator is --url / --spreadsheet-token (XOR), with --token accepted
// as a parse-time alias for --spreadsheet-token via the shared PostMount hook.

// historyLocatorFlags is the --url / --spreadsheet-token XOR locator pair
// shared by the three history shortcuts. Mirrors +workbook-info's flag-defs
// entry; XOR is enforced in Validate via parseSpreadsheetRef, not by Required.
func historyLocatorFlags() []common.Flag {
	return []common.Flag{
		{Name: "url", Type: "string", Desc: "Spreadsheet locator (a /sheets/ or /wiki/ URL)."},
		{Name: "spreadsheet-token", Type: "string", Desc: "Spreadsheet locator (raw spreadsheet token)."},
	}
}

// HistoryList wraps the history_list tool: list a spreadsheet's history
// versions. Each item carries history_version_id / create_time / action /
// all_block_revision (projected server-side). An empty sheet yields an empty
// list and exit 0.
//
// Backward pagination: --end-version (optional int) maps to the tool's
// `end_version` parameter. Omit on the first call to fetch the latest page.
// On subsequent pages pass the previous response's next_end_version as
// --end-version. The tool returns next_end_version + has_more only when
// more history exists; both fields are absent at the earliest page.
var HistoryList = common.Shortcut{
	Service:     "sheets",
	Command:     "+history-list",
	Description: "List a spreadsheet's edit history versions (history_version_id, create_time, action, all_block_revision).",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: append(historyLocatorFlags(),
		common.Flag{Name: "end-version", Type: "int", Desc: "Max version to query (descending pagination). Omit on the first call; pass the previous response's next_end_version on subsequent pages."},
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := resolveSpreadsheetToken(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		return invokeToolDryRun(token, ToolKindRead, "history_list", historyListInput(runtime, token))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindRead, "history_list", historyListInput(runtime, token))
		if err != nil {
			return err
		}
		// Pass the tool output through verbatim — facade-agg already shaped it.
		runtime.Out(out, nil)
		return nil
	},
	Tips: []string{
		"Capture a history_version_id from the result to feed +history-revert.",
		"For older history, capture next_end_version from the response and pass it as --end-version on the next call (omitted by the server when the earliest page is reached).",
	},
}

// historyListInput composes the history_list tool input. --end-version is
// optional: include it only when explicitly set so the server treats absence
// as "first page (latest)".
func historyListInput(runtime *common.RuntimeContext, token string) map[string]interface{} {
	in := map[string]interface{}{"excel_id": token}
	if runtime.Changed("end-version") {
		in["end_version"] = runtime.Int("end-version")
	}
	return in
}
