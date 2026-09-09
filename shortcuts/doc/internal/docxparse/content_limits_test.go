// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestXMLContentLimitsUseSDKUTF16AndPlaceholderSemantics(t *testing.T) {
	nodes, err := parseXML(`<p>a😀b<br/>c<cite type="user" user-id="u"></cite></p>`)
	if err != nil {
		t.Fatalf("parseXML() error: %v", err)
	}
	statistics := collectXMLContentStatistics(nodes)
	if statistics.MaxBlockCharacters != 7 {
		t.Fatalf("MaxBlockCharacters = %d, want 7", statistics.MaxBlockCharacters)
	}
}

func TestXMLContentLimitBoundaries(t *testing.T) {
	t.Run("block characters", func(t *testing.T) {
		if _, err := PlanCreateBatches(`<p>`+strings.Repeat("a", 100_000)+`</p>`, 5_000, 40_000); err != nil {
			t.Fatalf("100000 UTF-16 units: %v", err)
		}
		assertContentLimit(t,
			planCreateXMLError(`<p>`+strings.Repeat("a", 100_001)+`</p>`),
			ContentLimitBlockCharacters, 100_001, 100_000)

		if _, err := PlanCreateBatches(`<p>`+strings.Repeat("😀", 50_000)+`</p>`, 5_000, 40_000); err != nil {
			t.Fatalf("50000 supplementary runes: %v", err)
		}
		assertContentLimit(t,
			planCreateXMLError(`<p>`+strings.Repeat("😀", 50_000)+`a</p>`),
			ContentLimitBlockCharacters, 100_001, 100_000)
	})

	t.Run("table columns", func(t *testing.T) {
		if _, err := PlanCreateBatches(xmlTable(1, 100), 5_000, 40_000); err != nil {
			t.Fatalf("100 columns: %v", err)
		}
		assertContentLimit(t, planCreateXMLError(xmlTable(1, 101)), ContentLimitTableColumns, 101, 100)
	})

	t.Run("table cells", func(t *testing.T) {
		if _, err := PlanCreateBatches(xmlTable(2_000, 1), 5_000, 40_000); err != nil {
			t.Fatalf("2000 cells: %v", err)
		}
		assertContentLimit(t, planCreateXMLError(xmlTable(2_001, 1)), ContentLimitTableCells, 2_001, 2_000)
	})
}

func TestXMLContentLimitsUseEffectiveRectangularTableCells(t *testing.T) {
	var source strings.Builder
	source.WriteString("<table>")
	for row := 0; row < 20; row++ {
		source.WriteString("<tr>")
		for column := 0; column < 100; column++ {
			source.WriteString("<td><p>x</p></td>")
		}
		source.WriteString("</tr>")
	}
	source.WriteString("<tr><td><p>x</p></td></tr></table>")

	assertContentLimit(t, planCreateXMLError(source.String()), ContentLimitTableCells, 2_100, 2_000)
}

func TestXMLContentLimitsMatchSDKRowspanAndColspanStatistics(t *testing.T) {
	nodes, err := parseXML(`<table><tbody><tr><td rowspan="2" colspan="2"><p>cell</p></td></tr><tr></tr></tbody></table>`)
	if err != nil {
		t.Fatalf("parseXML() error: %v", err)
	}
	statistics := collectXMLContentStatistics(nodes)
	if statistics.Blocks != 9 || statistics.MaxTableCells != 4 || statistics.MaxTableColumns != 2 {
		t.Fatalf("statistics = %#v, want blocks=9 cells=4 columns=2", statistics)
	}
}

func TestXMLContentLimitsDoNotExpandUnboundedTableSpans(t *testing.T) {
	nodes, err := parseXML(`<table><tr><td colspan="1000000000"><p>x</p></td></tr></table>`)
	if err != nil {
		t.Fatalf("parseXML() error: %v", err)
	}
	statistics := collectXMLContentStatistics(nodes)
	if statistics.MaxTableCells != 1_000_000_000 || statistics.MaxTableColumns != 1_000_000_000 {
		t.Fatalf("statistics = %#v, want saturated traversal with exact declared dimensions", statistics)
	}
}

func TestXMLContentLimitsCheckSupportedCaptionsOnly(t *testing.T) {
	longCaption := strings.Repeat("x", 100_001)
	if _, err := PlanCreateBatches(`<p caption="`+longCaption+`">text</p>`, 5_000, 40_000); err != nil {
		t.Fatalf("paragraph caption must not be validated as DocX block text: %v", err)
	}
	assertContentLimit(t,
		planCreateXMLError(`<img src="token" caption="`+longCaption+`"/>`),
		ContentLimitBlockCharacters, 100_001, 100_000)
}

