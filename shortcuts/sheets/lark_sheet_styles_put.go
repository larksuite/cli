// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// ─── +styles-put ──────────────────────────────────────────────────────
//
// Declarative visual spec for EXISTING spreadsheets. Eval attribution
// showed ~73% of real +batch-update calls were pure formatting finishers
// (style stamps + merges + resizes + freeze) hand-assembled as imperative
// operations arrays — the top error surface. +styles-put replaces that
// with the {styles:[...]} protocol already shared by +workbook-create /
// +table-put --styles (identical vocabulary, parsed by the same
// parseWorkbookCreateStyleItem), applied to a live workbook and expanded
// client-side into ONE atomic batch_update.
//
// Per-sheet expansion order (server behavior verified live: style stamps
// over merged regions are allowed — the top-left-only restriction applies
// to value writes, not styles):
//
//	cell_merges → cell_styles → row_sizes → col_sizes → freeze
var StylesPut = common.Shortcut{
	Service:     "sheets",
	Command:     "+styles-put",
	Description: "Apply one declarative visual spec (styles/merges/row-col sizes/freeze) to existing sheets in one batch request (fail-fast, no rollback).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+styles-put"),
	Tips: []string{
		`Example: lark-cli sheets +styles-put --url <URL> --styles '{"styles":[{"name":"Sheet1","cell_styles":[{"range":"A1:F1","font_weight":"bold"}],"freeze":{"rows":1}}]}'`,
		"Same --styles vocabulary as +workbook-create / +table-put; one item per target sheet, name = the real sheet name.",
		"Style stamps are safe to re-run; the whole spec goes out as one batch request — fail-fast, and applied sub-ops are NOT rolled back.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		_, err = stylesPutOperations(runtime, token)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		ops, _ := stylesPutOperations(runtime, token)
		return invokeToolDryRun(token, ToolKindWrite, "batch_update", map[string]interface{}{
			"excel_id":   token,
			"operations": ops,
		})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		ops, err := stylesPutOperations(runtime, token)
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
	},
}

// stylesPutOperations parses --styles ({styles:[...]}, one item per target
// sheet) and expands it into the MCP batch_update operations array. Reuses
// the shared workbook-create style item parser, so field validation, alias
// normalization (border "all" shorthand, style vocabulary) and the
// aggregate-all-issues error shape are identical across the three --styles
// carriers.
func stylesPutOperations(runtime flagView, token string) ([]interface{}, error) {
	if strings.TrimSpace(runtime.Str("styles")) == "" {
		return nil, sheetsValidationForFlag("styles", "--styles is required")
	}
	v, err := parseJSONFlag(runtime, "styles")
	if err != nil {
		return nil, err
	}
	items, err := parseWorkbookCreateStylesItems(v)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, sheetsValidationForFlag("styles", "--styles.styles must be a non-empty array (one item per target sheet)")
	}
	var probs []error
	type sheetSpec struct {
		name    string
		payload *workbookCreateStylePayload
	}
	specs := make([]sheetSpec, 0, len(items))
	seenName := map[string]bool{}
	for i, item := range items {
		path := fmt.Sprintf("--styles.styles[%d]", i)
		name, _ := item["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			probs = append(probs, common.ValidationErrorf("%s.name is required (the real sheet name; check +workbook-info)", path))
			continue
		}
		if seenName[name] {
			probs = append(probs, common.ValidationErrorf("%s.name %q appears twice; merge the two items", path, name))
			continue
		}
		seenName[name] = true
		payload, itemProbs := parseWorkbookCreateStyleItem(item, path)
		if len(itemProbs) > 0 {
			probs = append(probs, itemProbs...)
			continue
		}
		specs = append(specs, sheetSpec{name: name, payload: payload})
	}
	if err := joinStyleValidationErrors(probs); err != nil {
		return nil, err
	}

	ops := make([]interface{}, 0, len(specs)*4)
	var totalCells int64
	appendVisual := func(name string, op workbookCreateStyleOp) {
		input, toolName := workbookCreateVisualOpInput(token, "", name, op)
		if toolName == "" {
			return
		}
		ops = append(ops, map[string]interface{}{"tool_name": toolName, "input": input})
	}
	for _, spec := range specs {
		// merges first so subsequent style stamps see the final grid.
		for _, m := range spec.payload.CellMerges {
			appendVisual(spec.name, workbookCreateStyleOp{Kind: "cell_merge", Range: m.Range, MergeType: m.MergeType})
		}
		for _, cs := range coalesceStyleStamps(spec.payload.CellStyles) {
			rows, cols, err := rangeDimensions(cs.Range)
			if err != nil {
				return nil, sheetsValidationForFlag("styles", "cell_styles range %q: %v", cs.Range, err)
			}
			if err := checkStampMatrixBudget("styles", cs.Range, rows, cols); err != nil {
				return nil, err
			}
			totalCells += int64(rows) * int64(cols)
			if err := checkBatchStampBudget("styles", totalCells); err != nil {
				return nil, err
			}
			ops = append(ops, map[string]interface{}{
				"tool_name": "set_cell_range",
				"input": map[string]interface{}{
					"excel_id":   token,
					"sheet_name": spec.name,
					"range":      stripSheetPrefix(cs.Range),
					"cells":      fillCellsMatrix(rows, cols, cs.Style),
				},
			})
		}
		for _, rs := range spec.payload.RowSizes {
			appendVisual(spec.name, workbookCreateStyleOp{Kind: "row_size", Range: rs.Range, ResizeType: rs.ResizeType, Size: rs.Size})
		}
		for _, csz := range spec.payload.ColSizes {
			appendVisual(spec.name, workbookCreateStyleOp{Kind: "col_size", Range: csz.Range, ResizeType: csz.ResizeType, Size: csz.Size})
		}
		if f := spec.payload.Freeze; f != nil {
			appendVisual(spec.name, workbookCreateStyleOp{Kind: "freeze", FreezeRows: f.Rows, FreezeCols: f.Cols})
		}
	}
	if len(ops) > maxBatchOperations {
		return nil, sheetsValidationForFlag("styles",
			"--styles expands to %d operations even after merging adjacent same-style ranges, over the %d cap; split the spec into several +styles-put calls — and for alternating-row banding or value-dependent coloring use +cond-format-create instead of per-row stamps",
			len(ops), maxBatchOperations)
	}
	return ops, nil
}

