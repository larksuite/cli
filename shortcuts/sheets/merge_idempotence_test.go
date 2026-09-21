// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"testing"

	"github.com/larksuite/cli/errs"
)

func mergeErr(msg string) error {
	return errs.NewAPIError(errs.SubtypeServerError, "%s", msg)
}

// 57% of the 38,491 overlap rejections in the 09-11..15 catalog name the
// rectangle that is already there. Those are no-ops, not conflicts.
func TestRedundantMergeOp_ExactDuplicateIsRecognized(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want int
	}{
		{"batch op 0", `batch_update: 0 succeeded, 1 failed — operations[0] (merge_cells): Range A1:F1 overlaps existing merged cells: [0,0-0,5]. Unmerge them first.`, 0},
		{"batch op 3", `batch_update: 0 succeeded, 1 failed — operations[3] (merge_cells): Range B2:D2 overlaps existing merged cells: [1,1-1,3].`, 3},
		{"single merge, no batch wrapper", `Range A1:C1 overlaps existing merged cells: [0,0-0,2].`, -1},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			idx, _, ok := redundantMergeOp(mergeErr(tt.msg))
			if !ok {
				t.Fatalf("exact duplicate not recognized: %s", tt.msg)
			}
			if idx != tt.want {
				t.Errorf("index = %d, want %d", idx, tt.want)
			}
		})
	}
}

// The other 43% overlap a DIFFERENT region. Those need a human decision about
// a merge nobody agreed to lose, and must keep the rejection.
func TestRedundantMergeOp_PartialOverlapKeepsTheRejection(t *testing.T) {
	cases := []string{
		// asked for A1:F1, existing region is only A1:E1
		`operations[0] (merge_cells): Range A1:F1 overlaps existing merged cells: [0,0-0,4].`,
		// asked for A1:F1, existing region starts a row lower
		`operations[0] (merge_cells): Range A1:F1 overlaps existing merged cells: [1,0-1,5].`,
		// a wider existing region that swallows the request
		`operations[2] (merge_cells): Range B1:C1 overlaps existing merged cells: [0,0-0,9].`,
	}
	for _, msg := range cases {
		if _, _, ok := redundantMergeOp(mergeErr(msg)); ok {
			t.Errorf("partial overlap was treated as redundant: %s", msg)
		}
	}
}

// Anything that is not this failure keeps its error untouched.
func TestRedundantMergeOp_UnrelatedErrorsAreIgnored(t *testing.T) {
	for _, msg := range []string{
		"cell at row 0, col 1 is inside a merged region (top-left: A1).",
		"API call failed: server time out error",
		"",
	} {
		if _, _, ok := redundantMergeOp(mergeErr(msg)); ok {
			t.Errorf("unrelated error classified as a redundant merge: %q", msg)
		}
	}
	if _, _, ok := redundantMergeOp(errs.NewValidationError(errs.SubtypeInvalidArgument, "plain")); ok {
		t.Error("a validation error was classified as a redundant merge")
	}
}

func TestDropOperation(t *testing.T) {
	ops := []interface{}{"a", "b", "c"}
	got, ok := dropOperation(ops, 1)
	if !ok || len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Fatalf("dropOperation(1) = %v ok=%v", got, ok)
	}
	// A message that disagrees with the batch we sent costs the optimization,
	// never an operation.
	for _, bad := range []int{-1, 3, 99} {
		if got, ok := dropOperation(ops, bad); ok || len(got) != 3 {
			t.Errorf("dropOperation(%d) = %v ok=%v, want the input untouched", bad, got, ok)
		}
	}
}
