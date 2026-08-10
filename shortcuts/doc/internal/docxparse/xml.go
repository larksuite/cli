// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

import (
	"html"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxInputBytes   = 20_000_000
	MaxNestingDepth = 1024
)

func validateSource(source string) error {
	if len(source) > MaxInputBytes {
		return newParseError("input is too large (%d bytes, limit %d)", len(source), MaxInputBytes)
	}
	if !utf8.ValidString(source) {
		return newParseError("input must be valid UTF-8")
	}
	for offset, r := range source {
		if !isXML10Character(r) {
			return newParseError("input contains an XML 1.0 forbidden character U+%04X at byte %d", r, offset)
		}
	}
	if containsForbiddenXMLDeclaration(source) {
		return newParseError("XML input must not contain DOCTYPE or ENTITY declarations")
	}
	return nil
}

func isXML10Character(r rune) bool {
	return r == '\t' || r == '\n' || r == '\r' ||
		r >= 0x20 && r <= 0xD7FF ||
		r >= 0xE000 && r <= 0xFFFD ||
		r >= 0x10000 && r <= 0x10FFFF
}

func containsForbiddenXMLDeclaration(source string) bool {
	for offset := 0; offset < len(source); {
		relative := strings.IndexByte(source[offset:], '<')
		if relative < 0 {
			return false
		}
		start := offset + relative
		switch {
		case strings.HasPrefix(source[start:], "<!--"):
			if end := strings.Index(source[start+4:], "-->"); end >= 0 {
				offset = start + 4 + end + len("-->")
				continue
			}
			return false
		case strings.HasPrefix(source[start:], "<![CDATA["):
			if end := strings.Index(source[start+len("<![CDATA["):], "]]>"); end >= 0 {
				offset = start + len("<![CDATA[") + end + len("]]>")
				continue
			}
			return false
		case strings.HasPrefix(source[start:], "<?"):
			if end := strings.Index(source[start+2:], "?>"); end >= 0 {
				offset = start + 2 + end + len("?>")
				continue
			}
			return false
		}

		position := start + 1
		if position < len(source) && source[position] == '!' {
			position++
			for position < len(source) && isXMLSpace(source[position]) {
				position++
			}
			for _, declaration := range []string{"DOCTYPE", "ENTITY"} {
				end := position + len(declaration)
				if end <= len(source) && strings.EqualFold(source[position:end], declaration) &&
					(end == len(source) || !isTagNamePart(source[end])) {
					return true
				}
			}
		}
		offset = start + 1
	}
	return false
}

func parseXML(source string) ([]*Node, error) {
	return parseXMLWithCompatibility(source, false)
}

func parseXMLCompatible(source string) ([]*Node, error) {
	return parseXMLWithCompatibility(source, true)
}