func TestContentLimitsRunBeforeOtherCreateLimits(t *testing.T) {
	limits := CreateBatchLimits{
		TargetBlocks: 2, OperationBlocks: 2, TotalBlocks: 2,
		Content: ContentLimits{BlockCharacters: 1, TableCells: 1, TableColumns: 1},
	}
	_, err := PlanCreateBatchesWithLimits(`<p>xx</p><table><tr><td>x</td><td>x</td></tr></table>`, limits)
	assertContentLimit(t, err, ContentLimitBlockCharacters, 2, 1)
}

func TestCompatibleXMLContentLimitsStillPreflightLegacyShapes(t *testing.T) {
	_, err := ValidateCompatibleXMLCreateLimits(`<p>`+strings.Repeat("x", 100_001), CreateBatchLimits{
		TargetBlocks: 2_000, OperationBlocks: 5_000, TotalBlocks: 40_000, Content: DefaultContentLimits(),
	})
	assertContentLimit(t, err, ContentLimitBlockCharacters, 100_001, 100_000)
}

func TestCompatibleXMLCreateLimitsRejectUnpartitionableBlockCounts(t *testing.T) {
	limits := CreateBatchLimits{
		TargetBlocks: 2_000, OperationBlocks: 5_000, TotalBlocks: 40_000, Content: DefaultContentLimits(),
	}
	tests := []struct {
		name         string
		body         int
		kind         CreateBatchPlanErrorKind
		materialized int
		total        int
	}{
		{name: "single request", body: 5_000, kind: CreateBatchSubtreeLimit, materialized: 5_001, total: 5_002},
		{name: "document total", body: 40_000, kind: CreateBatchTotalLimit, materialized: 40_001, total: 40_002},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statistics, err := ValidateCompatibleXMLCreateLimits(
				"<callout>"+strings.Repeat("<p>x</p>", tt.body), limits)
			var planErr *CreateBatchPlanError
			if !errors.As(err, &planErr) || planErr.Kind != tt.kind || planErr.Blocks != tt.total {
				t.Fatalf("error = %#v, statistics = %#v, want kind=%s total=%d", planErr, statistics, tt.kind, tt.total)
			}
			if statistics.Blocks != tt.materialized {
				t.Fatalf("statistics.Blocks = %d, want %d", statistics.Blocks, tt.materialized)
			}
		})
	}
}

func TestMarkdownContentLimitBoundaries(t *testing.T) {
	t.Run("paragraph", func(t *testing.T) {
		if _, err := PlanCreateMarkdownBatches(strings.Repeat("a", 100_000), 5_000, 40_000); err != nil {
			t.Fatalf("100000 characters: %v", err)
		}
		assertContentLimit(t,
			planCreateMarkdownError(strings.Repeat("a", 100_001)),
			ContentLimitBlockCharacters, 100_001, 100_000)
	})

	t.Run("fenced code", func(t *testing.T) {
		assertContentLimit(t,
			planCreateMarkdownError("```text\n"+strings.Repeat("a", 100_001)+"\n```\n"),
			ContentLimitBlockCharacters, 100_001, 100_000)
	})

	t.Run("image caption", func(t *testing.T) {
		assertContentLimit(t,
			planCreateMarkdownError("!["+strings.Repeat("a", 100_001)+"](https://example.com/image.png)\n"),
			ContentLimitBlockCharacters, 100_001, 100_000)
	})

	t.Run("gfm table columns", func(t *testing.T) {
		if _, err := PlanCreateMarkdownBatches(markdownTable(2, 100), 5_000, 40_000); err != nil {
			t.Fatalf("100 columns: %v", err)
		}
		assertContentLimit(t,
			planCreateMarkdownError(markdownTable(2, 101)),
			ContentLimitTableColumns, 101, 100)
	})

	t.Run("gfm table cells", func(t *testing.T) {
		if _, err := PlanCreateMarkdownBatches(markdownTable(2_000, 1), 5_000, 40_000); err != nil {
			t.Fatalf("2000 cells: %v", err)
		}
		assertContentLimit(t,
			planCreateMarkdownError(markdownTable(2_001, 1)),
			ContentLimitTableCells, 2_001, 2_000)
	})
}

