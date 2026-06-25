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
		return strings.TrimSpace(raw)
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
			if isHTMLBlockBoundary(top.node) && buf.Len() > 0 && bufLastByte(&buf) != '\n' {
				buf.WriteByte('\n')
			}
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
			text := collapseHTMLWhitespace(n.Data)
			if text != "" {
				if last := bufLastByte(&buf); last != 0 && last != '\n' && last != ' ' {
					buf.WriteByte(' ')
				}
				buf.WriteString(text)
			}
		}

		// pre-children block boundary newline
		if isHTMLBlockBoundary(n) && buf.Len() > 0 && bufLastByte(&buf) != '\n' {
			buf.WriteByte('\n')
		}

		// push this node so its children get processed next
		if n.FirstChild != nil {
			stack = append(stack, pendingEntry{node: n, child: n.FirstChild})
		}
	}

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
