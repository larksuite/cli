// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/csv"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// ─── lark_sheet_write_cells ───────────────────────────────────────────
//
// Wraps:
//   - set_cell_range     (powers +cells-set / +cells-set-style /
//                        +dropdown-set / +dropdown-update / +dropdown-delete)
//   - set_range_from_csv (powers +csv-put)
//
// +cells-set-image is a `cli_only_derivative` shortcut (needs a local file
// upload before calling set_cell_range); it lives in the cli-only batch
// where the upload helper is shared with +workbook-create / +dim-move /
// +workbook-export.
//
// All set_cell_range-backed shortcuts construct a cells matrix whose
// dimensions exactly match the target range — the tool errors on mismatch.

// CellsSet wraps set_cell_range: caller provides the cells matrix via --cells
// (JSON), with an optional --copy-to-range to replicate the written block
// across a larger area (formula refs auto-shift).
var CellsSet = common.Shortcut{
	Service:     "sheets",
	Command:     "+cells-set",
	Description: "Write values / formulas / styles / comments / data validation / embed-image to a cell range.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+cells-set"),
	Validate:    validateViaInput(cellsSetInput),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := cellsSetInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "set_cell_range", input)
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
		input, err := cellsSetInput(runtime, token, sheetID, sheetName)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "set_cell_range", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

func cellsSetInput(runtime flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	if strings.TrimSpace(runtime.Str("range")) == "" {
		return nil, sheetsValidationForFlag("range", "--range is required")
	}
	cells, err := requireJSONArray(runtime, "cells")
	if err != nil {
		return nil, err
	}
	input := map[string]interface{}{
		"excel_id": token,
		"range":    strings.TrimSpace(runtime.Str("range")),
		"cells":    cells,
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if !runtime.Bool("allow-overwrite") {
		input["allow_overwrite"] = false
	}
	if copyTo := strings.TrimSpace(runtime.Str("copy-to-range")); copyTo != "" {
		input["copy_to_range"] = copyTo
	}
	if err := validateInputAgainstSchema(runtime, input); err != nil {
		return nil, err
	}
	return input, nil
}

// CellsSetStyle stamps a single style block across every cell in --range.
// Style is composed from a dozen flat flags (background-color, font-color,
// font-size, font-style, font-weight, font-line, horizontal-alignment,
// vertical-alignment, word-wrap, number-format) plus --border-styles for
// the only field that still needs a nested object. At least one flag must
// be set.
var CellsSetStyle = common.Shortcut{
	Service:     "sheets",
	Command:     "+cells-set-style",
	Description: "Apply style flags to every cell in a range (values / formulas untouched).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+cells-set-style"),
	Validate:    validateViaInput(cellsSetStyleInput),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := cellsSetStyleInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "set_cell_range", input)
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
		input, err := cellsSetStyleInput(runtime, token, sheetID, sheetName)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "set_cell_range", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

