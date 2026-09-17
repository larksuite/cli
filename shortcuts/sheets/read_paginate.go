// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// ─── lark_sheet read → automatic pagination ───────────────────────────
//
// A read that does not fit one backend call is completed by the CLI, not by
// the caller. The backend clips at max_chars and reports the clip in
// has_more + actual_range; left to the caller, finishing the read means
// noticing has_more, deriving the next range from actual_range, and looping —
// three steps that eval traces show being skipped, mis-stepped (a fixed row
// stride that overshoots the rows actually returned, silently dropping the
// gap) or mistaken for done (wc -l on a payload whose multi-line cells inflate
// the physical line count). The data then looks self-consistent while being a
// fraction of the sheet.
//
// So completeness stops being an action the caller performs and becomes a
// property of what the read returns: omit --max-chars and the result is the
// whole range, or the command fails. An explicit --max-chars keeps the old
// single-call truncating behaviour, because asking for a cap is how a caller
// samples a sheet or bounds what enters a context window.

// perBatchReadChars is the char cap sent per underlying request when the CLI
// is completing a read itself. Larger than the stdout-oriented default so
// fewer round trips are needed, but deliberately far below the ~10MB API
// transport ceiling: chars are not bytes (a CJK-heavy payload measures ~2.6
// bytes per char, ASCII ~1), so a cap that looks safe in chars can sit at
// very different byte sizes, and a bigger single request is also likelier to
// hit the backend's own timeout.
const perBatchReadChars = 1_000_000

// avgCellRangeBytes is the measured cost of one cell in a get_cell_ranges
// payload — every cell is an object, so this holds whether or not styles were
// requested (558 bytes/cell on a 22-column sheet, against 33 for the same data
// as CSV).
const avgCellRangeBytes = 558

// maxReadBatches bounds the loop so a backend that keeps reporting has_more
// without advancing actual_range cannot spin forever. Reaching it is a
// failure, never a quiet partial result.
const maxReadBatches = 64

// autoPaginate reports whether this invocation should complete the read
// itself. An explicit --max-chars is a request to truncate and is honoured as
// one; anything else (flag absent, or explicitly zeroed to mean "no cap of my
// own") reads everything.
func autoPaginate(runtime *common.RuntimeContext) bool {
	return !(runtime.Changed("max-chars") && runtime.Int("max-chars") > 0)
}

// a1RangePattern splits an A1 range into its column/row parts. Row numbers
// are optional so whole-column forms (C:C, A:ZZZ) parse too.
var a1RangePattern = regexp.MustCompile(`^([A-Za-z]+)(\d*):([A-Za-z]+)(\d*)$`)

// nextBatchRange derives the range for the batch after one that came back
// clipped: same columns, starting one row past what actual_range reports,
// ending where the original request ended.
//
// The next start is taken from the server's actual_range rather than from a
// client-side stride, because how many rows fit in a batch varies with the
// content in them (2937 / 2916 / 520 rows across one real sheet's batches).
// A stride that guesses high leaves an unread gap between batches and the
// concatenated result silently omits those rows.
func nextBatchRange(requested, actual string) (string, bool) {
	reqM := a1RangePattern.FindStringSubmatch(strings.TrimSpace(requested))
	actM := a1RangePattern.FindStringSubmatch(strings.TrimSpace(actual))
	if reqM == nil || actM == nil {
		return "", false
	}
	actualEndRow, err := strconv.Atoi(actM[4])
	if err != nil || actualEndRow <= 0 {
		return "", false
	}
	startCol, endCol := reqM[1], reqM[3]
	// An absent end row in the request means "to the grid edge"; keep it
	// absent so the backend clips again rather than inventing a bound.
	endRow := reqM[4]
	if endRow != "" {
		if n, err := strconv.Atoi(endRow); err == nil && actualEndRow >= n {
			return "", false // the request is already fully covered
		}
	}
	return fmt.Sprintf("%s%d:%s%s", startCol, actualEndRow+1, endCol, endRow), true
}

