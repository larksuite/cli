// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

// This file contains the small Goldmark extensions needed to match the
// LarkOpenCLI's Markdown surface: math, DocxXML tag names containing
// underscores, and Markdown-aware callout/grid/column containers.

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	gmutil "github.com/yuin/goldmark/util"
)

// ---------- Math ----------

var kindMathInline = gast.NewNodeKind("DocxMathInline")
var kindMathBlock = gast.NewNodeKind("DocxMathBlock")

type mathInline struct {
	gast.BaseInline
	content []byte
}

func (n *mathInline) Kind() gast.NodeKind { return kindMathInline }
func (n *mathInline) Dump(source []byte, level int) {
	gast.DumpHelper(n, source, level, nil, nil)
}

type mathBlock struct {
	gast.BaseInline
	content []byte
}

func (n *mathBlock) Kind() gast.NodeKind { return kindMathBlock }
func (n *mathBlock) Dump(source []byte, level int) {
	gast.DumpHelper(n, source, level, nil, nil)
}

type mathInlineParser struct{}

func (p *mathInlineParser) Trigger() []byte { return []byte{'$'} }

func (p *mathInlineParser) Parse(_ gast.Node, reader text.Reader, _ parser.Context) gast.Node {
	line, _ := reader.PeekLine()
	if len(line) == 0 || line[0] != '$' {
		return nil
	}
	if len(line) >= 2 && line[1] == '$' {
		startLine, startPosition := reader.Position()
		if content, ok := scanMathCloseReader(reader, "$$", 2); ok {
			content = bytes.TrimSpace(content)
			if len(content) > 0 {
				return &mathBlock{content: content}
			}
		}
		reader.SetPosition(startLine, startPosition)
		return nil
	}
	if len(line) < 2 || line[1] == ' ' || line[1] == '\t' || line[1] == '$' {
		return nil
	}
	startLine, startPosition := reader.Position()
	content, ok := scanMathCloseReader(reader, "$", 1)
	if !ok {
		return nil
	}
	trimmed := bytes.TrimRight(content, "\n\r")
	if len(trimmed) == 0 || trimmed[len(trimmed)-1] == ' ' || trimmed[len(trimmed)-1] == '\t' {
		reader.SetPosition(startLine, startPosition)
		return nil
	}
	return &mathInline{content: trimmed}
}

func scanMathCloseReader(reader text.Reader, delimiter string, openerLength int) ([]byte, bool) {
	startLine, startPosition := reader.Position()
	restore := func() {
		reader.SetPosition(startLine, startPosition)
	}
	reader.Advance(openerLength)

	var content []byte
	containsLatexTag := false
	delim := []byte(delimiter)
	for {
		line, _ := reader.PeekLine()
		if line == nil {
			restore()
			return nil, false
		}
		searchAt := 0
		for searchAt < len(line) {
			relative := bytes.Index(line[searchAt:], delim)
			if relative < 0 {
				break
			}
			end := searchAt + relative
			backslashes := 0
			for i := end - 1; i >= 0 && line[i] == '\\'; i-- {
				backslashes++
			}
			if backslashes%2 == 1 {
				// Delimiters can overlap: in \$$$ the first dollar is
				// escaped while the following two form a valid $$ closer.
				searchAt = end + 1
				continue
			}
			appendMathContent(&content, line[:end], &containsLatexTag)
			if containsLatexTag {
				restore()
				return nil, false
			}
			reader.Advance(end + len(delim))
			return content, true
		}
		appendMathContent(&content, line, &containsLatexTag)
		if containsLatexTag {
			restore()
			return nil, false
		}
		reader.AdvanceLine()
	}
}

func appendMathContent(content *[]byte, chunk []byte, containsLatexTag *bool) {
	if !*containsLatexTag {
		const marker = "<latex"
		start := len(*content) - len(marker) + 1
		if start < 0 {
			start = 0
		}
		window := make([]byte, 0, len(*content)-start+len(chunk))
		window = append(window, (*content)[start:]...)
		window = append(window, chunk...)
		*containsLatexTag = bytes.Contains(window, []byte(marker))
	}
	*content = append(*content, chunk...)
}

type mathExtension struct{}

func (e *mathExtension) Extend(markdown goldmark.Markdown) {
	markdown.Parser().AddOptions(parser.WithInlineParsers(
		gmutil.Prioritized(&mathInlineParser{}, 100),
	))
}

