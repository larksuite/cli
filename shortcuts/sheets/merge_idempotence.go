// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// ─── an already-applied merge is not a conflict ───────────────────────
//
// "Range A1:F1 overlaps existing merged cells: [0,0-0,5]" is 38,491 calls in
// the 09-11..15 catalog, 35,517 of them on +styles-put. In **57% of them the
// requested range and the existing region are the SAME rectangle** — the
// caller asked for a merge that already exists, and the backend answered with
// an overlap conflict.
//
// That is not a conflict. The requested end state already holds, so the
// operation is a no-op and refusing it is the CLI reporting failure for work
// that is done. The remaining 43% genuinely overlap a DIFFERENT region and
// keep the rejection and its +cells-unmerge prescription: those need a human
// decision about a merge nobody agreed to lose.
//
// This reads the backend's message and never rewrites the request — the same
// rule annotateMergedRegionConflict follows. Dropping a redundant operation is
// the one edit it permits, because removing an operation the backend has
// already performed cannot change the resulting sheet.
//
// It also lifts a constraint +styles-put documents against itself: re-running
// a spec was "only safe when it holds no merges", precisely because a replayed
// merge came back as an overlap rejection "about work that already succeeded".
// With redundant merges recognized, that is no longer true.

// redundantMergeRE pulls the three facts out of a batch_update merge failure:
// which operation failed, the A1 range it asked to merge, and the 0-based
// bounds of the region already there.
//
// The operations[N] prefix is optional so the same parse serves a direct
// +cells-merge, whose message carries the range and bounds without a batch
// wrapper.
var redundantMergeRE = regexp.MustCompile(
	`(?:operations\[(\d+)\][^:]*:\s*)?Range ([A-Z]+\d+:[A-Z]+\d+) overlaps existing merged cells: \[(\d+),(\d+)-(\d+),(\d+)\]`)

// mergeRect is the shape of the region a redundant merge names, in the only
// terms that decide whether two merge_types mean the same thing.
type mergeRect struct{ singleRow, singleCol bool }

// redundantMergeOp reports the index of an operation whose merge names exactly
// the region already present, and the shape of that region. ok is false for a
// partial overlap, an unparsable message, or any error that is not this
// failure — all of which keep the rejection.
//
// index is -1 when the message carried no operations[N] prefix, i.e. the call
// was a single merge rather than a batch.
func redundantMergeOp(err error) (index int, rect mergeRect, ok bool) {
	p, problem := errs.ProblemOf(err)
	if !problem {
		return 0, rect, false
	}
	m := redundantMergeRE.FindStringSubmatch(p.Message)
	if m == nil {
		return 0, rect, false
	}
	existing, rendered := a1RangeFromZeroBased(m[3], m[4], m[5], m[6])
	if !rendered {
		return 0, rect, false
	}
	// Compare the rectangles, not the strings: the backend prints its bounds
	// 0-based and the caller wrote A1 notation, and a1RangeFromZeroBased has
	// already normalized one into the other's form.
	if !strings.EqualFold(strings.TrimSpace(m[2]), existing) {
		return 0, rect, false
	}
	r1, c1 := m[3], m[4]
	r2, c2 := m[5], m[6]
	rect = mergeRect{singleRow: r1 == r2, singleCol: c1 == c2}
	if m[1] == "" {
		return -1, rect, true
	}
	idx, convErr := strconv.Atoi(m[1])
	if convErr != nil || idx < 0 {
		return 0, rect, false
	}
	return idx, rect, true
}

// mergeOpIsRedundantFor reports whether dropping this operation leaves the
// sheet in the state the operation asked for.
//
// Matching rectangles are not enough. The backend reports the overlap by
// rectangle alone, and merge_type decides what that rectangle becomes: on a
// 3x3 range, `all` is one cell while `rows` is three. An existing `all` would
// swallow a requested `rows` and this code would report success for a
// structure that was never built.
//
// Where the two provably agree is a rectangle one cell thick, and that is
// 21,471 of the 21,535 exact-duplicate merges in the 09-11..15 catalog
// (99.7%) — a title row spanning A1:F1, over and over:
//
//	one row    → `all` and `rows` both merge it into one cell; `columns`
//	             merges each 1-cell column, i.e. does nothing
//	one column → `all` and `columns` agree; `rows` does nothing
//	1x1        → every merge_type is the same no-op
//
// Anything thicker, or a merge_type that does not match the axis, keeps the
// conflict and its +cells-unmerge prescription.
func mergeOpIsRedundantFor(op interface{}, rect mergeRect) bool {
	m, ok := op.(map[string]interface{})
	if !ok {
		return false
	}
	if name, _ := m["tool_name"].(string); name != "merge_cells" {
		return false
	}
	mergeType := "all"
	if input, ok := m["input"].(map[string]interface{}); ok {
		if v, ok := input["merge_type"].(string); ok && strings.TrimSpace(v) != "" {
			mergeType = normalizeMergeType(strings.TrimSpace(v))
		}
	}
	switch mergeType {
	case "all":
		return rect.singleRow || rect.singleCol
	case "rows":
		return rect.singleRow
	case "columns":
		return rect.singleCol
	}
	return false
}

// dropOperation returns ops without the entry at index. Out-of-range indexes
// return the input untouched, so a message that disagrees with the batch we
// sent costs the optimization rather than an operation.
func dropOperation(ops []interface{}, index int) ([]interface{}, bool) {
	if index < 0 || index >= len(ops) {
		return ops, false
	}
	out := make([]interface{}, 0, len(ops)-1)
	out = append(out, ops[:index]...)
	out = append(out, ops[index+1:]...)
	return out, true
}

// callBatchDroppingRedundantMerges issues one batch_update, retrying without
// any operation the backend rejects as a merge that is already exactly there.
//
// Retrying is only correct when nothing from the attempt landed, so the loop
// requires the backend's own "0 succeeded" before reissuing: batch_update is
// fail-fast, and a partially applied chunk reissued in full would replay the
// operations that did land. toolReportedZeroApplied is the same signal
// +styles-put already trusts to tell an untouched sheet from a dirty one.
//
// Bounded by the operation count: each pass removes exactly one operation, so
// a message that keeps naming the same index cannot spin.
func callBatchDroppingRedundantMerges(
	ctx context.Context,
	runtime *common.RuntimeContext,
	token string,
	ops []interface{},
) (interface{}, error) {
	remaining := ops
	for pass := 0; pass <= len(ops); pass++ {
		if len(remaining) == 0 {
			// Every operation in the chunk was a merge that already existed.
			// The requested state holds, so the chunk succeeded; there is no
			// backend response to return because nothing needed sending.
			return map[string]interface{}{
				"applied":          0,
				"skipped_existing": len(ops),
				"note":             "every operation in this batch was a merge that already covered exactly that range; the sheet already had the requested state",
			}, nil
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "batch_update", map[string]interface{}{
			"excel_id":   token,
			"operations": remaining,
		})
		if err == nil {
			return out, nil
		}
		if !toolReportedZeroApplied(err) {
			return out, err
		}
		idx, rect, isRedundant := redundantMergeOp(err)
		if !isRedundant || idx < 0 || idx >= len(remaining) {
			return out, err
		}
		if !mergeOpIsRedundantFor(remaining[idx], rect) {
			return out, err
		}
		next, dropped := dropOperation(remaining, idx)
		if !dropped {
			return out, err
		}
		remaining = next
	}
	return callTool(ctx, runtime, token, ToolKindWrite, "batch_update", map[string]interface{}{
		"excel_id":   token,
		"operations": remaining,
	})
}