// ─── shrink-on-timeout ────────────────────────────────────────────────
//
// The backend answers a range it cannot finish in time with 900015214, and
// the size of the range is what decides that: a whole-column read of a
// 15121-row sheet failed 3 of 6 attempts, while narrower reads of the same
// column went through. Reissuing the identical request — which is all the
// transport-level retry does — waits for a condition that the request itself
// is causing.
//
// So a batch that keeps timing out is retried smaller. Each shrink halves the
// row span; the read still completes, just in more batches, which is strictly
// better than failing the whole thing.

const (
	// maxShrinkSteps bounds how far one batch may be narrowed (1/2, 1/4, 1/8).
	maxShrinkSteps = 3
	// minShrinkRows stops shrinking once a batch is small enough that the
	// range is no longer plausibly the cause.
	minShrinkRows = 200
	// openRangeShrinkRows is the first bounded window tried for an open-ended
	// range (C:C), where there is no end row to halve and no actual_range yet
	// to learn one from.
	openRangeShrinkRows = 2000
)

// shrinkRange narrows a batch's range after a timeout, returning false when
// there is nothing left worth narrowing.
func shrinkRange(rng string) (string, bool) {
	m := a1RangePattern.FindStringSubmatch(strings.TrimSpace(rng))
	if m == nil {
		return "", false
	}
	startCol, endCol := m[1], m[3]
	start := 1
	if m[2] != "" {
		n, err := strconv.Atoi(m[2])
		if err != nil {
			return "", false
		}
		start = n
	}
	if m[4] == "" {
		// Open-ended: bound it instead of halving. The end stays open in the
		// requested range, so later batches still run to the grid edge.
		return fmt.Sprintf("%s%d:%s%d", startCol, start, endCol, start+openRangeShrinkRows-1), true
	}
	end, err := strconv.Atoi(m[4])
	if err != nil || end < start {
		return "", false
	}
	span := end - start + 1
	if span <= minShrinkRows {
		return "", false
	}
	return fmt.Sprintf("%s%d:%s%d", startCol, start, endCol, start+span/2-1), true
}

// csvBatchState accumulates the batches of one csv-get read.
type csvBatchState struct {
	merged     map[string]interface{}
	annotated  []string
	rowIndices []interface{}
	rowCount   float64
	firstRange string
	lastRange  string
	revision   interface{}
	// hasRevision distinguishes "the first batch reported no revision" from
	// "it reported one that happens to be nil".
	hasRevision bool
	batches     int
	bytes       int
}

// accumulatedBytes is what this read is holding so far, used to stop before a
// merge outgrows the process rather than after.
func (s *csvBatchState) accumulatedBytes() int { return s.bytes }

func (s *csvBatchState) absorb(batch map[string]interface{}) error {
	if s.merged == nil {
		s.merged = batch
		s.revision, s.hasRevision = batch["revision"]
		s.firstRange = strFromMap(batch, "actual_range")
		// Presence is tracked with the comma-ok form rather than by testing
		// the value against nil: an interface holding a typed nil is itself
		// non-nil, so `!= nil` answers a question about the box, not the
		// revision. The map either carried the key or it did not.
	} else if rev, ok := batch["revision"]; ok && s.hasRevision && rev != s.revision {
		// The sheet changed mid-read. Concatenating batches taken from two
		// revisions yields rows that never coexisted, so this fails rather
		// than stitching them.
		return common.ValidationErrorf(
			"the spreadsheet was edited while it was being read (revision %v → %v); re-run the read",
			s.revision, rev)
	}
	if a, ok := batch["annotated_csv"].(string); ok && a != "" {
		s.annotated = append(s.annotated, strings.TrimRight(a, "\n"))
		s.bytes += len(a)
	}
	if ri, ok := batch["row_indices"].([]interface{}); ok {
		s.rowIndices = append(s.rowIndices, ri...)
	}
	if rc, ok := batch["row_count"].(float64); ok {
		s.rowCount += rc
	}
	if ar := strFromMap(batch, "actual_range"); ar != "" {
		s.lastRange = ar
	}
	s.batches++
	return nil
}

