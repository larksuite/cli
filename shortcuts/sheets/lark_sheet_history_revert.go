// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"strings"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// ─── lark_sheet_history (BE-2: +history-revert / +history-revert-status) ──
//
// Two thin callTool wrappers over the facade-agg history tools:
//   - +history-revert        → history_revert        (write) — async revert
//   - +history-revert-status → history_revert_status (read)  — poll outcome
//
// Both target a single history version via --history-version-id (the id
// surfaced by +history-list). Revert is asynchronous: it returns a receipt /
// transaction id that +history-revert-status then polls, distinguishing
// in-progress / success / failure from the tool output (passed through
// verbatim — no client-side shaping).
//
// ⚠️ Backend state: the facade-agg history_revert / history_revert_status
// tools are registered but their downstream RPC wiring is a DEFERRED
// follow-up; today they return a "not wired yet" guard error from the gateway,
// which surfaces here as a normal tool error. These CLI shortcuts are correct
// thin wrappers and will work end-to-end once the backend follow-up lands —
// this is NOT a CLI blocker. See self_check.md.

func historyRevertFlags() []common.Flag {
	return flagsFor("+history-revert")
}

// validateHistoryVersionID enforces the required, control-char-clean
// --history-version-id. Returns the trimmed value so callers reuse it.
func validateHistoryVersionID(runtime *common.RuntimeContext) (string, error) {
	id := strings.TrimSpace(runtime.Str("history-version-id"))
	if id == "" {
		return "", sheetsValidationForFlag("history-version-id", "--history-version-id is required")
	}
	if err := validate.RejectControlChars(id, "--history-version-id"); err != nil {
		return "", err
	}
	return id, nil
}

func historyRevertInput(token, versionID string) map[string]interface{} {
	return map[string]interface{}{
		"excel_id":           token,
		"history_version_id": versionID,
	}
}

func historyRevertStatusFlags() []common.Flag {
	return flagsFor("+history-revert-status")
}

// validateTransactionID enforces the required, trimmed --transaction-id and
// returns it for reuse.
func validateTransactionID(runtime *common.RuntimeContext) (string, error) {
	id := strings.TrimSpace(runtime.Str("transaction-id"))
	if id == "" {
		return "", sheetsValidationForFlag("transaction-id", "--transaction-id is required")
	}
	if err := validate.RejectControlChars(id, "--transaction-id"); err != nil {
		return "", err
	}
	return id, nil
}

func historyRevertStatusInput(token, transactionID string) map[string]interface{} {
	return map[string]interface{}{
		"excel_id":       token,
		"transaction_id": transactionID,
	}
}

// HistoryRevert wraps the history_revert tool (write): asynchronously revert a
// spreadsheet to the given history version. --history-version-id is required
// at the cli surface (cobra MarkFlagRequired); a missing flag fails before
// Validate runs with cobra's standard "required flag(s)" error (which the
// dispatcher classifies as a typed *errs.ValidationError, exit 2). We still
// trim + reject empty / control-char values in Validate to catch the
// case where cobra accepts --history-version-id with an empty-string value.
var HistoryRevert = common.Shortcut{
	Service:     "sheets",
	Command:     "+history-revert",
	Description: "Revert a spreadsheet to a given history version (asynchronous; poll with +history-revert-status).",
	Risk:        "high-risk-write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       historyRevertFlags(),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		_, err := validateHistoryVersionID(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		versionID := strings.TrimSpace(runtime.Str("history-version-id"))
		return invokeToolDryRun(token, ToolKindWrite, "history_revert", historyRevertInput(token, versionID))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		versionID, err := validateHistoryVersionID(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "history_revert", historyRevertInput(token, versionID))
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
	Tips: []string{
		"Revert overwrites the current spreadsheet content. Always run with --dry-run first to verify the target spreadsheet and history_version_id.",
		"Revert is asynchronous — pass the returned id to +history-revert-status to track in-progress / success / failure.",
	},
}

// HistoryRevertStatus wraps the history_revert_status tool (read): poll the
// outcome of a prior +history-revert. The tool output distinguishes
// in-progress / success / failure and is passed through verbatim.
var HistoryRevertStatus = common.Shortcut{
	Service:     "sheets",
	Command:     "+history-revert-status",
	Description: "Poll the status of a history revert (in-progress / success / failure).",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       historyRevertStatusFlags(),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		_, err := validateTransactionID(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		txnID := strings.TrimSpace(runtime.Str("transaction-id"))
		return invokeToolDryRun(token, ToolKindRead, "history_revert_status", historyRevertStatusInput(token, txnID))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		txnID, err := validateTransactionID(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindRead, "history_revert_status", historyRevertStatusInput(token, txnID))
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}
