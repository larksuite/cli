// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

// This file mirrors the Markdown parser contract used by docx_xml-go's
// protocol/gmf package. Keep the extension set, parser priorities and the
// block-affecting preprocessors in sync with that package. The CLI cannot
// import the internal SDK module, so the small parser adapter is maintained
// here as a compatibility boundary.

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	gmutil "github.com/yuin/goldmark/util"
)

var sdkMarkdownParser = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		extension.DefinitionList,
		&sdkMathExtension{},
		&sdkUnderscoreHTMLExt{},
	),
	goldmark.WithParserOptions(
		parser.WithBlockParsers(gmutil.Prioritized(&sdkContainerBlockParser{}, 90)),
		parser.WithBlockParsers(gmutil.Prioritized(&sdkXMLBlockParser{}, 91)),
		parser.WithInlineParsers(gmutil.Prioritized(&sdkXMLInlineParser{}, 80)),
	),
).Parser()

func parseSDKMarkdown(source string, preprocess bool) ([]byte, gast.Node) {
	if preprocess {
		source = preprocessSDKMarkdownBlocks(source)
	}
	data := []byte(source)
	return data, sdkMarkdownParser.Parse(text.NewReader(data))
}

// preprocessSDKMarkdownBlocks follows the SDK preprocessing order. Inline
// markup normalization is required now that the same AST also owns per-block
// character statistics; omitting it would count repaired Markdown delimiters
// as visible DocX text.
func preprocessSDKMarkdownBlocks(source string) string {
	source = preprocessSDKPreCodeLineBreaks(source)
	source = normalizeSDKListIndent(source)
	source = preprocessSDKAdjacentInlineMarkup(source)
	source = preprocessSDKAdjacentContainerTags(source)
	return source
}

func normalizeSDKListIndent(source string) string {
	lines := strings.Split(source, "\n")
	type stackEntry struct{ originalIndent int }
	var stack []stackEntry
	inCodeFence := false
	changed := false
	lastOriginalIndent := 0
	lastNewIndent := 0
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inCodeFence = !inCodeFence
			continue
		}
		if inCodeFence || trimmed == "" {
			continue
		}
		indent := len(line) - len(trimmed)
		if sdkListMarkerLength(trimmed) > 0 {
			for len(stack) > 0 && indent <= stack[len(stack)-1].originalIndent {
				stack = stack[:len(stack)-1]
			}
			newIndent := len(stack) * 4
			stack = append(stack, stackEntry{originalIndent: indent})
			lastOriginalIndent = indent
			lastNewIndent = newIndent
			if newIndent != indent {
				lines[i] = strings.Repeat(" ", newIndent) + trimmed
				changed = true
			}
		} else if len(stack) > 0 && indent > lastOriginalIndent {
			delta := lastNewIndent - lastOriginalIndent
			if delta != 0 {
				newIndent := maxInt(indent+delta, 0)
				lines[i] = strings.Repeat(" ", newIndent) + trimmed
				changed = true
			}
		}
	}
	if !changed {
		return source
	}
	return strings.Join(lines, "\n")
}

func sdkListMarkerLength(value string) int {
	if len(value) >= 2 && (value[0] == '-' || value[0] == '*' || value[0] == '+') && value[1] == ' ' {
		return 2
	}
	position := 0
	for position < len(value) && value[position] >= '0' && value[position] <= '9' {
		position++
	}
	if position > 0 && position+1 < len(value) && (value[position] == '.' || value[position] == ')') && value[position+1] == ' ' {
		return position + 2
	}
	return 0
}

var sdkAdjacentContainerTagPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(<grid\b[^>]*>)\s*(<column\b)`),
	regexp.MustCompile(`(</column>)\s*(<column\b)`),
	regexp.MustCompile(`(</column>)\s*(</grid>)`),
}

var sdkSameLineContainerTagPattern = regexp.MustCompile(
	`(<(?:callout|grid|column)\b[^>]*>)([^\n]*?)(</(?:callout|grid|column)>)`,
)

func preprocessSDKAdjacentContainerTags(md string) string {
	for _, re := range sdkAdjacentContainerTagPatterns {
		md = re.ReplaceAllString(md, "$1\n$2")
	}
	md = preprocessSDKAdjacentOKRTags(md)
	return sdkSameLineContainerTagPattern.ReplaceAllStringFunc(md, func(s string) string {
		match := sdkSameLineContainerTagPattern.FindStringSubmatch(s)
		open, inner, close := match[1], match[2], match[3]
		if strings.TrimSpace(inner) == "" {
			return open + "\n" + close
		}
		return open + "\n" + inner + "\n" + close
	})
}

var (
	sdkOKRTagNamePattern     = `(?:okr|okr-objective|okr-key-result|okr-progress)`
	sdkOKRTagFollowedByTagRE = regexp.MustCompile(`(</?` + sdkOKRTagNamePattern + `\b[^>]*>)\s*(<)`)
	sdkTagFollowedByOKRTagRE = regexp.MustCompile(`(>)\s*(</?` + sdkOKRTagNamePattern + `\b)`)
)

func preprocessSDKAdjacentOKRTags(md string) string {
	if !strings.Contains(md, "<okr") {
		return md
	}
	md = sdkOKRTagFollowedByTagRE.ReplaceAllString(md, "$1\n$2")
	return sdkTagFollowedByOKRTagRE.ReplaceAllString(md, "$1\n$2")
}

// The SDK protects raw newlines in <pre><code> before Goldmark sees them. For
// batching, replacing them with <br/> is sufficient: text escaping has no
// bearing on the block AST, while the line protection behavior stays the same.
func preprocessSDKPreCodeLineBreaks(source string) string {
	if !containsASCIIFold(source, "<pre") || !containsASCIIFold(source, "<code") || !strings.ContainsAny(source, "\r\n") {
		return source
	}
	return rewriteTagContents(source, "pre", func(preInner string) string {
		return rewriteTagContents(preInner, "code", func(codeInner string) string {
			if !strings.ContainsAny(codeInner, "\r\n") {
				return codeInner
			}
			return protectSDKRawCodeLineBreaks(codeInner)
		})
	})
}

func protectSDKRawCodeLineBreaks(content string) string {
	var out strings.Builder
	segmentStart := 0
	for i := 0; i < len(content); i++ {
		if content[i] != '\r' && content[i] != '\n' {
			continue
		}
		out.WriteString(escapeSDKXMLText(content[segmentStart:i]))
		if content[i] == '\r' && i+1 < len(content) && content[i+1] == '\n' {
			i++
		}
		out.WriteString("<br/>")
		segmentStart = i + 1
	}
	out.WriteString(escapeSDKXMLText(content[segmentStart:]))
	return out.String()
}

func escapeSDKXMLText(source string) string {
	if !strings.ContainsAny(source, "&<>") {
		return source
	}
	var out strings.Builder
	out.Grow(len(source) + 8)
	for i := 0; i < len(source); i++ {
		switch source[i] {
		case '&':
			out.WriteString("&amp;")
		case '<':
			out.WriteString("&lt;")
		case '>':
			out.WriteString("&gt;")
		default:
			out.WriteByte(source[i])
		}
	}
	return out.String()
}

func rewriteTagContents(source, tag string, rewrite func(string) string) string {
	openNeedle := "<" + tag
	closeNeedle := "</" + tag + ">"
	var out strings.Builder
	for offset := 0; offset < len(source); {
		relOpen := indexASCIIFold(source[offset:], openNeedle)
		if relOpen < 0 {
			out.WriteString(source[offset:])
			break
		}
		openStart := offset + relOpen
		openEndRel := strings.IndexByte(source[openStart:], '>')
		if openEndRel < 0 {
			out.WriteString(source[offset:])
			break
		}
		openEnd := openStart + openEndRel + 1
		if openEnd > openStart+1 && !isXMLTagNameBoundary(source[openStart+1+len(tag)]) {
			out.WriteString(source[offset:openEnd])
			offset = openEnd
			continue
		}
		relClose := indexASCIIFold(source[openEnd:], closeNeedle)
		if relClose < 0 {
			out.WriteString(source[offset:])
			break
		}
		closeStart := openEnd + relClose
		closeEnd := closeStart + len(closeNeedle)
		out.WriteString(source[offset:openEnd])
		out.WriteString(rewrite(source[openEnd:closeStart]))
		out.WriteString(source[closeStart:closeEnd])
		offset = closeEnd
	}
	return out.String()
}

func containsASCIIFold(source, needle string) bool {
	return indexASCIIFold(source, needle) >= 0
}

func indexASCIIFold(source, needle string) int {
	if needle == "" {
		return 0
	}
	for start := 0; start+len(needle) <= len(source); start++ {
		matched := true
		for i := 0; i < len(needle); i++ {
			left, right := source[start+i], needle[i]
			if left >= 'A' && left <= 'Z' {
				left += 'a' - 'A'
			}
			if right >= 'A' && right <= 'Z' {
				right += 'a' - 'A'
			}
			if left != right {
				matched = false
				break
			}
		}
		if matched {
			return start
		}
	}
	return -1
}

func isXMLTagNameBoundary(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' || ch == '>' || ch == '/'
}

var sdkTagAliases = map[string]string{
	"strong":           "b",
	"text":             "span",
	"font":             "span",
	"equation":         "latex",
	"lark-table":       "table",
	"lark-tr":          "tr",
	"lark-td":          "td",
	"image":            "img",
	"reference-synced": "synced_reference",
	"source-synced":    "synced-source",
	"at":               "cite",
	"chat-card":        "chat_card",
	"folder_manager":   "folder-manager",
}

func normalizeSDKTag(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if canonical, ok := sdkTagAliases[tag]; ok {
		return canonical
	}
	return tag
}

func isSDKAllowedTag(tag string) bool {
	key := strings.ToLower(strings.TrimSpace(tag))
	_, alias := sdkTagAliases[key]
	return key == "doc" || alias || isKnownTag(normalizeSDKTag(tag))
}

var sdkMarkdownNativeBlockTags = map[string]bool{
	"p": true, "h1": true, "h2": true, "h3": true, "h4": true,
	"h5": true, "h6": true, "h7": true, "h8": true, "h9": true,
	"ul": true, "ol": true, "li": true, "blockquote": true,
	"pre": true, "hr": true, "br": true, "img": true, "a": true,
	"b": true, "strong": true, "em": true, "i": true, "u": true,
	"del": true, "span": true, "table": true, "thead": true,
	"tbody": true, "tfoot": true, "tr": true, "th": true, "td": true,
	"colgroup": true, "col": true, "checkbox": true, "title": true,
	"figure": true,
}

var sdkContainerBlockParserTags = map[string]bool{"callout": true, "grid": true, "column": true}

var sdkXMLOpenTagPrefixRegexp = regexp.MustCompile(`^<([A-Za-z][A-Za-z0-9_-]*)`)

var kindSDKMarkdownBlock = gast.NewNodeKind("SDKMarkdownBlock")

type sdkMarkdownBlock struct {
	gast.BaseBlock
	TagName       string
	RawTagName    string
	Attrs         map[string]string
	SourceStart   int
	SourceStop    int
	IsSelfClosing bool
	IsVoid        bool
	PreserveText  bool
}

func (n *sdkMarkdownBlock) Kind() gast.NodeKind { return kindSDKMarkdownBlock }
func (n *sdkMarkdownBlock) Dump(source []byte, level int) {
	gast.DumpHelper(n, source, level, nil, nil)
}
func (n *sdkMarkdownBlock) SourceSpan() (int, int, bool) {
	return n.SourceStart, n.SourceStop, n.SourceStop > n.SourceStart
}

type sdkContainerBlockParser struct{}

func (p *sdkContainerBlockParser) Trigger() []byte { return []byte{'<'} }

func (p *sdkContainerBlockParser) Open(_ gast.Node, reader text.Reader, _ parser.Context) (gast.Node, parser.State) {
	line, segment := reader.PeekLine()
	trimmed := bytes.TrimLeft(line, " \t")
	leading := len(line) - len(trimmed)
	if len(trimmed) < 2 || trimmed[0] != '<' {
		return nil, parser.NoChildren
	}
	match := sdkXMLOpenTagPrefixRegexp.FindSubmatch(trimmed)
	if match == nil {
		return nil, parser.NoChildren
	}
	tag := strings.ToLower(string(match[1]))
	if !sdkContainerBlockParserTags[tag] {
		return nil, parser.NoChildren
	}
	openEnd := bytes.IndexByte(trimmed, '>')
	if openEnd < 0 || openEnd >= 1 && trimmed[openEnd-1] == '/' {
		return nil, parser.NoChildren
	}
	reader.Advance(leading + openEnd + 1)
	return &sdkMarkdownBlock{
		TagName: tag, RawTagName: tag, Attrs: parseSDKMarkdownTagAttributes(trimmed[:openEnd+1]),
		SourceStart: segment.Start + leading,
	}, parser.HasChildren
}

func (p *sdkContainerBlockParser) Continue(node gast.Node, reader text.Reader, _ parser.Context) parser.State {
	block := node.(*sdkMarkdownBlock)
	line, segment := reader.PeekLine()
	trimmed := bytes.TrimLeft(line, " \t")
	if hasSDKXMLCloseTagPrefix(trimmed, block.RawTagName) {
		leading := len(line) - len(trimmed)
		block.SourceStop = segment.Start + leading + sdkXMLCloseTagLen(block.RawTagName)
		reader.Advance(leading + sdkXMLCloseTagLen(block.RawTagName))
		return parser.Close
	}
	if isSDKXMLTagLine(trimmed) {
		if indent := len(line) - len(trimmed); indent > 0 {
			reader.AdvanceAndSetPadding(indent, 0)
		}
	}
	return parser.Continue | parser.HasChildren
}

func (p *sdkContainerBlockParser) Close(gast.Node, text.Reader, parser.Context) {}
func (p *sdkContainerBlockParser) CanInterruptParagraph() bool                  { return true }
func (p *sdkContainerBlockParser) CanAcceptIndentedLine() bool                  { return true }

type sdkXMLBlockParser struct{}

func (p *sdkXMLBlockParser) Trigger() []byte { return []byte{'<'} }

func (p *sdkXMLBlockParser) Open(_ gast.Node, reader text.Reader, _ parser.Context) (gast.Node, parser.State) {
	line, segment := reader.PeekLine()
	trimmed := bytes.TrimLeft(line, " \t")
	leading := len(line) - len(trimmed)
	if leading >= 4 || len(trimmed) < 2 || trimmed[0] != '<' || trimmed[1] == '/' || trimmed[1] == '!' || trimmed[1] == '?' {
		return nil, parser.NoChildren
	}
	match := sdkXMLOpenTagPrefixRegexp.FindSubmatch(trimmed)
	if match == nil {
		return nil, parser.NoChildren
	}
	rawTag := string(match[1])
	key := strings.ToLower(strings.TrimSpace(rawTag))
	canonical := normalizeSDKTag(rawTag)
	if !isSDKAllowedTag(rawTag) || sdkMarkdownNativeBlockTags[key] || sdkContainerBlockParserTags[key] {
		return nil, parser.NoChildren
	}
	openEnd := bytes.IndexByte(trimmed, '>')
	if openEnd < 0 {
		return nil, parser.NoChildren
	}
	isSelfClosing := openEnd >= 1 && trimmed[openEnd-1] == '/'
	isVoid := isVoidTag(canonical)
	rest := trimmed[openEnd+1:]
	if !isSelfClosing && !isVoid {
		if closeAt := findSDKXMLCloseTag(rest, rawTag); closeAt >= 0 {
			if canonical != rawTag {
				rawLen := openEnd + 1 + closeAt + sdkXMLCloseTagLen(rawTag)
				reader.Advance(leading + rawLen)
				return &sdkMarkdownBlock{
					TagName: canonical, RawTagName: rawTag, Attrs: parseSDKMarkdownTagAttributes(trimmed[:openEnd+1]),
					SourceStart: segment.Start + leading, SourceStop: segment.Start + leading + rawLen,
				}, parser.NoChildren
			}
			return nil, parser.NoChildren
		}
	}
	block := &sdkMarkdownBlock{
		TagName: canonical, RawTagName: rawTag, SourceStart: segment.Start + leading,
		Attrs:         parseSDKMarkdownTagAttributes(trimmed[:openEnd+1]),
		IsSelfClosing: isSelfClosing, IsVoid: isVoid, PreserveText: canonical == "whiteboard" || canonical == "code",
	}
	if isSelfClosing || isVoid {
		block.SourceStop = block.SourceStart + openEnd + 1
	}
	reader.Advance(leading + openEnd + 1)
	if isSelfClosing || isVoid {
		return block, parser.NoChildren
	}
	return block, parser.HasChildren
}

func (p *sdkXMLBlockParser) Continue(node gast.Node, reader text.Reader, _ parser.Context) parser.State {
	block := node.(*sdkMarkdownBlock)
	if block.IsSelfClosing || block.IsVoid {
		return parser.Close
	}
	line, segment := reader.PeekLine()
	trimmed := bytes.TrimLeft(line, " \t")
	if hasSDKXMLCloseTagPrefix(trimmed, block.RawTagName) {
		leading := len(line) - len(trimmed)
		block.SourceStop = segment.Start + leading + sdkXMLCloseTagLen(block.RawTagName)
		reader.Advance(leading + sdkXMLCloseTagLen(block.RawTagName))
		return parser.Close
	}
	if block.PreserveText {
		if closeAt := findSDKXMLCloseTag(line, block.RawTagName); closeAt >= 0 {
			block.SourceStop = segment.Start + closeAt + sdkXMLCloseTagLen(block.RawTagName)
			reader.Advance(closeAt + sdkXMLCloseTagLen(block.RawTagName))
			return parser.Close
		}
	}
	if isSDKXMLTagLine(trimmed) {
		if indent := len(line) - len(trimmed); indent > 0 {
			reader.AdvanceAndSetPadding(indent, 0)
		}
	}
	return parser.Continue | parser.HasChildren
}

func (p *sdkXMLBlockParser) Close(gast.Node, text.Reader, parser.Context) {}
func (p *sdkXMLBlockParser) CanInterruptParagraph() bool                  { return true }
func (p *sdkXMLBlockParser) CanAcceptIndentedLine() bool                  { return true }

func sdkXMLCloseTagLen(tag string) int { return len(tag) + len("</>") }

func hasSDKXMLCloseTagPrefix(line []byte, tag string) bool {
	length := sdkXMLCloseTagLen(tag)
	return len(line) >= length && bytes.EqualFold(line[:length], []byte("</"+tag+">"))
}

func findSDKXMLCloseTag(line []byte, tag string) int {
	return indexASCIIFold(string(line), "</"+tag+">")
}

func isSDKXMLTagLine(line []byte) bool {
	if len(line) < 2 || line[0] != '<' {
		return false
	}
	if line[1] == '/' {
		return len(line) >= 3 && isASCIIAlpha(line[2])
	}
	return isASCIIAlpha(line[1])
}

func isASCIIAlpha(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z'
}

var kindSDKXMLInline = gast.NewNodeKind("SDKXMLInline")

type sdkXMLInline struct {
	gast.BaseInline
	TagName       string
	RawTagName    string
	Attrs         map[string]string
	Content       []byte
	IsSelfClosing bool
	IsVoid        bool
}

func (n *sdkXMLInline) Kind() gast.NodeKind { return kindSDKXMLInline }
func (n *sdkXMLInline) Dump(source []byte, level int) {
	gast.DumpHelper(n, source, level, nil, nil)
}

var sdkMarkdownNativeInlineTags = map[string]bool{
	"b": true, "strong": true, "em": true, "i": true, "u": true,
	"del": true, "a": true, "img": true, "br": true, "span": true,
}

type sdkXMLInlineParser struct{}

func (p *sdkXMLInlineParser) Trigger() []byte { return []byte{'<'} }

func (p *sdkXMLInlineParser) Parse(_ gast.Node, reader text.Reader, _ parser.Context) gast.Node {
	line, _ := reader.PeekLine()
	if len(line) < 2 || line[0] != '<' || line[1] == '/' || line[1] == '!' || line[1] == '?' {
		return nil
	}
	match := sdkXMLOpenTagPrefixRegexp.FindSubmatch(line)
	if match == nil {
		return nil
	}
	rawTag := string(match[1])
	key := strings.ToLower(strings.TrimSpace(rawTag))
	if !isSDKAllowedTag(rawTag) || sdkMarkdownNativeInlineTags[key] || sdkContainerBlockParserTags[key] {
		return nil
	}
	openEnd := bytes.IndexByte(line, '>')
	if openEnd < 0 {
		return nil
	}
	canonical := normalizeSDKTag(rawTag)
	isSelfClosing := openEnd >= 1 && line[openEnd-1] == '/'
	isVoid := isVoidTag(canonical)
	attrs := parseSDKMarkdownTagAttributes(line[:openEnd+1])
	if isSelfClosing || isVoid {
		reader.Advance(openEnd + 1)
		return &sdkXMLInline{
			TagName: canonical, RawTagName: rawTag, Attrs: attrs,
			IsSelfClosing: isSelfClosing, IsVoid: isVoid,
		}
	}
	closeAt := findSDKXMLCloseTag(line[openEnd+1:], rawTag)
	if closeAt < 0 {
		return nil
	}
	advance := openEnd + 1 + closeAt + sdkXMLCloseTagLen(rawTag)
	content := append([]byte(nil), line[openEnd+1:openEnd+1+closeAt]...)
	reader.Advance(advance)
	return &sdkXMLInline{TagName: canonical, RawTagName: rawTag, Attrs: attrs, Content: content}
}

// parseSDKMarkdownTagAttributes mirrors the SDK's permissive XML attribute
// parser for Markdown-owned DocxXML tags. Attribute values are unescaped by
// parseAttributes, matching the DOM seen by document-limit statistics.
func parseSDKMarkdownTagAttributes(openTag []byte) map[string]string {
	value := strings.TrimSpace(string(openTag))
	value = strings.TrimPrefix(value, "<")
	value = strings.TrimSuffix(value, ">")
	value = strings.TrimSuffix(value, "/")
	value = strings.TrimSpace(value)
	position := 0
	for position < len(value) && isTagNamePart(value[position]) {
		position++
	}
	if position >= len(value) {
		return nil
	}
	return parseAttributes(value[position:])
}

var (
	sdkKindMathInline = gast.NewNodeKind("SDKMathInline")
	sdkKindMathBlock  = gast.NewNodeKind("SDKMathBlock")
)

type sdkMathInline struct {
	gast.BaseInline
	Content []byte
	block   bool
}

func (n *sdkMathInline) Kind() gast.NodeKind {
	if n.block {
		return sdkKindMathBlock
	}
	return sdkKindMathInline
}
func (n *sdkMathInline) Dump(source []byte, level int) {
	gast.DumpHelper(n, source, level, nil, nil)
}

var (
	sdkMathBlockMultiLineRegexp  = regexp.MustCompile(`(?s)^\$\$(.+?)\$\$`)
	sdkMathInlineMultiLineRegexp = regexp.MustCompile(`(?s)^\$([^ \t$].*?)\$`)
)

type sdkMathInlineParser struct{}

func (p *sdkMathInlineParser) Trigger() []byte { return []byte{'$'} }

func (p *sdkMathInlineParser) Parse(_ gast.Node, reader text.Reader, _ parser.Context) gast.Node {
	line, _ := reader.PeekLine()
	if len(line) == 0 || line[0] != '$' {
		return nil
	}
	if len(line) >= 2 && line[1] == '$' {
		if content, advance := scanSDKMathClose(line[2:], "$$"); advance >= 0 && len(content) > 0 {
			reader.Advance(2 + advance)
			return &sdkMathInline{Content: content, block: true}
		}
		if match := reader.FindSubMatch(sdkMathBlockMultiLineRegexp); len(match) >= 2 && len(bytes.TrimSpace(match[1])) > 0 && !bytes.Contains(match[1], []byte("<latex")) {
			return &sdkMathInline{Content: bytes.TrimSpace(match[1]), block: true}
		}
		return nil
	}
	if len(line) < 2 || line[1] == ' ' || line[1] == '\t' || line[1] == '$' {
		return nil
	}
	if content, advance := scanSDKMathClose(line[1:], "$"); advance >= 0 && len(content) > 0 {
		if content[len(content)-1] != ' ' && content[len(content)-1] != '\t' {
			reader.Advance(1 + advance)
			return &sdkMathInline{Content: content}
		}
		return nil
	}
	if match := reader.FindSubMatch(sdkMathInlineMultiLineRegexp); len(match) >= 2 && !bytes.Contains(match[1], []byte("<latex")) {
		trimmed := bytes.TrimRight(match[1], "\n\r")
		if len(trimmed) > 0 && trimmed[len(trimmed)-1] != ' ' && trimmed[len(trimmed)-1] != '\t' {
			return &sdkMathInline{Content: trimmed}
		}
	}
	return nil
}

func scanSDKMathClose(data []byte, delimiter string) ([]byte, int) {
	for offset := 0; offset < len(data); {
		if data[offset] == '\\' && offset+1 < len(data) && data[offset+1] == '$' {
			offset += 2
			continue
		}
		relative := bytes.Index(data[offset:], []byte(delimiter))
		if relative < 0 {
			return nil, -1
		}
		end := offset + relative
		if bytes.Contains(data[offset:end], []byte("<latex")) {
			return nil, -1
		}
		return data[:end], end + len(delimiter)
	}
	return nil, -1
}

type sdkMathExtension struct{}

func (e *sdkMathExtension) Extend(markdown goldmark.Markdown) {
	markdown.Parser().AddOptions(parser.WithInlineParsers(gmutil.Prioritized(&sdkMathInlineParser{}, 100)))
}

type sdkUnderscoreHTMLExt struct{}

func (e *sdkUnderscoreHTMLExt) Extend(markdown goldmark.Markdown) {
	markdown.Parser().AddOptions(
		parser.WithInlineParsers(gmutil.Prioritized(&sdkUnderscoreRawHTMLParser{}, 99)),
		parser.WithBlockParsers(gmutil.Prioritized(&sdkUnderscoreHTMLBlockParser{}, 99)),
	)
}

var (
	sdkExtendedTagNamePattern = `([A-Za-z][A-Za-z0-9_-]*)`
	sdkExtendedAttrPattern    = `(?:\s+[a-zA-Z_:][a-zA-Z0-9:._-]*(?:\s*=\s*(?:[^\"'=<>` + "`" + `\x00-\x20]+|'[^']*'|"[^"]*"))?)`
	sdkExtendedOpenTagRE      = regexp.MustCompile("^<" + sdkExtendedTagNamePattern + sdkExtendedAttrPattern + `*\s*/?>`)
	sdkExtendedCloseTagRE     = regexp.MustCompile("^</" + sdkExtendedTagNamePattern + `\s*>`)
	sdkPeekOpenTagRE          = regexp.MustCompile(`^<([A-Za-z][A-Za-z0-9_-]*)`)
	sdkPeekCloseTagRE         = regexp.MustCompile(`^</([A-Za-z][A-Za-z0-9_-]*)`)
	sdkExtendedBlockType7RE   = regexp.MustCompile(`^[ ]{0,3}<(/)?\s*([a-zA-Z0-9_\-]+)(` + sdkExtendedAttrPattern + `*)\s*(:?>|/>)\s*\n?$`)
)

type sdkUnderscoreRawHTMLParser struct{}

func (p *sdkUnderscoreRawHTMLParser) Trigger() []byte { return []byte{'<'} }

func (p *sdkUnderscoreRawHTMLParser) Parse(_ gast.Node, reader text.Reader, _ parser.Context) gast.Node {
	line, _ := reader.PeekLine()
	if len(line) > 1 && gmutil.IsAlphaNumeric(line[1]) {
		if match := sdkPeekOpenTagRE.FindSubmatch(line); match != nil && bytes.IndexByte(match[1], '_') >= 0 {
			return parseSDKMultiLineRawHTML(sdkExtendedOpenTagRE, reader)
		}
		return nil
	}
	if len(line) > 2 && line[1] == '/' && gmutil.IsAlphaNumeric(line[2]) {
		if match := sdkPeekCloseTagRE.FindSubmatch(line); match != nil && bytes.IndexByte(match[1], '_') >= 0 {
			return parseSDKMultiLineRawHTML(sdkExtendedCloseTagRE, reader)
		}
	}
	return nil
}

func parseSDKMultiLineRawHTML(pattern *regexp.Regexp, reader text.Reader) gast.Node {
	startLine, startSegment := reader.Position()
	if !reader.Match(pattern) {
		return nil
	}
	node := gast.NewRawHTML()
	endLine, endSegment := reader.Position()
	reader.SetPosition(startLine, startSegment)
	for {
		line, segment := reader.PeekLine()
		if line == nil {
			break
		}
		lineNumber, _ := reader.Position()
		start := segment.Start
		if lineNumber == startLine {
			start = startSegment.Start
		}
		end := segment.Stop
		if lineNumber == endLine {
			end = endSegment.Start
		}
		node.Segments.Append(text.NewSegment(start, end))
		if lineNumber == endLine {
			reader.Advance(end - start)
			break
		}
		reader.AdvanceLine()
	}
	return node
}

type sdkUnderscoreHTMLBlockParser struct{}

func (p *sdkUnderscoreHTMLBlockParser) Trigger() []byte { return []byte{'<'} }

func (p *sdkUnderscoreHTMLBlockParser) Open(_ gast.Node, reader text.Reader, context parser.Context) (gast.Node, parser.State) {
	line, segment := reader.PeekLine()
	if offset := context.BlockOffset(); offset < 0 || offset >= len(line) || line[offset] != '<' {
		return nil, parser.NoChildren
	}
	match := sdkExtendedBlockType7RE.FindSubmatchIndex(line)
	if match == nil {
		return nil, parser.NoChildren
	}
	tag := string(line[match[4]:match[5]])
	if !strings.Contains(tag, "_") {
		return nil, parser.NoChildren
	}
	isClose := match[2] > -1 && bytes.Equal(line[match[2]:match[3]], []byte("/"))
	if isClose && match[6] != match[7] {
		return nil, parser.NoChildren
	}
	node := gast.NewHTMLBlock(gast.HTMLBlockType7)
	node.Lines().Append(segment)
	reader.Advance(segment.Len() - 1)
	return node, parser.NoChildren
}

func (p *sdkUnderscoreHTMLBlockParser) Continue(node gast.Node, reader text.Reader, _ parser.Context) parser.State {
	line, segment := reader.PeekLine()
	if gmutil.IsBlank(line) {
		return parser.Close
	}
	node.Lines().Append(segment)
	reader.Advance(segment.Len() - 1)
	return parser.Continue | parser.NoChildren
}

func (p *sdkUnderscoreHTMLBlockParser) Close(gast.Node, text.Reader, parser.Context) {}
func (p *sdkUnderscoreHTMLBlockParser) CanInterruptParagraph() bool                  { return false }
func (p *sdkUnderscoreHTMLBlockParser) CanAcceptIndentedLine() bool                  { return false }
