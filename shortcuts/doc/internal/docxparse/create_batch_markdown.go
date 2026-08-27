// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

import (
	"bytes"
	"strings"

	gast "github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
)

func PlanCreateMarkdownBatches(source string, batchLimit, totalLimit int) (CreateBatchPlan, error) {
	return PlanCreateMarkdownBatchesWithLimits(source, CreateBatchLimits{
		TargetBlocks:    batchLimit,
		OperationBlocks: batchLimit,
		TotalBlocks:     totalLimit,
		Content:         DefaultContentLimits(),
	})
}

func PlanCreateMarkdownBatchesWithLimits(source string, limits CreateBatchLimits) (CreateBatchPlan, error) {
	if err := validateCreateBatchLimits(limits); err != nil {
		return CreateBatchPlan{}, err
	}
	if err := validateSource(source); err != nil {
		return CreateBatchPlan{}, err
	}
	nodes, starts, err := markdownTopLevelBoundaries(source)
	if err != nil {
		return CreateBatchPlan{}, err
	}
	if len(nodes) == 0 {
		return CreateBatchPlan{Batches: []string{source}, BatchBlocks: []int{1}, TotalBlocks: 1}, nil
	}

	units := make([]createBatchUnit, len(nodes))
	statistics := ContentStatistics{}
	for i, node := range nodes {
		end := len(source)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		raw := source[starts[i]:end]
		tag, _ := markdownUnitTag(node, raw)
		unitStatistics := markdownMaterializedSourceStatistics(raw)
		statistics = statistics.merge(unitStatistics)
		units[i] = createBatchUnit{
			start:  starts[i],
			tag:    tag,
			blocks: unitStatistics.Blocks,
		}
	}
	if err := validateContentStatistics(statistics, limits.Content); err != nil {
		return CreateBatchPlan{}, err
	}
	hasTitle := applySDKMarkdownCreateTitleSemantics(source, nodes, starts, units)
	return packCreateUnits(source, units, hasTitle, limits)
}

// markdownTopLevelBoundaries parses the original bytes without applying the
// SDK's source-mutating preprocessors. An SDK container that opens and closes
// on one line intentionally remains open in the raw parser and can absorb the
// remaining document; the SDK later repairs that shape by inserting newlines.
// Recover only that lexical close boundary, then resume the same parser on the
// untouched tail. Counting still runs on the fully preprocessed fragment.
func markdownTopLevelBoundaries(source string) ([]gast.Node, []int, error) {
	var nodes []gast.Node
	var starts []int
	offset := 0
	if titleStart, titleStop, ok := leadingSDKMarkdownTitleSpan(source); ok {
		nodes = append(nodes, &sdkMarkdownBlock{
			TagName:     "title",
			RawTagName:  "title",
			SourceStart: titleStart,
			SourceStop:  titleStop,
		})
		starts = append(starts, titleStart)
		offset = titleStop
	}
	for offset < len(source) {
		chunk := source[offset:]
		sourceBytes, document := parseSDKMarkdown(chunk, false)
		var chunkNodes []gast.Node
		for node := document.FirstChild(); node != nil; node = node.NextSibling() {
			chunkNodes = append(chunkNodes, node)
		}
		if len(chunkNodes) == 0 {
			break
		}

		restarted := false
		for i, node := range chunkNodes {
			start, ok := markdownNodeStart(node, sourceBytes)
			if !ok {
				if heading, isHeading := node.(*gast.Heading); isHeading && heading.Level == 1 && sdkMarkdownHeadingStrictlyEmpty(heading, sourceBytes) {
					searchFrom := 0
					if len(starts) > 0 {
						searchFrom = starts[len(starts)-1] - offset + 1
					}
					start, ok = nextSDKEmptyATXH1Start(sourceBytes, searchFrom)
				}
			}
			if !ok && node.Kind() == gast.KindFencedCodeBlock {
				searchFrom := 0
				if i > 0 {
					searchFrom = starts[len(starts)-1] - offset + 1
				}
				start, ok = nextMarkdownFenceStart(sourceBytes, searchFrom)
			}
			absoluteStart := offset + start
			if !ok || len(starts) > 0 && absoluteStart <= starts[len(starts)-1] {
				return nil, nil, newParseError("cannot determine markdown block boundary at index %d", len(nodes))
			}
			nodes = append(nodes, node)
			starts = append(starts, absoluteStart)

			block, unfinishedContainer := node.(*sdkMarkdownBlock)
			if !unfinishedContainer || block.SourceStop > block.SourceStart || !sdkContainerBlockParserTags[block.TagName] {
				continue
			}
			closeAt := findSDKXMLCloseTag([]byte(source[absoluteStart:]), block.RawTagName)
			if closeAt < 0 {
				continue
			}
			nextOffset := absoluteStart + closeAt + sdkXMLCloseTagLen(block.RawTagName)
			if nextOffset < len(source) {
				offset = nextOffset
				restarted = true
			}
			break
		}
		if !restarted {
			break
		}
	}
	return nodes, starts, nil
}

