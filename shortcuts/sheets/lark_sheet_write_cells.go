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
	"unicode"

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
// across a larger area (formula refs auto-shift). The plural form --writes
// ([{sheet_name, range, cells}, …]) fans scattered regions — cross-sheet
// allowed — into ONE atomic batch_update: eval traces show "fix all broken
// formulas across ranges/sheets" as the dominant homogeneous scenario still
// hand-assembled as +batch-update operations arrays.
var CellsSet = common.Shortcut{
	Service:     "sheets",
	Command:     "+cells-set",
	Description: "Write values / formulas / styles / comments / data validation / embed-image to a cell range.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+cells-set"),
	Tips: []string{
		`Example: lark-cli sheets +cells-set --url <URL> --sheet-name Sheet1 --range A1:B1 --cells '[[{"value":"名称"},{"formula":"=SUM(B2:B9)"}]]'`,
		`--cells is always a 2D array (rows × cells), even for one cell: [[{"value":…}]].`,
		`Scattered regions (e.g. fixing formulas across ranges/sheets): --writes '[{"sheet_name":…,"range":…,"cells":[[…]]}, …]' — one batch request (fail-fast, no rollback), sheet selector inside each item.`,
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Changed("writes") {
			token, err := resolveSpreadsheetToken(runtime)
			if err != nil {
				return err
			}
			_, err = cellsSetWritesOps(runtime, token)
			return err
		}
		return validateViaInput(cellsSetInput)(ctx, runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		if runtime.Changed("writes") {
			ops, _ := cellsSetWritesOps(runtime, token)
			return invokeToolDryRun(token, ToolKindWrite, "batch_update", map[string]interface{}{
				"excel_id":   token,
				"operations": ops,
			})
		}
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := cellsSetInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "set_cell_range", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		if runtime.Changed("writes") {
			ops, err := cellsSetWritesOps(runtime, token)
			if err != nil {
				return err
			}
			out, err := callTool(ctx, runtime, token, ToolKindWrite, "batch_update", map[string]interface{}{
				"excel_id":   token,
				"operations": ops,
			})
			if err != nil {
				return err
			}
			runtime.Out(out, nil)
			return nil
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

// cellsSetWritesOps parses --writes ([{sheet_name|sheet_id, range, cells}, …])
// and expands it into set_cell_range operations for ONE atomic batch_update.
// Single source of truth per item: the sheet selector LIVES IN THE ITEM (same
// convention as +batch-update sub-ops and +styles-put items — no top-level
// fallback, no precedence table to remember). Every item runs through the
// exact standalone pipeline (key vocabulary, style acceptance layer, matrix
// precheck, schema validation) via a per-item flag view, and item errors are
// aggregated so one retry fixes them all. The payload rewrites land one step
// earlier still, in normalizeWritesFlagValue, since the array is schema-checked
// before it gets here.
func cellsSetWritesOps(runtime *common.RuntimeContext, token string) ([]interface{}, error) {
	for _, conflicting := range []string{"range", "cells", "copy-to-range"} {
		if runtime.Changed(conflicting) {
			return nil, sheetsValidationForFlag("writes", "--writes and --%s are mutually exclusive: single region → --range + --cells; multiple regions → --writes alone", conflicting)
		}
	}
	if strings.TrimSpace(runtime.Str("sheet-name")) != "" || strings.TrimSpace(runtime.Str("sheet-id")) != "" {
		return nil, sheetsValidationForFlag("writes", "--writes does not accept a top-level sheet selector — put sheet_name (or sheet_id) inside each writes item, same as +batch-update sub-ops")
	}
	raw, err := requireJSONArray(runtime, "writes")
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, sheetsValidationForFlag("writes", "--writes must be a non-empty JSON array of {sheet_name, range, cells} items")
	}
	if len(raw) > maxBatchOperations {
		return nil, sheetsValidationForFlag("writes", "--writes accepts at most %d items; got %d — merge adjacent regions or split into several calls", maxBatchOperations, len(raw))
	}
	topLevelOverwrite := runtime.Bool("allow-overwrite")
	ops := make([]interface{}, 0, len(raw))
	var probs []error
	var totalCells int64
	for i, v := range raw {
		item, ok := v.(map[string]interface{})
		if !ok {
			probs = append(probs, common.ValidationErrorf("--writes[%d] must be an object like {\"sheet_name\":…,\"range\":…,\"cells\":[[…]]}", i))
			continue
		}
		if err := normalizeSubOpInputKeys("+cells-set", item); err != nil {
			probs = append(probs, common.ValidationErrorf("--writes[%d]: %v", i, err))
			continue
		}
		if runtime.Changed("allow-overwrite") {
			if _, has := item["allow_overwrite"]; !has {
				item["allow_overwrite"] = topLevelOverwrite
			}
		}
		fv := newMapFlagViewForCommand("+cells-set", item)
		// Fill the selector from a "Sheet1!A1" range before it is read below.
		fv.normalizeRangeSheetPrefix()
		sheetID := strings.TrimSpace(fv.Str("sheet-id"))
		sheetName := strings.TrimSpace(fv.Str("sheet-name"))
		input, err := cellsSetInput(fv, token, sheetID, sheetName)
		if err != nil {
			// Prefix with the item index WITHOUT flattening: cellsSetInput's
			// errors carry the domain's prescriptions in Hint (requireSheetSelector's
			// "+workbook-info" pointer, for one) and "%v" would render only the
			// message, silently costing exactly the guidance this path exists to
			// deliver. joinWritesValidationErrors re-reads both fields.
			probs = append(probs, prefixValidationIssue(fmt.Sprintf("--writes[%d]", i), err))
			continue
		}
		if cells, ok := input["cells"].([]interface{}); ok {
			for _, row := range cells {
				if r, ok := row.([]interface{}); ok {
					totalCells += int64(len(r))
				}
			}
		}
		if err := checkBatchStampBudget("writes", totalCells); err != nil {
			return nil, err
		}
		ops = append(ops, map[string]interface{}{
			"tool_name": "set_cell_range",
			"input":     input,
		})
	}
	if err := joinWritesValidationErrors(probs); err != nil {
		return nil, err
	}
	return ops, nil
}

// joinWritesValidationErrors mirrors joinStyleValidationErrors for --writes:
// every item's first error in one message, so the whole payload is fixed in
// a single retry.
func joinWritesValidationErrors(probs []error) error {
	switch len(probs) {
	case 0:
		return nil
	case 1:
		// Re-attribute to the outer flag even for a single issue: the inner
		// error is scoped to a nested path and carries no Param, so an agent
		// would have to parse prose to learn which flag to fix. Message text
		// is preserved; only the typed attribution is added — and the inner
		// hint rides along, since a lone issue has the outer Hint slot free.
		msg, hint := aggregatedIssueParts(probs[0])
		verr := sheetsValidationForFlag("writes", "%s", msg).WithCause(probs[0])
		if hint != "" {
			verr = verr.WithHint("%s", hint)
		}
		return verr
	}
	const maxShown = 8
	msgs := make([]string, 0, len(probs))
	for _, e := range probs {
		msgs = append(msgs, aggregatedIssueText(e))
	}
	suffix := ""
	if len(msgs) > maxShown {
		suffix = fmt.Sprintf(" (+%d more)", len(msgs)-maxShown)
		msgs = msgs[:maxShown]
	}
	return sheetsValidationForFlag("writes", "--writes has %d issues: %s%s", len(probs), strings.Join(msgs, " | "), suffix).
		WithCause(probs[0])
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
	if err := normalizeTypedCellsStyleAliases(cells, "--cells"); err != nil {
		return nil, err
	}
	rangeStr := expandAnchorRange(strings.TrimSpace(runtime.Str("range")), cells)
	if err := checkCellsMatchRange(cells, rangeStr); err != nil {
		return nil, err
	}
	input := map[string]interface{}{
		"excel_id": token,
		"range":    rangeStr,
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
// font-family, font-size, font-style, font-weight, font-line,
// horizontal-alignment, vertical-alignment, word-wrap, number-format) plus
// --border-styles for the only field that still needs a nested object. At
// least one flag must be set.
var CellsSetStyle = common.Shortcut{
	Service:     "sheets",
	Command:     "+cells-set-style",
	Description: "Apply style flags to every cell in a range (values / formulas untouched).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+cells-set-style"),
	Tips: []string{
		`Example: lark-cli sheets +cells-set-style --url <URL> --sheet-name Sheet1 --range A1:D1 --font-weight bold --background-color "#F0F0F0" --horizontal-alignment center`,
		`Borders take JSON: --border-styles '{"top":{"style":"solid","weight":"thin","color":"#000000"}}' (sides: top/bottom/left/right).`,
	},
	Validate: validateViaInput(cellsSetStyleInput),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := cellsSetStyleInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "set_cell_range", input)
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
	if err := checkStampMatrixBudget("range", rangeStr, rows, cols); err != nil {
		return nil, err
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

// CsvPut wraps set_range_from_csv: dump a CSV blob into a sheet. A cell whose
// text starts with = is evaluated as a formula; use +cells-set for styles / notes / images.
var CsvPut = common.Shortcut{
	Service:     "sheets",
	Command:     "+csv-put",
	Description: "Paste RFC-4180 CSV into a sheet at --start-cell (values or formulas: a leading = is evaluated as a formula; no styles / comments; auto-expands sheet if needed).",
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
		token, err := resolveSpreadsheetTokenExec(runtime)
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
// would otherwise be written silently as the literal text "data.csv" — a wrong
// value in the sheet plus a success exit code, which costs more than a
// rejection because nothing surfaces it. It runs in +csv-put's Validate, after
// resolveInputFlags — so an @file / stdin value is already its contents (a real
// CSV blob, never a path) and only a bare value reaches here unchanged.
//
// Two tiers, because the fix differs:
//
//   - the value names an existing file in the cwd subtree → a forgotten "@";
//   - the file does not exist but the value is unmistakably path-shaped →
//     usually an absolute path (which "@" rejects) that the caller retried
//     without the "@", or a stale relative path from another working
//     directory. Same silent-write outcome, different prescription: stdin.
//
// Everything else passes through. Existence alone can't carry tier two, so
// shape does — but only the narrow shape defined by csvValueLooksLikePath,
// which is what keeps prose that merely mentions a filename out of it.
// Fails open: any Stat error or a directory falls through to the shape check.
// Scoped to --csv only — no other flag is affected.
//
// A value that arrived via @file / stdin is skipped entirely
// (InputResolvedFromSource): its content was already read from the right
// place and may legitimately look like anything, including a path. That
// also makes stdin the guard-proof way to write such text verbatim.
func guardCSVValueIsNotFilePath(runtime *common.RuntimeContext) error {
	if runtime.InputResolvedFromSource("csv") {
		return nil
	}
	raw := strings.TrimSpace(runtime.Str("csv"))
	if raw == "" {
		return nil
	}
	// Hints below use <path> placeholders instead of echoing the raw value
	// into command-shaped text: the value is untrusted, and a hint like
	// "--csv - < $(id).csv" hands an agent a copy-pasteable command that a
	// POSIX shell would expand.
	if fio := runtime.FileIO(); fio != nil {
		info, err := fio.Stat(raw)
		if err == nil && info != nil && !info.IsDir() {
			return sheetsValidationForFlag("csv",
				"--csv value %q is an existing file, not inline CSV; to read it, pass the same path with an @ prefix (--csv @<path>), or pipe the literal text via stdin (--csv -)",
				raw,
			)
		}
	}
	if !csvValueLooksLikePath(raw) {
		return nil
	}
	return sheetsValidationForFlag("csv",
		"--csv value %q looks like a file path, not inline CSV, and no such file exists under the current directory",
		raw,
	).WithHint(
		"to read a file: --csv @<path> (relative to the current directory; @ rejects absolute paths — pipe such a file in via stdin instead: --csv - < <path>). To write this text into the cell verbatim, pass it on stdin the same way (--csv -); values arriving via stdin or @file skip this check",
	)
}

// csvValueLooksLikePath reports whether a --csv value is unmistakably a path
// rather than CSV content. Deliberately narrow: the guard rejects on it, so a
// false positive blocks a legitimate write, and an earlier name-shape
// heuristic was replaced by an existence check precisely because it misjudged
// prose ("改完记得更新config.json"). Three conditions, all required:
//
//	no comma / newline / whitespace  — real CSV has separators, prose has spaces
//	pure ASCII                       — CJK text is content, never a path here
//	a .csv/.tsv extension, or an explicit ./ ../ / ~/ prefix
//
// The extension-or-prefix rule is what keeps ordinary single-cell values safe:
// "N/A" contains a slash but neither, and "README.md" is a filename but not a
// CSV one. A caller who genuinely means such a literal still has stdin.
func csvValueLooksLikePath(s string) bool {
	if strings.ContainsAny(s, ", \t\r\n\"") {
		return false
	}
	for _, r := range s {
		if r > unicode.MaxASCII {
			return false
		}
	}
	lower := strings.ToLower(s)
	if strings.HasSuffix(lower, ".csv") || strings.HasSuffix(lower, ".tsv") {
		return true
	}
	return strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") ||
		strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~/")
}

func csvPutInput(runtime flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	if !runtime.InputResolvedFromSource("csv") {
		rawCSV := strings.TrimSpace(runtime.Str("csv"))
		if rawCSV != "" && csvValueLooksLikePath(rawCSV) {
			return nil, sheetsValidationForFlag("csv", "--csv value %q looks like a file path; use @<path> or stdin", rawCSV)
		}
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
		token, err := resolveSpreadsheetTokenExec(runtime)
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
	if err := checkStampMatrixBudget("range", rangeStr, rows, cols); err != nil {
		return nil, err
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

// checkCellsMatchRange rejects, before any network call, the cells-vs-range
// mismatches the server would otherwise fail mid-batch ("cells row count (N)
// does not match range row count (M)" — a recurring server-side error cluster
// in eval traces, and the failure leaves earlier batch sub-ops applied).
// Single-cell ranges are checked too: the server enforces the same strict
// match on a bare "A1" (07-21 rerun, 12 rows against range row count 1).
// Callers reach this with the anchor already resolved by expandAnchorRange,
// so what still fails here is a range that states an extent and disagrees
// with the payload. An unparsable range is the range validator's job, not
// ours.
//
// The message states BOTH axes and hands back the range that fits the payload.
// This is the largest single --cells failure class in the corpus (132
// rejections across 93 case-runs), driven by off-by-one on the inclusive end
// (A1:C10 is 10 rows, not 9) and by hand-counted ranges against real data;
// reporting one axis at a time cost a second round trip whenever both were
// off, and 16 of the 132 retried straight into the same error.
//
// The computed range is NOT applied automatically: growing it would overwrite
// rows the caller never mentioned and shrinking it would silently drop data.
func checkCellsMatchRange(cells []interface{}, rangeStr string) error {
	if len(cells) == 0 {
		return sheetsValidationForFlag("cells",
			"--cells is empty; to clear values use +cells-clear --scope content (needs --yes), or pass a non-empty 2D array")
	}
	target, err := parseCellRange(rangeStr)
	if err != nil {
		return nil //nolint:nilerr // an unparsable range is reported by the range validation path with proper context
	}
	payloadRows, payloadCols, ok := cellsExtent(cells)
	if !ok {
		// A payload with no single extent has nothing to compare against the
		// range, so it is its own bug and gets its own message — reporting it
		// as a range mismatch would send the caller off to edit --range.
		return raggedCellsError(cells)
	}
	if payloadRows == target.rows && payloadCols == target.cols {
		return nil
	}
	return sheetsValidationForFlag("cells",
		"--cells is %d rows × %d columns but --range %q spans %d rows × %d columns; either write this payload to --range %q (same top-left, sized to the cells passed) or resize --cells to %d rows × %d columns — an A1 range covers both ends, so %q spans %d rows",
		payloadRows, payloadCols, rangeStr, target.rows, target.cols,
		target.sized(payloadRows, payloadCols), target.rows, target.cols, rangeStr, target.rows)
}

// cellsExtent measures a --cells payload: its row count and the width every
// row shares. ok is false when the payload has no single extent — a row that
// isn't an array, rows of differing widths, or nothing at all — which is the
// one authority both the anchor expansion and the dimension check consult,
// so neither can decide the payload is rectangular while the other doesn't.
// raggedCellsError names the offender once ok is false.
func cellsExtent(cells []interface{}) (rows, cols int, ok bool) {
	if len(cells) == 0 {
		return 0, 0, false
	}
	width := -1
	for _, rowRaw := range cells {
		row, isArray := rowRaw.([]interface{})
		if !isArray {
			return 0, 0, false
		}
		if width < 0 {
			width = len(row)
			continue
		}
		if len(row) != width {
			return 0, 0, false
		}
	}
	if width <= 0 {
		return 0, 0, false
	}
	return len(cells), width, true
}

// raggedCellsError describes the first row that breaks the rectangle. Only
// reached after cellsExtent has already said the payload has no extent, so
// the walk is the error message's, not the decision's.
func raggedCellsError(cells []interface{}) error {
	width := -1
	for r, rowRaw := range cells {
		row, isArray := rowRaw.([]interface{})
		if !isArray {
			return sheetsValidationForFlag("cells",
				"--cells[%d] must be an array (one row of cells) — --cells is always a 2D array, a single cell is [[{…}]]", r)
		}
		if width < 0 {
			width = len(row)
			continue
		}
		if len(row) != width {
			return sheetsValidationForFlag("cells",
				"--cells[%d] has %d columns but --cells[0] has %d; every row must be the same width (pad short rows with {} to keep those cells unchanged)",
				r, len(row), width)
		}
	}
	// Every row is an array of the same width, so the only extent cellsExtent
	// can have refused is zero: rows carrying no cells at all.
	return sheetsValidationForFlag("cells",
		"--cells has %d rows but every row is empty; each row needs one entry per column, e.g. [[{\"value\":…}]]", len(cells))
}

// expandAnchorRange gives a bare single-cell --range the anchor semantics
// every spreadsheet library these callers arrive from already has (gspread's
// update("A1", values), openpyxl's ws["A1"] = …): a top-left alone plus a
// multi-cell payload means "start writing here", so the extent of the block
// is computed rather than demanded from the caller.
//
// The range is resolved locally and shipped in full, so the server still sees
// the strict match it enforces (07-21 rerun: it rejects anchors of its own).
// +csv-put already infers --start-cell's bottom-right from the CSV's own
// counts; +cells-set was the odd one out.
//
// Only a bare "A1" expands: an explicit "A1:A1" states a 1×1 block, and a
// payload disagreeing with a stated extent is a real mismatch. A ragged or
// non-array payload has no extent to compute and falls through to
// checkCellsMatchRange's prescription.
//
// A qualified anchor ("Sheet1!A1") does not expand either. It only reaches
// here when the caller also passed --sheet-id / --sheet-name, since all three
// entry points consume the prefix into the selector when none was given — so
// the prefix is one that disagrees with the selector, and sizing it would ship
// a range naming one sheet next to a sheet_name naming another. Left alone it
// keeps failing checkCellsMatchRange locally, which is what it did before
// anchors were inferred at all.
func expandAnchorRange(rangeStr string, cells []interface{}) string {
	anchor, err := parseCellRange(rangeStr)
	if err != nil || !anchor.anchored || anchor.sheetQualifier != "" {
		return rangeStr
	}
	rows, cols, ok := cellsExtent(cells)
	if !ok || (rows == 1 && cols == 1) {
		return rangeStr
	}
	return anchor.sized(rows, cols)
}

// cellRange is a rectangular A1 range taken apart once, so the things callers
// keep re-deriving from the string — the sheet part it carries, its top-left,
// the extent it states — are read off fields instead of re-parsed. anchored
// marks a bare "A1": a top-left that states no extent (rows/cols are still 1,
// since that is the block it covers on its own).
type cellRange struct {
	// sheetQualifier is the sheet part exactly as written, separator included
	// ("Sheet1!", "'My Sheet'！"), or "" when the range names no sheet. Ranges
	// rendered from it are shipped to the server and printed for the caller to
	// paste back, so the spelling has to survive verbatim — a name unquoted and
	// then re-quoted would not. Callers that want the name itself unquoted go
	// through splitRangeSheetPrefix.
	sheetQualifier string

	start      string // the top-left as written, e.g. "B2"
	col, row   int    // 0-based top-left
	rows, cols int    // extent, in cells
	anchored   bool
}

// sized renders the range this one's top-left fills with a rows×cols block.
func (r cellRange) sized(rows, cols int) string {
	return fmt.Sprintf("%s%s:%s%d", r.sheetQualifier, r.start, columnIndexToLetter(r.col+cols-1), r.row+rows)
}

// parseCellRange splits "sheet1!B2:D10" into the sheet part it carries, its
// top-left and its extent. Errors on non-rectangular forms like "A:C"
// (whole-column) or "3:6" (whole-row) — those need a row/col total from
// get_sheet_structure, outside the scope of pure local parsing. The error
// wording is load-bearing: +styles-put surfaces it verbatim
// ("cell_styles range %q: %v").
//
// The sheet part is cut by scanSheetQualifier, the same grammar the selector
// rewrite uses, so both agree with the front-end ref lexer on what counts as a
// separator. Splitting on the first "!" instead would miss the full-width
// separator entirely and would cut a quoted name in half at its own "!".
func parseCellRange(s string) (cellRange, error) {
	out := cellRange{}
	// Trim before cutting the qualifier, not after: otherwise " sheet1!B2"
	// carries the leading space into it and into every range rendered from it.
	body := strings.TrimSpace(s)
	if _, end, ok := scanSheetQualifier(body); ok {
		out.sheetQualifier = body[:end]
		body = strings.TrimSpace(body[end:])
	}
	if body == "" {
		return out, fmt.Errorf("empty range") //nolint:forbidigo // intermediate error; callers wrap it into a typed --range/--source-range validation error
	}
	parts := strings.SplitN(body, ":", 2)
	out.start = strings.TrimSpace(parts[0])
	startCol, startRow, ok := splitCellRef(out.start)
	out.col, out.row = startCol, startRow
	if len(parts) == 1 {
		// single cell, e.g. "A1"
		if !ok {
			return cellRange{}, fmt.Errorf("invalid cell ref %q", parts[0]) //nolint:forbidigo // intermediate error; callers wrap it into a typed --range/--source-range validation error
		}
		out.rows, out.cols, out.anchored = 1, 1, true
		return out, nil
	}
	endCol, endRow, okEnd := splitCellRef(parts[1])
	if !ok || !okEnd {
		return cellRange{}, fmt.Errorf("unsupported range form %q (need rectangular A1:B2)", body) //nolint:forbidigo // intermediate error; callers wrap it into a typed --range/--source-range validation error
	}
	if endRow < startRow || endCol < startCol {
		return cellRange{}, fmt.Errorf("end %q must be at or after start %q", parts[1], parts[0]) //nolint:forbidigo // intermediate error; callers wrap it into a typed --range/--source-range validation error
	}
	out.rows, out.cols = endRow-startRow+1, endCol-startCol+1
	return out, nil
}

func rangeDimensions(rangeStr string) (rows, cols int, err error) {
	r, err := parseCellRange(rangeStr)
	if err != nil {
		return 0, 0, err
	}
	return r.rows, r.cols, nil
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

// maxStampMatrixCells bounds how many per-cell maps a fan-out / stamp shortcut
// will materialize from a single A1 range. The backing tools take an explicit
// cells matrix, so the CLI must expand a range like "A1:Z100000" into rows×cols
// maps before sending it — an unbounded blow-up (2.6M cells ≈ 900MB heap, then
// doubled again by json.Marshal) that OOMs the process before the request even
// leaves. The 200000 ceiling is the selected fan-out guardrail; the separately
// documented --max-cells flag defaults to 50000.
const maxStampMatrixCells = 200000

// checkStampMatrixBudget rejects a range whose materialized cell count would
// exceed maxStampMatrixCells, before fillCellsMatrix allocates it. rows*cols is
// computed in int64 to stay safe against overflow on pathological ranges.
func checkStampMatrixBudget(flagName, rangeStr string, rows, cols int) error {
	if total := int64(rows) * int64(cols); total > maxStampMatrixCells {
		return sheetsValidationForFlag(flagName,
			"range %q covers %d cells, over the %d-cell safety cap; narrow the range or split it across smaller ranges",
			rangeStr, total, maxStampMatrixCells)
	}
	return nil
}

// fillCellsMatrix returns a rows×cols matrix where every cell is the same
// (shallow-copied) prototype map. Use for fan-out shortcuts that stamp a
// single attribute (style / data_validation) across an entire range.
// Callers MUST gate the dimensions through checkStampMatrixBudget first.
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
			Desc("upload local image to drive (parent_type=" + sheetMediaParentType(token) + ")").
			Body(map[string]interface{}{
				"file_name":   fileName,
				"parent_type": sheetMediaParentType(token),
				"parent_node": token,
				"size":        "<file_size>",
				"file":        "@" + imgPath,
			}).
			POST(toolInvokePath(token, ToolKindWrite)).
			Desc("embed file_token into the cell via set_cell_range").
			Body(setCellBody)
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
		fileToken, err := uploadSheetImage(runtime, token, imgPath, fileName, info.Size())
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
