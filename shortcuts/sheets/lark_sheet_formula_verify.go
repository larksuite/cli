// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/util"
	"github.com/larksuite/cli/shortcuts/common"
)

// ─── lark_sheet_formula_verify ───────────────────────────────────────
//
// Wraps verify_formula (read): scan formulas + cell error states across one
// or more sub-sheets and aggregate Excel errors (#REF! / #DIV/0! / #VALUE! /
// #NAME? / #NULL! / #NUM! / #N/A) plus compile failures (formula_errors)
// into a recalc.py-shaped JSON status report. The contract is the single
// AI self-check entry point for the R10 "write → verify zero-error"
// invariant — see canonical-spec/references/lark_sheet_formula_verify/.

// FormulaVerify wraps verify_formula. Sheet selection is optional (both
// --sheet-id and --sheet-name are repeatable); when omitted, the tool scans
// every visible sub-sheet's current_region.
var FormulaVerify = common.Shortcut{
	Service:     "sheets",
	Command:     "+formula-verify",
	Description: "Scan formulas / cell errors and return a recalc.py-shaped status report (success / errors_found / partial).",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+formula-verify"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if err := validateFormulaVerifySheetSelector(runtime); err != nil {
			return err
		}
		return validateFormulaVerifyLimits(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		return invokeToolDryRun(token, ToolKindRead, "verify_formula", formulaVerifyInput(runtime, token))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindRead, "verify_formula", formulaVerifyInput(runtime, token))
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		if runtime.Bool("exit-on-error") {
			return formulaVerifyExitOnError(out)
		}
		return nil
	},
}

// validateFormulaVerifySheetSelector enforces XOR-like guarantees on the
// two multi-value selectors: at most one of --sheet-id / --sheet-name may be
// non-empty (passing both is the high-frequency reflex confusion when the
// caller cargo-cults the single-sheet shortcut signature). Both empty is the
// documented "scan every visible sub-sheet" path. Control-char checks reuse
// requireSheetSelector's logic on each item.
func validateFormulaVerifySheetSelector(runtime *common.RuntimeContext) error {
	ids := nonEmptySliceItems(runtime.StrSlice("sheet-id"))
	names := nonEmptySliceItems(runtime.StrSlice("sheet-name"))
	if len(ids) > 0 && len(names) > 0 {
		return common.ValidationErrorf("--sheet-id and --sheet-name are mutually exclusive; pick one selector to identify sub-sheets").
			WithParams(
				sheetsInvalidParam("sheet-id", "mutually exclusive"),
				sheetsInvalidParam("sheet-name", "mutually exclusive"),
			)
	}
	for _, id := range ids {
		if err := requireSheetSelector(id, ""); err != nil {
			return err
		}
	}
	for _, name := range names {
		if err := requireSheetSelector("", name); err != nil {
			return err
		}
	}
	return nil
}

// validateFormulaVerifyLimits rejects non-positive caps so a misplaced 0 or
// negative flag value can't silently degrade the scan (the server-side
// default would otherwise mask the typo).
func validateFormulaVerifyLimits(runtime *common.RuntimeContext) error {
	if runtime.Changed("max-locations") && runtime.Int("max-locations") <= 0 {
		return sheetsValidationForFlag("max-locations", "--max-locations must be > 0")
	}
	return nil
}

// nonEmptySliceItems trims and drops blanks from a repeated-flag value so
// `--sheet-id ""` doesn't masquerade as a real entry.
func nonEmptySliceItems(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// formulaVerifyInput builds the verify_formula tool input map from CLI flags.
// excel_id is required; everything else is optional per the schema.
func formulaVerifyInput(runtime *common.RuntimeContext, token string) map[string]interface{} {
	input := map[string]interface{}{
		"excel_id": token,
	}
	if ids := nonEmptySliceItems(runtime.StrSlice("sheet-id")); len(ids) > 0 {
		input["sheet_ids"] = ids
	} else if names := nonEmptySliceItems(runtime.StrSlice("sheet-name")); len(names) > 0 {
		// The verify_formula schema only declares sheet_ids; the facade
		// accepts sheet_names as a parallel optional field so name-based
		// selection works without forcing the caller to pre-resolve. Mirrors
		// how the other read shortcuts pack both fields via
		// sheetSelectorForToolInput.
		input["sheet_names"] = names
	}
	if ranges := nonEmptySliceItems(runtime.StrSlice("range")); len(ranges) > 0 {
		input["ranges"] = ranges
	}
	if runtime.Changed("max-locations") {
		input["max_locations_per_error"] = runtime.Int("max-locations")
	}
	return input
}

// formulaVerifyExitOnError converts a verify_formula status into a non-zero
// CLI exit when the caller passed --exit-on-error. status="errors_found"
// is the only failure mode for this flag: "partial" means truncated but the
// scanned slice is clean, and "success" is obviously clean. A missing /
// unknown status is treated as a typed internal error because the tool's
// schema guarantees the field and we don't want a silent zero-exit.
func formulaVerifyExitOnError(out interface{}) error {
	m, ok := out.(map[string]interface{})
	if !ok {
		return errs.NewInternalError(errs.SubtypeInvalidResponse,
			"verify_formula: missing status field in tool output")
	}
	status, _ := m["status"].(string)
	switch status {
	case "success", "partial":
		return nil
	case "errors_found":
		total, _ := util.ToFloat64(m["total_errors"])
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"verify_formula: %d formula error(s) detected; resolve and re-run", int(total)).
			WithHint("inspect error_summary[*] / compile_errors[*] in the JSON output, fix or wrap with IFERROR, then re-run +formula-verify until status=success")
	default:
		return errs.NewInternalError(errs.SubtypeInvalidResponse,
			"verify_formula: unexpected status %q", status)
	}
}