// coalesceStyleStamps merges cell_styles entries that carry the IDENTICAL
// style into larger rectangles: same column span + contiguous/overlapping
// rows fuse vertically, same row span + contiguous columns fuse
// horizontally, iterated to a fixpoint. Models routinely emit one entry per
// row (07-21 rerun: specs expanding to 184/203/861 operations against the
// 100-op cap); a declarative spec describes intent, so execution shape is
// the CLI's to optimize. Entries with unparsable ranges pass through
// untouched (the per-op validation reports them with proper context).
func coalesceStyleStamps(ops []workbookCreateCellStyleOp) []workbookCreateCellStyleOp {
	if len(ops) < 2 {
		return ops
	}
	type rect struct{ c1, r1, c2, r2 int }
	type entry struct {
		op     workbookCreateCellStyleOp
		rc     rect
		key    string
		parsed bool
		alive  bool
	}
	entries := make([]entry, len(ops))
	for i, op := range ops {
		e := entry{op: op, alive: true}
		c1, r1, c2, r2, err := workbookCreateStyleRangeBounds(op.Range)
		key, jerr := json.Marshal(op.Style) // map keys marshal sorted → canonical
		if err == nil && jerr == nil {
			e.rc, e.key, e.parsed = rect{c1, r1, c2, r2}, string(key), true
		}
		entries[i] = e
	}
	intersects := func(a, b rect) bool {
		return a.c1 <= b.c2 && b.c1 <= a.c2 && a.r1 <= b.r2 && b.r1 <= a.r2
	}
	// union returns the rectangle covering exactly a ∪ b, and whether the two
	// are mergeable at all: only same-column-span rows or same-row-span columns
	// that touch or overlap, so the union introduces no cell outside a ∪ b.
	union := func(a, b rect) (rect, bool) {
		switch {
		case a.c1 == b.c1 && a.c2 == b.c2 && b.r1 <= a.r2+1 && a.r1 <= b.r2+1:
			return rect{a.c1, min(a.r1, b.r1), a.c2, max(a.r2, b.r2)}, true
		case a.r1 == b.r1 && a.r2 == b.r2 && b.c1 <= a.c2+1 && a.c1 <= b.c2+1:
			return rect{min(a.c1, b.c1), a.r1, max(a.c2, b.c2), a.r2}, true
		}
		return rect{}, false
	}
	// Merging op j (later) into op i (earlier) moves j's write forward to i's
	// position, so it is only sound when nothing between them touches j's
	// cells — otherwise that intermediate op, which j used to overwrite, would
	// now land last and win. Style writes are field-wise last-write-wins
	// (mergeWorkbookCreateStyle), so silently reordering same-style stamps
	// around a differing one changes the final appearance.
	for i := range entries {
		if !entries[i].alive || !entries[i].parsed {
			continue
		}
		for j := i + 1; j < len(entries); j++ {
			if !entries[j].alive || !entries[j].parsed || entries[j].key != entries[i].key {
				continue
			}
			merged, ok := union(entries[i].rc, entries[j].rc)
			if !ok {
				continue
			}
			safe := true
			for k := i + 1; k < j && safe; k++ {
				if !entries[k].alive {
					continue
				}
				// An unparsable range has unknown coverage: assume it collides.
				if !entries[k].parsed || intersects(entries[k].rc, entries[j].rc) {
					safe = false
				}
			}
			if !safe {
				continue
			}
			entries[i].rc = merged
			entries[j].alive = false
			j = i // rescan: the grown rectangle may now absorb earlier misses
		}
	}
	out := make([]workbookCreateCellStyleOp, 0, len(ops))
	for _, e := range entries {
		if !e.alive {
			continue
		}
		if !e.parsed {
			out = append(out, e.op)
			continue
		}
		out = append(out, workbookCreateCellStyleOp{
			Range: fmt.Sprintf("%s%d:%s%d",
				columnIndexToLetter(e.rc.c1), e.rc.r1+1,
				columnIndexToLetter(e.rc.c2), e.rc.r2+1),
			Style: e.op.Style,
		})
	}
	return out
}

// stripSheetPrefix drops an optional "Sheet!"-style prefix from an A1 range:
// the target sheet is already carried by the spec item's name, and the
// batch sub-op input names the sheet separately.
func stripSheetPrefix(rangeStr string) string {
	if idx := strings.Index(rangeStr, "!"); idx >= 0 {
		return strings.TrimSpace(rangeStr[idx+1:])
	}
	return strings.TrimSpace(rangeStr)
}
