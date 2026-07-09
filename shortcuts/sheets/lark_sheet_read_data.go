// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"strconv"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// ─── lark_sheet_read_data ─────────────────────────────────────────────
//
// Wraps:
//   - get_cell_ranges  (powers +cells-get and +dropdown-get)
//   - get_range_as_csv (powers +csv-get)
//
// The sandbox tool (export_sheet_to_sandbox) is Sheet-Tool-only and has no
// CLI surface here.

// unboundedReadLimit is pinned into the tool's cell_limit / max_rows so that
// --max-chars is the single effective read cap. The underlying tools default
// those two to smaller values; without an explicit high value they could
// truncate before max_chars. The CLI no longer exposes --cell-limit / --max-rows
// (only --max-chars), so we pass this sentinel to neutralize the tool defaults.
// Large enough to never bind on any real sheet.
const unboundedReadLimit = 1_000_000_000

// CellsGet wraps get_cell_ranges: read multiple A1 ranges and return per-cell
// values, formulas, styles, and other metadata as requested via --include.
var CellsGet = common.Shortcut{
	Service:     "sheets",
	Command:     "+cells-get",
	Description: "Read one or more cell ranges with values, formulas, and optional styles / comments / data validation.",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+cells-get"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, _, err := resolveSheetSelector(runtime); err != nil {
			return err
		}
		if strings.TrimSpace(runtime.Str("range")) == "" {
			return sheetsValidationForFlag("range", "--range is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		return invokeToolDryRun(token, ToolKindRead, "get_cell_ranges", cellsGetInput(runtime, token, sheetID, sheetName))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindRead, "get_cell_ranges", cellsGetInput(runtime, token, sheetID, sheetName))
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

func cellsGetInput(runtime *common.RuntimeContext, token, sheetID, sheetName string) map[string]interface{} {
	input := map[string]interface{}{
		"excel_id": token,
		"ranges":   []string{strings.TrimSpace(runtime.Str("range"))},
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	applyIncludeToCellsGet(input, runtime.StrSlice("include"))
	if runtime.Bool("skip-hidden") {
		input["skip_hidden"] = true
	}
	// --cell-limit was removed from the CLI surface; --max-chars is the single
	// read cap. Pin cell_limit very high so the tool's own default never binds
	// before max_chars.
	input["cell_limit"] = unboundedReadLimit
	if n := runtime.Int("max-chars"); n > 0 {
		input["max_chars"] = n
	}
	return input
}

// applyIncludeToCellsGet maps the fine-grained --include vocabulary to the
// tool's two coarse switches:
//
//   - include_styles (bool) — toggled by "style" presence
//   - value_render_option (enum) — "formula" → formula; otherwise omitted
//
// "value", "comment", and "data_validation" are always returned by the tool
// per the schema; they have no dedicated knob today but are accepted in
// --include for forward-compat with finer-grained server support.
func applyIncludeToCellsGet(input map[string]interface{}, include []string) {
	if len(include) == 0 {
		return
	}
	want := map[string]bool{}
	for _, v := range include {
		want[v] = true
	}
	if want["style"] {
		input["include_styles"] = true
	} else {
		input["include_styles"] = false
	}
	if want["formula"] {
		input["value_render_option"] = "formula"
	}
}

// CsvGet wraps get_range_as_csv: pull one range as RFC 4180 CSV with optional
// [row=N] line prefix for easy row-number lookup.
var CsvGet = common.Shortcut{
	Service:     "sheets",
	Command:     "+csv-get",
	Description: "Read a range as CSV (with [row=N] line prefix by default).",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+csv-get"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, _, err := resolveSheetSelector(runtime); err != nil {
			return err
		}
		if strings.TrimSpace(runtime.Str("range")) == "" {
			return sheetsValidationForFlag("range", "--range is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		return invokeToolDryRun(token, ToolKindRead, "get_range_as_csv", csvGetInput(runtime, token, sheetID, sheetName))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindRead, "get_range_as_csv", csvGetInput(runtime, token, sheetID, sheetName))
		if err != nil {
			return err
		}
		if !runtime.Bool("include-row-prefix") {
			out = stripRowPrefixFromCsvOutput(out)
		}
		runtime.Out(out, nil)
		return nil
	},
}

func csvGetInput(runtime *common.RuntimeContext, token, sheetID, sheetName string) map[string]interface{} {
	input := map[string]interface{}{"excel_id": token}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if r := strings.TrimSpace(runtime.Str("range")); r != "" {
		input["range"] = r
	}
	if runtime.Bool("skip-hidden") {
		input["skip_hidden"] = true
	}
	// --max-rows was removed from the CLI surface; --max-chars is the single
	// read cap. Pin max_rows very high so the tool's own default never binds
	// before max_chars.
	input["max_rows"] = unboundedReadLimit
	if n := runtime.Int("max-chars"); n > 0 {
		input["max_chars"] = n
	}
	return input
}

// stripRowPrefixFromCsvOutput removes "[row=N]" line prefixes from the tool's
// annotated_csv field. Operates client-side because the tool only emits the
// annotated form.
func stripRowPrefixFromCsvOutput(out interface{}) interface{} {
	m, ok := out.(map[string]interface{})
	if !ok {
		return out
	}
	csv, ok := m["annotated_csv"].(string)
	if !ok {
		return out
	}
	lines := strings.Split(csv, "\n")
	for i, line := range lines {
		if idx := strings.Index(line, "]"); idx >= 0 && strings.HasPrefix(line, "[row=") {
			rest := line[idx+1:]
			lines[i] = strings.TrimPrefix(rest, ",")
		}
	}
	m["annotated_csv"] = strings.Join(lines, "\n")
	return m
}

// a1EndRow extracts the ending row number from an A1 range, e.g. "A1:N51" → 51,
// "Sheet1!B2:D9" → 9, "C5" → 5. Returns 0 when no row number is present.
func a1EndRow(rng string) int {
	rng = strings.TrimSpace(rng)
	if i := strings.LastIndex(rng, "!"); i >= 0 {
		rng = rng[i+1:]
	}
	if i := strings.LastIndex(rng, ":"); i >= 0 {
		rng = rng[i+1:]
	}
	var digits strings.Builder
	for _, c := range rng {
		if c >= '0' && c <= '9' {
			digits.WriteRune(c)
		}
	}
	if digits.Len() == 0 {
		return 0
	}
	n, _ := strconv.Atoi(digits.String())
	return n
}

// DropdownGet wraps get_cell_ranges scoped to data_validation: read the
// dropdown configuration on a range. Aligned with its sibling +cells-get
// — sheet selection is via --sheet-id / --sheet-name (XOR), and --range
// is a bare A1 reference. The earlier "must include a sheet prefix"
// shape was the odd one out among the get_cell_ranges wrappers and made
// callers treat the prefix as either name or id; folding it into the
// canonical --sheet-id selector removes that ambiguity.
var DropdownGet = common.Shortcut{
	Service:     "sheets",
	Command:     "+dropdown-get",
	Description: "Read the dropdown / data-validation configuration on a range.",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+dropdown-get"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, _, err := resolveSheetSelector(runtime); err != nil {
			return err
		}
		if strings.TrimSpace(runtime.Str("range")) == "" {
			return sheetsValidationForFlag("range", "--range is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		return invokeToolDryRun(token, ToolKindRead, "get_cell_ranges", dropdownGetInput(runtime, token, sheetID, sheetName))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindRead, "get_cell_ranges", dropdownGetInput(runtime, token, sheetID, sheetName))
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

func dropdownGetInput(runtime *common.RuntimeContext, token, sheetID, sheetName string) map[string]interface{} {
	input := map[string]interface{}{
		"excel_id":            token,
		"ranges":              []string{strings.TrimSpace(runtime.Str("range"))},
		"include_styles":      false,
		"value_render_option": "formatted_value",
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	return input
}
