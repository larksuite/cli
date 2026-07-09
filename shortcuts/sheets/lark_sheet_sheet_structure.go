// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"
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
	Validate:    validateViaInput(dimInsertInput),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := dimInsertInput(runtime, token, sheetID, sheetName)
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
		input, err := dimInsertInput(runtime, token, sheetID, sheetName)
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

// dimInsertInput passes --position (1-based row number "3" or column letter
// "C") straight to the tool's `position` field; --count maps to `count`.
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
	switch runtime.Str("inherit-style") {
	case "before":
		input["side"] = "before"
	case "after":
		input["side"] = "after"
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
	Validate:    validateDimRangeOp("delete"),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
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
	},
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

// DimFreeze freezes the first N rows or columns; --count 0 unfreezes that
// dimension.
var DimFreeze = common.Shortcut{
	Service:     "sheets",
	Command:     "+dim-freeze",
	Description: "Freeze the first N rows or columns; --count 0 unfreezes the chosen dimension.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+dim-freeze"),
	Validate:    validateViaInput(dimFreezeInput),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := dimFreezeInput(runtime, token, sheetID, sheetName)
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
		input, err := dimFreezeInput(runtime, token, sheetID, sheetName)
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

func dimFreezeInput(runtime flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	if !runtime.Changed("dimension") {
		return nil, sheetsValidationForFlag("dimension", "--dimension is required")
	}
	if !runtime.Changed("count") {
		return nil, sheetsValidationForFlag("count", "--count is required (0 unfreezes)")
	}
	if runtime.Int("count") < 0 {
		return nil, sheetsValidationForFlag("count", "--count must be >= 0")
	}
	dim := runtime.Str("dimension")
	count := runtime.Int("count")
	op := "freeze"
	if count == 0 {
		op = "unfreeze"
	}
	input := map[string]interface{}{"excel_id": token, "operation": op}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if op == "freeze" {
		if dim == "row" {
			input["freeze_rows"] = count
		} else {
			input["freeze_columns"] = count
		}
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
		return "", 0, 0, fmt.Errorf("range is empty")
	}
	parts := strings.Split(s, ":")
	if len(parts) > 2 {
		return "", 0, 0, fmt.Errorf("expected \"start:end\" or single element")
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
		return "", 0, 0, fmt.Errorf("cannot mix row (digits) and column (letters) in one range")
	}
	if idx2 < idx1 {
		return "", 0, 0, fmt.Errorf("end position is before start")
	}
	return dim1, idx1, idx2, nil
}

// parseA1Position parses a single A1 position element: pure digits → row
// (1-based number, returned as 0-based idx); pure letters → column (letters
// case-insensitive, "A" → 0, "AA" → 26).
func parseA1Position(s string) (dimension string, idx int, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", 0, fmt.Errorf("position is empty")
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
			return "", 0, fmt.Errorf("row number must be >= 1 (got %q)", s)
		}
		return "row", n - 1, nil
	}
	if isLetters {
		return "column", letterToColumnIndex(s), nil
	}
	return "", 0, fmt.Errorf("expected pure digits (row number) or letters (column letter), got %q", s)
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
