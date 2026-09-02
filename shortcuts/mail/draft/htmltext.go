// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"bytes"
	"strings"

	xhtml "golang.org/x/net/html"
)

// plainTextFromHTML produces a conservative plain-text fallback from HTML.
// It is used only for shortcut ergonomics when a draft effectively has a
// generated text/plain fallback paired with the authored text/html body.
//
// The implementation uses an explicit stack instead of recursion so that
// deeply nested HTML cannot cause a goroutine stack overflow.
func plainTextFromHTML(raw string) string {
	doc, err := xhtml.Parse(strings.NewReader(raw))
	if err != nil {
		// x/net/html rejects documents whose open-element stack exceeds 512
		// nodes (its stack-exhaustion CVE fix). The tokenizer has no such
		// limit, so hostile nesting still yields text instead of raw markup.
		return plainTextFromHTMLTokens(raw)
	}

	var buf bytes.Buffer

	type pendingEntry struct {
		node  *xhtml.Node // the element whose children we are iterating
		child *xhtml.Node // next child to visit (nil = done)
	}

	stack := []pendingEntry{{node: doc, child: doc.FirstChild}}

	for len(stack) > 0 {
		top := &stack[len(stack)-1]

		// all children processed — emit post-children block boundary, then pop
		if top.child == nil {
			writeBlockBoundary(&buf, top.node)
			stack = stack[:len(stack)-1]
			continue
		}

		n := top.child
		top.child = top.child.NextSibling

		// skip non-text tags and their entire subtree
		if isHTMLNonTextTag(n) {
			continue
		}

		// emit text content
		if n.Type == xhtml.TextNode {
			writePlainText(&buf, n.Data)
		}

		// pre-children block boundary newline
		writeBlockBoundary(&buf, n)

		// push this node so its children get processed next
		if n.FirstChild != nil {
			stack = append(stack, pendingEntry{node: n, child: n.FirstChild})
		}
	}

	return joinPlainTextLines(&buf)
}

// plainTextFromHTMLTokens extracts text with the streaming tokenizer, which
// builds no tree and therefore has no nesting limit. It mirrors the parser's
// head handling so both paths drop the same content: text inside <head>
// (including <template> and <noframes> content) is never emitted, and head
// ends at </head>, at the first start tag that is not allowed in head, or at
// the first non-whitespace text — the parser's implicit </head> rules.
func plainTextFromHTMLTokens(raw string) string {
	w := &tokenTextWriter{}
	z := xhtml.NewTokenizer(strings.NewReader(raw))
	for {
		switch z.Next() {
		case xhtml.ErrorToken:
			return joinPlainTextLines(&w.buf)
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			w.startTag(tokenElement(z))
		case xhtml.EndTagToken:
			w.endTag(tokenElement(z))
		case xhtml.TextToken:
			w.text(string(z.Text()))
		}
	}
}

// tokenTextWriter holds the tokenizer walk state. skipDepth counts open
// containers whose text is never emitted; inHead never changes while
// skipDepth > 0 because template contents are inert.
type tokenTextWriter struct {
	buf       bytes.Buffer
	inHead    bool
	skipDepth int
}

func (w *tokenTextWriter) startTag(el *xhtml.Node) {
	if w.skipDepth > 0 {
		if w.skipsText(el) {
			w.skipDepth++
		}
		return
	}
	name := strings.ToLower(el.Data)
	switch {
	case name == "head":
		w.inHead = true
		return
	case w.inHead && name == "html":
		return
	case w.skipsText(el):
		w.skipDepth++
		return
	case w.inHead && isHeadElement(name):
		return
	}
	w.inHead = false
	writeBlockBoundary(&w.buf, el)
}

func (w *tokenTextWriter) endTag(el *xhtml.Node) {
	if w.skipsText(el) {
		if w.skipDepth > 0 {
			w.skipDepth--
		}
		return
	}
	if w.skipDepth > 0 {
		return
	}
	if strings.EqualFold(el.Data, "head") {
		w.inHead = false
		return
	}
	if w.inHead {
		return
	}
	writeBlockBoundary(&w.buf, el)
}

func (w *tokenTextWriter) text(s string) {
	if w.skipDepth > 0 {
		return
	}
	if w.inHead {
		if collapseHTMLWhitespace(s) == "" {
			return
		}
		w.inHead = false
	}
	writePlainText(&w.buf, s)
}

