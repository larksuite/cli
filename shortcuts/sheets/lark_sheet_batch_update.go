// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// ─── lark_sheet_batch_update ──────────────────────────────────────────
//
// One tool (batch_update), four shortcuts:
//
//   - +batch-update            raw passthrough of an operations array
//                              (high-risk-write — anything can be inside)
//   - +cells-batch-set-style   fan a single style across many ranges
//   - +dropdown-update         install/replace the same dropdown across
//                              many ranges in one atomic batch
//   - +dropdown-delete         clear data_validation across many ranges
//                              (high-risk-write)
//
// The tool's contract:
//   { excel_id, operations: [{tool_name, input}, ...], continue_on_error? }
//
// continue_on_error defaults to false (strict transaction): any failure
// rolls back the whole batch. CLI leaves the default in place for the
// three "fan-out" shortcuts since they're meant to be all-or-nothing;
// only +batch-update lets callers flip it via --continue-on-error.

// BatchUpdate is the raw passthrough — caller hands in the operations
// array as --data. high-risk-write because it can wrap anything.
var BatchUpdate = common.Shortcut{
	Service:     "sheets",
	Command:     "+batch-update",
	Description: "Execute a batch of write tools as a single atomic request (rolls back on failure by default).",
	Risk:        "high-risk-write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+batch-update"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		ops, err := parseBatchOperationsFlag(runtime)
		if err != nil {
			return err
		}
		if len(ops) == 0 {
			return common.FlagErrorf("--operations must be a non-empty JSON array")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		input, _ := batchUpdateRawInput(runtime, token)
		return invokeToolDryRun(token, ToolKindWrite, "batch_update", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		input, err := batchUpdateRawInput(runtime, token)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "batch_update", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
	Tips: []string{
		"Default is strict transaction — any sub-tool failure rolls the whole batch back. Pass --continue-on-error to keep partial successes.",
	},
}

func batchUpdateRawInput(runtime *common.RuntimeContext, token string) (map[string]interface{}, error) {
	ops, err := parseBatchOperationsFlag(runtime)
	if err != nil {
		return nil, err
	}
	input := map[string]interface{}{
		"excel_id":   token,
		"operations": ops,
	}
	if runtime.Bool("continue-on-error") {
		input["continue_on_error"] = true
	} else if envelope, _ := parseJSONFlag(runtime, "operations"); envelope != nil {
		// Honor an inline override when --operations is an envelope object
		// rather than a bare operations array.
		if m, ok := envelope.(map[string]interface{}); ok {
			if v, ok := m["continue_on_error"].(bool); ok && v {
				input["continue_on_error"] = true
			}
		}
	}
	return input, nil
}

// parseBatchOperationsFlag accepts --operations as either a JSON array (the
// operations list directly) or an envelope object { operations, continue_on_error }
// for back-compat with the legacy --data shape. Returns the operations array.
func parseBatchOperationsFlag(runtime *common.RuntimeContext) ([]interface{}, error) {
	v, err := parseJSONFlag(runtime, "operations")
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, common.FlagErrorf("--operations is required")
	}
	if arr, ok := v.([]interface{}); ok {
		return arr, nil
	}
	if m, ok := v.(map[string]interface{}); ok {
		if ops, ok := m["operations"].([]interface{}); ok {
			return ops, nil
		}
	}
	return nil, common.FlagErrorf("--operations must be a JSON array (or { operations: [...] } envelope)")
}

// CellsBatchSetStyle stamps one style block across many sheet-prefixed
// ranges atomically. --ranges is a JSON array of sheet-prefixed A1
// strings; the style is composed from the same flat flags as
// +cells-set-style. CLI fans each range into a separate set_cell_range
// op inside one batch_update.
var CellsBatchSetStyle = common.Shortcut{
	Service:     "sheets",
	Command:     "+cells-batch-set-style",
	Description: "Apply one style block to many sheet-prefixed ranges in one atomic batch.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+cells-batch-set-style"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, err := validateDropdownRanges(runtime); err != nil {
			return err
		}
		if err := requireAnyStyleFlag(runtime); err != nil {
			return err
		}
		if _, err := borderStylesFromFlag(runtime); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		input, _ := cellsBatchSetStyleInput(runtime, token)
		return invokeToolDryRun(token, ToolKindWrite, "batch_update", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		input, err := cellsBatchSetStyleInput(runtime, token)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "batch_update", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

func cellsBatchSetStyleInput(runtime *common.RuntimeContext, token string) (map[string]interface{}, error) {
	ranges, err := validateDropdownRanges(runtime)
	if err != nil {
		return nil, err
	}
	cellStyle := buildCellStyleFromFlags(runtime)
	borderStyles, err := borderStylesFromFlag(runtime)
	if err != nil {
		return nil, err
	}
	prototype := map[string]interface{}{}
	if len(cellStyle) > 0 {
		prototype["cell_styles"] = cellStyle
	}
	if borderStyles != nil {
		prototype["border_styles"] = borderStyles
	}
	var ops []interface{}
	for _, rng := range ranges {
		sheet, sub, err := splitSheetPrefixedRange(rng)
		if err != nil {
			return nil, err
		}
		rows, cols, err := rangeDimensions(sub)
		if err != nil {
			return nil, common.FlagErrorf("range %q: %v", rng, err)
		}
		cells := fillCellsMatrix(rows, cols, prototype)
		ops = append(ops, map[string]interface{}{
			"tool_name": "set_cell_range",
			"input": map[string]interface{}{
				"excel_id":   token,
				"sheet_name": sheet,
				"range":      sub,
				"cells":      cells,
			},
		})
	}
	return map[string]interface{}{
		"excel_id":   token,
		"operations": ops,
	}, nil
}

// DropdownUpdate installs/replaces a single dropdown on many ranges in one
// atomic batch. Sheet ids come from the per-range sheet prefix.
var DropdownUpdate = common.Shortcut{
	Service:     "sheets",
	Command:     "+dropdown-update",
	Description: "Install or replace one dropdown across many sheet-prefixed ranges atomically.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+dropdown-update"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, err := validateDropdownRanges(runtime); err != nil {
			return err
		}
		if _, err := validateDropdownOptionsColors(runtime); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		input, _ := dropdownBatchInput(runtime, token, false)
		return invokeToolDryRun(token, ToolKindWrite, "batch_update", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		input, err := dropdownBatchInput(runtime, token, false)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "batch_update", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

// DropdownDelete clears data_validation across many ranges atomically.
var DropdownDelete = common.Shortcut{
	Service:     "sheets",
	Command:     "+dropdown-delete",
	Description: "Clear dropdowns from many sheet-prefixed ranges atomically (irreversible).",
	Risk:        "high-risk-write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+dropdown-delete"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		ranges, err := validateDropdownRanges(runtime)
		if err != nil {
			return err
		}
		if len(ranges) > 100 {
			return common.FlagErrorf("--ranges accepts at most 100 entries; got %d", len(ranges))
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		input, _ := dropdownBatchInput(runtime, token, true)
		return invokeToolDryRun(token, ToolKindWrite, "batch_update", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		input, err := dropdownBatchInput(runtime, token, true)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "batch_update", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

// dropdownBatchInput builds the batch_update payload for both
// +dropdown-update (clear=false, data_validation populated) and
// +dropdown-delete (clear=true, data_validation: null).
func dropdownBatchInput(runtime *common.RuntimeContext, token string, clear bool) (map[string]interface{}, error) {
	ranges, err := validateDropdownRanges(runtime)
	if err != nil {
		return nil, err
	}
	var prototype map[string]interface{}
	if clear {
		prototype = map[string]interface{}{"data_validation": nil}
	} else {
		validation, err := buildDropdownValidation(runtime)
		if err != nil {
			return nil, err
		}
		prototype = map[string]interface{}{"data_validation": validation}
	}
	var ops []interface{}
	for _, rng := range ranges {
		sheet, sub, err := splitSheetPrefixedRange(rng)
		if err != nil {
			return nil, err
		}
		rows, cols, err := rangeDimensions(sub)
		if err != nil {
			return nil, common.FlagErrorf("range %q: %v", rng, err)
		}
		cells := fillCellsMatrix(rows, cols, prototype)
		ops = append(ops, map[string]interface{}{
			"tool_name": "set_cell_range",
			"input": map[string]interface{}{
				"excel_id":   token,
				"sheet_name": sheet,
				"range":      sub,
				"cells":      cells,
			},
		})
	}
	return map[string]interface{}{
		"excel_id":   token,
		"operations": ops,
	}, nil
}

// ─── helpers resurrected from B3 (used here + future skills) ──────────

// validateDropdownRanges parses --ranges, requires every entry to carry a
// sheet prefix, and returns the parsed list.
func validateDropdownRanges(runtime *common.RuntimeContext) ([]string, error) {
	raw, err := requireJSONArray(runtime, "ranges")
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(raw))
	for i, v := range raw {
		s, ok := v.(string)
		if !ok {
			return nil, common.FlagErrorf("--ranges[%d] must be a string", i)
		}
		s = strings.TrimSpace(s)
		if !strings.Contains(s, "!") {
			return nil, common.FlagErrorf("--ranges[%d] (%q) must include a sheet prefix", i, s)
		}
		out = append(out, s)
	}
	return out, nil
}

// splitSheetPrefixedRange splits "sheet1!A2:A100" into ("sheet1", "A2:A100").
func splitSheetPrefixedRange(rng string) (sheet, sub string, err error) {
	idx := strings.Index(rng, "!")
	if idx <= 0 || idx == len(rng)-1 {
		return "", "", common.FlagErrorf("range %q must use sheet!range form", rng)
	}
	return strings.TrimSpace(rng[:idx]), strings.TrimSpace(rng[idx+1:]), nil
}
