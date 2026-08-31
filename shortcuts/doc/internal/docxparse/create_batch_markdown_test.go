// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestPlanCreateMarkdownBatchesSplitsParagraphsAndPreservesSource(t *testing.T) {
	source := strings.Repeat("paragraph\n\n", 5_001)

	plan, err := PlanCreateMarkdownBatches(source, 5_000, 40_000)
	if err != nil {
		t.Fatalf("PlanCreateMarkdownBatches() error: %v", err)
	}
	if len(plan.Batches) != 2 || strings.Join(plan.Batches, "") != source {
		t.Fatalf("plan batches=%d preserve=%v", len(plan.Batches), strings.Join(plan.Batches, "") == source)
	}
	if plan.BatchBlocks[0] != 5_000 || plan.BatchBlocks[1] != 2 || plan.TotalBlocks != 5_002 {
		t.Fatalf("block plan = %#v", plan)
	}
}

func TestPlanCreateMarkdownBatchesPromotesLeadingH1AsTitle(t *testing.T) {
	source := "# Document\n\n" + strings.Repeat("paragraph\n\n", 5_000)

	plan, err := PlanCreateMarkdownBatches(source, 5_000, 40_000)
	if err != nil {
		t.Fatalf("PlanCreateMarkdownBatches() error: %v", err)
	}
	if plan.TotalBlocks != 5_001 || plan.BatchBlocks[0] != 5_000 || plan.BatchBlocks[1] != 1 {
		t.Fatalf("block plan = %#v", plan)
	}
}

func TestPlanCreateMarkdownBatchesRejectsSplitThatWouldPromoteAmbiguousH1(t *testing.T) {
	source := "# First\n\n" + strings.Repeat("paragraph\n\n", 4_998) + "# Second\n"

	_, err := PlanCreateMarkdownBatches(source, 5_000, 40_000)

	var planErr *CreateBatchPlanError
	if !errors.As(err, &planErr) || planErr.Kind != CreateBatchTitleAfterCreate {
		t.Fatalf("error = %#v, want title_after_create", planErr)
	}
}

func TestPlanCreateMarkdownBatchesKeepsAmbiguousH1sWhenTheyFitCreate(t *testing.T) {
	source := "# First\n\nbody\n\n# Second\n"

	plan, err := PlanCreateMarkdownBatches(source, 5_000, 40_000)
	if err != nil {
		t.Fatalf("PlanCreateMarkdownBatches() error: %v", err)
	}
	if plan.TotalBlocks != 4 || len(plan.Batches) != 1 {
		t.Fatalf("block plan = %#v", plan)
	}
}

func TestPlanCreateMarkdownBatchesKeepsLatePromotedH1InCreate(t *testing.T) {
	source := strings.Repeat("paragraph\n\n", 5_000) + "# Document title\n"

	_, err := PlanCreateMarkdownBatches(source, 5_000, 40_000)

	var planErr *CreateBatchPlanError
	if !errors.As(err, &planErr) || planErr.Kind != CreateBatchTitleAfterCreate {
		t.Fatalf("error = %#v, want title_after_create", planErr)
	}
}

func TestPlanCreateMarkdownBatchesCountsRemovedExplicitTitleDuplicate(t *testing.T) {
	source := "<title>Document title</title>\n\n" + strings.Repeat("paragraph\n\n", 4_999) + "# Document title\n"

	plan, err := PlanCreateMarkdownBatches(source, 5_000, 40_000)
	if err != nil {
		t.Fatalf("PlanCreateMarkdownBatches() error: %v", err)
	}
	if plan.TotalBlocks != 5_000 || len(plan.Batches) != 1 {
		t.Fatalf("block plan = %#v", plan)
	}
}

func TestPlanCreateMarkdownBatchesKeepsFencedCodeAtomic(t *testing.T) {
	source := "# T\n\n```go\n\nx := 1\n```\n\nafter\n"

	plan, err := PlanCreateMarkdownBatches(source, 2, 40_000)
	if err != nil {
		t.Fatalf("PlanCreateMarkdownBatches() error: %v", err)
	}
	if len(plan.Batches) != 2 || !strings.Contains(plan.Batches[0], "```go\n\nx := 1\n```") {
		t.Fatalf("fenced code was split: %#v", plan.Batches)
	}
}

func TestPlanCreateMarkdownBatchesFindsEmptyFenceBoundary(t *testing.T) {
	source := "# T\n\n" + strings.Repeat("paragraph\n\n", 4_998) + "```\n```\n\nafter\n"

	plan, err := PlanCreateMarkdownBatches(source, 5_000, 40_000)
	if err != nil {
		t.Fatalf("PlanCreateMarkdownBatches() error: %v", err)
	}
	if len(plan.Batches) != 2 || !strings.Contains(plan.Batches[0], "```\n```") {
		t.Fatalf("empty fenced code was not kept in create batch: %#v", plan.BatchBlocks)
	}
}

