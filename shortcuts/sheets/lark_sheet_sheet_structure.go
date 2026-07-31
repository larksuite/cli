// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// ─── lark_sheet_sheet_structure ───────────────────────────────────────
//
// Wraps get_sheet_structure (read) and modify_sheet_structure (write,
// operation-enum dispatch). All region/position arguments use A1-style
// strings (1-based row numbers like "3:7" / "5", or column letters like
// "C:F" / "C"); dim-* / resize never expose 0-based int indices on the CLI
// surface, so there is no inclusive/exclusive ambiguity across commands.
// parseA1Range / parseA1Position handle parsing into the 0-based ints that
// dim-move's native v3 endpoint expects.
//
// +rows-resize / +cols-resize live in lark_sheet_range_operations (different
// tool); they are only grouped under "工作表" for discoverability.

// SheetInfo wraps get_sheet_structure: row heights, column widths, hidden
// rows/cols, merged cells, row/column groups, and freeze counts for one
// sub-sheet (optionally limited to a range).
var SheetInfo = common.Shortcut{
	Service:     "sheets",
	Command:     "+sheet-info",
	Description: "Get a sub-sheet's layout metadata: row heights, column widths, hidden rows/cols, merges, groups, freeze.",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+sheet-info"),
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
		return invokeToolDryRun(token, ToolKindRead, "get_sheet_structure", sheetInfoInput(runtime, token, sheetID, sheetName))
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
		out, err := callTool(ctx, runtime, token, ToolKindRead, "get_sheet_structure", sheetInfoInput(runtime, token, sheetID, sheetName))
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
	Tips: []string{
		"Frozen rows / columns are top-level fields and are returned regardless of --include.",
	},
}

func sheetInfoInput(runtime *common.RuntimeContext, token, sheetID, sheetName string) map[string]interface{} {
	input := map[string]interface{}{"excel_id": token}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if r := strings.TrimSpace(runtime.Str("range")); r != "" {
		input["range"] = r
	}
	if include := runtime.StrSlice("include"); len(include) > 0 {
		if t := infoTypeFromInclude(include); t != "" {
			input["info_type"] = t
		}
	}
	return input
}

// infoTypeFromInclude maps the fine-grained --include vocabulary to the
// tool's coarse info_type enum. When --include spans multiple categories
// (or asks for "frozen", which is always returned), we fall back to "all".
func infoTypeFromInclude(include []string) string {
	groups := map[string]string{
		"row_heights": "row_heights_column_widths",
		"col_widths":  "row_heights_column_widths",
		"hidden_rows": "hidden_infos",
		"hidden_cols": "hidden_infos",
		"groups":      "group_infos",
		"merges":      "merged_cells_infos",
		"frozen":      "", // any info_type returns frozen; falling back to all is fine
	}
	seen := map[string]struct{}{}
	for _, v := range include {
		g, ok := groups[v]
		if !ok || g == "" {
			return "all"
		}
		seen[g] = struct{}{}
	}
	if len(seen) != 1 {
		return "all"
	}
	for g := range seen {
		return g
	}
	return "all"
}

// ─── +dim-* (modify_sheet_structure) ──────────────────────────────────

