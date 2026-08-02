// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"encoding/xml"
	"errors"
	"io"
	"slices"
	"strings"
)

// textBearingTags are the SML elements whose schema content model is
// mixed (arbitrary text interleaved with inline markup): the <p> paragraph
// container and its inline formatting children, plus chart title/subtitle.
// See slides_xml_schema_definition.xml, <p> element docs: a deliberate space
// or tab is represented via &#32;/&#9; character references. Reindentation
// never descends into these elements; their entire subtree is copied
// verbatim from the input, so those references keep their exact spelling.
var textBearingTags = map[string]bool{
	"p":             true,
	"strong":        true,
	"em":            true,
	"u":             true,
	"span":          true,
	"del":           true,
	"a":             true,
	"shadow":        true,
	"outline":       true,
	"chartTitle":    true,
	"chartSubTitle": true,
}

// tokenKind classifies a raw XML token for reindentation purposes.
type tokenKind uint8

const (
	tokenStartElement tokenKind = iota // <name ...> or <name .../>
	tokenEndElement                    // </name>, or zero-width after <name .../>
	tokenCharData                      // text, character/entity references, or one CDATA section
	tokenOther                         // comment, processing instruction, or directive
)

// rawToken records where one XML token lives inside the original input:
// input[start:end] is the token's exact source bytes. The decoded token
// value is deliberately discarded (only the element's local name is kept),
// which is the core invariant of this formatter: output can only ever be
// assembled from verbatim slices of the input, never from re-encoded data.
type rawToken struct {
	kind  tokenKind
	start int    // byte offset of the token's first source byte
	end   int    // byte offset one past the token's last source byte
	local string // local element name (namespace prefix stripped); start elements only
	match int    // start element: index of its matching end token; -1 otherwise
}

// tokenize runs encoding/xml over the whole input purely as a tokenizer and
// returns every token annotated with its raw byte range. Ranges come from
// Decoder.InputOffset, which counts bytes (multi-byte UTF-8 content cannot
// skew them), and consecutive tokens tile the input exactly, so slicing
// between them loses nothing.
//
// The full document is decoded before anything is emitted: any syntax error
// (mismatched or unclosed tags, invalid characters such as \x0b, undefined
// entities, bare ]]> in text, ...) fails the whole pretty-print, keeping the
// strict-parse behavior the fallback path in prettyPrintXMLOrOriginal
// depends on.
func tokenize(input string) ([]rawToken, error) {
	decoder := xml.NewDecoder(strings.NewReader(input))
	var tokens []rawToken
	var openElements []int // indices into tokens of currently open start elements
	pos := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		end := int(decoder.InputOffset())
		raw := rawToken{start: pos, end: end, match: -1}
		switch t := token.(type) {
		case xml.StartElement:
			raw.kind = tokenStartElement
			raw.local = t.Name.Local
			openElements = append(openElements, len(tokens))
		case xml.EndElement:
			// A strict decoder never emits an end element without its start
			// element; guard anyway so a decoder change cannot panic here.
			if len(openElements) == 0 {
				return nil, errors.New("xml: unexpected end element")
			}
			raw.kind = tokenEndElement
			startIndex := openElements[len(openElements)-1]
			openElements = openElements[:len(openElements)-1]
			tokens[startIndex].match = len(tokens)
		case xml.CharData:
			raw.kind = tokenCharData
		default: // xml.Comment, xml.ProcInst, xml.Directive
			raw.kind = tokenOther
		}
		tokens = append(tokens, raw)
		pos = end
	}
	// A strict decoder reports unclosed elements as a syntax error before
	// returning io.EOF; guard anyway so truncated output is impossible.
	if len(openElements) != 0 {
		return nil, errors.New("xml: unexpected EOF: unclosed element")
	}
	return tokens, nil
}