func parseXMLWithCompatibility(source string, compatible bool) ([]*Node, error) {
	if err := validateSource(source); err != nil {
		return nil, err
	}
	if compatible {
		source = normalizeCompatibleXMLInput(source)
	}
	source = strings.TrimPrefix(source, "\uFEFF")

	root := newElement("__fragment__", nil)
	stack := []*Node{root}
parseLoop:
	for i := 0; i < len(source); {
		lt := strings.IndexByte(source[i:], '<')
		if lt < 0 {
			if !compatible {
				if err := validateXMLText(source[i:], i); err != nil {
					return nil, err
				}
			}
			appendText(stack[len(stack)-1], source[i:])
			break
		}
		lt += i
		if !compatible {
			if err := validateXMLText(source[i:lt], i); err != nil {
				return nil, err
			}
		}
		appendText(stack[len(stack)-1], source[i:lt])

		token, end, state := scanXMLToken(source, lt)
		switch state {
		case tokenComment, tokenProcessingInstruction:
			i = end
			continue
		case tokenCDATA:
			appendTextValue(stack[len(stack)-1], token.text)
			i = end
			continue
		case tokenInvalid:
			if !compatible {
				return nil, newParseError("invalid XML token at byte %d", lt)
			}
			if strings.HasPrefix(source[lt:], "<!--") {
				if closeAt := strings.Index(source[lt+4:], "-->"); closeAt >= 0 {
					i = lt + 4 + closeAt + len("-->")
					continue
				}
				if nextRelative := strings.IndexByte(source[lt+4:], '<'); nextRelative >= 0 {
					i = lt + 4 + nextRelative
					continue
				}
				break parseLoop
			}
			if token.name == "" {
				next := compatibleTokenBoundary(source, lt, end)
				if closingTag, ok := scanCompatibleIncompleteClosingTag(source[lt:next]); ok {
					stack = closeCompatibleStack(stack, closingTag)
					i = next
					continue
				}
				if openingTag, ok := scanCompatibleIncompleteOpeningTag(source[lt:next]); ok {
					token = openingTag
					end = next
					break
				}
				if end > lt+1 {
					appendTextValue(stack[len(stack)-1], source[lt:end])
					i = end
				} else {
					appendTextValue(stack[len(stack)-1], "<")
					i = lt + 1
				}
				continue
			}
		case tokenIncomplete:
			if !compatible {
				return nil, newParseError("unterminated XML tag at byte %d", lt)
			}
			switch {
			case strings.HasPrefix(source[lt:], "<![CDATA["):
				i = lt + len("<![CDATA[")
				continue
			case strings.HasPrefix(source[lt:], "<!--"), strings.HasPrefix(source[lt:], "<?"):
				prefixLength := len("<?")
				if strings.HasPrefix(source[lt:], "<!--") {
					prefixLength = len("<!--")
				}
				if nextRelative := strings.IndexByte(source[lt+prefixLength:], '<'); nextRelative >= 0 {
					i = lt + prefixLength + nextRelative
					continue
				}
				break parseLoop
			}
			next := compatibleTokenBoundary(source, lt, end)
			if closingTag, ok := scanCompatibleIncompleteClosingTag(source[lt:next]); ok {
				stack = closeCompatibleStack(stack, closingTag)
				i = next
				continue
			}
			if openingTag, ok := scanCompatibleIncompleteOpeningTag(source[lt:next]); ok {
				token = openingTag
				end = next
				break
			}
			appendTextValue(stack[len(stack)-1], source[lt:next])
			i = next
			continue
		}

		tag := token.name
		if compatible && !isKnownTag(tag) {
			i = end
			continue
		}
		if token.spacingNormalized && !compatible {
			return nil, newParseError("invalid whitespace in XML tag <%s> at byte %d", token.name, lt)
		}
		if compatible && stack[len(stack)-1].tag == "whiteboard" &&
			!(token.closing && tag == "whiteboard") &&
			!(!token.closing && tag == "br") {
			i = end
			continue
		}

		if token.closing {
			if isVoidTag(tag) {
				if compatible {
					i = end
					continue
				}
				return nil, newParseError("void tag <%s/> must not have a closing tag", tag)
			}
			if len(stack) == 1 {
				if compatible {
					i = end
					continue
				}
				return nil, newParseError("unexpected closing tag </%s> at byte %d", tag, lt)
			}
			open := stack[len(stack)-1].tag
			if open != tag {
				if compatible {
					stack = closeCompatibleStack(stack, tag)
					i = end
					continue
				}
				return nil, newParseError("mismatched closing tag </%s> at byte %d; expected </%s>", tag, lt, open)
			}
			stack = stack[:len(stack)-1]
			i = end
			continue
		}

		if compatible && len(stack) > 1 && shouldAutoClose(stack[len(stack)-1].tag, tag) {
			for len(stack) > 1 && shouldAutoClose(stack[len(stack)-1].tag, tag) {
				stack = stack[:len(stack)-1]
			}
		}
		node := newElement(tag, token.attrs)
		stack[len(stack)-1].addChild(node)
		if !token.selfClosing && !isVoidTag(tag) {
			if len(stack) > MaxNestingDepth {
				return nil, newParseError("XML nesting exceeds limit %d at byte %d", MaxNestingDepth, lt)
			}
			stack = append(stack, node)
		}
		i = end
	}

	if len(stack) > 1 && !compatible {
		return nil, newParseError("missing closing tag </%s> at end of input", stack[len(stack)-1].tag)
	}
	normalizeParsedLineBreaks(root.children, false, false)
	for _, child := range root.children {
		child.parent = nil
	}
	return root.children, nil
}

