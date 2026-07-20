// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/util"
	"github.com/larksuite/cli/shortcuts/common"
)

// ─── +table-put (cli-only, typed DataFrame-style import) ───────────
//
// Imports a typed, column-described table into Lark Sheets, type-faithfully:
// numbers stay numbers, dates land as real dates (serial + date number_format),
// not look-alike text. The wire protocol is deliberately a "table with column
// types" — pandas DataFrames are its most common producer, but the CLI never
// has to know about pandas.
//
// Writes into an existing spreadsheet (addressed by --url / --spreadsheet-token);
// to create a fresh workbook first, use +workbook-create, then point +table-put
// at the returned token. Multiple DataFrames → multiple sheets in one call: the
// top-level `sheets` array carries one entry per sub-sheet, each matched to an
// existing sub-sheet by name (created when absent).
//
// Date faithfulness was verified empirically (see isoDateToSerial): the only
// way to get a *real* date (ISNUMBER=TRUE, sortable / pivotable) is to write
// the Excel serial number AND set a date number_format. A date *string*, even
// with a date format, stays text — so date columns always go through the
// serial conversion below.

// TablePut is the typed table-put shortcut. It writes into an existing
// spreadsheet, composing get_workbook_structure / modify_workbook_structure /
// set_cell_range — no new backend tool, and no workbook creation (use
// +workbook-create for that, consistent with every other write shortcut).
var TablePut = common.Shortcut{
	Service:     "sheets",
	Command:     "+table-put",
	Description: "Write a typed table (columns with types + rows) into an existing spreadsheet; numbers and dates stay type-faithful.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:read", "sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+table-put"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		payload, err := resolveTablePayload(runtime)
		if err != nil {
			return err
		}
		// --styles is parsed (and aligned against the payload's sheets) up front
		// so a malformed style item fails before any write lands — mirroring
		// +workbook-create's Validate.
		styles, err := parseWorkbookCreateSheetStyles(runtime, payload)
		if err != nil {
			return err
		}
		return payload.checkCellBudgetWithStyles(styles)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return tablePutDryRun(runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		payload, err := resolveTablePayload(runtime)
		if err != nil {
			return err
		}
		styles, err := parseWorkbookCreateSheetStyles(runtime, payload)
		if err != nil {
			return err
		}
		return tablePutWrite(ctx, runtime, token, payload, styles)
	},
	Tips: []string{
		"Writes into an existing spreadsheet — pass --url or --spreadsheet-token. To create a new workbook first, use +workbook-create, then point --spreadsheet-token here.",
		"Payload sheets are matched to existing sub-sheets by name (created when absent). Date columns take ISO yyyy-mm-dd strings — converted to real dates (serial + date format).",
		"--styles applies number formats, colors, merges, and row/col sizes in the same call (same shape as +workbook-create's --styles): one styles item per written sheet, name-matched. Skips the separate +cells-set-style round-trip.",
	},
}

// resolveTablePayload parses --sheets (typed JSON, multi-sheet) into the
// unified internal tablePayload. Both +table-put and +workbook-create funnel
// through here so the two entry points stay in lockstep; Validate / Execute /
// DryRun / workbookCreateData all share this one decision. Network-free.
func resolveTablePayload(rctx *common.RuntimeContext) (*tablePayload, error) {
	sheetsGiven := rctx.Changed("sheets") && strings.TrimSpace(rctx.Str("sheets")) != ""
	if !sheetsGiven {
		return nil, common.ValidationErrorf("--sheets is required")
	}
	return parseTablePutPayload(rctx)
}

// ─── protocol ─────────────────────────────────────────────────────────

type tablePayload struct {
	Sheets []tableSheetSpec `json:"sheets"`
}

// tableSheetSpec is the *internal* representation a sheet is normalized into
// after parsing the wire protocol. It carries everything buildSheetMatrix and
// friends need (typed columns + format + 2D row matrix) and is what the rest of
// this file works against. The wire shape — string columns + dtypes/formats
// maps + `data` — lives in tableSheetIn and is collapsed into this struct by
// (*tableSheetIn).normalize.
type tableSheetSpec struct {
	Name      string
	StartCell string
	// Mode controls write placement: "overwrite" (default) writes a header+data
	// block from start_cell; "append" writes data below the sheet's existing
	// data (start_cell's row is ignored, its column is honored).
	Mode string
	// Header is whether to write a header row of column names. nil defaults by
	// mode: true for overwrite, false for append (so appended rows don't repeat
	// the header). Set explicitly to override.
	Header *bool
	// AllowOverwrite, when explicitly false, makes the write fail if it would
	// land on a non-empty cell. nil defaults to true (overwrite).
	AllowOverwrite *bool
	Columns        []tableColumnSpec
	Rows           [][]interface{}
}

type tableColumnSpec struct {
	Name   string
	Type   string
	Format string
}

// tableSheetIn is the wire-level shape of one sheet in --sheets. It is
// pandas-DataFrame-shaped on purpose: `columns` is a plain string list, `data`
// is a 2D array, and the per-column type / display format are *separate*
// dtypes/formats maps keyed by column name. That gives agents a one-liner
// (`{**json.loads(df.to_json(orient="split")), "dtypes":
// df.dtypes.astype(str).to_dict()}`) and lets handwritten payloads stay flat
// rather than nest a {name, type, format} object per column.
type tableSheetIn struct {
	Name           string            `json:"name"`
	StartCell      string            `json:"start_cell"`
	Mode           string            `json:"mode"`
	Header         *bool             `json:"header"`
	AllowOverwrite *bool             `json:"allow_overwrite"`
	Columns        []string          `json:"columns"`
	Data           [][]interface{}   `json:"data"`
	Dtypes         map[string]string `json:"dtypes"`
	Formats        map[string]string `json:"formats"`
}

// dtypeToTypeFormat maps a pandas-style dtype string to the internal column
// (type, default format) pair. The mapping is deliberately permissive: a missing
// or unknown dtype falls through to string + text format (`@`) so a
// `to_json(orient="split")` payload that omits `dtypes` writes correctly as an
// all-string table. Recognized families:
//   - int*/uint* (lowercase numpy + capitalized nullable pandas) → number
//   - float* / Float* / complex*                                  → number
//   - bool / boolean (nullable)                                   → bool
//   - datetime*  (incl. tz-aware datetime64[ns, UTC])             → date, "yyyy-mm-dd"
//   - everything else (object, string, category, empty, unknown) → string, "@"
//
// Explicit `formats[col]` is layered on top of this default by normalize, so a
// user-supplied `#,##0.00` on a float64 column still wins.
func dtypeToTypeFormat(dtype string) (typ, format string) {
	d := strings.TrimSpace(dtype)
	if d == "" {
		return "string", "@"
	}
	lower := strings.ToLower(d)
	switch {
	case strings.HasPrefix(lower, "datetime"):
		return "date", "yyyy-mm-dd"
	case lower == "bool" || lower == "boolean":
		return "bool", ""
	case isNumericDtype(lower):
		return "number", ""
	default:
		return "string", "@"
	}
}