// DimInsert inserts blank rows / columns and optionally inherits style from
// the adjacent dimension.
var DimInsert = common.Shortcut{
	Service:     "sheets",
	Command:     "+dim-insert",
	Description: "Insert blank rows or columns at a given position.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+dim-insert"),
	Tips: []string{
		"Example: lark-cli sheets +dim-insert --url <URL> --sheet-name Sheet1 --position 3 --count 2 --inherit-style before",
		"Rows vs columns comes from --position alone: a row number (3) inserts rows, a column letter (C) inserts columns — there is no --dimension flag.",
	},
	Validate: validateViaInput(dimInsertInput),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := dimInsertInput(runtime, token, sheetID, sheetName)
		dr := invokeToolDryRun(token, ToolKindWrite, "modify_sheet_structure", input)
		if dimInsertNeedsBeforeStyleWarning(runtime) {
			dr.Set("warning_message", dimInsertBeforeStyleWarning)
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
		input, err := dimInsertInput(runtime, token, sheetID, sheetName)
		if err != nil {
			return err
		}
		if dimInsertNeedsBeforeStyleWarning(runtime) {
			fmt.Fprintln(runtime.IO().ErrOut, dimInsertBeforeStyleWarning)
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "modify_sheet_structure", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

// dimInsertBeforeStyleWarning fires only when the preceding-side style cannot
// be copied: --inherit-style before at the first row/column, where no
// preceding row/column exists. The row/column is still inserted before
// --position, just without style inheritance. (--inherit-style after has no
// such edge — a plain before-insert always has a following row/column.)
const dimInsertBeforeStyleWarning = "warning: --inherit-style before cannot copy the preceding row/column's style at the first row/column (no preceding row/column exists); inserting before --position without style inheritance. Copy styles separately if needed."

func dimInsertNeedsBeforeStyleWarning(runtime flagView) bool {
	if !runtime.Changed("inherit-style") || runtime.Str("inherit-style") != "before" {
		return false
	}
	// Only the first row/column (idx 0) has no preceding row/column.
	_, idx, err := parseA1Position(strings.TrimSpace(runtime.Str("position")))
	return err == nil && idx == 0
}

// dimInsertInput passes --position (1-based row number "3" or column letter
// "C") to the tool's `position` field; --count maps to `count`.
//
// +dim-insert's public contract is always "insert before --position";
// --inherit-style only selects which side's style the new row/column copies,
// never the insertion side. The sheet-ai tool always copies the *anchor*
// column's style (the target passed as position), regardless of side — so
// --inherit-style before is emulated by anchoring one unit earlier. See the
// switch below.
func dimInsertInput(runtime flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	if !runtime.Changed("position") {
		return nil, sheetsValidationForFlag("position", "--position is required")
	}
	if !runtime.Changed("count") {
		return nil, sheetsValidationForFlag("count", "--count is required")
	}
	position := strings.TrimSpace(runtime.Str("position"))
	if _, _, err := parseA1Position(position); err != nil {
		return nil, sheetsValidationForFlag("position", "invalid --position %q: %v", position, err)
	}
	count := runtime.Int("count")
	if count <= 0 {
		return nil, sheetsValidationForFlag("count", "--count must be > 0 (got %d)", count)
	}
	input := map[string]interface{}{
		"excel_id":  token,
		"operation": "insert",
		"position":  position,
		"count":     count,
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	// --inherit-style selects which side's style the blank row/column copies;
	// the insertion always lands *before* --position. Empirically the addCol
	// backend copies the *anchor* column's style (the target passed as
	// position), regardless of side — side only decides whether the blank lands
	// before or after that anchor (verified live, see
	// TestDimInsertInheritStyleSideMapping):
	//   after  → side=before at P: the blank lands at P and anchor P becomes the
	//            *following* neighbour, so the blank copies it. Position unchanged.
	//   before → side=after at P-1: the blank still lands at P (insert-after-(P-1)
	//            == insert-before-P) and anchor P-1 becomes the *preceding*
	//            neighbour, so the blank copies it.
	switch runtime.Str("inherit-style") {
	case "after":
		input["side"] = "before"
	case "before":
		if prev, ok := a1PositionBefore(position); ok {
			input["side"] = "after"
			input["position"] = prev
		}
		// First row/column: no preceding row/column exists, so fall back to a
		// plain before-insert (dimInsertNeedsBeforeStyleWarning surfaces this).
	}
	return input, nil
}

// DimDelete deletes rows / columns — irreversible, high-risk-write.
var DimDelete = common.Shortcut{
	Service:     "sheets",
	Command:     "+dim-delete",
	Description: "Delete rows or columns (irreversible).",
	Risk:        "high-risk-write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+dim-delete"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Changed("ranges") {
			if runtime.Changed("range") {
				return sheetsValidationForFlag("ranges", "--range and --ranges are mutually exclusive; put every range into --ranges")
			}
			token, err := resolveSpreadsheetToken(runtime)
			if err != nil {
				return err
			}
			sheetID, sheetName, err := resolveSheetSelector(runtime)
			if err != nil {
				return err
			}
			_, err = dimDeleteRangesOps(runtime, token, sheetID, sheetName)
			return err
		}
		return validateDimRangeOp("delete")(ctx, runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		if runtime.Changed("ranges") {
			ops, _ := dimDeleteRangesOps(runtime, token, sheetID, sheetName)
			return invokeToolDryRun(token, ToolKindWrite, "batch_update", map[string]interface{}{
				"excel_id":   token,
				"operations": ops,
			})
		}
		input, _ := dimRangeOpInput(runtime, token, sheetID, sheetName, "delete")
		return invokeToolDryRun(token, ToolKindWrite, "modify_sheet_structure", input)
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
		if runtime.Changed("ranges") {
			ops, err := dimDeleteRangesOps(runtime, token, sheetID, sheetName)
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
		input, err := dimRangeOpInput(runtime, token, sheetID, sheetName, "delete")
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "modify_sheet_structure", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
	Tips: []string{
		"Row/column deletion is irreversible. Always preview with --dry-run first.",
		`Scattered ranges: --ranges '["5:5","8:8","11:13"]' deletes them in one batch request (fail-fast, no rollback) — the CLI orders positions descending, so indexes never shift under you.`,
	},
}

// dimDeleteRangesOps parses --ranges into one atomic batch of
// modify_sheet_structure delete ops, ordered DESCENDING by start position:
// deleting an earlier row shifts every later index up, so ascending
// execution deletes the wrong rows — the recurring failure of hand-built
// dim-delete batches in eval traces. Same-dimension and non-overlap are
// enforced up front.
func dimDeleteRangesOps(runtime flagView, token, sheetID, sheetName string) ([]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	raw, err := requireJSONArray(runtime, "ranges")
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, sheetsValidationForFlag("ranges", "--ranges must be a non-empty JSON array")
	}
	if len(raw) > maxBatchRanges {
		return nil, sheetsValidationForFlag("ranges", "--ranges accepts at most %d entries; got %d", maxBatchRanges, len(raw))
	}
	type span struct {
		raw        string
		start, end int
	}
	spans := make([]span, 0, len(raw))
	dimension := ""
	for i, v := range raw {
		s, ok := v.(string)
		if !ok {
			return nil, sheetsValidationForFlag("ranges", "--ranges[%d] must be a string", i)
		}
		dim, start, end, err := parseA1Range(s)
		if err != nil {
			return nil, sheetsValidationForFlag("ranges", "--ranges[%d] %q: %v", i, s, err)
		}
		if dimension == "" {
			dimension = dim
		} else if dim != dimension {
			return nil, sheetsValidationForFlag("ranges", "--ranges[%d] %q is a %s range but earlier entries are %s ranges; one call deletes rows OR columns, not both", i, s, dim, dimension)
		}
		spans = append(spans, span{raw: strings.TrimSpace(s), start: start, end: end})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start > spans[j].start })
	for i := 1; i < len(spans); i++ {
		// Descending order: spans[i-1] starts at or after spans[i]. Overlap
		// (or duplicate) makes the later delete hit already-shifted positions.
		if spans[i].end >= spans[i-1].start {
			return nil, sheetsValidationForFlag("ranges", "--ranges entries %q and %q overlap; merge them into one range", spans[i].raw, spans[i-1].raw)
		}
	}
	ops := make([]interface{}, 0, len(spans))
	for _, sp := range spans {
		input := map[string]interface{}{
			"excel_id":  token,
			"operation": "delete",
			"range":     sp.raw,
		}
		sheetSelectorForToolInput(input, sheetID, sheetName)
		ops = append(ops, map[string]interface{}{
			"tool_name": "modify_sheet_structure",
			"input":     input,
		})
	}
	return ops, nil
}

// validateDimRangeOp returns a Validate closure that delegates to
// dimRangeOpInput for shortcuts (delete/hide/unhide) whose builder takes an
// extra `op` argument. Token check happens here; the rest is the builder.
func validateDimRangeOp(op string) func(ctx context.Context, runtime *common.RuntimeContext) error {
	return func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID := strings.TrimSpace(runtime.Str("sheet-id"))
		sheetName := strings.TrimSpace(runtime.Str("sheet-name"))
		_, err = dimRangeOpInput(runtime, token, sheetID, sheetName, op)
		return err
	}
}

// validateDimGroupOp is the group/ungroup counterpart of validateDimRangeOp.
func validateDimGroupOp(op string) func(ctx context.Context, runtime *common.RuntimeContext) error {
	return func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID := strings.TrimSpace(runtime.Str("sheet-id"))
		sheetName := strings.TrimSpace(runtime.Str("sheet-name"))
		_, err = dimGroupInput(runtime, token, sheetID, sheetName, op)
		return err
	}
}

// DimHide / DimUnhide toggle visibility on a row/column range.
var DimHide = newDimRangeOpShortcut(
	"+dim-hide", "Hide rows or columns within a range.", "hide", "write",
)
var DimUnhide = newDimRangeOpShortcut(
	"+dim-unhide", "Unhide rows or columns within a range.", "unhide", "write",
)

// DimGroup / DimUngroup manage row/column outline groups.
var DimGroup = newDimGroupShortcut(
	"+dim-group", "Group rows or columns into an outline (collapsible).", "group",
)
var DimUngroup = newDimGroupShortcut(
	"+dim-ungroup", "Remove a row/column outline group.", "ungroup",
)

// DimFreeze sets the sheet's freeze state. Freeze is full-state replacement
// server-side (verified 07-31 live), so every call states the WHOLE state:
// --rows/--cols name both axes at once, while the older --dimension/--count
// pair can only name one and therefore unfreezes the other.
var DimFreeze = common.Shortcut{
	Service:     "sheets",
	Command:     "+dim-freeze",
	Description: "Freeze the first N rows and/or columns; this sets the whole freeze state, so an axis you do not name ends up unfrozen.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+dim-freeze"),
	Tips: []string{
		"Example: lark-cli sheets +dim-freeze --url <URL> --sheet-name Sheet1 --rows 1 --cols 2 (holds the header row and the first 2 columns in one call)",
		"Freezing is not additive: --dimension row --count 1 followed by --dimension column --count 2 leaves ONLY the columns frozen. Pass --rows/--cols together instead of calling twice",
		"To unfreeze one axis but keep the other, state the survivor: --rows 0 --cols 2. Bare --count 0 clears both",
	},
	Validate: validateViaInput(dimFreezeInput),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := dimFreezeInput(runtime, token, sheetID, sheetName)
		dr := invokeToolDryRun(token, ToolKindWrite, "modify_sheet_structure", input)
		// Surface the deprecation steer during the preview too: agents dry-run
		// before executing, so a note only on the execute path arrives after the
		// spelling is already committed to.
		if note := dimFreezeLegacyNote(runtime); note != "" {
			dr.Set("warning_message", note)
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
		input, err := dimFreezeInput(runtime, token, sheetID, sheetName)
		if err != nil {
			return err
		}
		if note := dimFreezeLegacyNote(runtime); note != "" {
			fmt.Fprintln(runtime.IO().ErrOut, note)
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "modify_sheet_structure", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

// DEPRECATED(phase-2): +dim-freeze --dimension / --count — replaced by
// --rows / --cols. Phase 1: the flags keep working, are retired from the skill
// docs via bundle.json doc_hidden_flags in sheet-skill-spec and from --help via
// their hidden mark, and every use is steered by dimFreezeLegacyNote.
// Phase 2 removal: drop both rows from spec-tables/flags.json + their
// doc_hidden_flags entry, then dimFreezeLegacyNote, dimFreezeEquivalent, their
// call sites (this shortcut's DryRun/Execute and batchLegacyDimFreezeNotes) and
// the legacy branch in dimFreezeInput.
//
// The pair is a strict subset of --rows/--cols — every --dimension/--count call
// has a byte-identical --rows/--cols spelling (TestDimFreezeEquivalent pins
// this) — and it is the form that reads as if it scoped to one axis when the
// backend replaces the whole freeze state.
//
// dimFreezeLegacyNote returns "" for the modern form. It takes a flagView
// rather than a RuntimeContext so +batch-update can render the identical
// wording for a sub-op (see batchLegacyDimFreezeNotes).
func dimFreezeLegacyNote(runtime flagView) string {
	if !runtime.Changed("dimension") && !runtime.Changed("count") {
		return ""
	}
	return fmt.Sprintf(
		"note: --dimension/--count is superseded by --rows/--cols, which state both axes at once; this call is equivalent to %s",
		dimFreezeEquivalent(runtime))
}

// dimFreezeEquivalent renders the --rows/--cols spelling of a legacy
// --dimension/--count call, so the deprecation note carries the exact
// replacement instead of a generic pointer.
func dimFreezeEquivalent(runtime flagView) string {
	count := runtime.Int("count")
	if count == 0 {
		return "--rows 0 --cols 0"
	}
	if runtime.Str("dimension") == "row" {
		return fmt.Sprintf("--rows %d", count)
	}
	return fmt.Sprintf("--cols %d", count)
}

// dimFreezeInput builds the freeze body for both the standalone shortcut and
// the +batch-update sub-op, so the two stay byte-identical (see
// TestBatchOp_BodyMatchesStandalone).
//
// Two request forms, deliberately not mixable:
//
//   - --rows / --cols state the complete target state in ONE operation. This
//     is the only form that can hold both axes, because freeze is full-state
//     replacement server-side (verified 07-31 live: freeze rows=1 then
//     columns=2 in two calls ends at 0 rows / 2 columns — the second call
//     drops the first axis). It is also the only form usable inside
//     +batch-update, whose sub-ops are a static array that cannot read the
//     current state to preserve an axis.
//   - --dimension + --count is the original single-axis form, kept for
//     compatibility. It necessarily unfreezes the axis it does not name.
func dimFreezeInput(runtime flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	pairForm := runtime.Changed("dimension") || runtime.Changed("count")
	axisForm := runtime.Changed("rows") || runtime.Changed("cols")
	switch {
	case pairForm && axisForm:
		return nil, sheetsValidationForFlag("rows",
			"give either --rows/--cols or --dimension/--count, not both — they are two ways to say the same thing; --rows/--cols is the one that can hold both axes at once")
	case !pairForm && !axisForm:
		return nil, sheetsValidationForFlag("rows",
			"nothing to freeze: pass --rows N and/or --cols N (e.g. --rows 1 --cols 2 holds the header row and the first 2 columns), or the single-axis --dimension row --count 1")
	}

	rows, cols := 0, 0
	if axisForm {
		for _, name := range []string{"rows", "cols"} {
			if runtime.Changed(name) && runtime.Int(name) < 0 {
				return nil, sheetsValidationForFlag(name, "--%s must be >= 0 (0 leaves that axis unfrozen)", name)
			}
		}
		rows, cols = runtime.Int("rows"), runtime.Int("cols")
	} else {
		if !runtime.Changed("dimension") {
			return nil, sheetsValidationForFlag("dimension", "--dimension is required alongside --count (or use --rows/--cols to set both axes at once)")
		}
		if !runtime.Changed("count") {
			return nil, sheetsValidationForFlag("count", "--count is required alongside --dimension (0 unfreezes)")
		}
		if runtime.Int("count") < 0 {
			return nil, sheetsValidationForFlag("count", "--count must be >= 0")
		}
		if runtime.Str("dimension") == "row" {
			rows = runtime.Int("count")
		} else {
			cols = runtime.Int("count")
		}
	}

	// An all-zero target is the bare "unfreeze" operation, which carries no
	// dimension and clears everything — the same request the old --count 0
	// always sent.
	input := map[string]interface{}{"excel_id": token, "operation": "unfreeze"}
	if rows > 0 || cols > 0 {
		input["operation"] = "freeze"
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if rows > 0 {
		input["freeze_rows"] = rows
	}
	if cols > 0 {
		input["freeze_columns"] = cols
	}
	return input, nil
}

// dimRangeOpInput builds the tool input for delete/hide/unhide/group/ungroup
// which all take a `range` string field. --range is a 1-based A1 closed range
// ("3:7" / "5" for rows, "C:F" / "C" for columns) and passes straight through
// after format validation.
func dimRangeOpInput(runtime flagView, token, sheetID, sheetName, op string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	if !runtime.Changed("range") {
		return nil, sheetsValidationForFlag("range", "--range is required")
	}
	rangeStr := strings.TrimSpace(runtime.Str("range"))
	if _, _, _, err := parseA1Range(rangeStr); err != nil {
		return nil, sheetsValidationForFlag("range", "invalid --range %q: %v", rangeStr, err)
	}
	input := map[string]interface{}{
		"excel_id":  token,
		"operation": op,
		"range":     rangeStr,
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	return input, nil
}

// newDimRangeOpShortcut builds the shared shape for hide / unhide.
func newDimRangeOpShortcut(command, desc, op, risk string) common.Shortcut {
	return common.Shortcut{
		Service:     "sheets",
		Command:     command,
		Description: desc,
		Risk:        risk,
		Scopes:      []string{"sheets:spreadsheet:write_only"},
		AuthTypes:   []string{"user", "bot"},
		HasFormat:   true,
		Flags:       flagsFor(command),
		Validate:    validateDimRangeOp(op),
		DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			token, _ := resolveSpreadsheetToken(runtime)
			sheetID, sheetName, _ := resolveSheetSelector(runtime)
			input, _ := dimRangeOpInput(runtime, token, sheetID, sheetName, op)
			return invokeToolDryRun(token, ToolKindWrite, "modify_sheet_structure", input)
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
			input, err := dimRangeOpInput(runtime, token, sheetID, sheetName, op)
			if err != nil {
				return err
			}
			out, err := callTool(ctx, runtime, token, ToolKindWrite, "modify_sheet_structure", input)
			if err != nil {
				return err
			}
			runtime.Out(out, nil)
			return nil
		},
	}
}

// newDimGroupShortcut builds the shared shape for group / ungroup. It adds
// --depth (currently unused server-side — accepted for forward-compat per
// the canonical spec) and --group-state (group only, defaults to expand).
func newDimGroupShortcut(command, desc, op string) common.Shortcut {
	flags := flagsFor(command)
	return common.Shortcut{
		Service:     "sheets",
		Command:     command,
		Description: desc,
		Risk:        "write",
		Scopes:      []string{"sheets:spreadsheet:write_only"},
		AuthTypes:   []string{"user", "bot"},
		HasFormat:   true,
		Flags:       flags,
		Validate:    validateDimGroupOp(op),
		DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			token, _ := resolveSpreadsheetToken(runtime)
			sheetID, sheetName, _ := resolveSheetSelector(runtime)
			input, _ := dimGroupInput(runtime, token, sheetID, sheetName, op)
			return invokeToolDryRun(token, ToolKindWrite, "modify_sheet_structure", input)
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
			input, err := dimGroupInput(runtime, token, sheetID, sheetName, op)
			if err != nil {
				return err
			}
			out, err := callTool(ctx, runtime, token, ToolKindWrite, "modify_sheet_structure", input)
			if err != nil {
				return err
			}
			runtime.Out(out, nil)
			return nil
		},
	}
}

func dimGroupInput(runtime flagView, token, sheetID, sheetName, op string) (map[string]interface{}, error) {
	input, err := dimRangeOpInput(runtime, token, sheetID, sheetName, op)
	if err != nil {
		return nil, err
	}
	if op == "group" {
		if gs := runtime.Str("group-state"); gs != "" {
			input["group_state"] = gs
		}
	}
	return input, nil
}

// ─── A1 parsing helpers ───────────────────────────────────────────────

// parseA1Range parses an A1 closed range ("3:7" / "5" / "C:F" / "C") into
// the inferred dimension ("row" or "column") and 0-based inclusive indices.
// Single-element form yields startIdx == endIdx. Mixing digits and letters
// across the two sides ("3:C") is rejected.
func parseA1Range(s string) (dimension string, startIdx, endIdx int, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", 0, 0, fmt.Errorf("range is empty") //nolint:forbidigo // intermediate error; callers wrap it into a typed flag validation error
	}
	parts := strings.Split(s, ":")
	if len(parts) > 2 {
		return "", 0, 0, fmt.Errorf("expected \"start:end\" or single element") //nolint:forbidigo // intermediate error; callers wrap it into a typed flag validation error
	}
	dim1, idx1, err := parseA1Position(parts[0])
	if err != nil {
		return "", 0, 0, err
	}
	if len(parts) == 1 {
		return dim1, idx1, idx1, nil
	}
	dim2, idx2, err := parseA1Position(parts[1])
	if err != nil {
		return "", 0, 0, err
	}
	if dim1 != dim2 {
		return "", 0, 0, fmt.Errorf("cannot mix row (digits) and column (letters) in one range") //nolint:forbidigo // intermediate error; callers wrap it into a typed flag validation error
	}
	if idx2 < idx1 {
		return "", 0, 0, fmt.Errorf("end position is before start") //nolint:forbidigo // intermediate error; callers wrap it into a typed flag validation error
	}
	return dim1, idx1, idx2, nil
}

// parseA1Position parses a single A1 position element: pure digits → row
// (1-based number, returned as 0-based idx); pure letters → column (letters
// case-insensitive, "A" → 0, "AA" → 26).
func parseA1Position(s string) (dimension string, idx int, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", 0, fmt.Errorf("position is empty") //nolint:forbidigo // intermediate error; callers wrap it into a typed flag validation error
	}
	isDigits := true
	isLetters := true
	for _, r := range s {
		if r < '0' || r > '9' {
			isDigits = false
		}
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')) {
			isLetters = false
		}
	}
	if isDigits {
		n, _ := strconv.Atoi(s)
		if n <= 0 {
			return "", 0, fmt.Errorf("row number must be >= 1 (got %q)", s) //nolint:forbidigo // intermediate error; callers wrap it into a typed flag validation error
		}
		return "row", n - 1, nil
	}
	if isLetters {
		return "column", letterToColumnIndex(s), nil
	}
	return "", 0, fmt.Errorf("expected pure digits (row number) or letters (column letter), got %q", s) //nolint:forbidigo // intermediate error; callers wrap it into a typed flag validation error
}

