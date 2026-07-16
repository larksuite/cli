// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package docxparse parses LarkOpenCLI DocxXML and Markdown into a small,
// offline DOM for the docs +script shortcut.
package docxparse

import (
	"sort"
	"strings"
)

// Format is an accepted source document format.
type Format string

const (
	FormatXML      Format = "xml"
	FormatMarkdown Format = "markdown"
)

// ParseResult is the complete result returned by Parse.
type ParseResult struct {
	Format  Format  `json:"format"`
	XML     string  `json:"xml"`
	Profile Profile `json:"profile"`
}

type nodeType uint8

const (
	nodeText nodeType = iota
	nodeElement
)

// Node is the internal DocxXML DOM representation.
type Node struct {
	typ      nodeType
	tag      string
	attrs    map[string]string
	children []*Node
	text     string
	parent   *Node
}

func newText(text string) *Node {
	return &Node{typ: nodeText, text: text}
}

func newElement(tag string, attrs map[string]string) *Node {
	return &Node{typ: nodeElement, tag: tag, attrs: attrs}
}

func (n *Node) addChild(child *Node) {
	if n == nil || child == nil {
		return
	}
	child.parent = n
	n.children = append(n.children, child)
}

func (n *Node) writeXML(out *strings.Builder) {
	if n == nil {
		return
	}
	if n.typ == nodeText {
		out.WriteString(escapeXMLText(n.text))
		return
	}

	out.WriteByte('<')
	out.WriteString(n.tag)
	keys := make([]string, 0, len(n.attrs))
	for key := range n.attrs {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		wi, iWeighted := attributeWeight[keys[i]]
		wj, jWeighted := attributeWeight[keys[j]]
		switch {
		case iWeighted && jWeighted && wi != wj:
			return wi < wj
		case iWeighted != jWeighted:
			return iWeighted
		default:
			return keys[i] < keys[j]
		}
	})
	for _, key := range keys {
		out.WriteByte(' ')
		out.WriteString(key)
		out.WriteString(`="`)
		out.WriteString(escapeXMLAttr(n.attrs[key]))
		out.WriteByte('"')
	}

	if isVoidTag(n.tag) {
		out.WriteString("/>")
		return
	}
	out.WriteByte('>')
	for _, child := range n.children {
		child.writeXML(out)
	}
	out.WriteString("</")
	out.WriteString(n.tag)
	out.WriteByte('>')
}

func renderNodes(nodes []*Node) string {
	var out strings.Builder
	for _, node := range nodes {
		node.writeXML(&out)
	}
	return out.String()
}

var attributeWeight = map[string]int{
	"id":                0,
	"name":              1,
	"top-block-id":      2,
	"parent-block-path": 3,
	"mode":              4,
	"start-block-id":    5,
	"end-block-id":      6,
	"hit-block-ids":     7,
}

func escapeXMLText(value string) string {
	if !strings.ContainsAny(value, "&<>") {
		return value
	}
	var out strings.Builder
	out.Grow(len(value) + 8)
	for _, r := range value {
		switch r {
		case '&':
			out.WriteString("&amp;")
		case '<':
			out.WriteString("&lt;")
		case '>':
			out.WriteString("&gt;")
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

func escapeXMLAttr(value string) string {
	if !strings.ContainsAny(value, "&<>\"'") {
		return value
	}
	var out strings.Builder
	out.Grow(len(value) + 8)
	for _, r := range value {
		switch r {
		case '&':
			out.WriteString("&amp;")
		case '<':
			out.WriteString("&lt;")
		case '>':
			out.WriteString("&gt;")
		case '"':
			out.WriteString("&#34;")
		case '\'':
			out.WriteString("&#39;")
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}
