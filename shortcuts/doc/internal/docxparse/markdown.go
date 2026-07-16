// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

// Markdown conversion is scoped to the docs +script business domain.

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	gmutil "github.com/yuin/goldmark/util"
)

var markdownParser parser.Parser

func init() {
	markdown := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.DefinitionList,
			&mathExtension{},
			&underscoreHTMLExtension{},
			&cjkAdjacentMarkupExtension{},
		),
		goldmark.WithParserOptions(
			parser.WithBlockParsers(gmutil.Prioritized(&containerBlockParser{}, 90)),
		),
	)
	markdownParser = markdown.Parser()
}

func parseMarkdown(source string) ([]*Node, error) {
	if err := validateSource(source); err != nil {
		return nil, err
	}
	source = strings.TrimPrefix(source, "\uFEFF")
	source = normalizeListIndent(source)
	data := []byte(source)
	document := markdownParser.Parse(text.NewReader(data))
	return renderBlockChildren(document, data)
}

func renderBlockChildren(parent gast.Node, source []byte) ([]*Node, error) {
	var out []*Node
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		nodes, err := renderBlockNode(child, source)
		if err != nil {
			return nil, err
		}
		out = append(out, nodes...)
	}
	return out, nil
}

func renderBlockNode(node gast.Node, source []byte) ([]*Node, error) {
	switch node.Kind() {
	case gast.KindParagraph, gast.KindTextBlock:
		children, err := renderInlineChildren(node, source)
		if err != nil {
			return nil, err
		}
		return wrapParagraphChildren(children), nil
	case gast.KindHeading:
		heading := newElement(headingTag(node.(*gast.Heading).Level), nil)
		children, err := renderInlineChildren(node, source)
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			heading.addChild(child)
		}
		return []*Node{heading}, nil
	case gast.KindBlockquote:
		return renderContainer("blockquote", nil, node, source)
	case gast.KindList:
		return renderList(node.(*gast.List), source)
	case gast.KindFencedCodeBlock:
		block := node.(*gast.FencedCodeBlock)
		language := string(block.Language(source))
		content := trimOneTrailingNewline(string(node.Lines().Value(source)))
		lowerLanguage := strings.ToLower(language)
		if content != "" && (lowerLanguage == "mermaid" || lowerLanguage == "plantuml" || lowerLanguage == "svg") {
			whiteboard := newElement("whiteboard", map[string]string{"type": lowerLanguage})
			appendRawTextWithBreaks(whiteboard, content)
			return []*Node{whiteboard}, nil
		}
		attrs := map[string]string(nil)
		if language != "" {
			attrs = map[string]string{"lang": language}
		}
		pre := newElement("pre", attrs)
		code := newElement("code", nil)
		appendRawTextWithBreaks(code, content)
		pre.addChild(code)
		return []*Node{pre}, nil
	case gast.KindCodeBlock:
		pre := newElement("pre", nil)
		code := newElement("code", nil)
		appendRawTextWithBreaks(code, trimOneTrailingNewline(string(node.Lines().Value(source))))
		pre.addChild(code)
		return []*Node{pre}, nil
	case gast.KindThematicBreak:
		return []*Node{newElement("hr", nil)}, nil
	case gast.KindHTMLBlock:
		nodes, err := parseMarkdownHTMLBlock(string(node.Lines().Value(source)))
		if err != nil {
			return nil, err
		}
		stripMarkdownEscapesInNodes(nodes, false, false)
		return nodes, nil
	case kindContainerBlock:
		container := node.(*containerBlock)
		if !container.closed {
			return nil, newParseError("Markdown <%s> container is missing closing tag </%s>", container.spec.tag, container.spec.tag)
		}
		return renderContainer(container.spec.tag, container.attrs, node, source)
	}

	switch node.Kind() {
	case extast.KindTable:
		return renderTable(node, source)
	case extast.KindDefinitionList:
		return renderDefinitionList(node, source)
	}

	return nil, newParseError("unsupported Markdown block node %v", node.Kind())
}