func cellsSetStyleInput(runtime flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	rangeStr := strings.TrimSpace(runtime.Str("range"))
	if rangeStr == "" {
		return nil, sheetsValidationForFlag("range", "--range is required")
	}
	rows, cols, err := rangeDimensions(rangeStr)
	if err != nil {
		return nil, sheetsValidationForFlag("range", "--range %q: %v", rangeStr, err)
	}
	if err := requireAnyStyleFlag(runtime); err != nil {
		return nil, err
	}
	cellStyle := buildCellStyleFromFlags(runtime)
	borderStyles, err := borderStylesFromFlag(runtime)
	if err != nil {
		return nil, err
	}
	cells := make([][]interface{}, rows)
	for r := range cells {
		row := make([]interface{}, cols)
		for c := range row {
			cell := map[string]interface{}{}
			if len(cellStyle) > 0 {
				cell["cell_styles"] = cellStyle
			}
			if borderStyles != nil {
				cell["border_styles"] = borderStyles
			}
			row[c] = cell
		}
		cells[r] = row
	}
	input := map[string]interface{}{
		"excel_id": token,
		"range":    rangeStr,
		"cells":    cells,
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if err := validateInputAgainstSchema(runtime, input); err != nil {
		return nil, err
	}
	return input, nil
}

// CsvPut wraps set_range_from_csv: dump a CSV blob into a sheet, only writing
// plain values. Use +cells-set for anything richer (formula / style / note).
var CsvPut = common.Shortcut{
	Service:     "sheets",
	Command:     "+csv-put",
	Description: "Paste RFC-4180 CSV into a sheet at --start-cell (plain values only, auto-expands sheet if needed).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+csv-put"), // includes the hidden --range alias (defined in the base flags table)
	PostMount: func(cmd *cobra.Command) {
		// --range is an accepted alias for --start-cell (see csvPutInput).
		// Neither is individually required; exactly one must be set. flag-defs
		// marks --start-cell required, so clear that annotation and switch to a
		// one-required group — otherwise cobra rejects `--range A1` for a
		// missing --start-cell before the handler ever runs.
		if fl := cmd.Flags().Lookup("start-cell"); fl != nil {
			delete(fl.Annotations, cobra.BashCompOneRequiredFlag)
		}
		cmd.MarkFlagsOneRequired("start-cell", "range")
		cmd.MarkFlagsMutuallyExclusive("start-cell", "range")
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := guardCSVValueIsNotFilePath(runtime); err != nil {
			return err
		}
		return validateViaInput(csvPutInput)(ctx, runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := csvPutInput(runtime, token, sheetID, sheetName)
		dr := invokeToolDryRun(token, ToolKindWrite, "set_range_from_csv", input)
		if rng, ok := csvPutWriteRangeFromInput(input); ok {
			dr = dr.Set("writes_range", rng)
		}
		return dr
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
		input, err := csvPutInput(runtime, token, sheetID, sheetName)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "set_range_from_csv", input)
		if err != nil {
			return err
		}
		if rng, ok := csvPutWriteRangeFromInput(input); ok {
			if m, isMap := out.(map[string]interface{}); isMap {
				m["writes_range"] = rng
			}
		}
		runtime.Out(out, nil)
		return nil
	},
}

// csvPutWriteRangeFromInput computes the rectangle +csv-put will actually write,
// from the built tool input (start_cell + csv). +csv-put pastes from the anchor
// and auto-expands to the CSV's own row/column count — the footprint is the
// result, not a user-set boundary. Surfacing it (e.g. "B2:D4") in dry-run and in
// the success envelope lets agents see how far a paste reaches before it
// silently overwrites neighbouring cells (use --allow-overwrite=false to block
// that). Returns ok=false when the anchor is not a single cell or the CSV has no
// parseable fields.
func csvPutWriteRangeFromInput(input map[string]interface{}) (string, bool) {
	anchor, _ := input["start_cell"].(string)
	csvText, _ := input["csv"].(string)
	if anchor == "" || csvText == "" {
		return "", false
	}
	col0, row0, ok := splitCellRef(anchor)
	if !ok {
		return "", false
	}
	r := csv.NewReader(strings.NewReader(csvText))
	r.FieldsPerRecord = -1 // tolerate ragged rows; we only need the max width
	records, err := r.ReadAll()
	if err != nil || len(records) == 0 {
		return "", false
	}
	cols := 0
	for _, rec := range records {
		if len(rec) > cols {
			cols = len(rec)
		}
	}
	if cols == 0 {
		return "", false
	}
	endCol := columnIndexToLetter(col0 + cols - 1)
	endRow := row0 + len(records) // row0 is 0-based; +len(records) is the 1-based bottom row
	return fmt.Sprintf("%s:%s%d", anchor, endCol, endRow), true
}

// guardCSVValueIsNotFilePath catches the common slip of passing a CSV file path
// to --csv without the "@" that reads it (e.g. `--csv data.csv` instead of
// `--csv @data.csv`). Because any string is a valid one-cell CSV, the mistake
// would otherwise be written silently as the literal text "data.csv". It runs
// in +csv-put's Validate, after resolveInputFlags — so an @file / stdin value is
// already its contents (a real CSV blob, never a path) and only a bare value
// reaches here unchanged. It flags the value only when it actually names an
// existing file in the cwd subtree; checking real existence (not name shape)
// means inline content that merely ends in a filename ("see config.json") is
// never misjudged. Fails open: any Stat error or a directory leaves the value
// untouched. Scoped to --csv only — no other flag is affected.
func guardCSVValueIsNotFilePath(runtime *common.RuntimeContext) error {
	raw := strings.TrimSpace(runtime.Str("csv"))
	if raw == "" {
		return nil
	}
	fio := runtime.FileIO()
	if fio == nil {
		return nil
	}
	info, err := fio.Stat(raw)
	if err != nil || info == nil || info.IsDir() {
		return nil //nolint:nilerr // fail-open: a missing/unreadable path is treated as inline content, not a forgotten @
	}
	return sheetsValidationForFlag("csv",
		"--csv value %q is an existing file, not inline CSV; to read it use --csv @%s, or pass the literal text via stdin (--csv -)",
		raw, raw,
	)
}