// result finalises the accumulated batches into one payload shaped exactly
// like a single-call result, so nothing downstream needs to know whether the
// read took one round trip or six.
func (s *csvBatchState) result() map[string]interface{} {
	if s.merged == nil {
		return nil
	}
	if len(s.annotated) > 0 {
		s.merged["annotated_csv"] = strings.Join(s.annotated, "\n")
	}
	if len(s.rowIndices) > 0 {
		s.merged["row_indices"] = s.rowIndices
	}
	if s.rowCount > 0 {
		s.merged["row_count"] = s.rowCount
	}
	if s.firstRange != "" && s.lastRange != "" {
		s.merged["actual_range"] = spanRanges(s.firstRange, s.lastRange)
	}
	// The caller asked for everything and got everything; a has_more left
	// over from the last batch would say otherwise.
	s.merged["has_more"] = false
	// The verdict as a field, not as prose. The receipt carries `complete`
	// only when a result is written to a file, so an inline result had no
	// language-independent way to say the same thing — and the warning text
	// that did say it is localised, which is no basis for a caller's control
	// flow.
	s.merged["complete"] = true
	// Only when it says something: batches=1 is the ordinary case and adds a
	// field with no information. The cell-ranges path already did this; the
	// two must not disagree about when the field appears.
	if s.batches > 1 {
		s.merged["batches"] = s.batches
	}
	clearStaleRowTruncationWarning(s.merged)
	return s.merged
}

// rowTruncationPattern matches the row half of the tool's "not everything was
// read" warning, which is phrased per batch.
var rowTruncationPattern = regexp.MustCompile(`行方向延伸到第\s*(\d+)\s*行，但只读到第\s*\d+\s*行（差\s*\d+\s*行）[；;]?`)

// clearStaleRowTruncationWarning drops the row-truncation sentence once the
// merged read reaches the bottom of the grid.
//
// Best-effort tidying of upstream prose, not a control signal: it matches the
// wording the tool emits today, and a different locale simply leaves the
// sentence in place. Callers decide on `complete` / `has_more`, which are set
// regardless of what the message says.
//
// The tool writes that sentence per batch ("rows run to 15121 but only 5853
// were read"), so after concatenation it describes a state that no longer
// exists — and it is the loudest thing in the payload, telling a caller who
// now holds every row that the data is short. The column half is left alone:
// it reflects the range the caller asked for, not a clip, and stays true.
//
// Only cleared when the merged range actually reaches current_region's last
// row. A read of C1:C100 on a 15121-row sheet is complete for what was asked
// and still genuinely short of the grid, which is worth saying.
func clearStaleRowTruncationWarning(m map[string]interface{}) {
	warning, ok := m["warning_message"].(string)
	if !ok || warning == "" {
		return
	}
	region := a1EndRow(strFromMap(m, "current_region"))
	if region == 0 || a1EndRow(strFromMap(m, "actual_range")) < region {
		return
	}
	stripped := rowTruncationPattern.ReplaceAllString(warning, "")
	if stripped == warning {
		return
	}
	// With the row half gone the banner may have nothing left to report;
	// drop the whole block rather than leave a headline with no findings.
	if !strings.Contains(stripped, "列方向") {
		if i := strings.Index(stripped, "⚠️【数据没读全 必读】"); i >= 0 {
			stripped = strings.TrimRight(stripped[:i], " \n")
		}
	}
	m["warning_message"] = stripped
}

// spanRanges joins the first batch's start with the last batch's end.
func spanRanges(first, last string) string {
	fm := a1RangePattern.FindStringSubmatch(first)
	lm := a1RangePattern.FindStringSubmatch(last)
	if fm == nil || lm == nil {
		return last
	}
	return fmt.Sprintf("%s%s:%s%s", fm[1], fm[2], lm[3], lm[4])
}

