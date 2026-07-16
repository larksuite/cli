// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

import (
	"bytes"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	gmutil "github.com/yuin/goldmark/util"
)

// cjkAdjacentMarkupExtension keeps CommonMark's delimiter stack and block
// boundaries, but permits a strong/emphasis/strikethrough closer immediately
// after non-ASCII punctuation even when ordinary text follows it. This is the
// narrow authoring form used in Chinese prose, for example **结论。**下一步.
//
// Implementing the compatibility rule in Goldmark's inline parser means code
// spans, links (including multiline titles), raw HTML, fenced blocks, nested
// lists, and blockquotes retain their normal parser semantics. The source is
// never rewritten or copied into a parallel scanner.
type cjkAdjacentMarkupExtension struct{}

func (e *cjkAdjacentMarkupExtension) Extend(markdown goldmark.Markdown) {
	markdown.Parser().AddOptions(parser.WithInlineParsers(
		gmutil.Prioritized(&cjkRawSourceStateParser{}, 399),
		gmutil.Prioritized(&cjkAdjacentMarkupParser{}, 499),
	))
}

var cjkRawSourceStackKey = parser.NewContextKey()

type cjkRawSourceFrame struct {
	tag                    string
	bottom                 *parser.Delimiter
	emphasisProcessor      *cjkDelimiterProcessor
	underscoreProcessor    *cjkDelimiterProcessor
	strikethroughProcessor *cjkDelimiterProcessor
}

// cjkRawSourceStateParser consumes source-bearing inline tags with the same
// reader-bounded matcher used by the extended raw-HTML parser. Owning these
// nodes lets it update literal-content state without scanning beyond the
// current block or leaving a competing parser to consume a different range.
type cjkRawSourceStateParser struct{}

func (p *cjkRawSourceStateParser) Trigger() []byte { return []byte{'<'} }

func (p *cjkRawSourceStateParser) Parse(_ gast.Node, reader text.Reader, pc parser.Context) gast.Node {
	line, _ := reader.PeekLine()
	closing, candidate := cjkRawSourceTagCandidate(line)
	if !candidate {
		return nil
	}
	pattern := extendedOpenTag
	if closing {
		pattern = extendedCloseTag
	}
	bottom := pc.LastDelimiter()
	node := (&underscoreRawHTMLParser{}).parseMultiLine(pattern, reader)
	if node == nil {
		return nil
	}
	fragment := string(node.(*gast.RawHTML).Segments.Value(reader.Source()))
	token, _, state := scanXMLToken(fragment, 0)
	tag := strings.ToLower(token.name)
	if state != tokenOK || tag != "code" && tag != "pre" && tag != "whiteboard" {
		return node
	}
	stack, _ := pc.Get(cjkRawSourceStackKey).([]cjkRawSourceFrame)
	if token.closing {
		if len(stack) > 0 && stack[len(stack)-1].tag == tag {
			parser.ProcessDelimiters(stack[len(stack)-1].bottom, pc)
			pc.Set(cjkRawSourceStackKey, stack[:len(stack)-1])
		}
	} else if !token.selfClosing {
		scope := len(stack) + 1
		pc.Set(cjkRawSourceStackKey, append(stack, cjkRawSourceFrame{
			tag:                    tag,
			bottom:                 bottom,
			emphasisProcessor:      &cjkDelimiterProcessor{marker: '*', scope: scope},
			underscoreProcessor:    &cjkDelimiterProcessor{marker: '_', scope: scope},
			strikethroughProcessor: &cjkDelimiterProcessor{marker: '~', strike: true, scope: scope},
		}))
	}
	return node
}

func cjkRawSourceTagCandidate(line []byte) (closing, ok bool) {
	nameStart := 1
	if len(line) > 1 && line[1] == '/' {
		closing = true
		nameStart++
	}
	for _, candidate := range [][]byte{[]byte("code"), []byte("pre"), []byte("whiteboard")} {
		nameEnd := nameStart + len(candidate)
		if nameEnd > len(line) || !bytes.EqualFold(line[nameStart:nameEnd], candidate) {
			continue
		}
		if nameEnd == len(line) || line[nameEnd] == '>' || line[nameEnd] == '/' || line[nameEnd] == ' ' || line[nameEnd] == '\t' || line[nameEnd] == '\r' || line[nameEnd] == '\n' {
			return closing, true
		}
	}
	return false, false
}

type cjkAdjacentMarkupParser struct{}

func (p *cjkAdjacentMarkupParser) Trigger() []byte { return []byte{'*', '_', '~'} }