// leadingSDKMarkdownTitleSpan mirrors the direct leading <title> carrier that
// SDK create-title analysis consumes before rendering Markdown. Goldmark type-7
// HTML blocks otherwise absorb the following paragraph when only one newline
// separates it from </title>, which would undercount the create body by one.
func leadingSDKMarkdownTitleSpan(source string) (start, stop int, ok bool) {
	trimmed := strings.TrimLeft(source, " \t\r\n")
	start = len(source) - len(trimmed)
	if !strings.HasPrefix(trimmed, "<title") {
		return 0, 0, false
	}
	after := trimmed[len("<title"):]
	if len(after) == 0 || after[0] != ' ' && after[0] != '\t' && after[0] != '>' && after[0] != '/' {
		return 0, 0, false
	}
	openEnd := strings.IndexByte(trimmed, '>')
	closeAt := strings.Index(trimmed, "</title>")
	if openEnd < 0 || closeAt < openEnd {
		return 0, 0, false
	}
	return start, start + closeAt + len("</title>"), true
}

func nextSDKEmptyATXH1Start(source []byte, from int) (int, bool) {
	inFence := false
	var fenceMarker byte
	var fenceLength int
	for offset := 0; offset < len(source); {
		line := markdownLine(source, offset)
		trimmed := bytes.TrimLeft(line, " \t")
		indent := len(line) - len(trimmed)
		if indent < 4 && len(trimmed) >= 3 && (trimmed[0] == '`' || trimmed[0] == '~') {
			length := 0
			for length < len(trimmed) && trimmed[length] == trimmed[0] {
				length++
			}
			if length >= 3 {
				if !inFence {
					inFence = true
					fenceMarker = trimmed[0]
					fenceLength = length
				} else if trimmed[0] == fenceMarker && length >= fenceLength && len(bytes.TrimSpace(trimmed[length:])) == 0 {
					inFence = false
				}
				offset = markdownNextLineStart(source, offset)
				continue
			}
		}
		if offset >= from && !inFence && isSDKEmptyATXH1Line(line) {
			return offset, true
		}
		next := markdownNextLineStart(source, offset)
		if next <= offset {
			break
		}
		offset = next
	}
	return 0, false
}

func markdownNextLineStart(source []byte, offset int) int {
	if relative := bytes.IndexByte(source[offset:], '\n'); relative >= 0 {
		return offset + relative + 1
	}
	return len(source)
}

func isSDKEmptyATXH1Line(line []byte) bool {
	trimmed := bytes.TrimLeft(line, " \t")
	if len(line)-len(trimmed) > 3 || len(trimmed) == 0 || trimmed[0] != '#' {
		return false
	}
	rest := bytes.TrimSpace(trimmed[1:])
	for _, ch := range rest {
		if ch != '#' {
			return false
		}
	}
	return true
}

func nextMarkdownFenceStart(source []byte, from int) (int, bool) {
	for offset := markdownLineStart(source, from); offset < len(source); {
		trimmed := bytes.TrimSpace(markdownLine(source, offset))
		if bytes.HasPrefix(trimmed, []byte("```")) || bytes.HasPrefix(trimmed, []byte("~~~")) {
			return offset, true
		}
		next := bytes.IndexByte(source[offset:], '\n')
		if next < 0 {
			break
		}
		offset += next + 1
	}
	return 0, false
}

