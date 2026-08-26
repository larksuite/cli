// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

import (
	"errors"
	"strings"
	"testing"
)

func TestPlanCreateBatchesPreservesSourceAndReservesImplicitPageBlock(t *testing.T) {
	source := strings.Repeat("<p>x</p>\n", 5_001)

	plan, err := PlanCreateBatches(source, 5_000, 40_000)
	if err != nil {
		t.Fatalf("PlanCreateBatches() error: %v", err)
	}
	if got := strings.Join(plan.Batches, ""); got != source {
		t.Fatalf("joined batches changed source: got %d bytes, want %d", len(got), len(source))
	}
	if len(plan.Batches) != 2 {
		t.Fatalf("batch count = %d, want 2", len(plan.Batches))
	}
	if plan.BatchBlocks[0] != 5_000 || plan.BatchBlocks[1] != 2 || plan.TotalBlocks != 5_002 {
		t.Fatalf("block plan = %#v", plan)
	}
}

func TestPlanCreateBatchesWithLimitsUsesReliableTarget(t *testing.T) {
	source := "<title>Doc</title>\n" + strings.Repeat("<p>x</p>\n", 5_000)

	plan, err := PlanCreateBatchesWithLimits(source, CreateBatchLimits{
		TargetBlocks:    3_000,
		OperationBlocks: 5_000,
		TotalBlocks:     40_000,
	})
	if err != nil {
		t.Fatalf("PlanCreateBatchesWithLimits() error: %v", err)
	}
	if got := strings.Join(plan.Batches, ""); got != source {
		t.Fatalf("joined batches changed source")
	}
	if len(plan.Batches) != 2 || plan.BatchBlocks[0] != 3_000 || plan.BatchBlocks[1] != 2_001 {
		t.Fatalf("block plan = %#v", plan)
	}
}

func TestPlanCreateBatchesWithLimitsTargetBoundary(t *testing.T) {
	tests := []struct {
		name        string
		paragraphs  int
		wantBatches []int
	}{
		{name: "at target", paragraphs: 1_999, wantBatches: []int{2_000}},
		{name: "above target", paragraphs: 2_000, wantBatches: []int{2_000, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := "<title>Doc</title>\n" + strings.Repeat("<p>x</p>\n", tt.paragraphs)
			plan, err := PlanCreateBatchesWithLimits(source, CreateBatchLimits{
				TargetBlocks:    2_000,
				OperationBlocks: 5_000,
				TotalBlocks:     40_000,
			})
			if err != nil {
				t.Fatalf("PlanCreateBatchesWithLimits() error: %v", err)
			}
			if len(plan.BatchBlocks) != len(tt.wantBatches) {
				t.Fatalf("batch blocks = %v, want %v", plan.BatchBlocks, tt.wantBatches)
			}
			for i, want := range tt.wantBatches {
				if plan.BatchBlocks[i] != want {
					t.Fatalf("batch blocks = %v, want %v", plan.BatchBlocks, tt.wantBatches)
				}
			}
		})
	}
}

func TestPlanCreateBatchesWithLimitsAllowsAtomicUnitAboveTarget(t *testing.T) {
	source := "<callout>" + strings.Repeat("<p>x</p>", 3_999) + "</callout>"

	plan, err := PlanCreateBatchesWithLimits(source, CreateBatchLimits{
		TargetBlocks:    3_000,
		OperationBlocks: 5_000,
		TotalBlocks:     40_000,
	})
	if err != nil {
		t.Fatalf("PlanCreateBatchesWithLimits() error: %v", err)
	}
	if len(plan.Batches) != 1 || plan.BatchBlocks[0] != 4_001 {
		t.Fatalf("block plan = %#v", plan)
	}
}

func TestPlanCreateBatchesWithLimitsHardOperationBoundary(t *testing.T) {
	limits := CreateBatchLimits{TargetBlocks: 2_000, OperationBlocks: 5_000, TotalBlocks: 40_000}
	t.Run("at hard limit", func(t *testing.T) {
		source := "<title>Doc</title><p>prefix</p><callout>" + strings.Repeat("<p>x</p>", 4_999) + "</callout>"
		plan, err := PlanCreateBatchesWithLimits(source, limits)
		if err != nil {
			t.Fatalf("PlanCreateBatchesWithLimits() error: %v", err)
		}
		if len(plan.BatchBlocks) != 2 || plan.BatchBlocks[1] != 5_000 {
			t.Fatalf("batch blocks = %v, want second batch at 5000", plan.BatchBlocks)
		}
	})
	t.Run("above hard limit", func(t *testing.T) {
		source := "<title>Doc</title><p>prefix</p><callout>" + strings.Repeat("<p>x</p>", 5_000) + "</callout>"
		_, err := PlanCreateBatchesWithLimits(source, limits)
		var planErr *CreateBatchPlanError
		if !errors.As(err, &planErr) || planErr.Kind != CreateBatchSubtreeLimit || planErr.Blocks != 5_001 || planErr.Limit != 5_000 {
			t.Fatalf("error = %#v, want subtree limit 5001/5000", planErr)
		}
	})
}

