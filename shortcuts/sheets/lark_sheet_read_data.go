// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/larksuite/cli/internal/citation"
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
		if err := validateSheetSelectorPreflight(runtime); err != nil {
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
		sheetID, sheetName, err := resolveSheetSelectorExec(ctx, runtime, token)
		if err != nil {
			return err
		}
		input := cellsGetInput(runtime, token, sheetID, sheetName)
		var out interface{}
		if autoPaginate(runtime) {
			// Same contract as +csv-get: no explicit --max-chars means the
			// read is completed here or it fails.
			out, err = fetchAllCellRanges(ctx, runtime, token, input)
			// A range too large to finish is not a failure the caller can
			// retry its way out of — say what to run instead, at the point
			// they hit it.
			var tooLarge *readTooLargeError
			if errors.As(err, &tooLarge) {
				return common.ValidationErrorf("%s. %s",
					tooLarge.Error(), tableGetAlternatives(sheetName, sheetID))
			}
		} else {
			out, err = callTool(ctx, runtime, token, ToolKindRead, "get_cell_ranges", input)
		}
		if err != nil {
			return err
		}
		return emitReadResult(runtime, out)
	},
}

func cellsGetInput(runtime *common.RuntimeContext, token, sheetID, sheetName string) map[string]interface{} {
	input := map[string]interface{}{
		"excel_id": token,
		"ranges":   splitRangeAreas(runtime.Str("range")),
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
	if n, ok := maxCharsInput(runtime); ok {
		input["max_chars"] = n
	}
	return input
}

// applyIncludeToCellsGet maps the fine-grained --include vocabulary to the
// tool's switches:
//
//   - include_styles (bool) — toggled by "style" presence
//   - value_render_option (enum) — "formula" → formula; otherwise omitted
//   - include_truncation_info (bool) — toggled by "truncation" presence; makes
//     the tool estimate and return per-cell isRowTruncated / isColTruncated
//   - include_conditional_format_style (bool) — toggled by "conditional_format"
//     presence; also enables include_styles because the calculated result is
//     returned through cell_styles
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
	if want["style"] || want["conditional_format"] {
		input["include_styles"] = true
	} else {
		input["include_styles"] = false
	}
	if want["conditional_format"] {
		input["include_conditional_format_style"] = true
	}
	if want["formula"] {
		input["value_render_option"] = "formula"
	}
	if want["truncation"] {
		input["include_truncation_info"] = true
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
	Citation: &common.CitationDefinition{
		SourceTypes: []citation.SourceType{citation.SourceSheet},
		Build:       workbookCitations,
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if err := validateSheetSelectorPreflight(runtime); err != nil {
			return err
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
		sheetID, sheetName, err := resolveSheetSelectorExec(ctx, runtime, token)
		if err != nil {
			return err
		}
		var out interface{}
		if autoPaginate(runtime) {
			// No explicit --max-chars: the read is completed here, across as
			// many backend calls as it takes, or it fails.
			out, err = readCsvComplete(ctx, runtime, token, sheetID, sheetName)
		} else {
			out, err = callTool(ctx, runtime, token, ToolKindRead, "get_range_as_csv", csvGetInput(runtime, token, sheetID, sheetName))
		}
		if err != nil {
			return err
		}
		out = addPlainCsvField(out)
		if !runtime.Bool("include-row-prefix") {
			out = dropAnnotatedCsv(out)
		}
		return emitReadResult(runtime, out)
	},
}

// csvGetFullSheetRange is the range sent when --range is omitted: the tool
// requires one, but clips anything past the grid bounds and reports the clip
// in actual_range — so an over-wide whole-columns range reads the entire
// sheet in one call, with no workbook-info pre-flight. Eval traces show
// "read the whole sheet" as a recurring intent (--range was the single most
// missed required flag once the rest of the surface was fixed).
const csvGetFullSheetRange = "A:ZZZ"

func csvGetInput(runtime *common.RuntimeContext, token, sheetID, sheetName string) map[string]interface{} {
	input := map[string]interface{}{"excel_id": token}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if r := strings.TrimSpace(runtime.Str("range")); r != "" {
		input["range"] = r
	} else {
		input["range"] = csvGetFullSheetRange
	}
	if runtime.Bool("skip-hidden") {
		input["skip_hidden"] = true
	}
	// --max-rows was removed from the CLI surface; --max-chars is the single
	// read cap. Pin max_rows very high so the tool's own default never binds
	// before max_chars.
	input["max_rows"] = unboundedReadLimit
	if n, ok := maxCharsInput(runtime); ok {
		input["max_chars"] = n
	}
	return input
}

// dropAnnotatedCsv removes the prefixed copy of the data, leaving the plain
// csv field as the single payload.
//
// The two fields carry the same rows twice — on a real column read that is
// ~530K chars duplicated per batch — and the prefixed form is the one callers
// then have to strip before parsing. It is kept only when asked for via
// --include-row-prefix, for callers addressing rows by literal number.
//
// The tool's warning_message tells the reader to take row numbers from
// annotated_csv's prefixes. With that field gone the instruction points at
// something absent, so the equivalent instruction for the plain payload is
// substituted in place; if the upstream wording changes and no longer matches,
// the message is left untouched rather than mangled.
func dropAnnotatedCsv(out interface{}) interface{} {
	m, ok := out.(map[string]interface{})
	if !ok {
		return out
	}
	delete(m, "annotated_csv")
	if w, ok := m["warning_message"].(string); ok {
		if fixed := strings.ReplaceAll(w,
			"annotated_csv 中每行开头的 [row=N] 前缀（N 即为实际表格行号）",
			"row_indices（与 csv 的数据行一一对应，即实际表格行号）"); fixed != w {
			m["warning_message"] = fixed
		}
	}
	return m
}

// addPlainCsvField publishes the read as plain RFC4180 CSV under "csv",
// alongside the annotated form.
//
// The tool only emits annotated_csv, whose "[row=N] " line prefixes have to
// be stripped before the text can be parsed — and eval traces show every
// caller writing that stripper by hand, as a regex, at the point where the
// data is about to be counted. That regex is where rows get dropped, and the
// prefix also collides with cell content that legitimately starts with
// "[row=". A shortcut named +csv-get returning no csv field is the reason
// callers reach for data["csv"] and get a KeyError; this supplies it.
//
// Row numbers survive without the prefix: the first row of actual_range plus
// the row's ordinal, or row_indices when rows are not contiguous (skip-hidden,
// filter views).
func addPlainCsvField(out interface{}) interface{} {
	m, ok := out.(map[string]interface{})
	if !ok {
		return out
	}
	annotated, ok := m["annotated_csv"].(string)
	if !ok {
		return out
	}
	lines := strings.Split(annotated, "\n")
	plain := make([]string, len(lines))
	for i, line := range lines {
		plain[i] = stripRowPrefix(line)
	}
	m["csv"] = strings.Join(plain, "\n")
	return m
}

// stripRowPrefix removes one leading "[row=N] " marker from a line, leaving
// lines that carry no marker (continuation lines of a multi-line cell) alone.
//
// Only the single space the tool puts after the marker is removed. A comma
// there belongs to the data: "[row=1] ,b,c" is a row whose first column is
// empty, and trimming that comma would shift every column one to the left
// while leaving the row looking well-formed.
func stripRowPrefix(line string) string {
	if !strings.HasPrefix(line, "[row=") {
		return line
	}
	idx := strings.Index(line, "]")
	if idx < 0 {
		return line
	}
	return strings.TrimPrefix(line[idx+1:], " ")
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
		if err := validateSheetSelectorPreflight(runtime); err != nil {
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
		sheetID, sheetName, err := resolveSheetSelectorExec(ctx, runtime, token)
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
		"ranges":              splitRangeAreas(runtime.Str("range")),
		"include_styles":      false,
		"value_render_option": "formatted_value",
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	return input
}

// CondFormatResultGet wraps get_cell_ranges with style and conditional-format
// style output hardcoded on, so cell_styles always include the calculated
// results (colorScale / aboveAverage / etc.).
var CondFormatResultGet = common.Shortcut{
	Service:     "sheets",
	Command:     "+cond-format-result-get",
	Description: "Read conditional format results as range coordinates and cell_styles only.",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+cond-format-result-get"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if err := validateSheetSelectorPreflight(runtime); err != nil {
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
		return invokeToolDryRun(token, ToolKindRead, "get_cell_ranges",
			condFormatResultGetInput(runtime, token, sheetID, sheetName))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelectorExec(ctx, runtime, token)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindRead, "get_cell_ranges",
			condFormatResultGetInput(runtime, token, sheetID, sheetName))
		if err != nil {
			return err
		}
		runtime.Out(condFormatResultOnly(out), nil)
		return nil
	},
}

// condFormatResultOnly removes cell content and unrelated metadata from the
// generic get_cell_ranges response. Range coordinates and truncation metadata
// stay intact so callers can map every retained cell_styles object back to the
// real sheet position and detect incomplete reads.
func condFormatResultOnly(out interface{}) interface{} {
	data, ok := out.(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}

	result := make(map[string]interface{})
	for _, key := range []string{"warning_message", "has_more", "returned_cell_count"} {
		if value, exists := data[key]; exists {
			result[key] = value
		}
	}

	rawRanges, ok := data["ranges"].([]interface{})
	if !ok {
		return result
	}
	ranges := make([]interface{}, 0, len(rawRanges))
	for _, rawRange := range rawRanges {
		rangeData, ok := rawRange.(map[string]interface{})
		if !ok {
			continue
		}
		filteredRange := make(map[string]interface{})
		for _, key := range []string{"range", "actual_range", "row_indices", "col_indices", "truncated"} {
			if value, exists := rangeData[key]; exists {
				filteredRange[key] = value
			}
		}
		if rawRows, ok := rangeData["cells"].([]interface{}); ok {
			rows := make([]interface{}, 0, len(rawRows))
			for _, rawRow := range rawRows {
				rawCells, ok := rawRow.([]interface{})
				if !ok {
					continue
				}
				cells := make([]interface{}, 0, len(rawCells))
				for _, rawCell := range rawCells {
					cell := make(map[string]interface{})
					if cellData, ok := rawCell.(map[string]interface{}); ok {
						if styles, exists := cellData["cell_styles"]; exists {
							cell["cell_styles"] = styles
						}
					}
					cells = append(cells, cell)
				}
				rows = append(rows, any(cells))
			}
			filteredRange["cells"] = rows
		}
		ranges = append(ranges, filteredRange)
	}
	result["ranges"] = ranges
	return result
}

func condFormatResultGetInput(runtime *common.RuntimeContext, token, sheetID, sheetName string) map[string]interface{} {
	input := map[string]interface{}{
		"excel_id":                         token,
		"ranges":                           splitRangeAreas(runtime.Str("range")),
		"include_styles":                   true,
		"include_conditional_format_style": true,
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	input["cell_limit"] = unboundedReadLimit
	if n, ok := maxCharsInput(runtime); ok {
		input["max_chars"] = n
	}
	return input
}