func TestPlanCreateMarkdownBatchesDropsSDKEmptyATXH1sFromCount(t *testing.T) {
	source := strings.Repeat("#\n\n", 5_001)

	plan, err := PlanCreateMarkdownBatches(source, 5_000, 40_000)
	if err != nil {
		t.Fatalf("PlanCreateMarkdownBatches() error: %v", err)
	}
	if plan.TotalBlocks != 1 || len(plan.Batches) != 1 {
		t.Fatalf("block plan = %#v", plan)
	}
}

func TestPlanCreateMarkdownBatchesLocatesEmptyH1AfterFence(t *testing.T) {
	source := "```text\n# inside fence\n```\n\n#\n"

	plan, err := PlanCreateMarkdownBatches(source, 5_000, 40_000)
	if err != nil {
		t.Fatalf("PlanCreateMarkdownBatches() error: %v", err)
	}
	if plan.TotalBlocks != 2 || len(plan.Batches) != 1 {
		t.Fatalf("block plan = %#v", plan)
	}
}

func TestPlanCreateMarkdownBatchesRejectsOversizedList(t *testing.T) {
	source := strings.Repeat("- item\n", 5_001)

	_, err := PlanCreateMarkdownBatches(source, 5_000, 40_000)

	var planErr *CreateBatchPlanError
	if !errors.As(err, &planErr) || planErr.Kind != CreateBatchSubtreeLimit || planErr.Tag != "ul" || planErr.Blocks != 5_001 {
		t.Fatalf("error = %#v, want oversized list", planErr)
	}
}

func TestPlanCreateMarkdownBatchesAllowsListAtHardOperationLimit(t *testing.T) {
	source := "<title>Document</title>\n\n" + strings.Repeat("- item\n", 5_000)

	plan, err := PlanCreateMarkdownBatchesWithLimits(source, CreateBatchLimits{
		TargetBlocks:    2_000,
		OperationBlocks: 5_000,
		TotalBlocks:     40_000,
		Content:         DefaultContentLimits(),
	})
	if err != nil {
		t.Fatalf("PlanCreateMarkdownBatchesWithLimits() error: %v", err)
	}
	if len(plan.BatchBlocks) != 2 || plan.BatchBlocks[1] != 5_000 {
		t.Fatalf("batch blocks = %v, want second batch at 5000", plan.BatchBlocks)
	}
}

func TestPlanCreateMarkdownBatchesRejectsTotalLimit(t *testing.T) {
	source := strings.Repeat("paragraph\n\n", 40_000)

	_, err := PlanCreateMarkdownBatches(source, 5_000, 40_000)

	var planErr *CreateBatchPlanError
	if !errors.As(err, &planErr) || planErr.Kind != CreateBatchTotalLimit || planErr.Blocks != 40_001 {
		t.Fatalf("error = %#v, want total 40001", planErr)
	}
}

func TestPlanCreateMarkdownBatchesCountsGFMTableCells(t *testing.T) {
	source := "| A | B |\n|---|---|\n| 1 | 2 |\n"

	_, err := PlanCreateMarkdownBatches(source, 8, 40_000)

	var planErr *CreateBatchPlanError
	if !errors.As(err, &planErr) || planErr.Kind != CreateBatchSubtreeLimit || planErr.Blocks != 9 {
		t.Fatalf("error = %#v, want 9-block table", planErr)
	}
}

func TestPlanCreateMarkdownBatchesKeepsDocxXMLContainerAtomic(t *testing.T) {
	source := "<callout>\n\nfirst\n\nsecond\n\n</callout>\n\nafter\n"

	plan, err := PlanCreateMarkdownBatches(source, 4, 40_000)
	if err != nil {
		t.Fatalf("PlanCreateMarkdownBatches() error: %v", err)
	}
	if len(plan.Batches) != 2 || !strings.Contains(plan.Batches[0], "</callout>") || strings.Join(plan.Batches, "") != source {
		t.Fatalf("DocxXML container was split: %#v", plan.Batches)
	}
}

func TestPlanCreateMarkdownBatchesRecognizesExplicitTitleTag(t *testing.T) {
	source := "<title>Document</title>\n\n" + strings.Repeat("paragraph\n\n", 5_000)

	plan, err := PlanCreateMarkdownBatches(source, 5_000, 40_000)
	if err != nil {
		t.Fatalf("PlanCreateMarkdownBatches() error: %v", err)
	}
	if plan.TotalBlocks != 5_001 || len(plan.Batches) != 2 || !strings.Contains(plan.Batches[0], "<title>Document</title>") {
		t.Fatalf("title plan = %#v", plan)
	}
}

func TestPlanCreateMarkdownBatchesSeparatesSDKTitleWithSingleNewline(t *testing.T) {
	source := "<title>Document</title>\n" + strings.Repeat("paragraph\n\n", 5_000)

	plan, err := PlanCreateMarkdownBatches(source, 5_000, 40_000)
	if err != nil {
		t.Fatalf("PlanCreateMarkdownBatches() error: %v", err)
	}
	if plan.TotalBlocks != 5_001 || len(plan.Batches) != 2 || strings.Join(plan.Batches, "") != source {
		t.Fatalf("title plan = %#v", plan)
	}
}