func TestPlanCreateBatchesKeepsLeadingTitleInCreate(t *testing.T) {
	source := "<title>Doc</title>\n" + strings.Repeat("<p>x</p>\n", 5_000)

	plan, err := PlanCreateBatches(source, 5_000, 40_000)
	if err != nil {
		t.Fatalf("PlanCreateBatches() error: %v", err)
	}
	if !strings.Contains(plan.Batches[0], "<title>Doc</title>") {
		t.Fatalf("create batch lost title: %q", plan.Batches[0][:minInt(len(plan.Batches[0]), 80)])
	}
	if plan.BatchBlocks[0] != 5_000 || plan.BatchBlocks[1] != 1 {
		t.Fatalf("batch blocks = %v, want [5000 1]", plan.BatchBlocks)
	}
}

func TestPlanCreateBatchesRejectsTitleAfterFirstBatch(t *testing.T) {
	source := strings.Repeat("<p>x</p>", 5_000) + "<title>Late</title>"

	_, err := PlanCreateBatches(source, 5_000, 40_000)

	var planErr *CreateBatchPlanError
	if !errors.As(err, &planErr) || planErr.Kind != CreateBatchTitleAfterCreate {
		t.Fatalf("error = %T %v, want title_after_create", err, err)
	}
}

func TestPlanCreateBatchesRejectsOversizedTopLevelSubtree(t *testing.T) {
	var table strings.Builder
	table.WriteString("<table><tbody><tr>")
	for i := 0; i < 2_500; i++ {
		table.WriteString("<td><p>x</p></td>")
	}
	table.WriteString("</tr></tbody></table>")

	_, err := PlanCreateBatches(table.String(), 5_000, 40_000)

	var planErr *CreateBatchPlanError
	if !errors.As(err, &planErr) || planErr.Kind != CreateBatchSubtreeLimit || planErr.Tag != "table" || planErr.Blocks <= 5_000 {
		t.Fatalf("error = %#v, want oversized table subtree", planErr)
	}
}

func TestPlanCreateBatchesRejectsFirstSubtreeThatLeavesNoRoomForImplicitPage(t *testing.T) {
	source := "<callout>" + strings.Repeat("<p>x</p>", 4_999) + "</callout>"

	_, err := PlanCreateBatches(source, 5_000, 40_000)

	var planErr *CreateBatchPlanError
	if !errors.As(err, &planErr) || planErr.Kind != CreateBatchInitialCapacity || planErr.Tag != "callout" || planErr.Blocks != 5_000 || planErr.Limit != 4_999 {
		t.Fatalf("error = %#v, want initial create capacity failure", planErr)
	}
}

func TestPlanCreateBatchesRejectsTotalLimit(t *testing.T) {
	source := strings.Repeat("<p>x</p>", 40_000)

	_, err := PlanCreateBatches(source, 5_000, 40_000)

	var planErr *CreateBatchPlanError
	if !errors.As(err, &planErr) || planErr.Kind != CreateBatchTotalLimit || planErr.Blocks != 40_001 {
		t.Fatalf("error = %#v, want total 40001", planErr)
	}
}

func TestMaterializedBlockCountMatchesTableAndStructuralRules(t *testing.T) {
	nodes, err := parseXML(`<ul><li>one</li><li>two</li></ul><table><tbody><tr><td>before<p>block</p>after<source token="file"/></td></tr></tbody></table>`)
	if err != nil {
		t.Fatalf("parseXML() error: %v", err)
	}
	count := 0
	for _, node := range nodes {
		count += materializedBlockCount(node)
	}
	// 2 list items + table + cell + p + 2 implicit text segments + inline file
	if count != 8 {
		t.Fatalf("materialized block count = %d, want 8", count)
	}
}