// columnIndexToLetter converts a 0-based column index to the spreadsheet
// letter notation (0 → "A", 25 → "Z", 26 → "AA", 701 → "ZZ", 702 → "AAA").
// Used by +workbook helpers that need to format absolute column references.
func columnIndexToLetter(idx int) string {
	if idx < 0 {
		return ""
	}
	idx++
	var out []byte
	for idx > 0 {
		idx--
		out = append([]byte{byte('A' + idx%26)}, out...)
		idx /= 26
	}
	return string(out)
}

// a1PositionBefore returns the A1 position one unit before s ("6" → "5",
// "C" → "B"), preserving row/column form. ok is false when s is the first
// row/column (row 1 / column A) — no earlier position — or is not a valid A1
// position. Callers validate via parseA1Position first, so in practice ok is
// false only at the first row/column.
func a1PositionBefore(s string) (pos string, ok bool) {
	dimension, idx, err := parseA1Position(s)
	if err != nil || idx == 0 {
		return "", false
	}
	if dimension == "row" {
		// idx is 0-based; the 1-based number one row earlier is idx itself.
		return strconv.Itoa(idx), true
	}
	return columnIndexToLetter(idx - 1), true
}

// ─── +dim-move (native v3 move_dimension, cli_status: cli-only) ──────
//
// Moves a contiguous block of rows or columns to a new index in the same
// sheet via the native v3 move_dimension endpoint (not the One-OpenAPI
// dispatcher). CLI accepts --source-range (A1 closed range like "3:7" or
// "C:F") + --target (A1 single position like "12" or "H"); both are parsed
// into the 0-based int indices that v3 move_dimension expects.