// isNumericDtype recognizes pandas/numpy numeric dtype strings (lowercased).
// Covers numpy ints (`int8`/`int64`/...), unsigned ints (`uint*`), floats
// (`float32`/`float64`), complex, and pandas' nullable variants
// (`int64`/`uint64`/`float64` lowercased from `Int64`/`UInt64`/`Float64`).
func isNumericDtype(lower string) bool {
	for _, p := range []string{"int", "uint", "float", "complex"} {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

// typeToDtype is the inverse used by +table-get to label each output column.
// Choices are picked to be safe under a single `df.astype(dtypes)` round-trip:
//   - string → object (pandas default, no-op astype)
//   - number → float64 (works for all numeric cells, including ones with NaN)
//   - date   → datetime64[ns] (matches the ISO strings we emit)
//   - bool   → bool (inferColumnType only picks bool when every cell is bool)
//
// Anything else (defensive default) maps to object.
func typeToDtype(typ string) string {
	switch typ {
	case "number":
		return "float64"
	case "date":
		return "datetime64[ns]"
	case "bool":
		return "bool"
	default:
		return "object"
	}
}

// decoderExpectEOF ensures the decoder has nothing left to read after a
// successful Decode. json.Decoder accepts trailing non-whitespace after the
// first JSON value (unlike json.Unmarshal), so a payload like `{...} trailing`
// would silently be treated as the leading object only. Use this after the
// first Decode to surface the trailing data as a validation error.
func decoderExpectEOF(dec *json.Decoder) error {
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err == nil {
		return fmt.Errorf("trailing data after JSON value") //nolint:forbidigo // intermediate error; the caller wraps it into a typed --sheets/--values validation error
	} else if !errors.Is(err, io.EOF) {
		return fmt.Errorf("trailing data after JSON value: %w", err) //nolint:forbidigo // intermediate error; the caller wraps it into a typed --sheets/--values validation error
	}
	return nil
}

// parseTablePutPayload reads --sheets (JSON, supports @file / stdin) into a
// validated payload. UseNumber keeps numeric cells as json.Number so large
// integers (order IDs, etc.) survive without precision loss or scientific
// notation. The wire shape (tableSheetIn: string columns + dtypes/formats maps
// + `data`) is normalized into the internal tableSheetSpec so the rest of the
// file (buildSheetMatrix, sheetCreateDims, …) is unaware of it. Network-free:
// safe from Validate and DryRun.
func parseTablePutPayload(runtime flagView) (*tablePayload, error) {
	raw := strings.TrimSpace(runtime.Str("sheets"))
	if raw == "" {
		return nil, common.ValidationErrorf("--sheets is required")
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var wire struct {
		Sheets []tableSheetIn `json:"sheets"`
	}
	if err := dec.Decode(&wire); err != nil {
		return nil, common.ValidationErrorf("--sheets: invalid JSON: %v", err).WithCause(err)
	}
	// Reject trailing non-whitespace after the first JSON value: json.Decoder
	// accepts it silently (unlike json.Unmarshal), so e.g. `--sheets '{...} oops'`
	// would otherwise pass Validate and surface as confusing downstream errors.
	if err := decoderExpectEOF(dec); err != nil {
		return nil, common.ValidationErrorf("--sheets: %v", err).WithCause(err)
	}
	p := &tablePayload{Sheets: make([]tableSheetSpec, 0, len(wire.Sheets))}
	for i := range wire.Sheets {
		spec, err := wire.Sheets[i].normalize(i)
		if err != nil {
			return nil, err
		}
		p.Sheets = append(p.Sheets, spec)
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	return p, nil
}

// normalize collapses the wire-level pandas-shaped tableSheetIn into the
// internal tableSheetSpec used by the writer. It pairs each column name with
// its dtype-derived (type, format) — with `formats[name]` overriding the
// default — and renames `data` back to the writer's `Rows`. Per-column
// validation that needs the resolved type lives in tablePayload.validate (so
// errors carry the sheet-index/name context the writer already prints).
func (in *tableSheetIn) normalize(idx int) (tableSheetSpec, error) {
	spec := tableSheetSpec{
		Name:           in.Name,
		StartCell:      in.StartCell,
		Mode:           in.Mode,
		Header:         in.Header,
		AllowOverwrite: in.AllowOverwrite,
		Rows:           in.Data,
	}
	seenCol := make(map[string]bool, len(in.Columns))
	spec.Columns = make([]tableColumnSpec, len(in.Columns))
	for j, name := range in.Columns {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return tableSheetSpec{}, common.ValidationErrorf("--sheets[%d] %q: columns[%d] name is required", idx, in.Name, j)
		}
		if seenCol[name] {
			return tableSheetSpec{}, common.ValidationErrorf("--sheets[%d] %q: duplicate column name %q", idx, in.Name, name)
		}
		seenCol[name] = true
		typ, format := dtypeToTypeFormat(in.Dtypes[name])
		if f, ok := in.Formats[name]; ok {
			format = strings.TrimSpace(f)
		}
		spec.Columns[j] = tableColumnSpec{Name: name, Type: typ, Format: format}
	}
	// Surface dtypes/formats entries that reference a column the sheet doesn't
	// have — almost always a typo (`"foramt"`, `"营 收"` with stray spaces) and
	// silently ignoring them would let the writer succeed with the wrong
	// formatting. The check runs after the column list is built so we can
	// compare against the canonical set.
	for k := range in.Dtypes {
		if !seenCol[k] {
			return tableSheetSpec{}, common.ValidationErrorf("--sheets[%d] %q: dtypes references unknown column %q", idx, in.Name, k).
				WithHint("%s", columnKeyHint("dtypes", k, in.Columns))
		}
	}
	for k := range in.Formats {
		if !seenCol[k] {
			return tableSheetSpec{}, common.ValidationErrorf("--sheets[%d] %q: formats references unknown column %q", idx, in.Name, k).
				WithHint("%s", columnKeyHint("formats", k, in.Columns))
		}
	}
	return spec, nil
}

// columnKeyHint explains a dtypes/formats key that matched no column. The
// dominant failure is Excel habit — keying by column letter (A/B/AA) instead
// of the column name — so call that out explicitly; either way, inline the
// declared column names so the retry needs no second look at the payload.
func columnKeyHint(field, key string, columns []string) string {
	shown := columns
	const maxShown = 12
	suffix := ""
	if len(shown) > maxShown {
		shown = shown[:maxShown]
		suffix = ", …"
	}
	list := `"` + strings.Join(shown, `", "`) + `"` + suffix
	if isColumnLetterKey(key) {
		return fmt.Sprintf("%s keys must be column names from `columns`, not A1-style column letters; this sheet's columns: %s", field, list)
	}
	return fmt.Sprintf("%s keys must exactly match a name in `columns`: %s", field, list)
}

// isColumnLetterKey reports whether key looks like an A1-style column letter
// (A, B, AA, …) rather than a real column name.
func isColumnLetterKey(key string) bool {
	if key == "" || len(key) > 3 {
		return false
	}
	for _, r := range key {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func (p *tablePayload) validate() error {
	if len(p.Sheets) == 0 {
		return common.ValidationErrorf("--sheets: must contain at least one sheet")
	}
	seen := make(map[string]bool, len(p.Sheets))
	for i := range p.Sheets {
		s := &p.Sheets[i]
		if strings.TrimSpace(s.Name) == "" {
			return common.ValidationErrorf("--sheets[%d]: name is required", i)
		}
		if seen[s.Name] {
			return common.ValidationErrorf("--sheets[%d]: duplicate sheet name %q", i, s.Name)
		}
		seen[s.Name] = true
		if len(s.Columns) == 0 {
			return common.ValidationErrorf("--sheets[%d] %q: columns must be non-empty", i, s.Name)
		}
		for j := range s.Columns {
			c := &s.Columns[j]
			// validColumnType still guards the internal Type so a future
			// dtype-mapping change (or a direct test-time construction of a
			// tableSheetSpec) can't silently route an unknown type into
			// buildTypedCell's default branch.
			if !validColumnType(c.Type) {
				return common.ValidationErrorf("--sheets[%d] %q: columns[%d] %q has invalid type %q (want string/number/date/bool)",
					i, s.Name, j, c.Name, c.Type)
			}
		}
		for r := range s.Rows {
			if len(s.Rows[r]) != len(s.Columns) {
				return common.ValidationErrorf("--sheets[%d] %q: row %d has %d cells, want %d (column count)",
					i, s.Name, r, len(s.Rows[r]), len(s.Columns))
			}
			// Validate each cell's value against its column type up front (pure,
			// network-free): a bad date/number/bool is caught here — before any
			// workbook is created — instead of failing mid-write and leaving a
			// stray empty spreadsheet behind.
			for c := range s.Columns {
				if _, err := buildTypedCell(&s.Columns[c], s.Rows[r][c]); err != nil {
					return common.ValidationErrorf("--sheets[%d] %q: row %d column %q: %v", i, s.Name, r, s.Columns[c].Name, err).WithCause(err)
				}
			}
		}
		if sc := strings.TrimSpace(s.StartCell); sc != "" {
			if _, _, ok := splitCellRef(sc); !ok {
				return common.ValidationErrorf("--sheets[%d] %q: start_cell %q must be a single cell ref (e.g. A1)", i, s.Name, sc)
			}
		}
		switch s.Mode {
		case "", "overwrite", "append":
		default:
			return common.ValidationErrorf("--sheets[%d] %q: mode %q is invalid (want \"overwrite\" or \"append\")", i, s.Name, s.Mode)
		}
	}
	return p.checkCellBudget()
}

// maxTablePutCells bounds how many cells a single +table-put / +workbook-create
// write may materialize. Unlike the fan-out stamp cap (maxStampMatrixCells),
// these cells come from the caller's own --sheets/--values payload rather than a
// range blow-up, so this is a generous OOM guardrail, not a usability limit:
// buildSheetMatrix builds the whole rows×cols matrix of per-cell maps in memory
// before slicing it into tablePutMaxCellsPerWrite-sized writes, so an unbounded
// payload (2.6M cells ≈ 900MB heap, doubled again by json.Marshal) OOMs the
// process before the first write leaves.
const maxTablePutCells = 1_000_000

// checkCellBudget rejects a payload whose total materialized cell count across
// all sheets exceeds maxTablePutCells. Counted in int64 to stay overflow-safe on
// pathological row/column counts.
func (p *tablePayload) checkCellBudget() error {
	var total int64
	for i := range p.Sheets {
		total += int64(len(p.Sheets[i].Rows)) * int64(len(p.Sheets[i].Columns))
	}
	return checkTablePutCellBudget(total)
}

// checkCellBudgetWithStyles includes the blank cells that cell_styles will add
// to each sheet's matrix. It must run before DryRun / Execute pads any matrix.
func (p *tablePayload) checkCellBudgetWithStyles(styles *workbookCreateSheetStyles) error {
	var total int64
	for i := range p.Sheets {
		s := &p.Sheets[i]
		rows, cols := len(s.Rows), len(s.Columns)
		_, baseCol, baseRow, _ := sheetAnchor(s)
		if s.Mode == "append" {
			// Append resolves its real row at Execute time. Zero is a safe upper
			// bound for the style-driven extent and keeps validation allocation-free.
			baseRow = 0
		}
		rows, cols = matrixDimensionsForStyles(rows, cols, styles.styleFor(i), baseCol, baseRow)
		total += int64(rows) * int64(cols)
	}
	return checkTablePutCellBudget(total)
}

func checkTablePutCellBudget(total int64) error {
	if total > maxTablePutCells {
		return common.ValidationErrorf(
			"--sheets/--values cover %d cells total, over the %d-cell safety cap; split the write across smaller payloads",
			total, maxTablePutCells)
	}
	return nil
}

// validColumnType reports whether a column type is one the writer understands.
// An empty type is valid and means "type-less" (raw passthrough): the value is
// written as-is and Lark Sheets auto-detects its type — see buildTypedCell.
// +workbook-create's --values synthesizes an all-type-less payload through this.
func validColumnType(t string) bool {
	switch t {
	case "", "string", "number", "date", "bool":
		return true
	}
	return false
}

// ─── type mapping ─────────────────────────────────────────────────────

// headerOn reports whether a header row of column names should be written. A
// nil Header defaults by mode: overwrite writes it; append omits it so the
// appended rows don't repeat the header below an existing one.
func headerOn(s *tableSheetSpec) bool {
	if s.Header != nil {
		return *s.Header
	}
	return s.Mode != "append"
}

// buildSheetMatrix turns a sheet spec into the set_cell_range cells matrix:
// optionally a header row of column names, then one row per data record with
// each cell mapped by its column type. Per-column number_format is attached so
// numbers/dates render correctly (and dates become real dates). Header cells
// carry no style of their own — style them via --styles like any other cell.
func buildSheetMatrix(s *tableSheetSpec, writeHeader bool) ([][]interface{}, error) {
	ncols := len(s.Columns)
	matrix := make([][]interface{}, 0, len(s.Rows)+1)

	if writeHeader {
		header := make([]interface{}, ncols)
		for c := range s.Columns {
			header[c] = map[string]interface{}{"value": s.Columns[c].Name}
		}
		matrix = append(matrix, header)
	}

	for r := range s.Rows {
		row := make([]interface{}, ncols)
		for c := range s.Columns {
			cell, err := buildTypedCell(&s.Columns[c], s.Rows[r][c])
			if err != nil {
				return nil, common.ValidationErrorf("sheet %q row %d column %q: %v", s.Name, r, s.Columns[c].Name, err).WithCause(err)
			}
			row[c] = cell
		}
		matrix = append(matrix, row)
	}
	return matrix, nil
}

// buildTypedCell maps one raw JSON value to a set_cell_range cell per its
// declared column type. A nil (JSON null) becomes an empty cell that still
// carries the column's number_format. number values are kept as json.Number to
// preserve precision; dates are converted to Excel serials. A type-less column
// (Type == "") writes the raw scalar unchanged, letting the backend auto-detect
// the type — the untyped --values path relies on this.
func buildTypedCell(col *tableColumnSpec, raw interface{}) (map[string]interface{}, error) {
	cell := map[string]interface{}{}
	nf := strings.TrimSpace(col.Format)
	if nf == "" {
		switch col.Type {
		case "date":
			nf = "yyyy-mm-dd"
		case "string":
			// Text format keeps digit-like strings (IDs / postcodes / phone numbers)
			// as text, and lets +table-get infer the column back as string instead
			// of guessing number from a numeric-looking value.
			nf = "@"
		}
	}
	if nf != "" {
		cell["cell_styles"] = map[string]interface{}{"number_format": nf}
	}
	if raw == nil {
		return cell, nil
	}
	switch col.Type {
	case "":
		// Type-less column: write the raw JSON scalar as-is so Lark Sheets
		// auto-detects the type (numeric → number, else text). json.Number is
		// kept verbatim for precision; an optional --styles number_format
		// controls display. This is the untyped --values behavior.
		cell["value"] = raw
	case "string":
		cell["value"] = stringifyCellValue(raw)
	case "number":
		n, ok := raw.(json.Number)
		if !ok {
			return nil, fmt.Errorf("number expects a numeric value, got %s", describeJSONType(raw)) //nolint:forbidigo // intermediate error; callers wrap it into a typed --sheets/--values validation error with row/column context
		}
		cell["value"] = n
	case "bool":
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("bool expects true/false, got %s", describeJSONType(raw)) //nolint:forbidigo // intermediate error; callers wrap it into a typed --sheets/--values validation error with row/column context
		}
		cell["value"] = b
	case "date":
		str, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("date expects an ISO yyyy-mm-dd string, got %s", describeJSONType(raw)) //nolint:forbidigo // intermediate error; callers wrap it into a typed --sheets/--values validation error with row/column context
		}
		serial, err := isoDateToSerial(str)
		if err != nil {
			return nil, err
		}
		cell["value"] = serial
	default:
		return nil, fmt.Errorf("unsupported type %q", col.Type) //nolint:forbidigo // intermediate error; callers wrap it into a typed --sheets/--values validation error with row/column context
	}
	return cell, nil
}

// stringifyCellValue renders any JSON scalar as the literal text a string
// column should hold. json.Number keeps its exact digits (no scientific
// notation), so IDs / postcodes survive as written.
func stringifyCellValue(raw interface{}) string {
	switch v := raw.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case bool:
		if v {
			return "TRUE"
		}
		return "FALSE"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func describeJSONType(raw interface{}) string {
	switch raw.(type) {
	case string:
		return "a string"
	case json.Number:
		return "a number"
	case bool:
		return "a boolean"
	case []interface{}:
		return "an array"
	case map[string]interface{}:
		return "an object"
	default:
		return fmt.Sprintf("%T", raw)
	}
}

// excelEpoch is the Excel / Lark Sheets serial-date origin (1899-12-30 = 0).
// Verified empirically: writing serial 45306 renders as 2024-01-15 in Lark
// Sheets, matching Excel's 1900 date system exactly.
var excelEpoch = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

// isoDateToSerial converts an ISO yyyy-mm-dd string to its Excel serial day
// number. The result is written as a numeric cell value with a date
// number_format, which is the only combination that yields a real (sortable,
// pivotable, ISNUMBER=TRUE) date in Lark Sheets.
//
// Accepts both bare dates (`2024-01-15`) and full ISO datetime strings with a
// `T` separator (`2024-01-15T00:00:00.000`, `2024-01-15T08:30:00+08:00`). The
// `T...` suffix is dropped before parsing so the pandas `df_to_sheet` helper
// — which uses `df.to_json(orient="split", date_format="iso")` and therefore
// always emits the full ISO form — round-trips without an extra string clean
// step on the agent side. A leading `T` (no date prefix) is left alone so the
// parser still rejects it cleanly.
func isoDateToSerial(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		// Empty cells in a date-typed column are the classic header/total-row
		// clash with the column-wide dtype declaration; name the three ways
		// out so the caller does not have to guess what "bad format" means.
		return 0, fmt.Errorf("date column has an empty cell — drop the empty rows, fill real yyyy-mm-dd dates, or declare the column dtype as object (text)") //nolint:forbidigo // intermediate error; callers wrap it into a typed --sheets/--values validation error with row/column context
	}
	if i := strings.Index(s, "T"); i > 0 {
		s = s[:i]
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return 0, fmt.Errorf("date %q must be ISO yyyy-mm-dd: %w", s, err) //nolint:forbidigo // intermediate error; callers wrap it into a typed --sheets/--values validation error with row/column context
	}
	return int(math.Round(t.Sub(excelEpoch).Hours() / 24)), nil
}

// ─── range helpers ────────────────────────────────────────────────────

// tablePutMaxCellsPerWrite caps a single set_cell_range write. Larger
// sheets are split into row batches so one oversized request can't exceed the
// tool's cell ceiling. Matches +cells-set's default --max-cells.
const tablePutMaxCellsPerWrite = 50000

// sheetAnchor returns the resolved start cell (default A1) and its 0-based
// column/row. Caller has already validated start_cell, so the parse can't fail
// in practice; the ok guard is defensive.
func sheetAnchor(s *tableSheetSpec) (anchor string, col0, row0 int, err error) {
	anchor = strings.TrimSpace(s.StartCell)
	if anchor == "" {
		anchor = "A1"
	}
	c, r, ok := splitCellRef(anchor)
	if !ok {
		return "", 0, 0, common.ValidationErrorf("start_cell %q must be a single cell ref (e.g. A1)", anchor)
	}
	return anchor, c, r, nil
}

// tablePutFullRange is the A1 rectangle the whole matrix (header + data)
// occupies, for reporting in the result / dry-run. Returns "" when there is
// nothing to write (e.g. header=false with no data rows) — the previous
// formula would have produced an invalid trailing row like "A1:C0".
func tablePutFullRange(s *tableSheetSpec, totalRows int) string {
	if totalRows <= 0 || len(s.Columns) == 0 {
		return ""
	}
	_, col0, row0, err := sheetAnchor(s)
	if err != nil {
		return strings.TrimSpace(s.StartCell)
	}
	ncols := len(s.Columns)
	return fmt.Sprintf("%s%d:%s%d",
		columnIndexToLetter(col0), row0+1,
		columnIndexToLetter(col0+ncols-1), row0+totalRows)
}

// ─── write path ───────────────────────────────────────────────────────

// writeSheetData writes one sheet's matrix via set_cell_range, splitting into
// row batches when the cell count would exceed tablePutMaxCellsPerWrite.
// Returns a per-sheet summary for the result envelope.
func writeSheetData(ctx context.Context, runtime *common.RuntimeContext, token, sheetID string, s *tableSheetSpec, styles *workbookCreateStylePayload, dims gridDims) (map[string]interface{}, error) {
	_, col0, row0, err := sheetAnchor(s)
	if err != nil {
		return nil, err
	}
	ncols := len(s.Columns)

	// append mode starts below the sheet's existing data; start_cell's row is
	// ignored (its column is still honored). overwrite mode anchors at row0.
	baseRow := row0
	writeHeader := headerOn(s)
	if s.Mode == "append" {
		lastRow, err := lastDataRow(ctx, runtime, token, sheetID, dims)
		if err != nil {
			return nil, fmt.Errorf("resolving last data row for append: %w", err) //nolint:forbidigo // intermediate error; surfaced as a partial_success message string via tablePutPartial, not a typed final error
		}
		if lastRow > 0 {
			baseRow = lastRow // 0-based index of the row just below the 1-based last data row
		} else if s.Header == nil {
			// appending to an empty sheet with no explicit header choice: write the
			// header so column names aren't lost (and a later +table-get doesn't
			// consume the first data row as the header).
			writeHeader = true
		}
	}

	matrix, err := buildSheetMatrix(s, writeHeader)
	if err != nil {
		return nil, err
	}
	matrix, err = applyWorkbookCreateStylesToMatrix(matrix, styles, col0, baseRow, fmt.Sprintf("--styles for sheet %q", s.Name))
	if err != nil {
		return nil, err
	}

	if len(matrix) == 0 {
		// header:false with no data rows — nothing to write.
		return map[string]interface{}{
			"name": s.Name, "sheet_id": sheetID, "range": "",
			"data_rows": 0, "columns": ncols, "writes": 0, "mode": writeModeName(s),
		}, nil
	}

	// styles can pad the matrix wider than the declared columns (cell_styles on
	// blank cells past the data extent), so the written width comes from the
	// padded matrix, not ncols.
	writeCols := len(matrix[0])
	startCol := columnIndexToLetter(col0)
	endCol := columnIndexToLetter(col0 + writeCols - 1)
	allowOverwrite := s.AllowOverwrite == nil || *s.AllowOverwrite

	rowsPerBatch := tablePutMaxCellsPerWrite / writeCols
	if rowsPerBatch < 1 {
		rowsPerBatch = 1
	}

	writes := 0
	for start := 0; start < len(matrix); start += rowsPerBatch {
		end := start + rowsPerBatch
		if end > len(matrix) {
			end = len(matrix)
		}
		batchRange := fmt.Sprintf("%s%d:%s%d", startCol, baseRow+start+1, endCol, baseRow+end)
		input := map[string]interface{}{
			"excel_id": token,
			"sheet_id": sheetID,
			"range":    batchRange,
			"cells":    matrix[start:end],
		}
		if !allowOverwrite {
			input["allow_overwrite"] = false
		}
		if _, err := callTool(ctx, runtime, token, ToolKindWrite, "set_cell_range", input); err != nil {
			return nil, fmt.Errorf("writing rows %d-%d: %w", start+1, end, err) //nolint:forbidigo // intermediate error; surfaced as a partial_success message string via tablePutPartial, not a typed final error
		}
		writes++
	}
	if err := applyWorkbookCreateVisualOps(ctx, runtime, token, sheetID, styles); err != nil {
		return nil, fmt.Errorf("applying visual styles: %w", err) //nolint:forbidigo // intermediate error; surfaced as a partial_success message string via tablePutPartial, not a typed final error
	}
	return map[string]interface{}{
		"name":      s.Name,
		"sheet_id":  sheetID,
		"range":     fmt.Sprintf("%s%d:%s%d", startCol, baseRow+1, endCol, baseRow+len(matrix)),
		"data_rows": len(s.Rows),
		"columns":   writeCols,
		"writes":    writes,
		"mode":      writeModeName(s),
	}, nil
}

// writeModeName normalizes the sheet's write mode to a non-empty label for
// result / dry-run reporting ("" defaults to "overwrite").
func writeModeName(s *tableSheetSpec) string {
	if s.Mode == "append" {
		return "append"
	}
	return "overwrite"
}

// lastDataRow returns the 1-based row number of the last row containing data in
// the sheet (0 when empty), so append mode can place new rows just below it. It
// reads current_region via get_range_as_csv — the backend's reported true data
// extent.
//
// The anchor range matters for the same reason it does in sheetCurrentRegion:
// current_region is the bounding box of non-empty cells WITHIN the requested
// range, so an A1 anchor stops at the first fully-empty row and reports a
// too-small last row — append would then write on top of and overwrite the data
// past that gap. Anchoring over the whole grid (A1:<lastCol><lastRow>, from the
// sheet's row_count / column_count) makes the probe span internal blank rows.
// When dimensions are unknown it falls back to the legacy A1 anchor.
func lastDataRow(ctx context.Context, runtime *common.RuntimeContext, token, sheetID string, dims gridDims) (int, error) {
	anchor := "A1"
	if dims.rows > 0 && dims.cols > 0 {
		anchor = "A1:" + columnIndexToLetter(dims.cols-1) + strconv.Itoa(dims.rows)
	}
	out, err := callTool(ctx, runtime, token, ToolKindRead, "get_range_as_csv", map[string]interface{}{
		"excel_id": token,
		"sheet_id": sheetID,
		"range":    anchor,
		"max_rows": unboundedReadLimit,
	})
	if err != nil {
		return 0, err
	}
	m, ok := out.(map[string]interface{})
	if !ok {
		return 0, nil // empty sheet — no output
	}
	region, _ := m["current_region"].(string)
	if region == "" {
		region, _ = m["actual_range"].(string)
	}
	return a1EndRow(region), nil
}

// writeTypedSheets writes a typed payload's sheets into a workbook and returns
// the per-sheet summaries. It deliberately does not emit output — the caller
// composes the envelope, because +table-put and +workbook-create report
// different top-level shapes (a bare token vs. the full spreadsheet metadata).
// Existing sub-sheets are matched by name in a single structure read; missing
// ones are created on demand.
//
// adoptSheetID, when non-empty, is the id of a freshly created workbook's
// default sub-sheet: the first payload sheet adopts it (the default sheet is
// renamed to that sheet's name and reused) so the new workbook isn't left with
// a stray empty "Sheet1" beside the typed sheets. +table-put passes "" (it
// writes into an existing workbook, with no default sheet to adopt);
// +workbook-create passes the default sheet's id.
//
// On failure it returns the summaries written so far alongside the error, so
// the caller can surface a partial_success.
func writeTypedSheets(ctx context.Context, runtime *common.RuntimeContext, token string, payload *tablePayload, adoptSheetID string, styles *workbookCreateSheetStyles) ([]interface{}, error) {
	byName, dimsByName, err := listSheetIDsByName(ctx, runtime, token)
	if err != nil {
		return nil, err
	}

	// Adopt the default sheet as the first payload sheet (rename + reuse), so a
	// just-created workbook doesn't keep its empty default sheet around. Skip if
	// a sheet by that name already exists (it'll be matched normally below).
	if adoptSheetID != "" && len(payload.Sheets) > 0 {
		first := payload.Sheets[0].Name
		if _, exists := byName[first]; !exists {
			if err := renameSheet(ctx, runtime, token, adoptSheetID, first); err != nil {
				return nil, fmt.Errorf("adopting the default sheet as %q failed: %w", first, err) //nolint:forbidigo // intermediate error; surfaced as a partial_success message string via tablePutPartial, not a typed final error
			}
			byName[first] = adoptSheetID
		}
	}

	written := make([]interface{}, 0, len(payload.Sheets))
	for i := range payload.Sheets {
		s := &payload.Sheets[i]
		sheetID, ok := byName[s.Name]
		if !ok {
			rows, cols := sheetCreateDims(s, styles.styleFor(i))
			sheetID, err = createSheet(ctx, runtime, token, s.Name, rows, cols)
			if err != nil {
				return written, fmt.Errorf("creating sheet %q failed: %w", s.Name, err) //nolint:forbidigo // intermediate error; surfaced as a partial_success message string via tablePutPartial, not a typed final error
			}
			byName[s.Name] = sheetID
			// A freshly created sheet's grid is exactly what we just asked for.
			dimsByName[s.Name] = gridDims{rows: rows, cols: cols}
		}
		summary, err := writeSheetData(ctx, runtime, token, sheetID, s, styles.styleFor(i), dimsByName[s.Name])
		if err != nil {
			return written, fmt.Errorf("writing sheet %q failed: %w", s.Name, err) //nolint:forbidigo // intermediate error; surfaced as a partial_success message string via tablePutPartial, not a typed final error
		}
		written = append(written, summary)
	}
	return written, nil
}

// renameSheet renames a sub-sheet in place via modify_workbook_structure. Used
// to adopt a freshly created workbook's default sheet as the first typed sheet
// (see writeTypedSheets); mirrors +sheet-rename's tool input.
func renameSheet(ctx context.Context, runtime *common.RuntimeContext, token, sheetID, newName string) error {
	_, err := callTool(ctx, runtime, token, ToolKindWrite, "modify_workbook_structure", map[string]interface{}{
		"excel_id":  token,
		"operation": "rename",
		"sheet_id":  sheetID,
		"new_name":  newName,
	})
	return err
}

// tablePutWrite writes the payload into an existing workbook and emits the
// +table-put envelope. The shared write loop lives in writeTypedSheets; this
// wrapper adds +table-put's output shape and partial-success reporting. styles
// (optional, parsed from --styles) is forwarded so per-sheet visual ops apply
// in the same call.
func tablePutWrite(ctx context.Context, runtime *common.RuntimeContext, token string, payload *tablePayload, styles *workbookCreateSheetStyles) error {
	written, err := writeTypedSheets(ctx, runtime, token, payload, "", styles)
	if err != nil {
		return tablePutPartial(runtime, token, nil, written, err.Error())
	}
	runtime.Out(map[string]interface{}{
		"spreadsheet_token": token,
		"sheets":            written,
	}, nil)
	return nil
}

// createSheet appends a new sub-sheet sized to hold the spec, then resolves its
// id. The backend's default sheet (20 cols × 200 rows) is too small for wide or
// long tables (e.g. a 37-column quarter matrix), so the create request sizes the
// sheet to the write range up front — otherwise the follow-up set_cell_range
// fails with "range … exceeds sheet bounds". modify_workbook_structure's create
// output shape isn't relied upon — the id is read back by name, which is robust
// across tool-response variations.
func createSheet(ctx context.Context, runtime *common.RuntimeContext, token, name string, rows, cols int) (string, error) {
	input := map[string]interface{}{
		"excel_id":   token,
		"operation":  "create",
		"sheet_name": name,
	}
	if rows > 0 {
		input["rows"] = rows
	}
	if cols > 0 {
		input["columns"] = cols
	}
	if _, err := callTool(ctx, runtime, token, ToolKindWrite, "modify_workbook_structure", input); err != nil {
		return "", err
	}
	id, _, err := lookupSheetIndex(ctx, runtime, token, "", name)
	if err != nil {
		return "", fmt.Errorf("sheet %q created but resolving its id failed: %w", name, err) //nolint:forbidigo // intermediate error; surfaced as a partial_success message string via tablePutPartial, not a typed final error
	}
	return id, nil
}

// sheetCreateDims sizes a to-be-created sheet to the spec's write range so the
// follow-up set_cell_range can't exceed sheet bounds. It accounts for the
// start_cell offset, the optional header row, and any --styles extent (so a
// cell_styles / merge / resize op past the data still fits the grid). The
// backend's 20×200 defaults are kept as floors (ordinary small tables are
// created exactly as before) and its hard limits (200 cols, 50000 rows) as
// ceilings.
func sheetCreateDims(s *tableSheetSpec, styles *workbookCreateStylePayload) (rows, cols int) {
	_, col0, row0, _ := sheetAnchor(s)
	cols = col0 + len(s.Columns)
	rows = row0 + len(s.Rows)
	// Match writeSheetData's header decision exactly. headerOn() is false for
	// append mode by default, but writeSheetData *forces* a header when append
	// hits an empty sheet with no explicit Header choice (so column names
	// aren't lost on the first append). sheetCreateDims runs only when the
	// sheet is being created — therefore the sheet IS empty and that forced
	// header WILL be written, so size for it here. Without this the sheet is
	// created one row short and append-near-50000 / append-at-N-cols-200
	// would bounce off the backend's hard cap.
	if headerOn(s) || (s.Mode == "append" && s.Header == nil) {
		rows++
	}
	// --styles can reach past the data (cell_styles on blank cells get padded
	// into the matrix and written; merges / resizes run as separate ops). Size
	// the grid to cover them too. workbookCreateStyleDimensions returns the
	// extent relative to the anchor, so add the anchor offset back.
	if styles != nil {
		styleRows, styleCols := workbookCreateStyleDimensions(styles, col0, row0)
		if col0+styleCols > cols {
			cols = col0 + styleCols
		}
		if row0+styleRows > rows {
			rows = row0 + styleRows
		}
	}
	if cols < 20 {
		cols = 20
	}
	if cols > 200 {
		cols = 200
	}
	if rows < 200 {
		rows = 200
	}
	if rows > 50000 {
		rows = 50000
	}
	return rows, cols
}

// gridDims is a sub-sheet's physical grid size (row_count × column_count from
// get_workbook_structure). A zero in either field means "unknown", which makes
// the used-range probes (sheetCurrentRegion / lastDataRow) fall back to their
// legacy A1 anchor instead of a full-grid one.
type gridDims struct {
	rows int
	cols int
}

// listSheetIDsByName maps every existing sub-sheet's display name to its id and
// physical grid dimensions via a single get_workbook_structure read. Used by
// write mode to decide which payload sheets already exist (and, for append, to
// anchor the last-data-row probe over the whole grid — see lastDataRow).
func listSheetIDsByName(ctx context.Context, runtime *common.RuntimeContext, token string) (map[string]string, map[string]gridDims, error) {
	out, err := callTool(ctx, runtime, token, ToolKindRead, "get_workbook_structure", map[string]interface{}{
		"excel_id": token,
	})
	if err != nil {
		return nil, nil, err
	}
	m, ok := out.(map[string]interface{})
	if !ok {
		return nil, nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "get_workbook_structure returned non-object output")
	}
	sheets, _ := m["sheets"].([]interface{})
	byName := make(map[string]string, len(sheets))
	dimsByName := make(map[string]gridDims, len(sheets))
	for _, raw := range sheets {
		id, name, rc, cc := tableGetSheetMeta(raw)
		if id != "" && name != "" {
			byName[name] = id
			dimsByName[name] = gridDims{rows: rc, cols: cc}
		}
	}
	return byName, dimsByName, nil
}

// tablePutPartial reports a multi-sheet write that failed partway. When some
// sheets already landed it is a partial_success: their summaries are the primary
// machine-readable output, so we emit an ok:false result envelope on stdout (via
// OutPartialFailure) carrying written_sheets, and return the partial-failure exit
// signal — callers can retry the rest or delete the workbook. When nothing landed
// — the first or only sheet failed — it is a plain failure, so we return a typed
// errs.APIError rather than misleadingly claiming "some sheets were written".
func tablePutPartial(runtime *common.RuntimeContext, token string, spreadsheet interface{}, written []interface{}, reason string) error {
	if len(written) == 0 {
		return errs.NewAPIError(errs.SubtypeServerError, "table-put failed on %s: %s", token, reason).
			WithHint("no sheets were written; fix the cause and retry")
	}
	data := map[string]interface{}{
		"spreadsheet_token": token,
		"written_sheets":    written,
		"reason":            reason,
		"hint":              "some sheets were written; inspect written_sheets, then retry the remaining sheets or delete the spreadsheet",
	}
	if spreadsheet != nil {
		data["spreadsheet"] = spreadsheet
	}
	return runtime.OutPartialFailure(data, nil)
}

// ─── dry-run ──────────────────────────────────────────────────────────

// tablePutDryRun renders the set_cell_range write the shortcut would send for
// each sheet, plus any --styles visual ops (cell_styles merged into the matrix;
// merges / row+col sizes as their own tool calls). Network-free; the payload,
// locator, and styles have already been validated by Validate, so errors here
// degrade to an empty preview rather than twice.
func tablePutDryRun(runtime *common.RuntimeContext) *common.DryRunAPI {
	dry := common.NewDryRunAPI()
	payload, err := resolveTablePayload(runtime)
	if err != nil {
		return dry
	}
	token, err := resolveSpreadsheetToken(runtime)
	if err != nil {
		return dry
	}
	sheetStyles, _ := parseWorkbookCreateSheetStyles(runtime, payload)
	for i := range payload.Sheets {
		s := &payload.Sheets[i]
		matrix, _ := buildSheetMatrix(s, headerOn(s))
		rng := tablePutFullRange(s, len(matrix))
		if s.Mode == "append" {
			rng = "<append below existing data>"
		} else {
			// cell_styles are merged into the matrix only for overwrite mode,
			// where the anchor row is known statically; append's base row is
			// resolved at execute time, so the preview leaves the matrix bare
			// (the merges / sizes ops below still render). Padding can widen /
			// lengthen the matrix past the data, so recompute the range from the
			// padded dims to match what Execute writes.
			_, col0, row0, _ := sheetAnchor(s)
			matrix, _ = applyWorkbookCreateStylesToMatrix(matrix, sheetStyles.styleFor(i), col0, row0, fmt.Sprintf("--styles for sheet %q", s.Name))
			if len(matrix) > 0 {
				rng = fmt.Sprintf("%s%d:%s%d",
					columnIndexToLetter(col0), row0+1,
					columnIndexToLetter(col0+len(matrix[0])-1), row0+len(matrix))
			}
		}
		writeCols := len(s.Columns)
		if len(matrix) > 0 {
			writeCols = len(matrix[0])
		}
		desc := fmt.Sprintf("write sheet %q (%d data rows × %d cols, mode=%s) via set_cell_range",
			s.Name, len(s.Rows), writeCols, writeModeName(s))
		input := map[string]interface{}{
			"excel_id":   token,
			"sheet_name": s.Name,
			"range":      rng,
			"cells":      matrix,
		}
		if s.AllowOverwrite != nil && !*s.AllowOverwrite {
			input["allow_overwrite"] = false
		}
		wireBody, _ := buildToolBody("set_cell_range", input)
		dry.POST(toolInvokePath(token, ToolKindWrite)).Desc(desc).Body(wireBody)
		appendWorkbookCreateVisualOpsDryRun(dry, token, "", s.Name, sheetStyles.styleFor(i))
	}
	return dry
}

// ─── +table-get (typed read-back, mirror of +table-put) ───────────────
//
// Reads a spreadsheet's sheets back into the same typed protocol +table-put
// consumes, so the output round-trips: pipe it straight back to +table-put, or
// load it into a DataFrame. Column types are inferred from each column's
// number_format (a date format → date, numeric/percent → number) and date
// serials are converted back to ISO strings — the exact inverse of the put path.

// TableGet reads sheets back into the typed table protocol.
var TableGet = common.Shortcut{
	Service:     "sheets",
	Command:     "+table-get",
	Description: "Read sheets back into the typed table protocol (mirror of +table-put); column types are inferred from number_format so the output feeds straight to +table-put or a DataFrame.",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+table-get"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if strings.TrimSpace(runtime.Str("sheet-id")) != "" && strings.TrimSpace(runtime.Str("sheet-name")) != "" {
			return common.ValidationErrorf("--sheet-id and --sheet-name are mutually exclusive")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		dry := common.NewDryRunAPI()
		// get_workbook_structure runs on every path now (not just whole-workbook):
		// the single-sheet selector path also reads it to learn the grid's physical
		// dimensions, which anchor the default used-range probe over the full grid.
		body, _ := buildToolBody("get_workbook_structure", map[string]interface{}{"excel_id": token})
		dry.POST(toolInvokePath(token, ToolKindRead)).Desc("read sub-sheets + grid dimensions via get_workbook_structure").Body(body)
		rng := strings.TrimSpace(runtime.Str("range"))
		if rng == "" {
			rng = "<each sheet's used range (full-grid current_region)>"
		}
		// Mirror the selector the Execute path will pass to get_cell_ranges so the
		// dry-run body matches the real request shape; agents that validate one and
		// run the other would otherwise see a sheet_id/sheet_name field appear out
		// of nowhere. Network-free: only echoes the flags the caller already gave.
		input := map[string]interface{}{
			"excel_id": token, "ranges": []string{rng},
			"include_styles": true, "value_render_option": "raw_value",
		}
		sheetSelectorForToolInput(input,
			strings.TrimSpace(runtime.Str("sheet-id")),
			strings.TrimSpace(runtime.Str("sheet-name")),
		)
		body, _ = buildToolBody("get_cell_ranges", input)
		dry.POST(toolInvokePath(token, ToolKindRead)).
			Desc(fmt.Sprintf("read cells (%s) + styles via get_cell_ranges, then infer column types", rng)).
			Body(body)
		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		targets, err := tableGetTargets(ctx, runtime, token)
		if err != nil {
			return err
		}
		noHeader := runtime.Bool("no-header")
		userRange := strings.TrimSpace(runtime.Str("range"))
		sheets := make([]interface{}, 0, len(targets))
		for _, t := range targets {
			spec, err := readSheetAsSpec(ctx, runtime, token, t, userRange, noHeader)
			if err != nil {
				return err
			}
			sheets = append(sheets, spec)
		}
		runtime.Out(map[string]interface{}{"sheets": sheets}, nil)
		return nil
	},
	Tips: []string{
		"Output is the same shape +table-put consumes — pipe it back in, or load sheets[].rows into a DataFrame keyed by columns[].name.",
		"Column types are inferred per column, but only when every non-empty cell agrees; a column mixing types (e.g. numbers + \"N/A\") degrades to string — lossless and round-trips cleanly. Numeric coercion of dirty cells is the caller's job (pandas to_numeric(errors=\"coerce\") on the string column).",
	},
}

// tableGetSheet identifies a sheet to read back.
//
// rowCount / colCount carry the sheet's physical grid dimensions
// (get_workbook_structure's row_count / column_count). They're used to anchor
// the default used-range probe over the whole grid instead of A1 — see
// sheetCurrentRegion. 0 means "unknown" (structure read skipped or failed), in
// which case the probe falls back to the legacy A1 anchor.
type tableGetSheet struct {
	id       string
	name     string
	rowCount int
	colCount int
}

// tableGetTargets resolves which sheets +table-get reads: the one named by
// --sheet-id / --sheet-name, or every sheet (in workbook order) when neither is
// given. Either way it reads get_workbook_structure once so each target carries
// its physical grid dimensions (rowCount / colCount) — the default used-range
// probe needs them to anchor over the full grid. For the single-sheet selector
// path this also backfills the missing name (handy when only --sheet-id was
// given, so the output spec isn't left nameless).
func tableGetTargets(ctx context.Context, runtime *common.RuntimeContext, token string) ([]tableGetSheet, error) {
	id := strings.TrimSpace(runtime.Str("sheet-id"))
	name := strings.TrimSpace(runtime.Str("sheet-name"))

	out, err := callTool(ctx, runtime, token, ToolKindRead, "get_workbook_structure", map[string]interface{}{"excel_id": token})
	if err != nil {
		// Single-sheet selector path can degrade gracefully without dimensions
		// (the probe falls back to the A1 anchor); the whole-workbook path can't
		// enumerate sheets without the structure, so it must surface the error.
		// Name doubles as id for --sheet-id so the output spec is never nameless
		// (an empty name would break +table-get → +table-put round-trip — the
		// writer requires a non-empty sheet name).
		if id != "" {
			return []tableGetSheet{{id: id, name: id}}, nil
		}
		if name != "" {
			return []tableGetSheet{{name: name}}, nil
		}
		return nil, err
	}
	m, _ := out.(map[string]interface{})
	raw, _ := m["sheets"].([]interface{})

	// Selector path: find the matching sheet to pick up its dimensions (and
	// backfill name when only --sheet-id was given). If no row matches, fall
	// back to a dimensionless target so the read still proceeds via the A1
	// anchor rather than erroring on a structure/selector mismatch — same
	// id-as-name backfill applies so the output spec round-trips.
	if id != "" || name != "" {
		for _, r := range raw {
			sid, sname, rc, cc := tableGetSheetMeta(r)
			if (id != "" && sid == id) || (name != "" && sname == name) {
				return []tableGetSheet{{id: sid, name: sname, rowCount: rc, colCount: cc}}, nil
			}
		}
		if id != "" {
			return []tableGetSheet{{id: id, name: id}}, nil
		}
		return []tableGetSheet{{name: name}}, nil
	}

	targets := make([]tableGetSheet, 0, len(raw))
	for _, r := range raw {
		sid, sname, rc, cc := tableGetSheetMeta(r)
		if sid != "" {
			targets = append(targets, tableGetSheet{id: sid, name: sname, rowCount: rc, colCount: cc})
		}
	}
	if len(targets) == 0 {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "get_workbook_structure returned no sheets")
	}
	return targets, nil
}