func strFromMap(m map[string]interface{}, key string) string {
	s, _ := m[key].(string)
	return s
}

// readCsvComplete runs csv-get to completion, looping while the backend
// reports has_more. Any batch that ultimately fails aborts the whole read:
// returning the batches gathered so far under a successful exit would be
// worse than today's behaviour, where has_more at least marks the payload as
// partial.
func readCsvComplete(
	ctx context.Context,
	runtime *common.RuntimeContext,
	token, sheetID, sheetName string,
) (interface{}, error) {
	input := csvGetInput(runtime, token, sheetID, sheetName)
	requested := strFromMap(input, "range")
	state := &csvBatchState{}

	shrinks := 0
	// Counts data batches, not requests: a shrink retry must not eat into the
	// ceiling, or a read that had to narrow three times would be cut short by
	// three batches it never used.
	for batch := 0; batch < maxReadBatches; {
		out, err := callTool(ctx, runtime, token, ToolKindRead, "get_range_as_csv", input)
		if err != nil {
			// A timeout is the backend saying this range was too big to finish,
			// so narrow it rather than reissuing the same request.
			if isTransientToolFailure(err) && shrinks < maxShrinkSteps {
				if smaller, ok := shrinkRange(strFromMap(input, "range")); ok {
					input["range"] = smaller
					shrinks++
					continue
				}
			}
			// Returned as it came: whatever ended the read is already typed
			// and classified, and the error contract is explicit that adding
			// a layer here re-decides the category — turning a permission
			// failure into an API one whenever the guess is wrong. How far
			// the read got is diagnostic anyway; there is no partial payload
			// to salvage, since a failed read writes nothing.
			return nil, err
		}
		shrinks = 0
		batch++
		m, ok := out.(map[string]interface{})
		if !ok {
			return out, nil
		}
		if err := state.absorb(m); err != nil {
			return nil, err
		}
		more, _ := m["has_more"].(bool)
		if !more {
			return state.result(), nil
		}
		if state.accumulatedBytes() > maxAccumulatedBytes {
			return nil, accumulationError(state.accumulatedBytes(), state.lastRange)
		}
		// Same early exit the cell-ranges path gets: a read that would need
		// more batches than the ceiling allows is abandoned after the first
		// one rather than minutes later, and both paths answer alike.
		if state.batches == 1 {
			if n := projectedBatches(requested, strFromMap(m, "actual_range")); n > maxReadBatches {
				return nil, &readTooLargeError{
					requested: requested,
					projected: n,
					message: fmt.Sprintf(
						"reading %s would take about %d batches, past the %d this command runs; "+
							"the first batch covered %s",
						requested, n, maxReadBatches, strFromMap(m, "actual_range")),
				}
			}
		}
		next, ok := nextBatchRange(requested, strFromMap(m, "actual_range"))
		if !ok {
			// has_more is set but the range cannot be advanced — returning
			// the partial payload as if complete is exactly the failure this
			// whole path exists to prevent.
			return nil, common.ValidationErrorf(
				"the read reported more data after %s but the next range could not be derived; narrow --range and read in parts",
				strFromMap(m, "actual_range"))
		}
		input = csvGetInput(runtime, token, sheetID, sheetName)
		input["range"] = next
	}
	return nil, common.ValidationErrorf(
		"the read did not finish within %d batches (covered %s); narrow --range and read in parts",
		maxReadBatches, state.lastRange)
	// (csv payloads are the cheap ones; a caller that exhausts the ceiling
	// here is reading something genuinely enormous, and has no lighter tool
	// to be pointed at.)
}

// ─── get_cell_ranges → the same completion, one implementation ─────────
//
// +cells-get and +table-get both read through get_cell_ranges, so they share
// one loop: same clip signal (truncated / has_more), same advance (from the
// server's actual_range), same all-or-fail contract as +csv-get.