// ---------- Underscore-bearing raw XML tags ----------

type underscoreHTMLExtension struct{}

func (e *underscoreHTMLExtension) Extend(markdown goldmark.Markdown) {
	markdown.Parser().AddOptions(
		parser.WithInlineParsers(gmutil.Prioritized(&underscoreRawHTMLParser{}, 99)),
		parser.WithBlockParsers(gmutil.Prioritized(&underscoreHTMLBlockParser{}, 99)),
	)
}

var (
	extendedTagNamePattern   = `([A-Za-z][A-Za-z0-9_-]*)`
	extendedAttributePattern = `(?:\s+[a-zA-Z_:][a-zA-Z0-9:._-]*(?:\s*=\s*(?:[^"'=<>` + "`" + `\x00-\x20]+|'[^']*'|"[^"]*"))?)`
	extendedOpenTag          = regexp.MustCompile("^<" + extendedTagNamePattern + extendedAttributePattern + `*\s*/?>`)
	extendedCloseTag         = regexp.MustCompile("^</" + extendedTagNamePattern + `\s*>`)
	peekExtendedOpenTag      = regexp.MustCompile(`^<([A-Za-z][A-Za-z0-9_-]*)`)
	peekExtendedCloseTag     = regexp.MustCompile(`^</([A-Za-z][A-Za-z0-9_-]*)`)
	extendedBlockTag         = regexp.MustCompile(`^[ ]{0,3}<(/)?\s*([a-zA-Z0-9_\-]+)(` + extendedAttributePattern + `*)\s*(?:>|/>)\s*\n?$`)
)

type underscoreRawHTMLParser struct{}

func (p *underscoreRawHTMLParser) Trigger() []byte { return []byte{'<'} }

func (p *underscoreRawHTMLParser) Parse(_ gast.Node, reader text.Reader, _ parser.Context) gast.Node {
	line, _ := reader.PeekLine()
	if len(line) > 1 && gmutil.IsAlphaNumeric(line[1]) {
		if match := peekExtendedOpenTag.FindSubmatch(line); match != nil && bytes.IndexByte(match[1], '_') >= 0 {
			return p.parseMultiLine(extendedOpenTag, reader)
		}
		return nil
	}
	if len(line) > 2 && line[1] == '/' && gmutil.IsAlphaNumeric(line[2]) {
		if match := peekExtendedCloseTag.FindSubmatch(line); match != nil && bytes.IndexByte(match[1], '_') >= 0 {
			return p.parseMultiLine(extendedCloseTag, reader)
		}
	}
	return nil
}

func (p *underscoreRawHTMLParser) parseMultiLine(re *regexp.Regexp, reader text.Reader) gast.Node {
	startLine, startSegment := reader.Position()
	if !reader.Match(re) {
		return nil
	}
	endLine, endSegment := reader.Position()
	reader.SetPosition(startLine, startSegment)
	node := gast.NewRawHTML()
	for {
		line, segment := reader.PeekLine()
		if line == nil {
			break
		}
		lineNo, _ := reader.Position()
		start := segment.Start
		if lineNo == startLine {
			start = startSegment.Start
		}
		end := segment.Stop
		if lineNo == endLine {
			end = endSegment.Start
		}
		node.Segments.Append(text.NewSegment(start, end))
		if lineNo == endLine {
			reader.Advance(end - start)
			break
		}
		reader.AdvanceLine()
	}
	return node
}

type underscoreHTMLBlockParser struct{}

func (p *underscoreHTMLBlockParser) Trigger() []byte { return []byte{'<'} }

func (p *underscoreHTMLBlockParser) Open(_ gast.Node, reader text.Reader, pc parser.Context) (gast.Node, parser.State) {
	line, segment := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 || pos >= len(line) || line[pos] != '<' {
		return nil, parser.NoChildren
	}
	match := extendedBlockTag.FindSubmatchIndex(line)
	if match == nil {
		return nil, parser.NoChildren
	}
	tag := string(line[match[4]:match[5]])
	if !strings.Contains(tag, "_") {
		return nil, parser.NoChildren
	}
	isClose := match[2] > -1 && bytes.Equal(line[match[2]:match[3]], []byte("/"))
	hasAttrs := match[6] != match[7]
	if isClose && hasAttrs {
		return nil, parser.NoChildren
	}
	node := gast.NewHTMLBlock(gast.HTMLBlockType7)
	node.Lines().Append(segment)
	reader.Advance(segment.Len() - 1)
	return node, parser.NoChildren
}