// tableGetSheetMeta pulls one sheet's id, name (title fallback), and physical
// grid dimensions out of a get_workbook_structure sheets[] entry. Dimensions
// default to 0 ("unknown") when absent or non-numeric.
func tableGetSheetMeta(r interface{}) (id, name string, rowCount, colCount int) {
	sm, ok := r.(map[string]interface{})
	if !ok {
		return "", "", 0, 0
	}
	id, _ = sm["sheet_id"].(string)
	name, _ = sm["sheet_name"].(string)
	if name == "" {
		name, _ = sm["title"].(string)
	}
	if f, ok := util.ToFloat64(sm["row_count"]); ok {
		rowCount = int(f)
	}
	if f, ok := util.ToFloat64(sm["column_count"]); ok {
		colCount = int(f)
	}
	return id, name, rowCount, colCount
}

// readSheetAsSpec reads one sheet's region and rebuilds it as a typed-protocol
// sheet — the inverse of the put path and the same wire shape +table-put
// accepts: a string `columns` list, a 2D `data` matrix, and `dtypes` / `formats`
// maps keyed by column name. That symmetry lets callers round-trip via the
// pandas-native idiom
//
//	pd.DataFrame(sheet["data"], columns=sheet["columns"]).astype(sheet["dtypes"])
//
// without a custom helper. `dtypes` is always emitted (one entry per column, so
// a single `astype()` call covers every column); `formats` is emitted only for
// columns whose source cells carry a non-empty number_format, since `astype`
// ignores it and we'd rather not pollute the output.
func readSheetAsSpec(ctx context.Context, runtime *common.RuntimeContext, token string, t tableGetSheet, userRange string, noHeader bool) (map[string]interface{}, error) {
	emptySpec := func() map[string]interface{} {
		return map[string]interface{}{
			"name":    t.name,
			"columns": []interface{}{},
			"data":    []interface{}{},
			"dtypes":  map[string]interface{}{},
			"range":   "",
		}
	}
	region := userRange
	if region == "" {
		r, err := sheetCurrentRegion(ctx, runtime, token, t)
		if err != nil {
			return nil, err
		}
		region = r
	}
	if region == "" {
		return emptySpec(), nil // empty sheet
	}
	input := map[string]interface{}{
		"excel_id":            token,
		"ranges":              []string{region},
		"include_styles":      true,
		"value_render_option": "raw_value",
		"cell_limit":          unboundedReadLimit,
	}
	sheetSelectorForToolInput(input, t.id, t.name)
	out, err := callTool(ctx, runtime, token, ToolKindRead, "get_cell_ranges", input)
	if err != nil {
		return nil, err
	}
	grid := extractCellGrid(out)
	if len(grid) == 0 {
		return emptySpec(), nil
	}

	var headerRow []map[string]interface{}
	dataRows := grid
	if !noHeader {
		headerRow = grid[0]
		dataRows = grid[1:]
	}
	ncols := 0
	for _, r := range grid {
		if len(r) > ncols {
			ncols = len(r)
		}
	}

	columnNames := make([]interface{}, ncols)
	colTypes := make([]string, ncols)
	dtypes := make(map[string]interface{}, ncols)
	formats := map[string]interface{}{}
	// Duplicate header names break +table-get → +table-put round-trip: the dtypes
	// map (keyed by name) silently collapses to a single entry and the writer
	// later rejects the duplicate columns in --sheets validation. Fail fast with
	// an actionable hint when the source sheet actually has duplicate headers.
	// noHeader mode is exempt because tableGetColumnName falls back to positional
	// col<N> names which are always unique.
	seenNames := map[string]int{}
	for c := 0; c < ncols; c++ {
		typ, format := inferColumnType(dataRows, c)
		colTypes[c] = typ
		name := tableGetColumnName(headerRow, c, noHeader)
		if !noHeader {
			if prev, dup := seenNames[name]; dup {
				return nil, common.ValidationErrorf(
					"sheet %q: duplicate header column name %q at columns %d and %d; this would break the +table-get → +table-put round-trip. Rename the headers or pass --no-header to read by position (col1/col2/…).",
					t.name, name, prev+1, c+1)
			}
			seenNames[name] = c
		}
		columnNames[c] = name
		dtypes[name] = typeToDtype(typ)
		// Only emit a format when the column actually has one and it's not the
		// implicit text-format we paint on string columns (the `@` is a writer
		// convention, not user intent — surfacing it would round-trip back as
		// an explicit format the user never set).
		if format != "" && !isTextNumberFormat(format) {
			formats[name] = format
		}
	}

	data := make([][]interface{}, 0, len(dataRows))
	for _, r := range dataRows {
		row := make([]interface{}, ncols)
		for c := 0; c < ncols; c++ {
			row[c] = cellToTyped(cellAt(r, c), colTypes[c])
		}
		data = append(data, row)
	}
	spec := map[string]interface{}{
		"name":    t.name,
		"columns": columnNames,
		"data":    data,
		"dtypes":  dtypes,
		// The range actually read — whether from --range or the computed used
		// range. get_cell_ranges has no has_more flag, so this is the only signal
		// a caller has to detect truncation (compare its extent against the source
		// xlsx / +workbook-info). Harmless on round-trip: +table-put ignores it.
		"range": region,
	}
	if len(formats) > 0 {
		spec["formats"] = formats
	}
	return spec, nil
}