// projectedBatches estimates how many batches a read will take, from what the
// first one actually returned.
//
// This replaces guessing from cell count. Batches are cut by max_chars, so
// their size is a constant (~2.5MB) no matter how dense the sheet is, while
// cells per batch is not: one real sheet fitted 2000 rows x 22 cols in a batch
// where a sheet of short values would fit far more. Measuring the first batch
// removes the guess — and a projection made after one round trip costs seconds,
// against the minutes it takes to discover the same thing by exhausting the
// ceiling.
//
// Returns 0 when the request has no end row to project against (C:C), where
// the total is unknown until the read reaches it.
func projectedBatches(requested, actual string) int {
	reqEnd := a1EndRow(requested)
	actEnd := a1EndRow(actual)
	if reqEnd <= 0 || actEnd <= 0 {
		return 0
	}
	start := 1
	if m := a1RangePattern.FindStringSubmatch(strings.TrimSpace(requested)); m != nil && m[2] != "" {
		if n, err := strconv.Atoi(m[2]); err == nil {
			start = n
		}
	}
	perBatch := actEnd - start + 1
	if perBatch <= 0 {
		return 0
	}
	total := reqEnd - start + 1
	return (total + perBatch - 1) / perBatch
}

// readTooLargeError marks a read abandoned because it would not finish, as
// opposed to one that failed. +table-get reports it per sheet and carries on;
// everywhere else it surfaces as the error it is.
type readTooLargeError struct {
	requested string
	projected int
	message   string
}

func (e *readTooLargeError) Error() string { return e.message }

// cellRangesEntry returns the first range entry of a get_cell_ranges payload,
// which is the only one these shortcuts request.
func cellRangesEntry(out interface{}) (map[string]interface{}, bool) {
	m, ok := out.(map[string]interface{})
	if !ok {
		return nil, false
	}
	items, ok := m["ranges"].([]interface{})
	if !ok || len(items) == 0 {
		return nil, false
	}
	e, ok := items[0].(map[string]interface{})
	return e, ok
}

// mergeCellRanges folds a follow-up batch into the accumulated payload: rows
// and their indices are appended, and the covered range grows to span both.
func mergeCellRanges(acc, batch map[string]interface{}) error {
	accEntry, ok1 := cellRangesEntry(acc)
	batchEntry, ok2 := cellRangesEntry(batch)
	if !ok1 || !ok2 {
		return common.ValidationErrorf("the read reported more data but the follow-up batch had no range payload to append")
	}
	if a, ok := acc["revision"]; ok {
		if b, ok2 := batch["revision"]; ok2 && a != b {
			return common.ValidationErrorf(
				"the spreadsheet was edited while it was being read (revision %v → %v); re-run the read", a, b)
		}
	}
	if cells, ok := batchEntry["cells"].([]interface{}); ok {
		accCells, _ := accEntry["cells"].([]interface{})
		accEntry["cells"] = append(accCells, cells...)
	}
	if ri, ok := batchEntry["row_indices"].([]interface{}); ok {
		accRI, _ := accEntry["row_indices"].([]interface{})
		accEntry["row_indices"] = append(accRI, ri...)
	}
	if first, last := strFromMap(accEntry, "actual_range"), strFromMap(batchEntry, "actual_range"); first != "" && last != "" {
		accEntry["actual_range"] = spanRanges(first, last)
	}
	if n, ok := batch["returned_cell_count"].(float64); ok {
		prev, _ := acc["returned_cell_count"].(float64)
		acc["returned_cell_count"] = prev + n
	}
	// The accumulated entry covers everything that was asked for; a marker
	// left over from the last batch would claim otherwise. The workbook-level
	// verdict is not set here — this merge finishes one batch, not the read,
	// and stamping `complete` per batch is what once let a payload say it was
	// whole while a second area was still clipped.
	delete(accEntry, "truncated")
	delete(accEntry, "truncation_warning")
	return nil
}