// parseMarkdownHTMLBlock handles the source-bearing LarkOpenCLI blocks whose
// Markdown bodies are literal text, then delegates every other XML fragment to
// the strict XML parser. Escaping literal code is part of Markdown conversion.
func parseMarkdownHTMLBlock(fragment string) ([]*Node, error) {
	trimmed := strings.TrimSpace(fragment)
	for _, tag := range []string{"code", "whiteboard"} {
		closing := "</" + tag + ">"
		if !strings.HasPrefix(trimmed, "<"+tag) || !strings.HasSuffix(trimmed, closing) {
			continue
		}
		token, contentStart, state := scanXMLToken(trimmed, 0)
		if state != tokenOK || token.closing || token.selfClosing || token.name != tag {
			return nil, newParseError("invalid Markdown <%s> block", tag)
		}
		contentEnd := len(trimmed) - len(closing)
		if contentStart > contentEnd {
			return nil, newParseError("invalid Markdown <%s> block", tag)
		}
		block := newElement(tag, token.attrs)
		appendRawTextWithBreaks(block, strings.Trim(trimmed[contentStart:contentEnd], "\r\n"))
		return []*Node{block}, nil
	}
	return parseXML(fragment)
}

func renderContainer(tag string, attrs map[string]string, node gast.Node, source []byte) ([]*Node, error) {
	container := newElement(tag, attrs)
	children, err := renderBlockChildren(node, source)
	if err != nil {
		return nil, err
	}
	for _, child := range children {
		container.addChild(child)
	}
	return []*Node{container}, nil
}

func renderList(list *gast.List, source []byte) ([]*Node, error) {
	if isTaskList(list) {
		return renderTaskList(list, source)
	}
	tag := "ul"
	if list.IsOrdered() {
		tag = "ol"
	}
	listNode := newElement(tag, nil)
	sequence := list.Start
	if sequence <= 0 {
		sequence = 1
	}
	itemIndex := 0
	for child := list.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() != gast.KindListItem {
			continue
		}
		item, err := renderListItem(child.(*gast.ListItem), list.IsTight, source)
		if err != nil {
			return nil, err
		}
		if list.IsOrdered() && itemIndex == 0 && sequence != 1 {
			setListItemSequence(item, sequence)
		}
		listNode.addChild(item)
		itemIndex++
	}
	return []*Node{listNode}, nil
}

func isTaskList(list *gast.List) bool {
	for child := list.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() == gast.KindListItem && findTaskCheckbox(child.(*gast.ListItem)) != nil {
			return true
		}
	}
	return false
}

func findTaskCheckbox(item *gast.ListItem) *extast.TaskCheckBox {
	for child := item.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() != gast.KindTextBlock && child.Kind() != gast.KindParagraph {
			continue
		}
		if first := child.FirstChild(); first != nil && first.Kind() == extast.KindTaskCheckBox {
			return first.(*extast.TaskCheckBox)
		}
	}
	return nil
}

func renderTaskList(list *gast.List, source []byte) ([]*Node, error) {
	var out []*Node
	listTag := "ul"
	if list.IsOrdered() {
		listTag = "ol"
	}
	sequence := list.Start
	if sequence <= 0 {
		sequence = 1
	}
	var plainItems *Node
	flushPlainItems := func() {
		if plainItems != nil {
			out = append(out, plainItems)
			plainItems = nil
		}
	}
	for child := list.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() != gast.KindListItem {
			continue
		}
		item := child.(*gast.ListItem)
		checkboxAST := findTaskCheckbox(item)
		if checkboxAST == nil {
			li, err := renderListItem(item, list.IsTight, source)
			if err != nil {
				return nil, err
			}
			if plainItems == nil {
				plainItems = newElement(listTag, nil)
				if list.IsOrdered() && sequence != 1 {
					setListItemSequence(li, sequence)
				}
			}
			plainItems.addChild(li)
			sequence++
			continue
		}
		flushPlainItems()
		done := "false"
		if checkboxAST.IsChecked {
			done = "true"
		}
		checkbox := newElement("checkbox", map[string]string{"done": done})
		for block := item.FirstChild(); block != nil; block = block.NextSibling() {
			if block.Kind() == gast.KindTextBlock || block.Kind() == gast.KindParagraph {
				fragment, err := renderInlineFragment(block, source, true)
				if err != nil {
					return nil, err
				}
				nodes, err := parseMarkdownInlineFragment(fragment)
				if err != nil {
					return nil, err
				}
				for _, node := range nodes {
					checkbox.addChild(node)
				}
				continue
			}
			nodes, err := renderBlockNode(block, source)
			if err != nil {
				return nil, err
			}
			for _, node := range nodes {
				checkbox.addChild(node)
			}
		}
		out = append(out, checkbox)
		sequence++
	}
	flushPlainItems()
	return out, nil
}

