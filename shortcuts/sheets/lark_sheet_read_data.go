// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
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
		if len(runtime.StrArray("range")) == 0 {
			return common.FlagErrorf("--range is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		return invokeToolDryRun(token, ToolKindRead, "get_cell_ranges", cellsGetInput(runtime, token, sheetID, sheetName))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
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
		"ranges":   runtime.StrArray("range"),
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	applyIncludeToCellsGet(input, runtime.StrSlice("include"))
	if runtime.Bool("skip-hidden") {
		input["skip_hidden"] = true
	}
	if n := runtime.Int("cell-limit"); n > 0 {
		input["cell_limit"] = n
	}
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
		_, _, err := resolveSheetSelector(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		return invokeToolDryRun(token, ToolKindRead, "get_range_as_csv", csvGetInput(runtime, token, sheetID, sheetName))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
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
	if v := runtime.Str("value-render-option"); v != "" {
		input["value_render_option"] = v
	}
	if runtime.Bool("skip-hidden") {
		input["skip_hidden"] = true
	}
	if n := runtime.Int("max-rows"); n > 0 {
		input["max_rows"] = n
	}
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

// DropdownGet wraps get_cell_ranges scoped to data_validation: read the
// dropdown configuration on a range. The range carries its own sheet prefix
// (e.g. "sheet1!A2:A100"), so no separate --sheet-id / --sheet-name is needed.
var DropdownGet = common.Shortcut{
	Service:     "sheets",
	Command:     "+dropdown-get",
	Description: "Read the dropdown / data-validation configuration on a sheet-prefixed range.",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+dropdown-get"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if strings.TrimSpace(runtime.Str("range")) == "" {
			return common.FlagErrorf("--range is required")
		}
		if !strings.Contains(runtime.Str("range"), "!") {
			return common.FlagErrorf("--range must include a sheet prefix (e.g. sheet1!A2:A100)")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		return invokeToolDryRun(token, ToolKindRead, "get_cell_ranges", dropdownGetInput(runtime, token))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindRead, "get_cell_ranges", dropdownGetInput(runtime, token))
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

func dropdownGetInput(runtime *common.RuntimeContext, token string) map[string]interface{} {
	return map[string]interface{}{
		"excel_id":            token,
		"ranges":              []string{strings.TrimSpace(runtime.Str("range"))},
		"include_styles":      false,
		"value_render_option": "formatted_value",
	}
}
