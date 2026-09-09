// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

// This file mirrors the Markdown-to-Docx materialization that feeds
// docx_xml-go/parser/command/document_limit. It deliberately consumes the
// SDK-aligned Goldmark AST from markdown_sdk_parser.go; do not add a second
// Markdown parser or source-pattern approximation here.

import (
	"strings"

	gast "github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
)

type markdownInlineLimitResult struct {
	characters    int
	meaningful    bool
	containsBlock bool
	dispatched    bool
	statistics    ContentStatistics
}

func markdownDocumentContentStatistics(document gast.Node, source []byte) ContentStatistics {
	statistics := ContentStatistics{}
	inlineRunCharacters := 0
	inlineRunActive := false
	flushInlineRun := func() {
		if !inlineRunActive {
			return
		}
		statistics.MaxBlockCharacters = maxInt(statistics.MaxBlockCharacters, inlineRunCharacters)
		inlineRunCharacters = 0
		inlineRunActive = false
	}

	for child := document.FirstChild(); child != nil; child = child.NextSibling() {
		raw := markdownNodeRaw(child, source)
		if markdownNodeRendersTopLevelInline(child, raw) {
			result := markdownXMLFragmentInlineLimits(raw)
			statistics = statistics.merge(result.statistics)
			inlineRunCharacters = saturatedAdd(inlineRunCharacters, result.characters)
			inlineRunActive = inlineRunActive || result.meaningful
			continue
		}
		flushInlineRun()
		statistics = statistics.merge(markdownNodeContentStatistics(child, source, raw))
	}
	flushInlineRun()
	return statistics
}

func markdownNodeContentStatistics(node gast.Node, source []byte, raw string) ContentStatistics {
	if block, ok := node.(*sdkMarkdownBlock); ok {
		return markdownSDKBlockContentStatistics(block, source, raw)
	}
	switch node.Kind() {
	case gast.KindParagraph, gast.KindTextBlock, gast.KindHeading:
		result := markdownInlineChildrenLimits(node, source)
		result.statistics.MaxBlockCharacters = maxInt(result.statistics.MaxBlockCharacters, result.characters)
		return result.statistics
	case gast.KindFencedCodeBlock:
		fenced := node.(*gast.FencedCodeBlock)
		language := strings.ToLower(string(fenced.Language(source)))
		content := markdownTrimOneTrailingNewline(string(node.Lines().Value(source)))
		if content != "" && (language == "mermaid" || language == "plantuml" || language == "svg") {
			return ContentStatistics{}
		}
		return ContentStatistics{MaxBlockCharacters: utf16CodeUnits(content)}
	case gast.KindCodeBlock:
		return ContentStatistics{MaxBlockCharacters: utf16CodeUnits(markdownTrimOneTrailingNewline(string(node.Lines().Value(source))))}
	case gast.KindBlockquote:
		return markdownChildContentStatistics(node, source)
	case gast.KindList:
		return markdownListContentStatistics(node.(*gast.List), source)
	case gast.KindHTMLBlock:
		if elements, err := parseXMLCompatible(strings.TrimSpace(raw)); err == nil {
			return collectXMLContentStatistics(elements)
		}
		return ContentStatistics{}
	case extast.KindTable:
		return markdownTableContentStatistics(node, source)
	case extast.KindDefinitionList:
		return markdownDefinitionListContentStatistics(node, source)
	case extast.KindDefinitionTerm:
		result := markdownInlineChildrenLimits(node, source)
		result.statistics.MaxBlockCharacters = maxInt(result.statistics.MaxBlockCharacters, result.characters)
		return result.statistics
	case extast.KindDefinitionDescription:
		return markdownChildContentStatistics(node, source)
	default:
		return markdownChildContentStatistics(node, source)
	}
}