func compatibleTokenBoundary(source string, start, scannedEnd int) int {
	if nextRelative := strings.IndexByte(source[start+1:], '<'); nextRelative >= 0 {
		return start + 1 + nextRelative
	}
	if scannedEnd > start {
		return scannedEnd
	}
	return len(source)
}

func scanCompatibleIncompleteClosingTag(fragment string) (string, bool) {
	position := 0
	if position >= len(fragment) || fragment[position] != '<' {
		return "", false
	}
	position++
	for position < len(fragment) && isXMLSpace(fragment[position]) {
		position++
	}
	if position >= len(fragment) || fragment[position] != '/' {
		return "", false
	}
	position++
	for position < len(fragment) && isXMLSpace(fragment[position]) {
		position++
	}
	if position >= len(fragment) || !isTagNameStart(fragment[position]) {
		return "", false
	}
	nameStart := position
	position++
	for position < len(fragment) && isTagNamePart(fragment[position]) {
		position++
	}
	if strings.TrimSpace(fragment[position:]) != "" {
		return "", false
	}
	tag := fragment[nameStart:position]
	if !isKnownTag(tag) || isVoidTag(tag) {
		return "", false
	}
	return tag, true
}

func scanCompatibleIncompleteOpeningTag(fragment string) (xmlToken, bool) {
	fragment = strings.TrimRightFunc(fragment, unicode.IsSpace)
	if fragment == "" || strings.HasSuffix(fragment, ">") {
		return xmlToken{}, false
	}
	synthetic := fragment + ">"
	token, end, state := scanXMLToken(synthetic, 0)
	if end != len(synthetic) || token.name == "" || token.closing ||
		state != tokenOK && state != tokenInvalid {
		return xmlToken{}, false
	}
	return token, true
}

func closeCompatibleStack(stack []*Node, tag string) []*Node {
	for i := len(stack) - 1; i > 0; i-- {
		if stack[i].tag == tag {
			return stack[:i]
		}
	}
	return stack
}

// normalizeParsedLineBreaks removes formatting newlines from ordinary XML,
// while source-bearing code/whiteboard blocks keep semantic
// line breaks as explicit <br/> nodes. str_replace pattern/replacement payloads
// retain raw newlines because their string matching semantics depend on them.
func normalizeParsedLineBreaks(nodes []*Node, sourceBlock, stringMutation bool) {
	for _, node := range nodes {
		if node == nil || node.typ != nodeElement {
			continue
		}
		nextSourceBlock := sourceBlock || node.tag == "code" || node.tag == "whiteboard"
		nextStringMutation := stringMutation || node.tag == "str_replace"
		preserveRaw := nextStringMutation && (node.tag == "pattern" || node.tag == "replacement")
		if node.tag == "code" || node.tag == "whiteboard" {
			trimSourceBlockBoundaryNewlines(node.children)
		}
		children := make([]*Node, 0, len(node.children))
		for _, child := range node.children {
			if child.typ != nodeText || !strings.ContainsAny(child.text, "\r\n") {
				children = append(children, child)
				continue
			}
			switch {
			case preserveRaw:
				children = append(children, child)
			case nextSourceBlock:
				for _, replacement := range rawTextWithBreakNodes(child.text) {
					replacement.parent = node
					children = append(children, replacement)
				}
			default:
				child.text = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(child.text)
				if child.text != "" {
					children = append(children, child)
				}
			}
		}
		node.children = children
		normalizeParsedLineBreaks(node.children, nextSourceBlock, nextStringMutation)
	}
}