func TestMarkdownMaterializedSourceCountMatchesSDKParserContract(t *testing.T) {
	tests := []struct {
		name   string
		source string
		blocks int
	}{
		{
			name:   "sdk create nested list",
			source: "- outer\n  - inner\n",
			blocks: 2,
		},
		{
			name:   "gfm task list",
			source: "- [ ] first\n- [x] second\n",
			blocks: 2,
		},
		{
			name:   "definition list",
			source: "Term\n: Definition\n",
			blocks: 3,
		},
		{
			name:   "same-line grid columns",
			source: "<grid><column>one</column><column>two</column></grid>\n",
			blocks: 5,
		},
		{
			name:   "empty callout receives sdk fallback paragraph",
			source: "<callout></callout>\n",
			blocks: 2,
		},
		{
			name:   "underscore resource tag",
			source: `<synced_reference token="token"/>`,
			blocks: 1,
		},
		{
			name:   "inline underscore resource tag",
			source: `before <synced_reference token="token"/> after`,
			blocks: 2,
		},
		{
			name:   "sdk block parser accepts top-level inline tags",
			source: "<cite/>\n<cite/>\n",
			blocks: 1,
		},
		{
			name:   "sdk block parser accepts dual code tag",
			source: "<code>\nliteral <callout> text\n</code>\n",
			blocks: 1,
		},
		{
			name:   "inline and display math stay in paragraph",
			source: "math $x$ and $$y$$\n",
			blocks: 1,
		},
		{
			name:   "markdown image materializes inside paragraph",
			source: "![caption](https://example.com/image.png)\n",
			blocks: 2,
		},
		{
			name:   "markdown image materializes inside tight list item",
			source: "- before ![caption](https://example.com/image.png) after\n",
			blocks: 2,
		},
		{
			name:   "markdown image splits table cell inline runs",
			source: "| A |\n|---|\n| before ![caption](https://example.com/image.png) after |\n",
			blocks: 7,
		},
		{
			name:   "multiline pre code",
			source: "<pre><code>\n    <callout>literal</callout>\n    line 2 & more\n</code></pre>\n",
			blocks: 1,
		},
		{
			name:   "whiteboard raw markdown is not parsed as children",
			source: "<whiteboard type=mermaid>\ngraph TD\n\nA-->B\n</whiteboard>\n",
			blocks: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := markdownMaterializedSourceCount(tt.source); got != tt.blocks {
				t.Fatalf("markdownMaterializedSourceCount() = %d, want %d", got, tt.blocks)
			}
		})
	}
}

func TestPlanCreateMarkdownBatchesUsesSDKPreprocessingWithoutChangingSource(t *testing.T) {
	source := "<grid><column>one</column><column>two</column></grid>\n\nafter\n"

	plan, err := PlanCreateMarkdownBatches(source, 6, 40_000)
	if err != nil {
		t.Fatalf("PlanCreateMarkdownBatches() error: %v", err)
	}
	if len(plan.Batches) != 2 || plan.BatchBlocks[0] != 6 || plan.BatchBlocks[1] != 1 {
		t.Fatalf("block plan = %#v", plan)
	}
	if got := strings.Join(plan.Batches, ""); got != source {
		t.Fatalf("joined batches changed source:\n got %q\nwant %q", got, source)
	}
}

func TestMarkdownTopLevelBoundariesFindAllSameLineContainers(t *testing.T) {
	const containers = 1_000
	source := strings.Repeat("<callout>x</callout>", containers)

	nodes, starts, err := markdownTopLevelBoundaries(source)
	if err != nil {
		t.Fatalf("markdownTopLevelBoundaries() error: %v", err)
	}
	if len(nodes) != containers || len(starts) != containers {
		t.Fatalf("boundaries = nodes:%d starts:%d, want %d", len(nodes), len(starts), containers)
	}
	for index, start := range starts {
		if want := index * len("<callout>x</callout>"); start != want {
			t.Fatalf("starts[%d] = %d, want %d", index, start, want)
		}
	}
}

func TestPlanCreateMarkdownBatchesAllowsXMLDeclarationsInsideFencedCode(t *testing.T) {
	source := "```html\n<!DOCTYPE html>\n<body>example</body>\n```\n"

	plan, err := PlanCreateMarkdownBatches(source, 5_000, 40_000)
	if err != nil {
		t.Fatalf("PlanCreateMarkdownBatches() error: %v", err)
	}
	if got := strings.Join(plan.Batches, ""); got != source {
		t.Fatalf("joined batches changed source:\n got %q\nwant %q", got, source)
	}
}

func BenchmarkPlanCreateMarkdownSameLineContainers(b *testing.B) {
	for _, containers := range []int{100, 500, 1_000} {
		source := strings.Repeat("<callout>x</callout>", containers)
		limits := CreateBatchLimits{
			TargetBlocks:    5_000,
			OperationBlocks: 5_000,
			TotalBlocks:     40_000,
			Content:         DefaultContentLimits(),
		}
		b.Run(fmt.Sprintf("containers=%d", containers), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			for range b.N {
				if _, err := PlanCreateMarkdownBatchesWithLimits(source, limits); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