func setListItemSequence(item *Node, sequence int) {
	if item.attrs == nil {
		item.attrs = make(map[string]string)
	}
	item.attrs["seq"] = strconv.Itoa(sequence)
}

func renderListItem(item *gast.ListItem, tight bool, source []byte) (*Node, error) {
	li := newElement("li", nil)
	children, err := renderBlockChildren(item, source)
	if err != nil {
		return nil, err
	}
	for _, child := range children {
		if child.tag == "p" && (tight || paragraphOnlyInline(child)) {
			for _, grandchild := range child.children {
				li.addChild(grandchild)
			}
			continue
		}
		li.addChild(child)
	}
	return li, nil
}

func renderInlineChildren(node gast.Node, source []byte) ([]*Node, error) {
	fragment, err := renderInlineFragment(node, source, false)
	if err != nil {
		return nil, err
	}
	nodes, err := parseMarkdownInlineFragment(fragment)
	if err != nil {
		return nil, err
	}
	stripMarkdownEscapesInNodes(nodes, false, false)
	return nodes, nil
}

// parseMarkdownInlineFragment wraps an inline fragment in a space-preserving
// parent while parsing so XML normalization keeps semantic spaces between
// adjacent inline elements. The wrapper is removed from the returned nodes.
func parseMarkdownInlineFragment(fragment string) ([]*Node, error) {
	nodes, err := parseXML("<p>" + fragment + "</p>")
	if err != nil {
		return nil, err
	}
	if len(nodes) != 1 || nodes[0].typ != nodeElement || nodes[0].tag != "p" {
		return nil, newParseError("invalid Markdown inline fragment")
	}
	children := nodes[0].children
	for _, child := range children {
		child.parent = nil
	}
	return children, nil
}

func renderInlineFragment(parent gast.Node, source []byte, skipCheckbox bool) (string, error) {
	var out strings.Builder
	inNativeLatex := false
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		if skipCheckbox && child.Kind() == extast.KindTaskCheckBox {
			continue
		}
		var (
			fragment string
			err      error
		)
		if inNativeLatex && child.Kind() == gast.KindText {
			fragment = renderMarkdownText(child.(*gast.Text), source, false)
		} else {
			fragment, err = renderInlineNode(child, source)
		}
		if err != nil {
			return "", err
		}
		out.WriteString(fragment)
		if child.Kind() == gast.KindRawHTML {
			if opening, closing := nativeLatexTagTransition(fragment); opening {
				inNativeLatex = true
			} else if closing {
				inNativeLatex = false
			}
		}
	}
	return out.String(), nil
}

func nativeLatexTagTransition(fragment string) (opening, closing bool) {
	trimmed := strings.TrimSpace(fragment)
	if !strings.HasPrefix(trimmed, "<") {
		return false, false
	}
	token, _, state := scanXMLToken(trimmed, 0)
	if state != tokenOK || !strings.EqualFold(token.name, "latex") {
		return false, false
	}
	return !token.closing && !token.selfClosing, token.closing
}