func markdownSDKBlockContentStatistics(block *sdkMarkdownBlock, source []byte, raw string) ContentStatistics {
	if block == nil {
		return ContentStatistics{}
	}
	// Direct title carriers, self-closing resources, aliases and other raw XML
	// nodes without Markdown children are materialized by the SDK sanitizer.
	if block.FirstChild() == nil && !sdkMarkdownParagraphContainerTags[block.TagName] {
		if elements, err := parseXMLCompatible(strings.TrimSpace(raw)); err == nil && len(elements) > 0 {
			return collectXMLContentStatistics(elements)
		}
	}

	statistics := ContentStatistics{}
	if caption := block.Attrs["caption"]; caption != "" {
		switch block.TagName {
		case "table", "img", "whiteboard", "pre", "code":
			statistics.MaxBlockCharacters = utf16CodeUnits(caption)
		}
	}
	if block.PreserveText && block.TagName == "code" {
		statistics.MaxBlockCharacters = maxInt(statistics.MaxBlockCharacters,
			utf16CodeUnits(markdownSDKBlockRawContent(raw, block.RawTagName)))
		return statistics
	}
	return statistics.merge(markdownChildContentStatistics(block, source))
}

func markdownSDKBlockRawContent(raw, rawTag string) string {
	openEnd := strings.IndexByte(raw, '>')
	if openEnd < 0 {
		return ""
	}
	if rawTag == "" {
		rawTag = "code"
	}
	closeAt := indexASCIIFold(raw[openEnd+1:], "</"+rawTag+">")
	if closeAt < 0 {
		return strings.Trim(raw[openEnd+1:], "\r\n")
	}
	return strings.Trim(raw[openEnd+1:openEnd+1+closeAt], "\r\n")
}

func markdownChildContentStatistics(parent gast.Node, source []byte) ContentStatistics {
	statistics := ContentStatistics{}
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		statistics = statistics.merge(markdownNodeContentStatistics(child, source, markdownNodeRaw(child, source)))
	}
	return statistics
}

func markdownListContentStatistics(list *gast.List, source []byte) ContentStatistics {
	statistics := ContentStatistics{}
	taskList := markdownIsTaskList(list)
	for child := list.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() != gast.KindListItem {
			continue
		}
		statistics = statistics.merge(markdownListItemContentStatistics(
			child, source, list.IsTight, taskList && markdownListItemHasTaskCheckbox(child),
		))
	}
	return statistics
}

func markdownListItemContentStatistics(item gast.Node, source []byte, tight, taskItem bool) ContentStatistics {
	statistics := ContentStatistics{}
	directCharacters := 0
	for child := item.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.Kind() {
		case gast.KindParagraph, gast.KindTextBlock:
			result := markdownInlineChildrenLimits(child, source)
			statistics = statistics.merge(result.statistics)
			if taskItem || tight || !result.containsBlock {
				directCharacters = saturatedAdd(directCharacters, result.characters)
			} else {
				statistics.MaxBlockCharacters = maxInt(statistics.MaxBlockCharacters, result.characters)
			}
		default:
			statistics = statistics.merge(markdownNodeContentStatistics(child, source, markdownNodeRaw(child, source)))
		}
	}
	statistics.MaxBlockCharacters = maxInt(statistics.MaxBlockCharacters, directCharacters)
	return statistics
}

func markdownDefinitionListContentStatistics(list gast.Node, source []byte) ContentStatistics {
	statistics := ContentStatistics{}
	for child := list.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.Kind() {
		case extast.KindDefinitionTerm:
			result := markdownInlineChildrenLimits(child, source)
			statistics = statistics.merge(result.statistics)
			statistics.MaxBlockCharacters = maxInt(statistics.MaxBlockCharacters, result.characters)
		case extast.KindDefinitionDescription:
			statistics = statistics.merge(markdownChildContentStatistics(child, source))
		}
	}
	return statistics
}

func markdownTableContentStatistics(table gast.Node, source []byte) ContentStatistics {
	statistics := ContentStatistics{}
	rowCount := 0
	columnCount := 0
	for row := table.FirstChild(); row != nil; row = row.NextSibling() {
		if row.Kind() != extast.KindTableHeader && row.Kind() != extast.KindTableRow {
			continue
		}
		rowCount++
		columns := 0
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			if cell.Kind() != extast.KindTableCell {
				continue
			}
			columns++
			statistics = statistics.merge(markdownTableCellContentStatistics(cell, source))
		}
		columnCount = maxInt(columnCount, columns)
	}
	rowCount = maxInt(rowCount, 1)
	columnCount = maxInt(columnCount, 1)
	statistics.MaxTableColumns = maxInt(statistics.MaxTableColumns, columnCount)
	statistics.MaxTableCells = maxInt(statistics.MaxTableCells, saturatedMultiply(rowCount, columnCount))
	return statistics
}