// skipsText reports whether el opens a container whose text must not be
// emitted: script/style/noscript/title anywhere, plus template and noframes
// while inside head (the parser drops them with the rest of the head subtree,
// but keeps their text in body).
func (w *tokenTextWriter) skipsText(el *xhtml.Node) bool {
	if isSkippedTextContainer(el) {
		return true
	}
	if !w.inHead {
		return false
	}
	switch strings.ToLower(el.Data) {
	case "template", "noframes":
		return true
	default:
		return false
	}
}

func tokenElement(z *xhtml.Tokenizer) *xhtml.Node {
	name, _ := z.TagName()
	return &xhtml.Node{Type: xhtml.ElementNode, Data: string(name)}
}

func isSkippedTextContainer(n *xhtml.Node) bool {
	switch strings.ToLower(n.Data) {
	case "script", "style", "noscript", "title":
		return true
	default:
		return false
	}
}

// isHeadElement lists the elements the HTML parser keeps inside <head>; any
// other start tag implicitly ends the head.
func isHeadElement(name string) bool {
	switch name {
	case "base", "basefont", "bgsound", "link", "meta", "noframes", "noscript", "script", "style", "template", "title":
		return true
	default:
		return false
	}
}

// writePlainText appends collapsed text, separating it from preceding inline
// text with a single space.
func writePlainText(buf *bytes.Buffer, s string) {
	text := collapseHTMLWhitespace(s)
	if text == "" {
		return
	}
	if last := bufLastByte(buf); last != 0 && last != '\n' && last != ' ' {
		buf.WriteByte(' ')
	}
	buf.WriteString(text)
}

// writeBlockBoundary starts a new line at a block-level element unless the
// buffer is empty or already ends with one.
func writeBlockBoundary(buf *bytes.Buffer, n *xhtml.Node) {
	if isHTMLBlockBoundary(n) && buf.Len() > 0 && bufLastByte(buf) != '\n' {
		buf.WriteByte('\n')
	}
}

func joinPlainTextLines(buf *bytes.Buffer) string {
	lines := strings.Split(buf.String(), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func bufLastByte(buf *bytes.Buffer) byte {
	if buf.Len() == 0 {
		return 0
	}
	return buf.Bytes()[buf.Len()-1]
}

// isHTMLNonTextTag reports whether n is an element whose text content
// should never appear in a plain-text conversion (scripts, styles, etc.).
func isHTMLNonTextTag(n *xhtml.Node) bool {
	if n == nil || n.Type != xhtml.ElementNode {
		return false
	}
	switch strings.ToLower(n.Data) {
	case "head", "meta", "script", "noscript", "style", "link", "title":
		return true
	default:
		return false
	}
}

func collapseHTMLWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func isHTMLBlockBoundary(n *xhtml.Node) bool {
	if n == nil || n.Type != xhtml.ElementNode {
		return false
	}
	switch strings.ToLower(n.Data) {
	case "address", "article", "aside", "blockquote", "br", "dd", "div", "dl", "dt",
		"figcaption", "figure", "footer", "form", "h1", "h2", "h3", "h4", "h5", "h6",
		"header", "hr", "li", "main", "nav", "ol", "p", "pre", "section", "table", "tr", "ul":
		return true
	default:
		return false
	}
}

// PlainTextFromHTML is the exported wrapper over plainTextFromHTML, so the
// mail package can render an HTML signature as a plain-text fallback when a
// message body is sent in plain-text mode. The conversion logic is unchanged.
func PlainTextFromHTML(raw string) string {
	return plainTextFromHTML(raw)
}

// bodyLooksLikeHTML reports whether raw appears to contain HTML markup.
// This is intentionally heuristic: it exists to reject obvious plain-text
// input when a draft's authored body is text/html.
func bodyLooksLikeHTML(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.Contains(lower, "<html") ||
		strings.Contains(lower, "<body") ||
		strings.Contains(lower, "<div") ||
		strings.Contains(lower, "<p") ||
		strings.Contains(lower, "<br") ||
		strings.Contains(lower, "<span") ||
		strings.Contains(lower, "<section") ||
		strings.Contains(lower, "<article") ||
		strings.Contains(lower, "<table") ||
		strings.Contains(lower, "<a ")
}