func renderInlineNode(node gast.Node, source []byte) (string, error) {
	switch node.Kind() {
	case gast.KindText:
		return renderMarkdownText(node.(*gast.Text), source, true), nil
	case gast.KindString:
		return escapeXMLText(string(node.(*gast.String).Value)), nil
	case gast.KindEmphasis:
		tag := "em"
		if node.(*gast.Emphasis).Level >= 2 {
			tag = "b"
		}
		return renderInlineContainer(node, tag, nil, source)
	case gast.KindCodeSpan:
		return elementXML("code", nil, escapeXMLText(collectMarkdownChildText(node, source))), nil
	case gast.KindLink:
		link := node.(*gast.Link)
		attrs := map[string]string{"href": string(link.Destination)}
		if len(link.Title) > 0 {
			attrs["title"] = string(link.Title)
		}
		children, err := renderInlineFragment(node, source, false)
		if err != nil {
			return "", err
		}
		if children == "" {
			children = escapeXMLText(string(link.Destination))
		}
		return elementXML("a", attrs, children), nil
	case gast.KindImage:
		image := node.(*gast.Image)
		destination := string(image.Destination)
		attrs := map[string]string{}
		if strings.HasPrefix(destination, "http://") || strings.HasPrefix(destination, "https://") {
			attrs["href"] = destination
		} else {
			attrs["src"] = destination
		}
		if len(image.Title) > 0 {
			attrs["title"] = string(image.Title)
		}
		return elementXML("img", attrs, ""), nil
	case gast.KindRawHTML:
		return string(node.(*gast.RawHTML).Segments.Value(source)), nil
	case gast.KindAutoLink:
		link := node.(*gast.AutoLink)
		return elementXML("a", map[string]string{"href": string(link.URL(source))}, escapeXMLText(string(link.Label(source)))), nil
	}

	switch node.Kind() {
	case extast.KindStrikethrough:
		return renderInlineContainer(node, "del", nil, source)
	case kindMathInline:
		return elementXML("latex", nil, escapeXMLText(stripLatexMarkdownEscapes(string(node.(*mathInline).content)))), nil
	case kindMathBlock:
		return elementXML("latex", nil, escapeXMLText(stripLatexMarkdownEscapes(string(node.(*mathBlock).content)))), nil
	case extast.KindTaskCheckBox:
		return "", nil
	}

	return "", newParseError("unsupported Markdown inline node %v", node.Kind())
}

func renderMarkdownText(textNode *gast.Text, source []byte, stripEscapes bool) string {
	value := string(textNode.Value(source))
	if stripEscapes {
		value = stripBackslashEscapes(value)
	}
	value = escapeXMLText(value)
	switch {
	case textNode.HardLineBreak():
		value += "<br/>"
	case textNode.SoftLineBreak():
		value += " "
	}
	return value
}

func renderInlineContainer(node gast.Node, tag string, attrs map[string]string, source []byte) (string, error) {
	children, err := renderInlineFragment(node, source, false)
	if err != nil {
		return "", err
	}
	return elementXML(tag, attrs, children), nil
}

func elementXML(tag string, attrs map[string]string, inner string) string {
	node := newElement(tag, attrs)
	rendered := renderNodes([]*Node{node})
	if inner == "" {
		return rendered
	}
	close := "</" + tag + ">"
	if strings.HasSuffix(rendered, close) {
		return strings.TrimSuffix(rendered, close) + inner + close
	}
	return rendered
}

func wrapParagraphChildren(children []*Node) []*Node {
	var out []*Node
	var inline []*Node
	flush := func() {
		if len(inline) == 0 {
			return
		}
		paragraph := newElement("p", nil)
		for _, child := range inline {
			paragraph.addChild(child)
		}
		out = append(out, paragraph)
		inline = nil
	}
	for _, child := range children {
		if child != nil && child.typ == nodeElement && layoutOf(child.tag) == layoutBlock {
			flush()
			out = append(out, child)
			continue
		}
		inline = append(inline, child)
	}
	flush()
	return out
}

func paragraphOnlyInline(node *Node) bool {
	if node == nil || node.typ != nodeElement || node.tag != "p" {
		return false
	}
	for _, child := range node.children {
		if child.typ == nodeElement && layoutOf(child.tag) == layoutBlock {
			return false
		}
	}
	return true
}