func TestMarkdownContentLimitsInspectDocxXMLContainersAndLateBatches(t *testing.T) {
	longText := strings.Repeat("x", 100_001)
	assertContentLimit(t,
		planCreateMarkdownError("<callout>\n\n"+longText+"\n\n</callout>\n"),
		ContentLimitBlockCharacters, 100_001, 100_000)

	late := strings.Repeat("small paragraph\n\n", 2_000) + longText
	assertContentLimit(t,
		planCreateMarkdownError(late),
		ContentLimitBlockCharacters, 100_001, 100_000)
}

func TestMarkdownCharacterStatisticsMirrorSDKCJKEmphasisRepair(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		characters int
	}{
		{name: "rehydrated bold", content: `**bold(half)**中文`, characters: 12},
		{name: "escaped literal", content: `\*\*X\*\*中文`, characters: 7},
		{name: "fenced code stays literal", content: "```\n**bold(half)**中文\n```\n", characters: 16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statistics := markdownMaterializedSourceStatistics(tt.content)
			if statistics.MaxBlockCharacters != tt.characters {
				t.Fatalf("MaxBlockCharacters = %d, want %d", statistics.MaxBlockCharacters, tt.characters)
			}
		})
	}

	crossPairedSource := `**技术细节也很讲究。**翟霖追问，他们**不是录视频，是截图**。`
	crossPairedVisible := `技术细节也很讲究。翟霖追问，他们不是录视频，是截图。`
	statistics := markdownMaterializedSourceStatistics(crossPairedSource)
	if statistics.MaxBlockCharacters != utf16CodeUnits(crossPairedVisible) {
		parsedSource, document := parseSDKMarkdown(crossPairedSource, true)
		var nodes []string
		for child := document.FirstChild(); child != nil; child = child.NextSibling() {
			raw := markdownNodeRaw(child, parsedSource)
			nodes = append(nodes, fmt.Sprintf("kind=%s inline=%t raw=%q", child.Kind(), markdownNodeRendersTopLevelInline(child, raw), raw))
		}
		t.Fatalf("cross-paired MaxBlockCharacters = %d, want %d; preprocessed=%q nodes=%v",
			statistics.MaxBlockCharacters, utf16CodeUnits(crossPairedVisible), preprocessSDKMarkdownBlocks(crossPairedSource), nodes)
	}
}

func planCreateXMLError(source string) error {
	_, err := PlanCreateBatches(source, 5_000, 40_000)
	return err
}

func planCreateMarkdownError(source string) error {
	_, err := PlanCreateMarkdownBatches(source, 5_000, 40_000)
	return err
}

func assertContentLimit(t *testing.T, err error, kind ContentLimitKind, actual, limit int) {
	t.Helper()
	var limitErr *ContentLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("error = %T %v, want ContentLimitError", err, err)
	}
	if limitErr.Kind != kind || limitErr.Actual != actual || limitErr.Limit != limit {
		t.Fatalf("limit error = %#v, want kind=%s actual=%d limit=%d", limitErr, kind, actual, limit)
	}
}

func BenchmarkPlanCreateXMLRejectsOversizedTableSpans(b *testing.B) {
	for _, spanCells := range []int{1, 10, 100} {
		source := "<table><tr>" + strings.Repeat(`<td rowspan="2001" colspan="101">x</td>`, spanCells) + "</tr></table>"
		b.Run(fmt.Sprintf("cells=%d", spanCells), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			for range b.N {
				_, err := PlanCreateBatches(source, 5_000, 40_000)
				if err == nil {
					b.Fatal("PlanCreateBatches() succeeded, want table limit error")
				}
			}
		})
	}
}

func xmlTable(rows, columns int) string {
	var source strings.Builder
	source.WriteString("<table>")
	for row := 0; row < rows; row++ {
		source.WriteString("<tr>")
		for column := 0; column < columns; column++ {
			source.WriteString("<td><p>x</p></td>")
		}
		source.WriteString("</tr>")
	}
	source.WriteString("</table>")
	return source.String()
}

func markdownTable(rows, columns int) string {
	var source strings.Builder
	writeRow := func(value string) {
		source.WriteByte('|')
		for column := 0; column < columns; column++ {
			fmt.Fprintf(&source, " %s |", value)
		}
		source.WriteByte('\n')
	}
	writeRow("h")
	writeRow("---")
	for row := 1; row < rows; row++ {
		writeRow("x")
	}
	return source.String()
}
