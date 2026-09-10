// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
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
	Description: "Apply one declarative visual spec (styles/merges/row-col sizes/freeze) to existing sheets; sent as one batch request, or several when the spec is large (each atomic on its own, no rollback).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+styles-put"),
	Tips: []string{
		`Example: lark-cli sheets +styles-put --url <URL> --styles '{"styles":[{"name":"Sheet1","cell_styles":[{"range":"A1:F1","font_weight":"bold"}],"freeze":{"rows":1}}]}'`,
		"Same --styles vocabulary as +workbook-create / +table-put; one item per target sheet, name = the real sheet name.",
		"A spec of up to 100 operations goes out as ONE atomic batch request; a larger one is split, and each request is atomic only on its own — a later failure leaves the earlier requests applied.",
		"Style stamps are safe to re-run. Merges are not: re-sending an applied merge_cells can be rejected as an overlap, so after a partial failure read the sheet back (+cells-get --include style) and resend only the merges that did not land.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		// Pre-flight runs offline, so it cannot know how far a whole-column
		// range reaches; the execute path asks the workbook. Everything else
		// about the item is still checked, against a stand-in rectangle.
		_, err = stylesPutOperations(runtime, token, preflightRangeBounder(runtime))
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		ops, _ := stylesPutOperations(runtime, token, nil)
		chunks := chunkOperations(ops, maxBatchOperations)
		dry := invokeToolDryRun(token, ToolKindWrite, "batch_update", map[string]interface{}{
			"excel_id":   token,
			"operations": chunks[0],
		})
		for _, chunk := range chunks[1:] {
			body, _ := buildToolBody("batch_update", map[string]interface{}{
				"excel_id":   token,
				"operations": chunk,
			})
			dry.POST(toolInvokePath(token, ToolKindWrite)).
				Desc(fmt.Sprintf("batch_update (%d operations)", len(chunk))).
				Body(body)
		}
		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		ops, err := stylesPutOperations(runtime, token, newSheetGridBounder(ctx, runtime, token))
		if err != nil {
			return err
		}
		chunks := chunkOperations(ops, maxBatchOperations)
		var out interface{}
		for i, chunk := range chunks {
			out, err = callTool(ctx, runtime, token, ToolKindWrite, "batch_update", map[string]interface{}{
				"excel_id":   token,
				"operations": chunk,
			})
			if err != nil {
				if len(chunks) == 1 {
					return err
				}
				// Say what landed before naming the failure: each request is
				// atomic on its own, so the sheet now carries the earlier
				// chunks. Re-running the WHOLE spec is only safe when it holds
				// no merges — a style stamp is idempotent, but replaying an
				// already-applied merge_cells can come back as an overlap
				// rejection before the failed chunk is even reached, which
				// would leave the caller stuck on an error about work that
				// already succeeded.
				if i == 0 {
					// A failed FIRST request is not the same as an untouched
					// sheet: batch_update is fail-fast but not transactional,
					// so operations before the failing one inside that request
					// stay applied, and a transport failure leaves the outcome
					// unknown entirely. Only the backend saying "0 succeeded"
					// settles it.
					if toolReportedZeroApplied(err) {
						return attachSheetsWarningsToError(err, []string{fmt.Sprintf(
							"--styles was sent as %d batch requests; the first one failed with nothing applied, so the sheet is unchanged — fix the spec and re-run it whole",
							len(chunks))})
					}
					return attachSheetsWarningsToError(err, []string{fmt.Sprintf(
						"--styles was sent as %d batch requests and the first one failed; the request is not transactional, so part of it may already be on the sheet. Read the affected sheets back (+cells-get --include style) before retrying, and resend only what did not land — style stamps are idempotent, but replaying an applied merge is rejected as an overlap",
						len(chunks))})
				}
				return attachSheetsWarningsToError(err, []string{fmt.Sprintf(
					"--styles was sent as %d batch requests and request %d failed; requests 1-%d already applied. Style stamps are idempotent, so a spec of styles/sizes/freeze alone is safe to re-run as-is; if it carries cell_merges, read the sheet back first and resend only the merges that did not land — replaying an applied merge is rejected as an overlap",
					len(chunks), i+1, i)})
			}
		}
		if len(chunks) > 1 {
			out = annotateSheetsResult(out, "batch_requests", len(chunks))
			out = appendSheetsWarnings(out, []string{fmt.Sprintf(
				"--styles expanded to %d operations, over the %d per-request cap, so it was sent as %d batch requests — each atomic on its own, not as a whole",
				len(ops), maxBatchOperations, len(chunks))})
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
func stylesPutOperations(runtime flagView, token string, bound sheetRangeBounder) ([]interface{}, error) {
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
	// A whole-column or whole-row range needs the grid it spans, which only
	// the execute path can ask for; Validate and DryRun pass no bounder and
	// keep the parse-time rejection.
	boundStyleItemRanges(items, bound)
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
		payload, itemProbs := parseWorkbookCreateStyleItem(item, path, true)
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
	if len(ops) > maxStylesPutOperations {
		return nil, sheetsValidationForFlag("styles",
			"--styles expands to %d operations even after merging adjacent same-style ranges, over the %d cap; for alternating-row banding or value-dependent coloring use +cond-format-create instead of per-row stamps, which one rule covers whatever the sheet grows to",
			len(ops), maxStylesPutOperations)
	}
	return ops, nil
}

// maxStylesPutOperations bounds the whole spec. It is far above the
// per-request cap because the spec is no longer one request: chunkOperations
// splits it. What it still bounds is materialization — every translated op
// with its own cells matrix is held at once — so it stays finite, and a spec
// that reaches it is stamping per row, which +cond-format-create expresses as
// one rule.
const maxStylesPutOperations = 1000

// chunkOperations splits an operation list into batch_update-sized requests.
// A declarative spec states intent, so its execution shape is the CLI's to
// choose — the same license coalesceStyleStamps already takes when it fuses
// adjacent stamps, and the same thing +table-put does when it slices a large
// write. 08-29..31 reflow: 48 rejections told the caller to split the spec by
// hand, which is work with no decision in it.
func chunkOperations(ops []interface{}, size int) [][]interface{} {
	if len(ops) <= size {
		return [][]interface{}{ops}
	}
	chunks := make([][]interface{}, 0, (len(ops)+size-1)/size)
	for start := 0; start < len(ops); start += size {
		end := start + size
		if end > len(ops) {
			end = len(ops)
		}
		chunks = append(chunks, ops[start:end])
	}
	return chunks
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

// ─── unbounded style ranges ───────────────────────────────────────────

// sheetRangeBounder turns a range that names whole columns ("A:C") or whole
// rows ("3:5") into the rectangle it covers on a given sheet, or reports that
// it could not. Nil on the paths that run offline.
type sheetRangeBounder func(sheetName, rangeStr string) (string, bool)

// boundStyleItemRanges rewrites the cell_styles ranges of every item, in
// place, before the item parser rejects the unbounded forms. Only cell_styles
// is touched: row_sizes and col_sizes take a dimension range BY DESIGN ("2:10",
// "A:C"), and bounding those would turn their own vocabulary into an error.
// 09-04..07: 1208 rejections read "unsupported range form" under --styles.
func boundStyleItemRanges(items []map[string]interface{}, bound sheetRangeBounder) {
	if bound == nil {
		return
	}
	for _, item := range items {
		name, _ := item["name"].(string)
		entries, isList := item["cell_styles"].([]interface{})
		if !isList {
			continue
		}
		for _, raw := range entries {
			entry, isMap := raw.(map[string]interface{})
			if !isMap {
				continue
			}
			rng, isStr := entry["range"].(string)
			if !isStr {
				continue
			}
			if fitted, ok := bound(strings.TrimSpace(name), rng); ok {
				entry["range"] = fitted
			}
		}
	}
}

// preflightRangeBounder keeps an unbounded range from failing a check that
// cannot answer it. It stands in a rectangle that keeps whichever axis the
// caller did state — "A:C" becomes A1:C1, "3:5" becomes A3:A5 — so the rest of
// the item is validated as usual, and the extent is settled for real on the
// execute path, where the grid is readable. Nil under --dry-run: a preview
// sends nothing, so it cannot resolve the range either and says so.
func preflightRangeBounder(runtime flagView) sheetRangeBounder {
	if runtime.Bool("dry-run") {
		return nil
	}
	return func(_, rangeStr string) (string, bool) {
		trimmed := strings.TrimSpace(rangeStr)
		if m := wholeColumnRange.FindStringSubmatch(trimmed); m != nil {
			return fmt.Sprintf("%s1:%s1", strings.ToUpper(m[1]), strings.ToUpper(m[2])), true
		}
		if m := wholeRowRange.FindStringSubmatch(trimmed); m != nil {
			return fmt.Sprintf("A%s:A%s", m[1], m[2]), true
		}
		return "", false
	}
}

// newSheetGridBounder returns a bounder backed by one workbook-structure read,
// taken lazily and at most once per invocation: a payload whose ranges are all
// rectangular never pays for it.
func newSheetGridBounder(ctx context.Context, runtime *common.RuntimeContext, token string) sheetRangeBounder {
	var grids map[string]sheetGrid
	var loaded bool
	return func(sheetName, rangeStr string) (string, bool) {
		if !isUnboundedRange(rangeStr) {
			return "", false
		}
		if !loaded {
			loaded = true
			grids, _ = workbookSheetGrids(ctx, runtime, token)
		}
		grid, ok := grids[sheetName]
		if !ok {
			return "", false
		}
		return boundRangeToGrid(rangeStr, grid)
	}
}

// isUnboundedRange reports whether a range names whole columns or whole rows,
// the two forms parseCellRange refuses because their extent lives on the sheet
// rather than in the string.
func isUnboundedRange(rangeStr string) bool {
	return wholeColumnRange.MatchString(strings.TrimSpace(rangeStr)) ||
		wholeRowRange.MatchString(strings.TrimSpace(rangeStr))
}

var (
	wholeColumnRange = regexp.MustCompile(`^([A-Za-z]+):([A-Za-z]+)$`)
	wholeRowRange    = regexp.MustCompile(`^([0-9]+):([0-9]+)$`)
)

// boundRangeToGrid closes an unbounded range against the sheet's own extent:
// "A:C" on a 200-row sheet is A1:C200, "3:5" on a 20-column one is A3:T5. The
// result is exact rather than a guess — the grid is what the caller meant by
// "the whole column" — and an oversized one then meets the same stamp budget
// any explicit range of that size would.
func boundRangeToGrid(rangeStr string, grid sheetGrid) (string, bool) {
	trimmed := strings.TrimSpace(rangeStr)
	if grid.rows <= 0 || grid.cols <= 0 {
		return "", false
	}
	if m := wholeColumnRange.FindStringSubmatch(trimmed); m != nil {
		return fmt.Sprintf("%s1:%s%d", strings.ToUpper(m[1]), strings.ToUpper(m[2]), grid.rows), true
	}
	if m := wholeRowRange.FindStringSubmatch(trimmed); m != nil {
		return fmt.Sprintf("A%s:%s%s", m[1], columnIndexToLetter(grid.cols-1), m[2]), true
	}
	return "", false
}