// sheetCurrentRegion returns the A1 range covering the sheet's existing data,
// or "" for an empty sheet.
//
// It reads get_range_as_csv's current_region, but the anchor range it requests
// is critical: current_region is the bounding box of the non-empty cells WITHIN
// the requested range, so an A1 anchor stops at the first fully-empty row or
// column and silently truncates everything past it (the pro016 / pro025
// incident). Anchoring over the whole physical grid (A1:<lastCol><lastRow>,
// from the sheet's row_count / column_count) instead makes the backend span
// internal gaps and report the true used range. When dimensions are unknown
// (structure read skipped / failed) it falls back to the legacy A1 anchor so
// the path degrades to its prior behavior rather than erroring.
func sheetCurrentRegion(ctx context.Context, runtime *common.RuntimeContext, token string, t tableGetSheet) (string, error) {
	anchor := "A1"
	if t.rowCount > 0 && t.colCount > 0 {
		anchor = "A1:" + columnIndexToLetter(t.colCount-1) + strconv.Itoa(t.rowCount)
	}
	input := map[string]interface{}{"excel_id": token, "range": anchor, "max_rows": unboundedReadLimit}
	sheetSelectorForToolInput(input, t.id, t.name)
	out, err := callTool(ctx, runtime, token, ToolKindRead, "get_range_as_csv", input)
	if err != nil {
		return "", err
	}
	m, ok := out.(map[string]interface{})
	if !ok {
		return "", nil
	}
	region, _ := m["current_region"].(string)
	if region == "" {
		region, _ = m["actual_range"].(string)
	}
	return region, nil
}