// cellReadBudget carries the limits that bound one +cells-get / +table-get
// invocation, across however many areas its --range names. They are spent
// jointly on purpose: the ceilings exist to stop a single command running for
// minutes or outgrowing memory, and an area-local budget would let a --range
// with four areas cost four times what one area is allowed.
type cellReadBudget struct {
	// batches counts data batches only. A shrink retry re-issues a request
	// that returned nothing, so charging the ceiling for it would cut a read
	// that had to narrow three times three batches short of where it could
	// have finished — the csv path counts the same way.
	batches int
	// accumulated is the estimated size of what has been merged so far.
	accumulated int
	// covered is the last range read, so a size failure can say how far it got.
	covered string
}

// cloneToolInput copies a tool input so the next request can change its range
// without editing the map a caller still holds.
func cloneToolInput(input map[string]interface{}) map[string]interface{} {
	next := make(map[string]interface{}, len(input))
	for k, v := range input {
		next[k] = v
	}
	return next
}

// fetchAllCellRanges runs a get_cell_ranges read to completion. input is the
// request for the first batch; follow-ups reuse it with an advanced range.
//
// A --range may name several disjoint areas ("A1:B10,D1:E10"), and a
// continuation carries exactly one of them: the next range is derived from one
// actual_range, so a follow-up asking for every area would re-read the ones
// already merged. Each area is therefore completed on its own and the results
// concatenated in the order they were asked for. Advancing only the first and
// leaving the rest as the backend clipped them — under a payload saying
// complete — is the silent shortfall this whole path exists to prevent.
func fetchAllCellRanges(
	ctx context.Context,
	runtime *common.RuntimeContext,
	token string,
	input map[string]interface{},
) (interface{}, error) {
	areas, _ := input["ranges"].([]string)
	budget := &cellReadBudget{}

	if len(areas) <= 1 {
		out, batches, err := fetchCellRangeArea(ctx, runtime, token, input, budget)
		if err != nil {
			return nil, err
		}
		acc, ok := out.(map[string]interface{})
		if !ok {
			return out, nil
		}
		return stampComplete(acc, batches), nil
	}

	var combined map[string]interface{}
	entries := make([]interface{}, 0, len(areas))
	total := 0
	for _, area := range areas {
		one := cloneToolInput(input)
		one["ranges"] = []string{area}
		out, batches, err := fetchCellRangeArea(ctx, runtime, token, one, budget)
		if err != nil {
			return nil, err
		}
		m, ok := out.(map[string]interface{})
		if !ok {
			return out, nil
		}
		total += batches
		if combined == nil {
			combined = m
		} else {
			if err := foldAreaInto(combined, m); err != nil {
				return nil, err
			}
		}
		if e, ok := cellRangesEntry(m); ok {
			entries = append(entries, e)
		}
	}
	if combined == nil {
		return nil, common.ValidationErrorf("the read returned no range payload for %s", strings.Join(areas, ", "))
	}
	combined["ranges"] = entries
	return stampComplete(combined, total), nil
}

// stampComplete records the verdict on a finished read: every area was read to
// the end, in this many round trips. batches is reported only when it says
// something — 1 is the ordinary case, and the csv path is silent there too.
func stampComplete(acc map[string]interface{}, batches int) map[string]interface{} {
	// Reached only once every area came back un-clipped, so the per-batch
	// markers the first response carried are now stale in every form the read
	// tools spell them.
	for _, key := range []string{"has_more", "truncated", "is_truncated"} {
		delete(acc, key)
	}
	acc["has_more"] = false
	acc["complete"] = true
	if batches > 1 {
		acc["batches"] = batches
	}
	return acc
}

// foldAreaInto merges a later area's top-level fields into the first area's
// payload. The areas themselves stay separate entries in ranges[]; only the
// workbook-wide facts have to agree or add up.
func foldAreaInto(combined, area map[string]interface{}) error {
	if a, ok := combined["revision"]; ok {
		if b, ok2 := area["revision"]; ok2 && a != b {
			// Areas read either side of an edit describe a workbook that never
			// existed in that state, which is worth failing over rather than
			// returning as one result.
			return common.ValidationErrorf(
				"the spreadsheet was edited while it was being read (revision %v → %v); re-run the read", a, b)
		}
	}
	if n, ok := area["returned_cell_count"].(float64); ok {
		prev, _ := combined["returned_cell_count"].(float64)
		combined["returned_cell_count"] = prev + n
	}
	return nil
}