func renderTable(node gast.Node, source []byte) ([]*Node, error) {
	table := newElement("table", nil)
	var body *Node
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.Kind() {
		case extast.KindTableHeader:
			head := newElement("thead", nil)
			row, err := renderTableRow(child, true, source)
			if err != nil {
				return nil, err
			}
			head.addChild(row)
			table.addChild(head)
		case extast.KindTableRow:
			if body == nil {
				body = newElement("tbody", nil)
				table.addChild(body)
			}
			row, err := renderTableRow(child, false, source)
			if err != nil {
				return nil, err
			}
			body.addChild(row)
		}
	}
	return []*Node{table}, nil
}

func renderTableRow(node gast.Node, header bool, source []byte) (*Node, error) {
	row := newElement("tr", nil)
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() != extast.KindTableCell {
			continue
		}
		cellAST := child.(*extast.TableCell)
		tag := "td"
		if header {
			tag = "th"
		}
		attrs := map[string]string(nil)
		switch cellAST.Alignment {
		case extast.AlignCenter:
			attrs = map[string]string{"align": "center"}
		case extast.AlignRight:
			attrs = map[string]string{"align": "right"}
		}
		cell := newElement(tag, attrs)
		content, err := renderInlineChildren(cellAST, source)
		if err != nil {
			return nil, err
		}
		for _, inline := range content {
			cell.addChild(inline)
		}
		row.addChild(cell)
	}
	return row, nil
}

func renderDefinitionList(node gast.Node, source []byte) ([]*Node, error) {
	var out []*Node
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.Kind() {
		case extast.KindDefinitionTerm:
			nodes, err := renderInlineChildren(child, source)
			if err != nil {
				return nil, err
			}
			paragraph := newElement("p", nil)
			bold := newElement("b", nil)
			for _, node := range nodes {
				bold.addChild(node)
			}
			paragraph.addChild(bold)
			out = append(out, paragraph)
		case extast.KindDefinitionDescription:
			quote, err := renderContainer("blockquote", nil, child, source)
			if err != nil {
				return nil, err
			}
			out = append(out, quote...)
		}
	}
	return out, nil
}

func appendRawTextWithBreaks(parent *Node, content string) {
	if content == "" {
		return
	}
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] != '\n' && content[i] != '\r' {
			continue
		}
		if i > start {
			parent.addChild(newText(content[start:i]))
		}
		if content[i] == '\r' && i+1 < len(content) && content[i+1] == '\n' {
			i++
		}
		parent.addChild(newElement("br", nil))
		start = i + 1
	}
	if start < len(content) {
		parent.addChild(newText(content[start:]))
	}
}

func stripMarkdownEscapesInNodes(nodes []*Node, inCode, inLatex bool) {
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.typ == nodeText {
			switch {
			case inCode:
			case inLatex:
			default:
				node.text = stripBackslashEscapes(node.text)
			}
			continue
		}
		stripMarkdownEscapesInNodes(node.children, inCode || node.tag == "code" || node.tag == "pre", inLatex || node.tag == "latex")
	}
}