func markdownTableCellContentStatistics(cell gast.Node, source []byte) ContentStatistics {
	statistics := ContentStatistics{}
	currentCharacters := 0
	active := false
	flush := func() {
		if !active {
			return
		}
		statistics.MaxBlockCharacters = maxInt(statistics.MaxBlockCharacters, currentCharacters)
		currentCharacters = 0
		active = false
	}
	forEachMarkdownInlineLimitResult(cell, source, func(result markdownInlineLimitResult) {
		statistics = statistics.merge(result.statistics)
		if result.dispatched {
			flush()
			return
		}
		if result.meaningful {
			active = true
			currentCharacters = saturatedAdd(currentCharacters, result.characters)
		}
	})
	flush()
	return statistics
}

func markdownInlineChildrenLimits(parent gast.Node, source []byte) markdownInlineLimitResult {
	result := markdownInlineLimitResult{}
	forEachMarkdownInlineLimitResult(parent, source, func(childResult markdownInlineLimitResult) {
		result.characters = saturatedAdd(result.characters, childResult.characters)
		result.meaningful = result.meaningful || childResult.meaningful
		result.containsBlock = result.containsBlock || childResult.containsBlock
		result.statistics = result.statistics.merge(childResult.statistics)
	})
	return result
}

// forEachMarkdownInlineLimitResult mirrors SDK renderInlineFragment's
// consecutive-Text buffering. Goldmark can split a literal delimiter pair
// across sibling Text nodes; character statistics must repair the combined
// text, not count each fragment independently.
func forEachMarkdownInlineLimitResult(parent gast.Node, source []byte, visit func(markdownInlineLimitResult)) {
	var textBuffer strings.Builder
	flushText := func() {
		if textBuffer.Len() == 0 {
			return
		}
		value := normalizeSDKRenderedTextForCharacters(textBuffer.String())
		characters := utf16CodeUnits(value)
		visit(markdownInlineLimitResult{characters: characters, meaningful: characters > 0})
		textBuffer.Reset()
	}
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() == gast.KindText {
			textNode := child.(*gast.Text)
			textBuffer.Write(textNode.Value(source))
			if textNode.HardLineBreak() || textNode.SoftLineBreak() {
				flushText()
				visit(markdownInlineLimitResult{characters: 1, meaningful: true})
			}
			continue
		}
		flushText()
		visit(markdownInlineNodeLimits(child, source))
	}
	flushText()
}

func markdownInlineNodeLimits(node gast.Node, source []byte) markdownInlineLimitResult {
	switch typed := node.(type) {
	case *sdkXMLInline:
		return markdownSDKXMLInlineLimits(typed)
	case *sdkMathInline:
		value := stripSDKLatexMarkdownEscapes(string(typed.Content))
		characters := utf16CodeUnits(value)
		return markdownInlineLimitResult{characters: characters, meaningful: characters > 0}
	}

	switch node.Kind() {
	case gast.KindText:
		textNode := node.(*gast.Text)
		value := normalizeSDKRenderedTextForCharacters(string(textNode.Value(source)))
		characters := utf16CodeUnits(value)
		if textNode.HardLineBreak() || textNode.SoftLineBreak() {
			characters = saturatedAdd(characters, 1)
		}
		return markdownInlineLimitResult{characters: characters, meaningful: characters > 0}
	case gast.KindString:
		characters := utf16CodeUnits(string(node.(*gast.String).Value))
		return markdownInlineLimitResult{characters: characters, meaningful: characters > 0}
	case gast.KindCodeSpan:
		characters := utf16CodeUnits(markdownCollectChildText(node, source))
		return markdownInlineLimitResult{characters: characters, meaningful: characters > 0}
	case gast.KindLink:
		result := markdownInlineChildrenLimits(node, source)
		if !result.meaningful {
			fallback := utf16CodeUnits(string(node.(*gast.Link).Destination))
			result.characters = saturatedAdd(result.characters, fallback)
			result.meaningful = fallback > 0
		}
		return result
	case gast.KindAutoLink:
		characters := utf16CodeUnits(string(node.(*gast.AutoLink).Label(source)))
		return markdownInlineLimitResult{characters: characters, meaningful: characters > 0}
	case gast.KindImage:
		alt := strings.TrimSpace(stripSDKBackslashEscapes(markdownCollectChildText(node, source)))
		return markdownInlineLimitResult{
			containsBlock: true,
			dispatched:    true,
			statistics:    ContentStatistics{MaxBlockCharacters: utf16CodeUnits(alt)},
		}
	case gast.KindRawHTML:
		raw := string(node.(*gast.RawHTML).Segments.Value(source))
		return markdownXMLFragmentInlineLimits(raw)
	case extast.KindTaskCheckBox:
		return markdownInlineLimitResult{}
	default:
		return markdownInlineChildrenLimits(node, source)
	}
}