// prettyPrintXML reindents xmlContent so structural elements (presentation,
// slide, shape, style, ...) each sit on their own line. The server returns
// XML as a single unbroken line, and this is what makes the --raw and
// --output text surfaces readable; the JSON envelope path never calls it
// (see outputSlidesXMLGetContent).
//
// Offset-slicing invariant: encoding/xml serves purely as a tokenizer, and
// every byte of the output is either a verbatim slice of the input or an
// inserted "\n"+indent run between the children of a structural element.
// Nothing is parsed-and-reserialized, so CDATA sections, whitespace
// character references in any spelling (&#32;, &#x20;, &#0009;, &#13;,
// &#10;, ...), entity lexical forms, attribute quoting, and in-tag
// whitespace all survive byte-for-byte.
//
// Reindentation never enters a textBearingTags element and never touches a
// leaf element (one with no element children), so document text — including
// whitespace-only leaves such as <title> </title> — is never altered.
func prettyPrintXML(xmlContent string) (string, error) {
	tokens, err := tokenize(xmlContent)
	if err != nil {
		return "", err
	}
	// The decoder tolerates element-free input (plain text, a lone comment,
	// nothing at all). A document without a root element is not XML the
	// formatter should claim success on; erroring routes it to the
	// original-content fallback instead of reporting pretty_printed: true.
	if !slices.ContainsFunc(tokens, func(t rawToken) bool { return t.kind == tokenStartElement }) {
		return "", errors.New("xml: no root element")
	}
	var out strings.Builder
	out.Grow(len(xmlContent) + len(xmlContent)/8)
	reindented := false
	for i := 0; i < len(tokens); {
		token := tokens[i]
		if token.kind == tokenStartElement {
			if reindented {
				// Any top-level element after the first is copied verbatim;
				// well-formed XML has a single root, so this arm only runs
				// on technically invalid multi-root input the decoder
				// happens to tolerate.
				out.WriteString(xmlContent[token.start:tokens[token.match].end])
			} else {
				writeElement(&out, xmlContent, tokens, i, 0)
				reindented = true
			}
			i = token.match + 1
			continue
		}
		// Document-level prolog and epilog (XML declaration, DOCTYPE,
		// comments, whitespace) pass through verbatim.
		out.WriteString(xmlContent[token.start:token.end])
		i++
	}
	formatted := out.String()
	if !strings.HasSuffix(formatted, "\n") {
		formatted += "\n"
	}
	return formatted, nil
}

// writeElement emits the element whose start token is tokens[startIndex],
// indented as if at the given depth (two spaces per level).
//
// Text-bearing elements and leaf elements (no element children) are emitted
// as a single verbatim input slice from open tag through close tag; for a
// self-closing tag the synthesized end token is zero-width and the slice is
// exactly the open tag. Structural elements (at least one element child,
// not text-bearing) are reindented: text children that are pure literal
// whitespace are dropped as pre-existing formatting, "\n"+indent is
// inserted before every element, comment, and processing-instruction child,
// kept text children stay glued in place with no indentation around them,
// and the close tag moves to its own line unless the last kept child is
// text.
//
// The whitespace-only test runs on the child's RAW source bytes: a
// character reference (&#32;) or a CDATA section is not literal whitespace
// there, so it is kept and its lexical form survives.
func writeElement(out *strings.Builder, input string, tokens []rawToken, startIndex, depth int) {
	start := tokens[startIndex]
	end := tokens[start.match]
	if textBearingTags[start.local] || !hasElementChild(tokens, startIndex) {
		out.WriteString(input[start.start:end.end])
		return
	}

	out.WriteString(input[start.start:start.end])
	childIndent := "\n" + strings.Repeat("  ", depth+1)
	lastKeptIsText := false
	for i := startIndex + 1; i < start.match; {
		child := tokens[i]
		switch child.kind {
		case tokenCharData:
			if !isAllWhitespace(input[child.start:child.end]) {
				out.WriteString(input[child.start:child.end])
				lastKeptIsText = true
			}
			i++
		case tokenStartElement:
			out.WriteString(childIndent)
			writeElement(out, input, tokens, i, depth+1)
			lastKeptIsText = false
			i = child.match + 1
		default: // comment, processing instruction, directive
			out.WriteString(childIndent)
			out.WriteString(input[child.start:child.end])
			lastKeptIsText = false
			i++
		}
	}
	if !lastKeptIsText {
		out.WriteString("\n")
		out.WriteString(strings.Repeat("  ", depth))
	}
	out.WriteString(input[end.start:end.end])
}

// hasElementChild reports whether the element starting at tokens[startIndex]
// has at least one direct element child. The first start-element token that
// appears before the matching end token is necessarily a direct child, so a
// linear scan without depth tracking suffices.
func hasElementChild(tokens []rawToken, startIndex int) bool {
	for i := startIndex + 1; i < tokens[startIndex].match; i++ {
		if tokens[i].kind == tokenStartElement {
			return true
		}
	}
	return false
}

// isAllWhitespace reports whether s is non-empty and consists only of
// literal XML whitespace bytes (space, tab, CR, LF). It is applied to raw
// source bytes, where character references and CDATA markers count as
// non-whitespace by construction.
func isAllWhitespace(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\n', '\r':
		default:
			return false
		}
	}
	return true
}