func stripBackslashEscapes(value string) string {
	if !strings.Contains(value, `\`) {
		return value
	}
	var out strings.Builder
	out.Grow(len(value))
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) && isASCIIPunctuation(value[i+1]) {
			out.WriteByte(value[i+1])
			i++
			continue
		}
		out.WriteByte(value[i])
	}
	return out.String()
}

func stripLatexMarkdownEscapes(value string) string {
	if !strings.Contains(value, `\`) {
		return value
	}
	var out strings.Builder
	out.Grow(len(value))
	for i := 0; i < len(value); {
		if value[i] != '\\' {
			out.WriteByte(value[i])
			i++
			continue
		}
		end := i
		for end < len(value) && value[end] == '\\' {
			end++
		}
		count := end - i
		if end < len(value) && value[end] == '$' && count%2 == 1 {
			out.WriteString(value[i : end-1])
			out.WriteByte('$')
			i = end + 1
			continue
		}
		out.WriteString(value[i:end])
		i = end
	}
	return out.String()
}

func isASCIIPunctuation(ch byte) bool {
	return ch >= '!' && ch <= '/' || ch >= ':' && ch <= '@' || ch >= '[' && ch <= '`' || ch >= '{' && ch <= '~'
}

func trimOneTrailingNewline(value string) string {
	if strings.HasSuffix(value, "\r\n") {
		return value[:len(value)-2]
	}
	return strings.TrimSuffix(value, "\n")
}

func collectMarkdownChildText(node gast.Node, source []byte) string {
	var out strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.Kind() {
		case gast.KindText:
			out.Write(child.(*gast.Text).Value(source))
		case gast.KindString:
			out.Write(child.(*gast.String).Value)
		default:
			out.WriteString(collectMarkdownChildText(child, source))
		}
	}
	return out.String()
}

func headingTag(level int) string {
	if level < 1 || level > 6 {
		return "p"
	}
	return fmt.Sprintf("h%d", level)
}

func normalizeListIndent(markdown string) string {
	lines := strings.Split(markdown, "\n")
	type stackEntry struct {
		indent        int
		contentIndent int
	}
	var stack []stackEntry
	fenceMarker := rune(0)
	fenceLength := 0
	fenceDelta := 0
	fenceCloseMaxIndent := 0
	changed := false
	lastOriginal, lastNormalized := 0, 0
	previousBlank := false
	for i, line := range lines {
		syntaxLine := strings.TrimSuffix(line, "\r")
		lineSuffix := line[len(syntaxLine):]
		trimmed := strings.TrimLeft(syntaxLine, " ")
		indent := len(syntaxLine) - len(trimmed)
		if fenceMarker != 0 {
			if fenceDelta != 0 && trimmed != "" {
				lines[i] = strings.Repeat(" ", max(0, indent+fenceDelta)) + trimmed + lineSuffix
				changed = true
			}
			if marker, length, ok := markdownFence(trimmed); ok && marker == fenceMarker && length >= fenceLength && indent <= fenceCloseMaxIndent && strings.TrimSpace(runeTail(trimmed, length)) == "" {
				fenceMarker, fenceLength, fenceDelta, fenceCloseMaxIndent = 0, 0, 0, 0
			}
			previousBlank = trimmed == ""
			continue
		}
		if trimmed == "" {
			previousBlank = true
			continue
		}
		if isMarkdownThematicBreak(trimmed) {
			if len(stack) > 0 && indent <= stack[0].indent {
				stack = stack[:0]
				lastOriginal, lastNormalized = 0, 0
			} else if len(stack) > 0 && indent > lastOriginal {
				delta := lastNormalized - lastOriginal
				if delta != 0 {
					lines[i] = strings.Repeat(" ", max(0, indent+delta)) + trimmed + lineSuffix
					changed = true
				}
			}
			previousBlank = false
			continue
		}
		if markerLength := markdownListMarkerLength(trimmed); markerLength > 0 {
			for len(stack) > 0 && indent <= stack[len(stack)-1].indent {
				stack = stack[:len(stack)-1]
			}
			normalized := len(stack) * 4
			stack = append(stack, stackEntry{indent: indent, contentIndent: indent + markerLength})
			lastOriginal, lastNormalized = indent, normalized
			if indent != normalized {
				lines[i] = strings.Repeat(" ", normalized) + trimmed + lineSuffix
				changed = true
			}
			previousBlank = false
			continue
		}

		if len(stack) > 0 && indent <= stack[0].indent && (previousBlank || interruptsLazyList(trimmed)) {
			stack = stack[:0]
			lastOriginal, lastNormalized = 0, 0
		}
		delta := 0
		if len(stack) > 0 && indent > lastOriginal {
			delta = lastNormalized - lastOriginal
			if delta != 0 {
				normalized := indent + delta
				if normalized < 0 {
					normalized = 0
				}
				lines[i] = strings.Repeat(" ", normalized) + trimmed + lineSuffix
				changed = true
			}
		}
		if marker, length, ok := markdownFence(trimmed); ok && (indent <= 3 || len(stack) > 0 && indent > lastOriginal) {
			fenceMarker, fenceLength, fenceDelta = marker, length, delta
			fenceCloseMaxIndent = 3
			if len(stack) > 0 {
				fenceCloseMaxIndent = stack[len(stack)-1].contentIndent + 3
			}
		}
		previousBlank = false
	}
	if !changed {
		return markdown
	}
	return strings.Join(lines, "\n")
}

func interruptsLazyList(value string) bool {
	if marker, _, ok := markdownFence(value); ok && marker != 0 {
		return true
	}
	if strings.HasPrefix(value, ">") || markdownHTMLBlockInterrupts(value) {
		return true
	}
	if value[0] == '#' {
		i := 0
		for i < len(value) && value[i] == '#' {
			i++
		}
		return i <= 6 && (i == len(value) || value[i] == ' ' || value[i] == '\t')
	}
	return isMarkdownThematicBreak(value)
}

func isMarkdownThematicBreak(value string) bool {
	marker := byte(0)
	count := 0
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case ' ', '\t':
			continue
		case '-', '*', '_':
			if marker == 0 {
				marker = value[i]
			} else if value[i] != marker {
				return false
			}
			count++
		default:
			return false
		}
	}
	return count >= 3
}