func markdownNodeStart(node gast.Node, source []byte) (int, bool) {
	if sdkNode, ok := node.(*sdkMarkdownBlock); ok {
		return sdkNode.SourceStart, sdkNode.SourceStart >= 0 && sdkNode.SourceStart < len(source)
	}
	if spanned, ok := node.(interface {
		SourceSpan() (int, int, bool)
	}); ok {
		start, _, valid := spanned.SourceSpan()
		return start, valid
	}
	start := len(source)
	var inspect func(gast.Node)
	inspect = func(current gast.Node) {
		if current == nil {
			return
		}
		if current.Type() == gast.TypeBlock {
			lines := current.Lines()
			for i := 0; i < lines.Len(); i++ {
				if segment := lines.At(i); segment.Start < start {
					start = segment.Start
				}
			}
		}
		if textNode, ok := current.(*gast.Text); ok && textNode.Segment.Start < start {
			start = textNode.Segment.Start
		}
		for child := current.FirstChild(); child != nil; child = child.NextSibling() {
			inspect(child)
		}
	}
	inspect(node)
	if start == len(source) {
		return 0, false
	}
	start = markdownLineStart(source, start)
	if node.Kind() == gast.KindFencedCodeBlock {
		start = markdownFenceOpeningStart(source, start)
	}
	return start, true
}

func markdownLineStart(source []byte, offset int) int {
	if offset > len(source) {
		offset = len(source)
	}
	for offset > 0 && source[offset-1] != '\n' {
		offset--
	}
	return offset
}

func markdownFenceOpeningStart(source []byte, currentLineStart int) int {
	line := bytes.TrimSpace(markdownLine(source, currentLineStart))
	if bytes.HasPrefix(line, []byte("```")) || bytes.HasPrefix(line, []byte("~~~")) {
		return currentLineStart
	}
	if currentLineStart == 0 {
		return currentLineStart
	}
	previous := markdownLineStart(source, currentLineStart-1)
	line = bytes.TrimSpace(markdownLine(source, previous))
	if bytes.HasPrefix(line, []byte("```")) || bytes.HasPrefix(line, []byte("~~~")) {
		return previous
	}
	return currentLineStart
}

func markdownLine(source []byte, start int) []byte {
	end := len(source)
	if relative := bytes.IndexByte(source[start:], '\n'); relative >= 0 {
		end = start + relative
	}
	return source[start:end]
}

func markdownUnitTag(node gast.Node, raw string) (string, bool) {
	if xmlNode, ok := node.(*sdkMarkdownBlock); ok {
		return xmlNode.TagName, xmlNode.TagName == "title"
	}
	if looksLikeMarkdownXML(raw) {
		if elements, err := parseXML(strings.TrimSpace(raw)); err == nil && len(elements) > 0 {
			for _, element := range elements {
				if element.typ == nodeElement {
					return element.tag, element.tag == "title"
				}
			}
		}
	}
	switch typed := node.(type) {
	case *gast.Heading:
		return "h" + string(rune('0'+typed.Level)), false
	case *gast.List:
		if typed.IsOrdered() {
			return "ol", false
		}
		return "ul", false
	}
	return strings.ToLower(node.Kind().String()), false
}

func markdownMaterializedSourceCount(raw string) int {
	return markdownMaterializedSourceStatistics(raw).Blocks
}

func markdownMaterializedSourceStatistics(raw string) ContentStatistics {
	source, document := parseSDKMarkdown(raw, true)
	statistics := markdownDocumentContentStatistics(document, source)
	statistics.Blocks = markdownMaterializedDocumentCount(document, source)
	return statistics
}

func markdownMaterializedDocumentCount(document gast.Node, source []byte) int {
	count := 0
	inlineRun := false
	flushInlineRun := func() {
		if inlineRun {
			count = saturatedAdd(count, 1)
			inlineRun = false
		}
	}
	for child := document.FirstChild(); child != nil; child = child.NextSibling() {
		childRaw := markdownNodeRaw(child, source)
		childBlocks := markdownMaterializedBlockCount(child, source, childRaw)
		if markdownNodeRendersTopLevelInline(child, childRaw) {
			inlineRun = true
			count = saturatedAdd(count, childBlocks)
			continue
		}
		flushInlineRun()
		count = saturatedAdd(count, childBlocks)
	}
	flushInlineRun()
	return count
}

func markdownNodeRendersTopLevelInline(node gast.Node, raw string) bool {
	if block, ok := node.(*sdkMarkdownBlock); ok {
		return layoutOf(block.TagName) == layoutInline
	}
	if node.Kind() != gast.KindHTMLBlock || !looksLikeMarkdownXML(raw) {
		return false
	}
	elements, err := parseXML(strings.TrimSpace(raw))
	if err != nil || len(elements) == 0 {
		return false
	}
	for _, element := range elements {
		if element == nil {
			continue
		}
		if element.typ == nodeText {
			continue
		}
		if layoutOf(element.tag) != layoutInline {
			return false
		}
	}
	return true
}