func trimSourceBlockBoundaryNewlines(children []*Node) {
	for _, child := range children {
		if child.typ == nodeText {
			child.text = strings.TrimLeft(child.text, "\r\n")
			break
		}
		if child.typ == nodeElement {
			break
		}
	}
	for i := len(children) - 1; i >= 0; i-- {
		child := children[i]
		if child.typ == nodeText {
			child.text = strings.TrimRight(child.text, "\r\n")
			break
		}
		if child.typ == nodeElement {
			break
		}
	}
}

func rawTextWithBreakNodes(content string) []*Node {
	if content == "" {
		return nil
	}
	var nodes []*Node
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] != '\n' && content[i] != '\r' {
			continue
		}
		if i > start {
			nodes = append(nodes, newText(content[start:i]))
		}
		if content[i] == '\r' && i+1 < len(content) && content[i+1] == '\n' {
			i++
		}
		nodes = append(nodes, newElement("br", nil))
		start = i + 1
	}
	if start < len(content) {
		nodes = append(nodes, newText(content[start:]))
	}
	return nodes
}

type tokenState uint8

const (
	tokenOK tokenState = iota
	tokenInvalid
	tokenIncomplete
	tokenComment
	tokenProcessingInstruction
	tokenCDATA
)

type xmlToken struct {
	name              string
	attrs             map[string]string
	text              string
	closing           bool
	selfClosing       bool
	spacingNormalized bool
}

func scanXMLToken(source string, start int) (xmlToken, int, tokenState) {
	if strings.HasPrefix(source[start:], "<![CDATA[") {
		const marker = "<![CDATA["
		contentStart := start + len(marker)
		if closeAt := strings.Index(source[contentStart:], "]]>"); closeAt >= 0 {
			contentEnd := contentStart + closeAt
			return xmlToken{text: source[contentStart:contentEnd]}, contentEnd + len("]]>"), tokenCDATA
		}
		return xmlToken{}, len(source), tokenIncomplete
	}
	if strings.HasPrefix(source[start:], "<!--") {
		if closeAt := strings.Index(source[start+4:], "-->"); closeAt >= 0 {
			if strings.Contains(source[start+4:start+4+closeAt], "--") {
				return xmlToken{}, start + 1, tokenInvalid
			}
			return xmlToken{}, start + 4 + closeAt + 3, tokenComment
		}
		return xmlToken{}, len(source), tokenIncomplete
	}
	if strings.HasPrefix(source[start:], "<?") {
		if closeAt := strings.Index(source[start+2:], "?>"); closeAt >= 0 {
			return xmlToken{}, start + 2 + closeAt + 2, tokenProcessingInstruction
		}
		return xmlToken{}, len(source), tokenIncomplete
	}

	quote := byte(0)
	end := -1
	for i := start + 1; i < len(source); i++ {
		switch source[i] {
		case '\'', '"':
			if quote == 0 {
				quote = source[i]
			} else if quote == source[i] {
				quote = 0
			}
		case '>':
			if quote == 0 {
				end = i + 1
				i = len(source)
			}
		case '<':
			// A second unquoted '<' cannot belong to the current XML tag.
			// Stop here so a long sequence of invalid tag starts is scanned
			// once instead of repeatedly searching to a distant '>'.
			if quote == 0 {
				return xmlToken{}, start + 1, tokenInvalid
			}
		}
	}
	if end < 0 {
		candidate := strings.TrimSpace(source[start+1:])
		if candidate == "" || !isTagNameStart(candidate[0]) && candidate[0] != '/' {
			return xmlToken{}, start + 1, tokenInvalid
		}
		return xmlToken{}, len(source), tokenIncomplete
	}

	body := source[start+1 : end-1]
	if body == "" {
		return xmlToken{}, end, tokenInvalid
	}
	token := xmlToken{}
	position := 0
	for position < len(body) && isXMLSpace(body[position]) {
		position++
	}
	if position > 0 {
		token.spacingNormalized = true
	}
	if position >= len(body) || body[position] == '!' {
		return xmlToken{}, end, tokenInvalid
	}
	if body[position] == '/' {
		token.closing = true
		position++
		spaceStart := position
		for position < len(body) && isXMLSpace(body[position]) {
			position++
		}
		if position > spaceStart {
			token.spacingNormalized = true
		}
	}
	if position >= len(body) || !isTagNameStart(body[position]) {
		return xmlToken{}, end, tokenInvalid
	}
	nameStart := position
	position++
	for position < len(body) && isTagNamePart(body[position]) {
		position++
	}
	token.name = body[nameStart:position]
	rawRemainder := body[position:]
	remainder := strings.TrimRightFunc(rawRemainder, unicode.IsSpace)
	if token.closing {
		if strings.TrimSpace(remainder) != "" {
			return token, end, tokenInvalid
		}
		return token, end, tokenOK
	}
	if strings.HasSuffix(remainder, "/") {
		token.selfClosing = true
		if len(remainder) != len(rawRemainder) {
			return token, end, tokenInvalid
		}
		remainder = strings.TrimRightFunc(strings.TrimSuffix(remainder, "/"), unicode.IsSpace)
	}
	trimmedAttrs := strings.TrimLeftFunc(remainder, unicode.IsSpace)
	if trimmedAttrs != "" && !isAttributeNameStart(trimmedAttrs[0]) {
		token.attrs = parseAttributes(remainder)
		return token, end, tokenInvalid
	}
	var ok bool
	token.attrs, ok = parseStrictAttributes(remainder)
	if !ok {
		token.attrs = parseAttributes(remainder)
		return token, end, tokenInvalid
	}
	return token, end, tokenOK
}