func markdownHTMLBlockInterrupts(value string) bool {
	lower := strings.ToLower(value)
	for _, prefix := range []string{"<!--", "<?", "<![cdata[", "<!doctype", "<!entity", "<!element", "<!attlist"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	if !strings.HasPrefix(value, "<") {
		return false
	}
	token, _, state := scanXMLToken(value, 0)
	if state != tokenOK && state != tokenInvalid || token.name == "" {
		return false
	}
	tag := strings.ToLower(token.name)
	if containerSpecs[tag] != nil {
		return true
	}
	_, ok := markdownInterruptingHTMLTags[tag]
	return ok
}

var markdownInterruptingHTMLTags = map[string]struct{}{
	"address": {}, "article": {}, "aside": {}, "base": {}, "basefont": {},
	"blockquote": {}, "body": {}, "caption": {}, "center": {}, "col": {},
	"colgroup": {}, "dd": {}, "details": {}, "dialog": {}, "dir": {},
	"div": {}, "dl": {}, "dt": {}, "fieldset": {}, "figcaption": {},
	"figure": {}, "footer": {}, "form": {}, "frame": {}, "frameset": {},
	"h1": {}, "h2": {}, "h3": {}, "h4": {}, "h5": {}, "h6": {},
	"head": {}, "header": {}, "hr": {}, "html": {}, "iframe": {},
	"legend": {}, "li": {}, "link": {}, "main": {}, "menu": {},
	"menuitem": {}, "nav": {}, "noframes": {}, "ol": {}, "optgroup": {},
	"option": {}, "p": {}, "param": {}, "pre": {}, "script": {},
	"search": {}, "section": {}, "style": {}, "summary": {}, "table": {},
	"tbody": {}, "td": {}, "textarea": {}, "tfoot": {}, "th": {},
	"thead": {}, "title": {}, "tr": {}, "track": {}, "ul": {},
}

func markdownListMarkerLength(value string) int {
	markerEnd := 0
	if len(value) > 0 && (value[0] == '-' || value[0] == '*' || value[0] == '+') {
		markerEnd = 1
	} else {
		for markerEnd < len(value) && markerEnd < 9 && value[markerEnd] >= '0' && value[markerEnd] <= '9' {
			markerEnd++
		}
		if markerEnd == 0 || markerEnd >= len(value) || value[markerEnd] != '.' && value[markerEnd] != ')' {
			return 0
		}
		markerEnd++
	}
	if markerEnd >= len(value) || value[markerEnd] != ' ' {
		return 0
	}
	padding := 0
	for markerEnd+padding < len(value) && value[markerEnd+padding] == ' ' {
		padding++
	}
	if padding > 4 {
		padding = 1
	}
	return markerEnd + padding
}