func markdownSDKXMLInlineLimits(inline *sdkXMLInline) markdownInlineLimitResult {
	if inline == nil {
		return markdownInlineLimitResult{}
	}
	element := newElement(inline.TagName, inline.Attrs)
	if len(inline.Content) > 0 {
		content := string(inline.Content)
		switch inline.TagName {
		case "code", "pre":
		case "latex":
			content = stripSDKLatexMarkdownEscapes(content)
		default:
			content = stripSDKBackslashEscapes(content)
		}
		element.addChild(newText(content))
	}
	return markdownXMLNodesInlineLimits([]*Node{element})
}

func markdownXMLFragmentInlineLimits(raw string) markdownInlineLimitResult {
	nodes, err := parseXMLCompatible(strings.TrimSpace(raw))
	if err != nil || len(nodes) == 0 {
		return markdownInlineLimitResult{}
	}
	return markdownXMLNodesInlineLimits(nodes)
}

func markdownXMLNodesInlineLimits(nodes []*Node) markdownInlineLimitResult {
	container := newElement("p", nil)
	for _, node := range nodes {
		container.addChild(node)
	}
	result := markdownInlineLimitResult{
		statistics: collectXMLContentStatistics(nodes),
	}
	result.characters = inlineCharacters(container)
	result.meaningful = result.characters > 0
	for _, node := range nodes {
		if markdownXMLContainsMaterializedBlock(node) {
			result.containsBlock = true
		}
		if isTableCellDispatchedBlock(node) {
			result.dispatched = true
		}
	}
	return result
}

func markdownXMLContainsMaterializedBlock(node *Node) bool {
	if node == nil {
		return false
	}
	if node.typ == nodeElement && isMaterializedBlock(node) {
		return true
	}
	for _, child := range node.children {
		if markdownXMLContainsMaterializedBlock(child) {
			return true
		}
	}
	return false
}

func markdownCollectChildText(node gast.Node, source []byte) string {
	var value strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.Kind() {
		case gast.KindText:
			value.Write(child.(*gast.Text).Value(source))
		case gast.KindString:
			value.Write(child.(*gast.String).Value)
		default:
			value.WriteString(markdownCollectChildText(child, source))
		}
	}
	return value.String()
}

func markdownTrimOneTrailingNewline(value string) string {
	if strings.HasSuffix(value, "\r\n") {
		return value[:len(value)-2]
	}
	if strings.HasSuffix(value, "\n") {
		return value[:len(value)-1]
	}
	return value
}

func stripSDKBackslashEscapes(value string) string {
	if strings.IndexByte(value, '\\') < 0 {
		return value
	}
	var out strings.Builder
	out.Grow(len(value))
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) && isSDKASCIIPunctuation(value[i+1]) {
			out.WriteByte(value[i+1])
			i++
			continue
		}
		out.WriteByte(value[i])
	}
	return out.String()
}

func stripSDKLatexMarkdownEscapes(value string) string {
	if strings.IndexByte(value, '\\') < 0 {
		return value
	}
	var out strings.Builder
	out.Grow(len(value))
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) && shouldStripSDKLatexEscapedPunctuation(value[i+1]) {
			out.WriteByte(value[i+1])
			i++
			continue
		}
		out.WriteByte(value[i])
	}
	return out.String()
}

func isSDKASCIIPunctuation(value byte) bool {
	return strings.ContainsRune(`!"#$%&'()*+,-./:;<=>?@[\]^_`+"`"+`{|}~`, rune(value))
}

func shouldStripSDKLatexEscapedPunctuation(value byte) bool {
	switch value {
	case '_', '^', '&', '*', '[', ']', '$', '~', '<', '>', '`', '#', '+', '-', '=', ':':
		return true
	default:
		return false
	}
}