// extractCellGrid pulls ranges[0].cells out of a get_cell_ranges response into a
// 2D grid of cell objects (each carrying value + cell_styles).
func extractCellGrid(out interface{}) [][]map[string]interface{} {
	m, ok := out.(map[string]interface{})
	if !ok {
		return nil
	}
	ranges, _ := m["ranges"].([]interface{})
	if len(ranges) == 0 {
		return nil
	}
	r0, _ := ranges[0].(map[string]interface{})
	rawCells, _ := r0["cells"].([]interface{})
	grid := make([][]map[string]interface{}, 0, len(rawCells))
	for _, rr := range rawCells {
		rowArr, _ := rr.([]interface{})
		row := make([]map[string]interface{}, 0, len(rowArr))
		for _, cc := range rowArr {
			cm, _ := cc.(map[string]interface{})
			row = append(row, cm)
		}
		grid = append(grid, row)
	}
	return grid
}

func cellAt(row []map[string]interface{}, c int) map[string]interface{} {
	if c >= 0 && c < len(row) {
		return row[c]
	}
	return nil
}

func readCellValue(cell map[string]interface{}) interface{} {
	if cell == nil {
		return nil
	}
	return cell["value"]
}

func readCellFormat(cell map[string]interface{}) string {
	if cell == nil {
		return ""
	}
	st, _ := cell["cell_styles"].(map[string]interface{})
	nf, _ := st["number_format"].(string)
	return nf
}