var DimMove = common.Shortcut{
	Service:     "sheets",
	Command:     "+dim-move",
	Description: "Move a contiguous block of rows or columns to a new position (re-numbers neighbors).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+dim-move"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, _, err := resolveSheetSelector(runtime); err != nil {
			return err
		}
		_, err := buildDimMovePlan(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		return common.NewDryRunAPI().
			POST(dimMovePath(token, sheetSelectorPlaceholder(sheetID, sheetName))).
			Body(dimMoveBody(runtime)).
			Set("spreadsheet_token", token)
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
		// v3 move_dimension carries sheet_id in the path. Resolve
		// sheet_name client-side when needed (reuses lookupSheetIndex
		// which fetches workbook structure).
		if sheetID == "" {
			lookedID, _, err := lookupSheetIndex(ctx, runtime, token, "", sheetName)
			if err != nil {
				return err
			}
			sheetID = lookedID
		}
		data, err := runtime.CallAPITyped("POST", dimMovePath(token, sheetID), nil, dimMoveBody(runtime))
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

// dimMovePlan is the parsed form of --source-range / --target.
type dimMovePlan struct {
	dimension string // "row" / "column"
	startIdx  int    // 0-based inclusive
	endIdx    int    // 0-based inclusive
	targetIdx int    // 0-based; destination position (move inserts before this)
}

// buildDimMovePlan parses --source-range + --target and enforces that the
// target dimension matches the source. Used by both Validate and Execute.
func buildDimMovePlan(runtime flagView) (*dimMovePlan, error) {
	if !runtime.Changed("source-range") || !runtime.Changed("target") {
		return nil, common.ValidationErrorf("--source-range and --target are required").WithParams(sheetsInvalidParam("source-range", "required"), sheetsInvalidParam("target", "required"))
	}
	src := strings.TrimSpace(runtime.Str("source-range"))
	dim, startIdx, endIdx, err := parseA1Range(src)
	if err != nil {
		return nil, sheetsValidationForFlag("source-range", "invalid --source-range %q: %v", src, err)
	}
	tgt := strings.TrimSpace(runtime.Str("target"))
	tgtDim, tgtIdx, err := parseA1Position(tgt)
	if err != nil {
		return nil, sheetsValidationForFlag("target", "invalid --target %q: %v", tgt, err)
	}
	if tgtDim != dim {
		return nil, common.ValidationErrorf("--target %q dimension (%s) must match --source-range %q dimension (%s)", tgt, tgtDim, src, dim).WithParams(sheetsInvalidParam("target", "dimension mismatch"), sheetsInvalidParam("source-range", "dimension mismatch"))
	}
	return &dimMovePlan{dimension: dim, startIdx: startIdx, endIdx: endIdx, targetIdx: tgtIdx}, nil
}

// dimMovePath builds the native v3 move_dimension endpoint. sheet_id lives in
// the path (unlike the v2 dimension_range body that the earlier build used).
func dimMovePath(token, sheetID string) string {
	return fmt.Sprintf("/open-apis/sheets/v3/spreadsheets/%s/sheets/%s/move_dimension",
		validate.EncodePathSegment(token), validate.EncodePathSegment(sheetID))
}

func dimMoveBody(runtime *common.RuntimeContext) map[string]interface{} {
	plan, err := buildDimMovePlan(runtime)
	if err != nil {
		// Validate has already rejected this case; emit an empty body
		// rather than panic on the dry-run path.
		return map[string]interface{}{}
	}
	dim := "ROWS"
	if plan.dimension == "column" {
		dim = "COLUMNS"
	}
	return map[string]interface{}{
		"source": map[string]interface{}{
			"major_dimension": dim,
			"start_index":     plan.startIdx,
			"end_index":       plan.endIdx,
		},
		"destination_index": plan.targetIdx,
	}
}