func isXMLSpace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n'
}

func isTagNameStart(ch byte) bool {
	return ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z'
}

func isTagNamePart(ch byte) bool {
	return isTagNameStart(ch) || ch >= '0' && ch <= '9' || ch == '_' || ch == '-' || ch == '.' || ch == ':'
}

func isAttributeNameStart(ch byte) bool {
	return isTagNameStart(ch) || ch == '_' || ch == ':'
}

func parseAttributes(source string) map[string]string {
	attrs := map[string]string{}
	for i := 0; i < len(source); {
		for i < len(source) && unicode.IsSpace(rune(source[i])) {
			i++
		}
		if i >= len(source) {
			break
		}
		start := i
		for i < len(source) && isAttributeNameByte(source[i]) {
			i++
		}
		if start == i {
			i++
			continue
		}
		name := source[start:i]
		for i < len(source) && unicode.IsSpace(rune(source[i])) {
			i++
		}
		value := ""
		if i < len(source) && source[i] == '=' {
			i++
			for i < len(source) && unicode.IsSpace(rune(source[i])) {
				i++
			}
			if i < len(source) && (source[i] == '\'' || source[i] == '"') {
				quote := source[i]
				i++
				start = i
				for i < len(source) && source[i] != quote {
					i++
				}
				value = source[start:i]
				if i < len(source) {
					i++
				}
			} else {
				start = i
				for i < len(source) && !unicode.IsSpace(rune(source[i])) {
					i++
				}
				value = source[start:i]
			}
		}
		attrs[name] = html.UnescapeString(value)
	}
	if len(attrs) == 0 {
		return nil
	}
	return attrs
}

