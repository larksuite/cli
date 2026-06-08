// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package signature

import (
	"bytes"
	"strings"

	xhtml "golang.org/x/net/html"
)

// PlainTextFromHTML returns the visible text of a rendered signature HTML.
// It uses an explicit stack so malformed or deeply nested signatures cannot
// overflow the goroutine stack.
func PlainTextFromHTML(raw string) string {
	doc, err := xhtml.Parse(strings.NewReader(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}

	var buf bytes.Buffer
	type pendingEntry struct {
		node  *xhtml.Node
		child *xhtml.Node
	}
	stack := []pendingEntry{{node: doc, child: doc.FirstChild}}

	for len(stack) > 0 {
		top := &stack[len(stack)-1]
		if top.child == nil {
			if isBlockBoundary(top.node) && buf.Len() > 0 && lastByte(&buf) != '\n' {
				buf.WriteByte('\n')
			}
			stack = stack[:len(stack)-1]
			continue
		}

		n := top.child
		top.child = top.child.NextSibling
		if isNonTextTag(n) {
			continue
		}
		if n.Type == xhtml.TextNode {
			text := collapseWhitespace(n.Data)
			if text != "" {
				if last := lastByte(&buf); last != 0 && last != '\n' && last != ' ' {
					buf.WriteByte(' ')
				}
				buf.WriteString(text)
			}
		}
		if isBlockBoundary(n) && buf.Len() > 0 && lastByte(&buf) != '\n' {
			buf.WriteByte('\n')
		}
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

func lastByte(buf *bytes.Buffer) byte {
	if buf.Len() == 0 {
		return 0
	}
	return buf.Bytes()[buf.Len()-1]
}

func isNonTextTag(n *xhtml.Node) bool {
	if n == nil || n.Type != xhtml.ElementNode {
		return false
	}
	switch strings.ToLower(n.Data) {
	case "head", "meta", "script", "noscript", "style", "link", "title", "img":
		return true
	default:
		return false
	}
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func isBlockBoundary(n *xhtml.Node) bool {
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