func csvPutInput(runtime flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	if strings.TrimSpace(runtime.Str("csv")) == "" {
		return nil, sheetsValidationForFlag("csv", "--csv is required")
	}
	if runtime.Changed("start-cell") && runtime.Changed("range") {
		return nil, common.ValidationErrorf("--start-cell and --range are mutually exclusive").WithParams(sheetsInvalidParam("start-cell", "mutually exclusive"), sheetsInvalidParam("range", "mutually exclusive"))
	}
	anchor := strings.TrimSpace(runtime.Str("start-cell"))
	// --range is accepted as an alias for --start-cell. +csv-get and +cells-set
	// locate with --range, so agents routinely carry --range over to +csv-put and
	// hit a guaranteed first-try failure. Honor it when --start-cell was not
	// explicitly set — guard on Changed, not emptiness, because --start-cell
	// defaults to "A1" and is therefore never empty. A range like "A1:H17"
	// collapses to its top-left cell; +csv-put pastes from the anchor and
	// auto-expands, so the range's lower-right bound is irrelevant.
	//
	// Standalone enforces exactly one of --start-cell / --range via cobra's
	// flag groups (see PostMount). A +batch-update sub-op never runs cobra, so
	// without explicit checks the default "A1" silently wins and the paste lands
	// at A1 instead of failing like the standalone command. Mirror the
	// standalone contract: double-set is invalid, and when --start-cell is
	// absent, --range is mandatory.
	if !runtime.Changed("start-cell") {
		rng := strings.TrimSpace(runtime.Str("range"))
		if rng == "" {
			return nil, common.ValidationErrorf("--start-cell or --range is required").WithParams(sheetsInvalidParam("start-cell", "required; specify exactly one"), sheetsInvalidParam("range", "required; specify exactly one"))
		}
		anchor = strings.TrimSpace(strings.SplitN(rng, ":", 2)[0])
		if idx := strings.Index(anchor, "!"); idx >= 0 {
			anchor = anchor[idx+1:]
		}
	}
	if anchor == "" {
		return nil, sheetsValidationForFlag("start-cell", "--start-cell is required")
	}
	if _, _, ok := splitCellRef(anchor); !ok {
		return nil, sheetsValidationForFlag("start-cell", "--start-cell %q must be a single cell ref (e.g. A1)", anchor)
	}
	input := map[string]interface{}{
		"excel_id":   token,
		"csv":        runtime.Str("csv"),
		"start_cell": anchor,
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if !runtime.Bool("allow-overwrite") {
		input["allow_overwrite"] = false
	}
	return input, nil
}

// ─── +dropdown-* (set_cell_range via data_validation) ─────────────────
//
// All three dropdown shortcuts stamp a `data_validation` block on every cell
// of the target range(s). set / update / delete differ in (a) how many
// ranges they accept and (b) whether the block is populated or null.

// DropdownSet places a single dropdown on one range.
var DropdownSet = common.Shortcut{
	Service:     "sheets",
	Command:     "+dropdown-set",
	Description: "Attach a dropdown / data-validation list to every cell in --range.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+dropdown-set"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateViaInput(dropdownSetInput)(ctx, runtime); err != nil {
			return err
		}
		warnDropdownSourceRangeHighlight(runtime)
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := dropdownSetInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "set_cell_range", input)
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
		input, err := dropdownSetInput(runtime, token, sheetID, sheetName)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "set_cell_range", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

func dropdownSetInput(runtime flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	rangeStr := strings.TrimSpace(runtime.Str("range"))
	if rangeStr == "" {
		return nil, sheetsValidationForFlag("range", "--range is required")
	}
	rows, cols, err := rangeDimensions(rangeStr)
	if err != nil {
		return nil, sheetsValidationForFlag("range", "--range %q: %v", rangeStr, err)
	}
	validation, err := buildDropdownValidation(runtime)
	if err != nil {
		return nil, err
	}
	cells := fillCellsMatrix(rows, cols, map[string]interface{}{"data_validation": validation})
	input := map[string]interface{}{
		"excel_id": token,
		"range":    rangeStr,
		"cells":    cells,
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if err := validateInputAgainstSchema(runtime, input); err != nil {
		return nil, err
	}
	return input, nil
}

// NOTE: +dropdown-update and +dropdown-delete were originally drafted here
// but moved to lark_sheet_batch_update (B7) per the spec: multi-range
// dropdown CRUD now goes through batch_update for atomicity. They'll land in
// the batch_update file alongside +cells-batch-set-style.

// ─── shared dropdown helpers ──────────────────────────────────────────

// buildDropdownValidation packs --options or --source-range plus --colors /
// --multiple / --highlight into the data_validation block expected by
// set_cell_range. Field names follow the canonical
// set_cell_range.data_validation schema:
//
//	--options       -> {type: "list",          items: <strings>}
//	--source-range  -> {type: "listFromRange", range: <A1+sheet prefix>}
//	--multiple      -> support_multiple_values  (bool)
//	--colors        -> highlight_colors         (string array, hex)
//	--highlight     -> enable_highlight         (bool, tri-state via Changed)
//
// --options and --source-range are XOR (caller must pass exactly one).
// --colors length may be shorter than the source size (options length or
// source-range cell count) — server cycles remaining slots through a
// built-in 10-color palette — but must not exceed it.
//
// --highlight is tri-state: omitted leaves enable_highlight off the body so the
// server's new default (true) applies; --highlight=true stamps an explicit true;
// --highlight=false stamps false to turn the highlight off. Using Changed() lets
// us distinguish "not passed" from "explicit false" — required because the
// server-side default flipped from false to true and a plain cobra Bool can no
// longer carry the opt-out signal.
func buildDropdownValidation(runtime flagView) (map[string]interface{}, error) {
	sourceSize, dv, err := dropdownTypeAndItems(runtime)
	if err != nil {
		return nil, err
	}
	if runtime.Str("colors") != "" {
		colors, err := requireJSONArray(runtime, "colors")
		if err != nil {
			return nil, err
		}
		if len(colors) > sourceSize {
			return nil, sheetsValidationForFlag("colors", "--colors length (%d) must not exceed dropdown source size (%d)", len(colors), sourceSize)
		}
		dv["highlight_colors"] = colors
	}
	if runtime.Bool("multiple") {
		dv["support_multiple_values"] = true
	}
	if runtime.Changed("highlight") {
		dv["enable_highlight"] = runtime.Bool("highlight")
	}
	return dv, nil
}

// dropdownTypeAndItems resolves the XOR between --options and --source-range
// and returns (sourceSize, partial dv with type+items|range set). sourceSize
// is the option count for `list` mode or the source-range cell count for
// `listFromRange` mode — used to validate --colors length.
func dropdownTypeAndItems(runtime flagView) (int, map[string]interface{}, error) {
	optsRaw := runtime.Str("options")
	sourceRange := strings.TrimSpace(runtime.Str("source-range"))
	switch {
	case optsRaw != "" && sourceRange != "":
		return 0, nil, common.ValidationErrorf("--options and --source-range are mutually exclusive; pass exactly one").WithParams(sheetsInvalidParam("options", "mutually exclusive"), sheetsInvalidParam("source-range", "mutually exclusive"))
	case optsRaw == "" && sourceRange == "":
		return 0, nil, common.ValidationErrorf("one of --options (inline list) or --source-range (listFromRange) is required").WithParams(sheetsInvalidParam("options", "required; specify exactly one"), sheetsInvalidParam("source-range", "required; specify exactly one"))
	case optsRaw != "":
		options, err := requireJSONArray(runtime, "options")
		if err != nil {
			return 0, nil, err
		}
		return len(options), map[string]interface{}{
			"type":  "list",
			"items": options,
		}, nil
	default: // sourceRange != ""
		rows, cols, err := rangeDimensions(sourceRange)
		if err != nil {
			return 0, nil, sheetsValidationForFlag("source-range", "--source-range %q: %v", sourceRange, err)
		}
		return rows * cols, map[string]interface{}{
			"type":  "listFromRange",
			"range": sourceRange,
		}, nil
	}
}

// validateDropdownSourceOrOptions runs the XOR + --colors length check at
// Validate time so +dropdown-update / +dropdown-delete can fail fast without
// reaching the body-build step. Returns the dropdown source size (options
// length for list mode, source-range cell count for listFromRange) so
// callers can size their cells matrix.
func validateDropdownSourceOrOptions(runtime flagView) (int, error) {
	sourceSize, _, err := dropdownTypeAndItems(runtime)
	if err != nil {
		return 0, err
	}
	if runtime.Str("colors") != "" {
		colors, err := requireJSONArray(runtime, "colors")
		if err != nil {
			return 0, err
		}
		if len(colors) > sourceSize {
			return 0, sheetsValidationForFlag("colors", "--colors length (%d) must not exceed dropdown source size (%d)", len(colors), sourceSize)
		}
	}
	return sourceSize, nil
}

// dropdownSourceRangeHighlightLimit is the cell-count cap above which the
// server marks the dropdown's options as invalid when highlight is on.
// Source: byted-sheet core LIST_WITH_COLOR_MAX_COUNT
// (sheet-packages/.../dataValidation/list/ListFromRangeValidation.ts:49).
// Beyond this, ListFromRangeValidation.checkOptionsValid() sets
// isOptionError=true (highlight + range > 2000 is an unsupported combo).
const dropdownSourceRangeHighlightLimit = 2000

// warnDropdownSourceRangeHighlight emits a soft stderr warning when the user
// targets a --source-range larger than dropdownSourceRangeHighlightLimit while
// highlight is on (the server-side default and the most common path).
// Inline --options is not subject to this limit (server has no inline count
// or per-item length cap; only the listFromRange + highlight combo is).
// Validate phase only — never blocks the request. Caller must already have
// confirmed the source-or-options validation passed.
func warnDropdownSourceRangeHighlight(runtime *common.RuntimeContext) {
	sourceRange := strings.TrimSpace(runtime.Str("source-range"))
	if sourceRange == "" {
		return // inline --options mode — no server-side size cap applies
	}
	// highlight is tri-state: omitted = ON (server default), --highlight=true
	// = ON, --highlight=false = OFF. Only the OFF case avoids the warning.
	if runtime.Changed("highlight") && !runtime.Bool("highlight") {
		return
	}
	rows, cols, err := rangeDimensions(sourceRange)
	if err != nil {
		return // already errored upstream; don't double-report
	}
	cellCount := rows * cols
	if cellCount <= dropdownSourceRangeHighlightLimit {
		return
	}
	fmt.Fprintf(runtime.IO().ErrOut,
		"warning: --source-range covers %d cells; server marks the dropdown as option-error when highlight is on and the source exceeds %d cells. Pass --highlight=false to suppress this.\n",
		cellCount, dropdownSourceRangeHighlightLimit)
}

// ─── range parsing helpers ────────────────────────────────────────────

// rangeDimensions parses an A1 range like "A1:C5" / "A1" / "sheet1!B2:D10"
// and returns its row / column counts. Errors on non-rectangular forms like
// "A:C" (whole-column) or "3:6" (whole-row) — those need a row/col total
// from get_sheet_structure, outside the scope of pure local parsing.
func rangeDimensions(rangeStr string) (rows, cols int, err error) {
	if idx := strings.Index(rangeStr, "!"); idx >= 0 {
		rangeStr = rangeStr[idx+1:]
	}
	rangeStr = strings.TrimSpace(rangeStr)
	if rangeStr == "" {
		return 0, 0, fmt.Errorf("empty range")
	}
	parts := strings.SplitN(rangeStr, ":", 2)
	if len(parts) == 1 {
		// single cell, e.g. "A1"
		if _, _, ok := splitCellRef(parts[0]); !ok {
			return 0, 0, fmt.Errorf("invalid cell ref %q", parts[0])
		}
		return 1, 1, nil
	}
	startCol, startRow, ok1 := splitCellRef(parts[0])
	endCol, endRow, ok2 := splitCellRef(parts[1])
	if !ok1 || !ok2 {
		return 0, 0, fmt.Errorf("unsupported range form %q (need rectangular A1:B2)", rangeStr)
	}
	if endRow < startRow || endCol < startCol {
		return 0, 0, fmt.Errorf("end %q must be at or after start %q", parts[1], parts[0])
	}
	return endRow - startRow + 1, endCol - startCol + 1, nil
}

// splitCellRef parses "A1" → (col=0, row=0, true). Returns false for any
// non-rectangular form (pure column "A", pure row "1", invalid chars).
func splitCellRef(s string) (col, row int, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}
	var colEnd int
	for i, r := range s {
		if r >= '0' && r <= '9' {
			colEnd = i
			break
		}
		colEnd = i + 1
	}
	if colEnd == 0 || colEnd == len(s) {
		return 0, 0, false
	}
	col = letterToColumnIndex(s[:colEnd])
	if col < 0 {
		return 0, 0, false
	}
	n, err := strconv.Atoi(s[colEnd:])
	if err != nil || n < 1 {
		return 0, 0, false
	}
	return col, n - 1, true
}

// letterToColumnIndex converts spreadsheet letter notation to a 0-based
// column index ("A" → 0, "Z" → 25, "AA" → 26). Returns -1 on bad input.
func letterToColumnIndex(letters string) int {
	letters = strings.ToUpper(strings.TrimSpace(letters))
	if letters == "" {
		return -1
	}
	n := 0
	for _, c := range letters {
		if c < 'A' || c > 'Z' {
			return -1
		}
		n = n*26 + int(c-'A'+1)
	}
	return n - 1
}

// fillCellsMatrix returns a rows×cols matrix where every cell is the same
// (shallow-copied) prototype map. Use for fan-out shortcuts that stamp a
// single attribute (style / data_validation) across an entire range.
func fillCellsMatrix(rows, cols int, prototype map[string]interface{}) [][]interface{} {
	cells := make([][]interface{}, rows)
	for r := range cells {
		row := make([]interface{}, cols)
		for c := range row {
			cell := make(map[string]interface{}, len(prototype))
			for k, v := range prototype {
				cell[k] = v
			}
			row[c] = cell
		}
		cells[r] = row
	}
	return cells
}

// ─── +cells-set-image (cli_only_derivative) ──────────────────────────
//
// The backing tool (set_cell_range) is in mcp-tools.json, but the CLI
// shortcut also needs a local-file upload before it can call the tool.
// That extra step doesn't fit the One-OpenAPI dispatcher, so the spec
// marks this shortcut cli_only_derivative — the CLI uploads the image
// to drive (parent_type=sheet_image) and then writes the returned
// file_token into the target cell via callTool(set_cell_range) with a
// rich_text embed-image entry.

// CellsSetImage uploads a local image to drive (parent_type=sheet_image,
// parent_node=spreadsheet token) and then writes a rich_text embed-image
// into the target single-cell range via the set_cell_range tool.
var CellsSetImage = common.Shortcut{
	Service:     "sheets",
	Command:     "+cells-set-image",
	Description: "Embed a local image into a single cell (uploads via drive, then set_cell_range with rich_text embed-image).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "drive:file:upload"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+cells-set-image"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, _, err := resolveSheetSelector(runtime); err != nil {
			return err
		}
		r := strings.TrimSpace(runtime.Str("range"))
		if r == "" {
			return sheetsValidationForFlag("range", "--range is required")
		}
		rows, cols, err := rangeDimensions(r)
		if err != nil {
			return sheetsValidationForFlag("range", "--range %q: %v", r, err)
		}
		if rows != 1 || cols != 1 {
			return sheetsValidationForFlag("range", "--range %q must be exactly one cell (got %d×%d)", r, rows, cols)
		}
		imgPath := strings.TrimSpace(runtime.Str("image"))
		if imgPath == "" {
			return sheetsValidationForFlag("image", "--image is required")
		}
		// Validate path safety here (not just at Execute) so --dry-run also
		// rejects unsafe paths instead of giving a false-positive preview.
		// SafeLocalFlagPath checks path safety only (abs/traversal/outside-cwd),
		// not existence, so legitimate relative paths still dry-run cleanly;
		// the Execute-time Stat below still reports a missing/unreadable file.
		if _, err := validate.SafeLocalFlagPath("--image", imgPath); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).
				WithParam("--image").
				WithCause(err)
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		imgPath := strings.TrimSpace(runtime.Str("image"))
		fileName := strings.TrimSpace(runtime.Str("name"))
		if fileName == "" {
			fileName = filepath.Base(imgPath)
		}
		setCellBody, _ := buildToolBody("set_cell_range", map[string]interface{}{
			"excel_id": token,
			"range":    strings.TrimSpace(runtime.Str("range")),
			"sheet_id": sheetSelectorPlaceholder(sheetID, sheetName),
			"cells": [][]interface{}{{map[string]interface{}{
				"rich_text": []map[string]interface{}{{
					"type":         "embed-image",
					"text":         "",
					"image_token":  "<file_token>",
					"image_width":  "<image_width>",
					"image_height": "<image_height>",
				}},
			}}},
		})
		return common.NewDryRunAPI().
			POST("/open-apis/drive/v1/medias/upload_all").
			Desc("upload local image to drive (parent_type=sheet_image)").
			Body(map[string]interface{}{
				"file_name":   fileName,
				"parent_type": "sheet_image",
				"parent_node": token,
				"size":        "<file_size>",
				"file":        "@" + imgPath,
			}).
			POST(toolInvokePath(token, ToolKindWrite)).
			Desc("embed file_token into the cell via set_cell_range").
			Body(setCellBody)
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
		imgPath := strings.TrimSpace(runtime.Str("image"))
		fileName := strings.TrimSpace(runtime.Str("name"))
		if fileName == "" {
			fileName = filepath.Base(imgPath)
		}
		info, err := runtime.FileIO().Stat(imgPath)
		if err != nil {
			return sheetsInputStatError("image", err)
		}
		imgFile, err := runtime.FileIO().Open(imgPath)
		if err != nil {
			return sheetsInputStatError("image", err)
		}
		imgCfg, _, err := image.DecodeConfig(imgFile)
		imgFile.Close()
		if err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "decode image dimensions: %s", err).
				WithParam("--image").
				WithCause(err)
		}
		fileToken, err := common.UploadDriveMediaAllTyped(runtime, common.DriveMediaUploadAllConfig{
			FilePath:   imgPath,
			FileName:   fileName,
			FileSize:   info.Size(),
			ParentType: "sheet_image",
			ParentNode: &token,
		})
		if err != nil {
			return err
		}

		setCellInput := map[string]interface{}{
			"excel_id": token,
			"range":    strings.TrimSpace(runtime.Str("range")),
			"cells": [][]interface{}{{map[string]interface{}{
				"rich_text": []map[string]interface{}{{
					"type":         "embed-image",
					"text":         "",
					"image_token":  fileToken,
					"image_width":  imgCfg.Width,
					"image_height": imgCfg.Height,
				}},
			}}},
		}
		sheetSelectorForToolInput(setCellInput, sheetID, sheetName)
		setCellOut, err := callTool(ctx, runtime, token, ToolKindWrite, "set_cell_range", setCellInput)
		if err != nil {
			return wrapCellsSetImageWriteError(err, fileToken)
		}
		runtime.Out(map[string]interface{}{
			"file_token":     fileToken,
			"file_name":      fileName,
			"set_cell_range": setCellOut,
		}, nil)
		return nil
	},
	Tips: []string{
		"--range must be a single cell. The uploaded image becomes a cell-internal embed; use +float-image-create for floating images.",
	},
}

func wrapCellsSetImageWriteError(err error, fileToken string) error {
	hint := fmt.Sprintf("image was uploaded as file_token=%s; retry only the cell write with that token or remove the uploaded media", fileToken)
	if p, ok := errs.ProblemOf(err); ok {
		if strings.TrimSpace(p.Hint) != "" {
			p.Hint += "\n" + hint
		} else {
			p.Hint = hint
		}
		return err
	}
	return errs.NewInternalError(errs.SubtypeSDKError, "image uploaded (file_token=%s) but cell write failed: %s", fileToken, err).
		WithHint(hint).
		WithCause(err)
}