// inferColumnType decides a column's type from its data cells: a date
// number_format guides each cell's type, but a column is given a non-string type
// only when EVERY non-empty cell agrees. Real sheet columns often mix types (a
// number column with a stray "N/A", a date column with a bare count); declaring
// number/date while a string value rides along makes the output inconsistent —
// it breaks round-trip back into +table-put (which rejects a string in a number
// column) and crashes pandas astype. So a mixed column degrades to string
// (lossless, self-consistent), keeping columns[].type faithful to every value in
// rows. Coercing dirty cells onto a numeric column is deliberately left to the
// caller (pandas to_numeric(errors="coerce") on the string column): lossless
// there — the original values stay in the frame — whereas doing it here would
// drop them silently and irrecoverably.
func inferColumnType(dataRows [][]map[string]interface{}, c int) (string, string) {
	seen := map[string]bool{}
	numberFormat, dateFormat := "", ""
	for _, r := range dataRows {
		cell := cellAt(r, c)
		v := readCellValue(cell)
		if v == nil {
			continue
		}
		if s, ok := v.(string); ok && s == "" {
			continue // empty string is empty, not a string value
		}
		nf := readCellFormat(cell)
		switch {
		case isDateNumberFormat(nf):
			// A date format yields date only when the cell is actually a serial
			// number; a date format painted on text is just text.
			if _, ok := tableGetToFloat(v); ok {
				seen["date"] = true
				if dateFormat == "" {
					dateFormat = nf
				}
			} else {
				seen["string"] = true
			}
		case isTextNumberFormat(nf):
			seen["string"] = true
		default:
			switch v.(type) {
			case float64, json.Number:
				seen["number"] = true
				if numberFormat == "" {
					numberFormat = nf
				}
			case bool:
				seen["bool"] = true
			default:
				seen["string"] = true
			}
		}
	}
	switch {
	case len(seen) == 0:
		return "string", "" // all empty
	case len(seen) == 1:
		switch {
		case seen["date"]:
			return "date", dateFormat
		case seen["number"]:
			return "number", numberFormat
		case seen["bool"]:
			return "bool", ""
		default:
			return "string", ""
		}
	default:
		return "string", "" // mixed types → string (self-consistent, lossless)
	}
}