func markdownMaterializedBlockCount(node gast.Node, source []byte, raw string) int {
	if xmlNode, ok := node.(*sdkMarkdownBlock); ok {
		count := 0
		layout := layoutOf(xmlNode.TagName)
		if (layout == layoutBlock || layout == layoutDual) && !isMarkdownXMLShell(xmlNode.TagName) {
			count = 1
			if xmlNode.TagName == "source" {
				count = 2
			}
		}
		if xmlNode.PreserveText {
			return count
		}
		childBlocks := 0
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			childBlocks = saturatedAdd(childBlocks, markdownMaterializedBlockCount(child, source, markdownNodeRaw(child, source)))
		}
		if childBlocks == 0 && sdkMarkdownParagraphContainerTags[xmlNode.TagName] {
			childBlocks = 1
		}
		return saturatedAdd(count, childBlocks)
	}
	switch node.Kind() {
	case gast.KindParagraph, gast.KindHeading, gast.KindFencedCodeBlock,
		gast.KindCodeBlock, gast.KindThematicBreak:
		return saturatedAdd(1, markdownInlineMaterializedBlockCount(node, source))
	case gast.KindBlockquote:
		children := markdownChildBlockCount(node, source)
		if children == 0 {
			children = 1
		}
		return saturatedAdd(1, children)
	case gast.KindList:
		return markdownListBlockCount(node.(*gast.List), source)
	case gast.KindHTMLBlock:
		if elements, err := parseXML(strings.TrimSpace(raw)); err == nil && len(elements) > 0 {
			count := 0
			for _, element := range elements {
				count = saturatedAdd(count, materializedBlockCount(element))
			}
			return count
		}
		return 1
	case extast.KindTable:
		return markdownTableBlockCount(node, source)
	case extast.KindDefinitionList:
		return markdownDefinitionListBlockCount(node, source)
	case extast.KindDefinitionTerm:
		return saturatedAdd(1, markdownInlineMaterializedBlockCount(node, source))
	case extast.KindDefinitionDescription:
		return saturatedAdd(1, markdownChildBlockCount(node, source))
	default:
		if count := markdownChildBlockCount(node, source); count > 0 {
			return count
		}
		return 1
	}
}

var sdkMarkdownParagraphContainerTags = map[string]bool{
	"callout":      true,
	"column":       true,
	"blockquote":   true,
	"okr-progress": true,
}

func markdownInlineMaterializedBlockCount(parent gast.Node, source []byte) int {
	count := 0
	_ = gast.Walk(parent, func(node gast.Node, entering bool) (gast.WalkStatus, error) {
		if !entering || node == parent {
			return gast.WalkContinue, nil
		}
		if ownBlocks := markdownInlineNodeOwnBlockCount(node, source); ownBlocks > 0 {
			count = saturatedAdd(count, ownBlocks)
			return gast.WalkSkipChildren, nil
		}
		return gast.WalkContinue, nil
	})
	return count
}

func markdownInlineNodeOwnBlockCount(node gast.Node, source []byte) int {
	if node.Kind() == gast.KindImage {
		return 1
	}
	if raw, ok := node.(*gast.RawHTML); ok {
		count := 0
		if elements, err := parseXML(strings.TrimSpace(string(raw.Segments.Value(source)))); err == nil {
			for _, element := range elements {
				count = saturatedAdd(count, materializedBlockCount(element))
			}
		}
		return count
	}
	inline, ok := node.(*sdkXMLInline)
	if !ok || layoutOf(inline.TagName) != layoutBlock || isMarkdownXMLShell(inline.TagName) {
		return 0
	}
	if inline.TagName == "source" {
		return 2
	}
	return 1
}

func markdownDefinitionListBlockCount(list gast.Node, source []byte) int {
	count := 0
	for child := list.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.Kind() {
		case extast.KindDefinitionTerm:
			count = saturatedAdd(count, saturatedAdd(1, markdownInlineMaterializedBlockCount(child, source)))
		case extast.KindDefinitionDescription:
			count = saturatedAdd(count, saturatedAdd(1, markdownChildBlockCount(child, source)))
		}
	}
	return count
}

func looksLikeMarkdownXML(raw string) bool {
	return strings.HasPrefix(strings.TrimSpace(raw), "<")
}

func markdownListBlockCount(list *gast.List, source []byte) int {
	count := 0
	taskList := markdownIsTaskList(list)
	for child := list.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() != gast.KindListItem {
			continue
		}
		count = saturatedAdd(count, markdownListItemBlockCount(child, source, list.IsTight, taskList && markdownListItemHasTaskCheckbox(child)))
	}
	return count
}