// fetchCellRangeArea reads one area to completion, looping while the backend
// reports the payload clipped. Returns the accumulated payload and how many
// data batches it took.
func fetchCellRangeArea(
	ctx context.Context,
	runtime *common.RuntimeContext,
	token string,
	input map[string]interface{},
	budget *cellReadBudget,
) (interface{}, int, error) {
	requested := ""
	if rs, ok := input["ranges"].([]string); ok && len(rs) > 0 {
		requested = rs[0]
	}
	var acc map[string]interface{}
	batches := 0

	shrinks := 0
	// The ceiling is checked, not counted into: a shrink retry produced no
	// data and must not spend it.
	for budget.batches < maxReadBatches {
		out, err := callTool(ctx, runtime, token, ToolKindRead, "get_cell_ranges", input)
		if err != nil {
			// Same as the csv path: a timeout means the range was too big.
			if isTransientToolFailure(err) && shrinks < maxShrinkSteps {
				if rs, ok := input["ranges"].([]string); ok && len(rs) > 0 {
					if smaller, ok := shrinkRange(rs[0]); ok {
						next := cloneToolInput(input)
						next["ranges"] = []string{smaller}
						input = next
						shrinks++
						continue
					}
				}
			}
			// Passed through untouched, for the reason readCsvComplete gives.
			return nil, batches, err
		}
		shrinks = 0
		m, ok := out.(map[string]interface{})
		if !ok {
			return out, batches, nil
		}
		batches++
		budget.batches++
		if acc == nil {
			acc = m
		} else if err := mergeCellRanges(acc, m); err != nil {
			return nil, batches, err
		}
		if !cellRangesTruncated(m) {
			return acc, batches, nil
		}
		entry, _ := cellRangesEntry(m)
		budget.covered = strFromMap(entry, "actual_range")
		// Estimated from the cell count: a cell-ranges cell is an object with
		// a fixed set of fields, measured at ~558 bytes on a real sheet
		// whether or not styles are asked for. Marshalling each batch to
		// measure it exactly would cost more than the guard saves.
		if cells, ok := sliceLen(entry["cells"]); ok {
			budget.accumulated += cells * avgCellRangeBytes
		}
		if budget.accumulated > maxAccumulatedBytes {
			return nil, batches, accumulationError(budget.accumulated, budget.covered)
		}
		if budget.batches == 1 {
			if n := projectedBatches(requested, budget.covered); n > maxReadBatches {
				return nil, batches, &readTooLargeError{
					requested: requested,
					projected: n,
					message: fmt.Sprintf(
						"reading %s would take about %d batches, past the %d this command runs; "+
							"the first batch covered %s",
						requested, n, maxReadBatches, budget.covered),
				}
			}
		}
		next, ok := nextBatchRange(requested, strFromMap(entry, "actual_range"))
		if !ok {
			return nil, batches, common.ValidationErrorf(
				"the read reported more data after %s but the next range could not be derived; narrow --range and read in parts",
				strFromMap(entry, "actual_range"))
		}
		nextInput := cloneToolInput(input)
		nextInput["ranges"] = []string{next}
		input = nextInput
	}
	// Worth naming the alternative here specifically: a cell-ranges payload
	// runs ~17x the bytes of the same data as CSV (558 vs 33 bytes per cell on
	// a real sheet), so a range that cannot finish here is often one +csv-get
	// reads comfortably.
	return nil, batches, common.ValidationErrorf(
		"the read did not finish within %d batches (covered %s); narrow --range and read in parts, "+
			"or use +csv-get if you only need values — it returns the same cells at a fraction of the payload",
		maxReadBatches, budget.covered)
}
