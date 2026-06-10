// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/csv"
	"regexp"
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
		switch {
		case runtime.Bool("rows-json"):
			// --rows-json reshapes the CSV response into structured rows
			// ({row_number, values:{col→cell}}); see assembleRowsJSON.
			out = assembleRowsJSON(out, strings.TrimSpace(runtime.Str("range")))
		case !runtime.Bool("include-row-prefix"):
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

// rowPrefixRe matches the leading "[row=N] " (or "[row=N],") annotation that
// the tool prepends to the first physical line of each logical CSV record.
var rowPrefixRe = regexp.MustCompile(`^\[row=(\d+)\][ ,]?`)

// assembleRowsJSON reshapes the tool's annotated_csv string into structured
// rows so callers never have to regex-parse "[row=N]" or RFC-4180 CSV by hand:
//
//	{
//	  "range": "A1:K3380",
//	  "current_region": "...",        // passthrough, if the tool returned it
//	  "rows": [{"row_number":1,"values":{"A":"姓名", ..., "K":"时间差_分钟"}},
//	           {"row_number":2,"values":{"A":"张三", ..., "K":"8.5"}}, ...]
//	}
//
// Every logical row is emitted, including the first — no row is assumed to be a
// header, since sheet data is not always tabular. Each cell is keyed by its
// column letter (from the tool's col_indices when present, else derived from the
// requested range's start column). On any parsing trouble it returns the
// original output unchanged.
func assembleRowsJSON(out interface{}, requestedRange string) interface{} {
	m, ok := out.(map[string]interface{})
	if !ok {
		return out
	}
	csvStr, ok := m["annotated_csv"].(string)
	if !ok {
		return out
	}

	// Group physical lines into logical records by [row=N] boundaries; lines
	// without a prefix are embedded-newline continuations of the current record.
	type logicalRow struct {
		num  int
		text string
	}
	var groups []logicalRow
	for _, line := range strings.Split(csvStr, "\n") {
		if mm := rowPrefixRe.FindStringSubmatch(line); mm != nil {
			n, _ := strconv.Atoi(mm[1])
			groups = append(groups, logicalRow{num: n, text: line[len(mm[0]):]})
		} else if len(groups) > 0 {
			groups[len(groups)-1].text += "\n" + line
		}
	}
	if len(groups) == 0 {
		return out
	}

	// Parse every logical row; widest row sets the column count. No row is
	// singled out as a header — that would assume the data is tabular, which it
	// often is not. The model reads row 1 like any other row and decides for
	// itself whether it is a header.
	parsed := make([][]string, len(groups))
	maxCols := 0
	for i, g := range groups {
		parsed[i] = parseCSVRecord(g.text)
		if len(parsed[i]) > maxCols {
			maxCols = len(parsed[i])
		}
	}
	if maxCols == 0 {
		return out
	}

	// Column letters key each cell. Prefer the tool's col_indices (authoritative,
	// length == col_count); otherwise derive from the requested range's start col.
	letters := coerceStringSlice(m["col_indices"])
	if len(letters) < maxCols {
		start := csvStartColIndex(requestedRange)
		letters = make([]string, maxCols)
		for j := 0; j < maxCols; j++ {
			letters[j] = csvColLetter(start + j)
		}
	}

	rows := make([]map[string]interface{}, 0, len(groups))
	for i := range groups {
		fields := parsed[i]
		values := make(map[string]interface{}, len(letters))
		for j := range letters {
			v := ""
			if j < len(fields) {
				v = fields[j]
			}
			values[letters[j]] = v
		}
		rows = append(rows, map[string]interface{}{
			"row_number": groups[i].num,
			"values":     values,
		})
	}

	result := map[string]interface{}{}
	for k, v := range m {
		result[k] = v
	}
	result["range"] = requestedRange
	result["rows"] = rows

	// Surface the backend's "数据没读全" signal structurally instead of leaving it
	// buried in warning_message prose. The tool flags it when current_region (the
	// true data extent) reaches past actual_range (what was actually read) — the
	// single most important anti-under-read hint. Mirror that same comparison
	// (regionEndRow > actualEndRow) from the already-passthrough A1 ranges so the
	// model gets the real data range as a first-class field, never having to
	// parse it out of prose.
	if cr, _ := m["current_region"].(string); cr != "" {
		ar, _ := m["actual_range"].(string)
		regionEnd := a1EndRow(cr)
		readEnd := a1EndRow(ar)
		if regionEnd > 0 && readEnd > 0 && regionEnd > readEnd {
			result["data_not_fully_read"] = map[string]interface{}{
				"read_through_row":         readEnd,
				"data_extends_through_row": regionEnd,
				"unread_rows":              regionEnd - readEnd,
				"reread_range":             cr,
			}
		}
	}

	// Drop the fields whose information rows-json fully carries elsewhere:
	//   - annotated_csv / row_indices / col_indices → reconstructed into
	//     columns + rows (with integer row_number), losslessly.
	//   - warning_message → its two halves are both handled: the static
	//     "[row=N] / col_indices[j]" parse nag is moot once those fields exist,
	//     and the dynamic "数据没读全" half is now the structured
	//     data_not_fully_read field above. (Confirmed against the backend's
	//     get-range-as-csv.ts — warning_message has no other content.)
	delete(result, "annotated_csv")
	delete(result, "row_indices")
	delete(result, "col_indices")
	delete(result, "warning_message")
	return result
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

// parseCSVRecord parses a single logical CSV record (which may span multiple
// physical lines via quoted embedded newlines) into its fields. An empty record
// yields no fields; a malformed record falls back to a naive comma split so a
// stray quote never drops a whole row.
func parseCSVRecord(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	r := csv.NewReader(strings.NewReader(text))
	r.FieldsPerRecord = -1
	fields, err := r.Read()
	if err != nil {
		return strings.Split(text, ",")
	}
	return fields
}

// coerceStringSlice returns v as []string when it is a homogeneous []interface{}
// of strings (the shape of the tool's col_indices), else nil.
func coerceStringSlice(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		s, ok := e.(string)
		if !ok {
			return nil
		}
		out = append(out, s)
	}
	return out
}

// csvStartColIndex returns the 0-based column index of a range's start column,
// e.g. "A1:K3380" → 0, "C5:F9" → 2, "Sheet1!D2" → 3. Unparseable input → 0.
func csvStartColIndex(rng string) int {
	rng = strings.TrimSpace(rng)
	if i := strings.LastIndex(rng, "!"); i >= 0 {
		rng = rng[i+1:]
	}
	var letters strings.Builder
	for _, c := range rng {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			letters.WriteRune(c)
			continue
		}
		break
	}
	if letters.Len() == 0 {
		return 0
	}
	return csvColToIndex(letters.String())
}

// csvColToIndex converts a column letter to its 0-based index ("A"→0, "K"→10,
// "AA"→26). Non-letter input → -1.
func csvColToIndex(s string) int {
	n := 0
	for _, c := range strings.ToUpper(s) {
		if c < 'A' || c > 'Z' {
			break
		}
		n = n*26 + int(c-'A'+1)
	}
	return n - 1
}

// csvColLetter converts a 0-based column index back to its letter (0→"A",
// 25→"Z", 26→"AA"). Negative input → "".
func csvColLetter(idx int) string {
	if idx < 0 {
		return ""
	}
	var b []byte
	for idx >= 0 {
		b = append([]byte{byte('A' + idx%26)}, b...)
		idx = idx/26 - 1
	}
	return string(b)
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
		token, err := resolveSpreadsheetToken(runtime)
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