func (p *underscoreHTMLBlockParser) Continue(node gast.Node, reader text.Reader, _ parser.Context) parser.State {
	line, segment := reader.PeekLine()
	if gmutil.IsBlank(line) {
		return parser.Close
	}
	node.Lines().Append(segment)
	reader.Advance(segment.Len() - 1)
	return parser.Continue | parser.NoChildren
}

func (p *underscoreHTMLBlockParser) Close(gast.Node, text.Reader, parser.Context) {}
func (p *underscoreHTMLBlockParser) CanInterruptParagraph() bool                  { return false }
func (p *underscoreHTMLBlockParser) CanAcceptIndentedLine() bool                  { return false }

// ---------- Markdown-aware DocxXML containers ----------

type containerSpec struct {
	tag string
}

var containerSpecs = map[string]*containerSpec{
	"callout": {tag: "callout"},
	"grid":    {tag: "grid"},
	"column":  {tag: "column"},
	"div":     {tag: "div"},
}

var kindContainerBlock = gast.NewNodeKind("DocxContainerBlock")

type containerBlock struct {
	gast.BaseBlock
	spec   *containerSpec
	attrs  map[string]string
	closed bool
}

func (n *containerBlock) Kind() gast.NodeKind { return kindContainerBlock }
func (n *containerBlock) Dump(source []byte, level int) {
	gast.DumpHelper(n, source, level, nil, nil)
}

type containerBlockParser struct{}

func (p *containerBlockParser) Trigger() []byte { return []byte{'<'} }

func (p *containerBlockParser) Open(_ gast.Node, reader text.Reader, _ parser.Context) (gast.Node, parser.State) {
	line, _ := reader.PeekLine()
	trimmed := bytes.TrimLeft(line, " \t")
	leading := len(line) - len(trimmed)
	if len(trimmed) < 2 || trimmed[0] != '<' {
		return nil, parser.NoChildren
	}
	token, openEnd, state := scanXMLToken(string(trimmed), 0)
	if state != tokenOK && state != tokenInvalid || token.name == "" || token.closing || token.selfClosing || token.spacingNormalized {
		return nil, parser.NoChildren
	}
	spec := containerSpecs[strings.ToLower(token.name)]
	if spec == nil {
		return nil, parser.NoChildren
	}
	node := &containerBlock{spec: spec, attrs: token.attrs}
	reader.Advance(leading + openEnd)
	return node, parser.HasChildren
}

func (p *containerBlockParser) Continue(node gast.Node, reader text.Reader, _ parser.Context) parser.State {
	container := node.(*containerBlock)
	line, segment := reader.PeekLine()
	trimmed := bytes.TrimLeft(line, " \t")
	if hasCloseTagPrefix(trimmed, container.spec.tag) {
		container.closed = true
		reader.Advance(len(line) - len(trimmed) + closeTagLength(container.spec.tag))
		return parser.Close
	}
	if isXMLTagLine(trimmed) {
		indent := len(line) - len(trimmed)
		if indent > 0 && segment.Start+indent <= segment.Stop {
			reader.AdvanceAndSetPadding(indent, 0)
		}
	}
	return parser.Continue | parser.HasChildren
}

func (p *containerBlockParser) Close(gast.Node, text.Reader, parser.Context) {}
func (p *containerBlockParser) CanInterruptParagraph() bool                  { return true }
func (p *containerBlockParser) CanAcceptIndentedLine() bool                  { return true }

func closeTagLength(tag string) int { return len(tag) + len("</>") }

func hasCloseTagPrefix(line []byte, tag string) bool {
	want := []byte("</" + tag + ">")
	return len(line) >= len(want) && bytes.EqualFold(line[:len(want)], want)
}

func isXMLTagLine(line []byte) bool {
	if len(line) < 2 || line[0] != '<' {
		return false
	}
	if line[1] == '/' {
		return len(line) >= 3 && isASCIILetter(line[2])
	}
	return isASCIILetter(line[1])
}

func isASCIILetter(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z'
}