func markdownIsTaskList(list *gast.List) bool {
	first := list.FirstChild()
	return first != nil && first.Kind() == gast.KindListItem && markdownListItemHasTaskCheckbox(first)
}

func markdownListItemHasTaskCheckbox(item gast.Node) bool {
	for child := item.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() != gast.KindTextBlock && child.Kind() != gast.KindParagraph {
			continue
		}
		first := child.FirstChild()
		return first != nil && first.Kind() == extast.KindTaskCheckBox
	}
	return false
}

func markdownListItemBlockCount(item gast.Node, source []byte, tight, taskItem bool) int {
	count := 1
	for child := item.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.Kind() {
		case gast.KindParagraph, gast.KindTextBlock:
			inlineBlocks := markdownInlineMaterializedBlockCount(child, source)
			if taskItem || tight {
				count = saturatedAdd(count, inlineBlocks)
			} else if inlineBlocks > 0 {
				count = saturatedAdd(count, saturatedAdd(1, inlineBlocks))
			}
		default:
			count = saturatedAdd(count, markdownMaterializedBlockCount(child, source, markdownNodeRaw(child, source)))
		}
	}
	return count
}

func markdownChildBlockCount(parent gast.Node, source []byte) int {
	count := 0
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		count = saturatedAdd(count, markdownMaterializedBlockCount(child, source, markdownNodeRaw(child, source)))
	}
	return count
}

func markdownNodeRaw(node gast.Node, source []byte) string {
	if spanned, ok := node.(interface {
		SourceSpan() (int, int, bool)
	}); ok {
		if start, stop, valid := spanned.SourceSpan(); valid && start >= 0 && stop <= len(source) {
			return string(source[start:stop])
		}
	}
	if node.Type() == gast.TypeBlock {
		if lines := node.Lines(); lines != nil && lines.Len() > 0 {
			return string(lines.Value(source))
		}
	}
	return ""
}

func markdownTableBlockCount(table gast.Node, source []byte) int {
	count := 1
	_ = gast.Walk(table, func(node gast.Node, entering bool) (gast.WalkStatus, error) {
		if !entering || node.Kind() != extast.KindTableCell {
			return gast.WalkContinue, nil
		}
		count = saturatedAdd(count, 1) // physical cell block
		blocks, inlineRuns := markdownTableCellContentCount(node, source)
		if blocks == 0 && inlineRuns == 0 {
			inlineRuns = 1
		}
		count = saturatedAdd(count, saturatedAdd(blocks, inlineRuns))
		return gast.WalkSkipChildren, nil
	})
	return count
}

func markdownTableCellContentCount(cell gast.Node, source []byte) (blocks, inlineRuns int) {
	activeInlineRun := false
	flush := func() {
		if activeInlineRun {
			inlineRuns++
			activeInlineRun = false
		}
	}
	for child := cell.FirstChild(); child != nil; child = child.NextSibling() {
		childBlocks := saturatedAdd(markdownInlineNodeOwnBlockCount(child, source), markdownInlineMaterializedBlockCount(child, source))
		if markdownInlineNodeIsMaterializedBlock(child, source) {
			flush()
			blocks = saturatedAdd(blocks, maxInt(childBlocks, 1))
			continue
		}
		blocks = saturatedAdd(blocks, childBlocks)
		if markdownInlineNodeMeaningful(child, source) {
			activeInlineRun = true
		}
	}
	flush()
	return blocks, inlineRuns
}

func markdownInlineNodeIsMaterializedBlock(node gast.Node, source []byte) bool {
	if node.Kind() == gast.KindImage {
		return true
	}
	if inline, ok := node.(*sdkXMLInline); ok {
		return layoutOf(inline.TagName) == layoutBlock && !isMarkdownXMLShell(inline.TagName)
	}
	if raw, ok := node.(*gast.RawHTML); ok {
		if elements, err := parseXML(strings.TrimSpace(string(raw.Segments.Value(source)))); err == nil {
			for _, element := range elements {
				if isMaterializedBlock(element) {
					return true
				}
			}
		}
	}
	return false
}

func markdownInlineNodeMeaningful(node gast.Node, source []byte) bool {
	if textNode, ok := node.(*gast.Text); ok {
		return strings.TrimSpace(string(textNode.Value(source))) != ""
	}
	return node.Kind() != extast.KindTaskCheckBox
}

func isMarkdownXMLShell(tag string) bool {
	switch tag {
	case "ul", "ol", "thead", "tbody", "tfoot", "tr", "div", "append":
		return true
	default:
		return false
	}
}