// isDateNumberFormat reports whether a number_format denotes a date/time. Date
// formats carry a year token (Excel's 'yy' or 'yyyy'); pure numeric formats
// (#,##0, 0.00, 0.00%, @) do not.
//
// Token-aware so currency / unit prefixes that happen to contain a lone 'y' or
// 'Y' — most notably "JPY #,##0" — are not misread as dates. The scanner skips:
//   - characters inside double-quoted literals  ("Yen ")
//   - the character following a backslash escape (\y)
//   - characters inside [...] sections          ([Red], [$EUR-2])
//
// and only fires on an unquoted/unescaped/unbracketed 'yy' (a single 'y' is
// not a year token in Excel; "JPY 0" has 'Y' but never 'yy').
func isDateNumberFormat(nf string) bool {
	s := strings.ToLower(nf)
	inQuote, inBracket, escape := false, false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escape {
			escape = false
			continue
		}
		switch {
		case c == '\\':
			escape = true
		case !inBracket && c == '"':
			inQuote = !inQuote
		case !inQuote && c == '[':
			inBracket = true
		case !inQuote && c == ']':
			inBracket = false
		case !inQuote && !inBracket:
			if c == 'y' && i+1 < len(s) && s[i+1] == 'y' {
				return true
			}
		}
	}
	return false
}

