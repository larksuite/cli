// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

// selectorDeferralCases is every command whose Execute settles a missing sheet
// selector against the workbook. Each one's Validate has to defer the same
// complaint, or the resolver it calls is unreachable and the deferral is a
// no-op for that command.
//
// The requirement is easy to satisfy in the Execute path and easy to forget in
// the Validate path, because the two read the selector from different places:
// Execute passes the RESOLVED pair into the input builder, Validate passes the
// raw flags. A builder that calls requireSheetSelector therefore passes for one
// caller and fails for the other, silently, with no test tying them together.
// This list is that tie.
var selectorDeferralCases = []struct {
	name string
	sc   common.Shortcut
	args []string
}{
	{"+cells-merge", CellsMerge, []string{"--range", "A1:B2"}},
	{"+cells-unmerge", CellsUnmerge, []string{"--range", "A1:B2"}},
	{"+cells-clear", CellsClear, []string{"--range", "A1:B2", "--scope", "content", "--yes"}},
	{"+rows-resize", RowsResize, []string{"--range", "1:2", "--height", "30"}},
	{"+cols-resize", ColsResize, []string{"--range", "A:B", "--width", "100"}},
	{"+range-copy", RangeCopy, []string{"--source-range", "A1:B2", "--target-range", "D1"}},
	{"+range-move", RangeMove, []string{"--source-range", "A1:B2", "--target-range", "D1"}},
	{"+range-fill", RangeFill, []string{"--source-range", "A1:B2", "--target-range", "A1:B10"}},
	{"+dim-insert", DimInsert, []string{"--position", "3", "--count", "1"}},
	{"+dim-delete", DimDelete, []string{"--range", "3:5", "--yes"}},
	{"+dim-delete --ranges", DimDelete, []string{"--ranges", `["3:5"]`, "--yes"}},
	{"+dim-freeze", DimFreeze, []string{"--rows", "1"}},
	{"+dim-move", DimMove, []string{"--source-range", "3:5", "--target", "9"}},
	{"+dim-hide", DimHide, []string{"--range", "3:5"}},
	{"+dim-group", DimGroup, []string{"--range", "3:5"}},
}

// A real run gets past pre-flight with no selector: the workbook is asked. The
// assertion is the absence of the pre-flight rejection rather than a completed
// call, since each command would need its own write stub and the point here is
// only which layer settles the selector. TestSheetSelector_ResolvedFromWorkbook
// covers one command end to end.
func TestSheetSelector_DeferralReachesEveryResolver(t *testing.T) {
	t.Parallel()
	for _, tc := range selectorDeferralCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			args := append([]string{"--url", testURL}, tc.args...)
			_, err := runShortcutWithStubs(t, tc.sc, args, structureStub("OnlySheet"))
			if err != nil && strings.Contains(err.Error(), "specify at least one of") {
				t.Errorf("pre-flight rejected a selector the workbook could have settled: %v", err)
			}
		})
	}
}

// --dry-run keeps the rejection. A preview sends nothing, so it has no way to
// ask the workbook, and printing a request whose sheet the CLI would have
// filled in would show the caller something the call never contained.
func TestSheetSelector_DryRunStillRequiresIt(t *testing.T) {
	t.Parallel()
	for _, tc := range selectorDeferralCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			args := append([]string{"--url", testURL}, tc.args...)
			// No stubs: a preview makes no call, and an unused stub is
			// itself a failure in this harness.
			_, err := runShortcutWithStubs(t, tc.sc, append(args, "--dry-run"))
			if err == nil || !strings.Contains(err.Error(), "specify at least one of") {
				t.Errorf("a preview must still name the sheet, got: %v", err)
			}
		})
	}
}