func (p *cjkAdjacentMarkupParser) Parse(_ gast.Node, reader text.Reader, pc parser.Context) gast.Node {
	line, segment := reader.PeekLine()
	if len(line) == 0 {
		return nil
	}
	stack, _ := pc.Get(cjkRawSourceStackKey).([]cjkRawSourceFrame)
	before := reader.PrecendingCharacter()
	var delimiter *parser.Delimiter
	switch line[0] {
	case '*':
		processor := cjkEmphasisDelimiterProcessor
		if len(stack) > 0 {
			processor = stack[len(stack)-1].emphasisProcessor
		}
		delimiter = parser.ScanDelimiter(line, before, 1, processor)
	case '_':
		processor := cjkUnderscoreDelimiterProcessor
		if len(stack) > 0 {
			processor = stack[len(stack)-1].underscoreProcessor
		}
		delimiter = parser.ScanDelimiter(line, before, 1, processor)
	case '~':
		processor := cjkStrikethroughDelimiterProcessor
		if len(stack) > 0 {
			processor = stack[len(stack)-1].strikethroughProcessor
		}
		delimiter = parser.ScanDelimiter(line, before, 1, processor)
		// Match Goldmark's GFM strikethrough parser outside the compatibility
		// case so installing this parser does not broaden the Markdown surface.
		if delimiter == nil || delimiter.OriginalLength > 2 || before == '~' {
			return nil
		}
	default:
		return nil
	}
	if delimiter == nil {
		return nil
	}
	if len(stack) == 0 && shouldRelaxCJKAdjacentCloser(delimiter, before, pc) {
		delimiter.CanClose = true
		if delimiter.Char == '~' {
			delimiter.Processor = cjkRelaxedStrikethroughDelimiterProcessor
		} else {
			delimiter.Processor = cjkRelaxedEmphasisDelimiterProcessor
		}
	}
	delimiter.Segment = segment.WithStop(segment.Start + delimiter.OriginalLength)
	reader.Advance(delimiter.OriginalLength)
	pc.PushDelimiter(delimiter)
	return delimiter
}

func shouldRelaxCJKAdjacentCloser(delimiter *parser.Delimiter, before rune, pc parser.Context) bool {
	if delimiter.Char == '_' || delimiter.CanClose || !delimiter.CanOpen || delimiter.OriginalLength < 2 || delimiter.OriginalLength > 3 {
		return false
	}
	if before <= unicode.MaxASCII || !unicode.IsPunct(before) && !unicode.IsSymbol(before) {
		return false
	}
	for opener := pc.LastDelimiter(); opener != nil; opener = opener.PreviousDelimiter {
		if opener.Char == delimiter.Char && opener.OriginalLength == delimiter.OriginalLength && opener.CanOpen {
			return true
		}
	}
	return false
}

type cjkDelimiterProcessor struct {
	marker  byte
	strike  bool
	relaxed bool
	scope   int
}

func (p *cjkDelimiterProcessor) IsDelimiter(value byte) bool { return value == p.marker }
func (p *cjkDelimiterProcessor) CanOpenCloser(opener, closer *parser.Delimiter) bool {
	if opener.Char != closer.Char {
		return false
	}
	processor, ok := closer.Processor.(*cjkDelimiterProcessor)
	if ok && p.scope != processor.scope {
		return false
	}
	return !ok || !processor.relaxed || opener.OriginalLength == closer.OriginalLength
}
func (p *cjkDelimiterProcessor) OnMatch(consumes int) gast.Node {
	if p.strike {
		return extast.NewStrikethrough()
	}
	return gast.NewEmphasis(consumes)
}

var (
	cjkEmphasisDelimiterProcessor             = &cjkDelimiterProcessor{marker: '*'}
	cjkRelaxedEmphasisDelimiterProcessor      = &cjkDelimiterProcessor{marker: '*', relaxed: true}
	cjkUnderscoreDelimiterProcessor           = &cjkDelimiterProcessor{marker: '_'}
	cjkStrikethroughDelimiterProcessor        = &cjkDelimiterProcessor{marker: '~', strike: true}
	cjkRelaxedStrikethroughDelimiterProcessor = &cjkDelimiterProcessor{marker: '~', strike: true, relaxed: true}
)

func markdownFence(line string) (rune, int, bool) {
	if len(line) < 3 || line[0] != '`' && line[0] != '~' {
		return 0, 0, false
	}
	marker := line[0]
	length := 0
	for length < len(line) && line[length] == marker {
		length++
	}
	return rune(marker), length, length >= 3
}

func runeTail(value string, start int) string {
	if start >= len(value) {
		return ""
	}
	return value[start:]
}