// parseStrictAttributes implements the quoted attribute grammar accepted by
// XML. parseAttributes remains intentionally permissive for compatibility
// recovery of malformed authoring output.
func parseStrictAttributes(source string) (map[string]string, bool) {
	attrs := map[string]string{}
	for i := 0; i < len(source); {
		spaceStart := i
		for i < len(source) && isXMLSpace(source[i]) {
			i++
		}
		if i >= len(source) {
			break
		}
		if i == spaceStart || !isAttributeNameStart(source[i]) {
			return nil, false
		}

		nameStart := i
		i++
		for i < len(source) && isTagNamePart(source[i]) {
			i++
		}
		name := source[nameStart:i]
		if _, exists := attrs[name]; exists {
			return nil, false
		}

		for i < len(source) && isXMLSpace(source[i]) {
			i++
		}
		if i >= len(source) || source[i] != '=' {
			return nil, false
		}
		i++
		for i < len(source) && isXMLSpace(source[i]) {
			i++
		}
		if i >= len(source) || (source[i] != '\'' && source[i] != '"') {
			return nil, false
		}

		quote := source[i]
		i++
		valueStart := i
		for i < len(source) && source[i] != quote {
			if source[i] == '<' {
				return nil, false
			}
			i++
		}
		if i >= len(source) {
			return nil, false
		}
		rawValue := source[valueStart:i]
		if invalidXMLEntityAt(rawValue) >= 0 {
			return nil, false
		}
		attrs[name] = html.UnescapeString(rawValue)
		i++
	}
	if len(attrs) == 0 {
		return nil, true
	}
	return attrs, true
}

func isAttributeNameByte(ch byte) bool {
	return ch > ' ' && ch != '=' && ch != '/' && ch != '>'
}

func appendText(parent *Node, raw string) {
	if parent == nil || raw == "" {
		return
	}
	appendTextValue(parent, html.UnescapeString(raw))
}

func appendTextValue(parent *Node, text string) {
	if parent == nil || text == "" {
		return
	}
	if strings.TrimSpace(text) == "" && !preserveSpaceTags[parent.tag] && parent.tag != "whiteboard" {
		return
	}
	if count := len(parent.children); count > 0 && parent.children[count-1].typ == nodeText {
		parent.children[count-1].text += text
		return
	}
	parent.addChild(newText(text))
}

func validateXMLText(value string, absoluteOffset int) error {
	if offset := strings.Index(value, "]]>"); offset >= 0 {
		return newParseError("invalid ]]> sequence in XML text at byte %d", absoluteOffset+offset)
	}
	if offset := invalidXMLEntityAt(value); offset >= 0 {
		return newParseError("invalid XML entity at byte %d", absoluteOffset+offset)
	}
	return nil
}

func invalidXMLEntityAt(value string) int {
	for cursor := 0; cursor < len(value); {
		relative := strings.IndexByte(value[cursor:], '&')
		if relative < 0 {
			return -1
		}
		start := cursor + relative
		endRelative := strings.IndexByte(value[start+1:], ';')
		if endRelative < 0 {
			return start
		}
		end := start + 1 + endRelative
		if !isValidXMLEntity(value[start+1 : end]) {
			return start
		}
		cursor = end + 1
	}
	return -1
}

func isValidXMLEntity(entity string) bool {
	switch entity {
	case "amp", "lt", "gt", "quot", "apos":
		return true
	}

	base := 10
	digits := ""
	switch {
	case strings.HasPrefix(entity, "#x"):
		base = 16
		digits = entity[2:]
	case strings.HasPrefix(entity, "#"):
		digits = entity[1:]
	default:
		return false
	}
	if digits == "" {
		return false
	}
	value, err := strconv.ParseUint(digits, base, 32)
	if err != nil {
		return false
	}
	r := rune(value)
	return r == '\t' || r == '\n' || r == '\r' ||
		r >= 0x20 && r <= 0xD7FF ||
		r >= 0xE000 && r <= 0xFFFD ||
		r >= 0x10000 && r <= utf8.MaxRune
}