// isTextNumberFormat reports whether a number_format is Excel/Lark text format
// ("@"), which +table-put writes on string columns so digit-like strings survive
// and read back as string instead of being inferred as number.
func isTextNumberFormat(nf string) bool {
	return strings.TrimSpace(nf) == "@"
}

// cellToTyped converts a read-back cell to the JSON-safe value its column type
// implies: date serials become ISO strings, numbers/bools pass through, empty
// cells become null, everything else is stringified. inferColumnType guarantees
// a non-string column is homogeneous, so the date/number branches never meet a
// stray off-type value.
func cellToTyped(cell map[string]interface{}, typ string) interface{} {
	v := readCellValue(cell)
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok && s == "" {
		return nil
	}
	switch typ {
	case "date":
		if f, ok := tableGetToFloat(v); ok {
			return serialToISO(f)
		}
		return v
	case "number", "bool":
		return v
	default:
		return stringifyCellValue(v)
	}
}

// tableGetColumnName returns the column's name: the header cell's text, or a
// positional col<N> when there is no header row.
func tableGetColumnName(headerRow []map[string]interface{}, c int, noHeader bool) string {
	if !noHeader {
		if v := readCellValue(cellAt(headerRow, c)); v != nil {
			if s := stringifyCellValue(v); s != "" {
				return s
			}
		}
	}
	return fmt.Sprintf("col%d", c+1)
}

func tableGetToFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

// serialToISO converts an Excel serial day number back to an ISO yyyy-mm-dd
// string — the inverse of isoDateToSerial.
func serialToISO(serial float64) string {
	return excelEpoch.AddDate(0, 0, int(serial)).Format("2006-01-02")
}
