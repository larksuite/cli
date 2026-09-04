// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

// Create title handling is part of the SDK Markdown contract: a leading
// explicit <title> wins; otherwise one reliable top-level ATX H1 is promoted.
// The promoted source must stay in the create request because append does not
// run create-title extraction.

import (
	"html"
	"strings"

	gast "github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"golang.org/x/text/unicode/norm"
)

type sdkMarkdownH1 struct {
	unitIndex int
	visible   string
	removable bool
}

func applySDKMarkdownCreateTitleSemantics(source string, nodes []gast.Node, starts []int, units []createBatchUnit) bool {
	data := []byte(source)
	for i := range nodes {
		heading, localSource, ok := sdkMarkdownUnitHeading(source, nodes, starts, i)
		if ok && heading.Level == 1 && isSDKATXH1(data, starts[i]) && sdkMarkdownHeadingStrictlyEmpty(heading, localSource) {
			units[i].blocks = 0
		}
	}
	explicitTitle, hasExplicit := leadingMarkdownExplicitTitle(source, units)
	candidates := collectSDKMarkdownH1s(source, nodes, starts)

	if hasExplicit {
		units[0].requiresCreate = true
		if len(candidates) == 1 && normalizeSDKTitleText(candidates[0].visible) == normalizeSDKTitleText(explicitTitle) && candidates[0].removable {
			index := candidates[0].unitIndex
			units[index].blocks = maxInt(0, units[index].blocks-1)
			units[index].requiresCreate = true // create-title analysis must see and remove it
		}
		return true
	}

	if len(candidates) > 1 {
		// If only a prefix of an ambiguous H1 set reached create, the SDK would
		// promote that prefix's sole H1 and append could not undo it. Treat all
		// title-analysis participants as create-only units; the packer rejects a
		// split that would change the SDK's full-input title result.
		for _, candidate := range candidates {
			units[candidate.unitIndex].requiresCreate = true
		}
		return false
	}
	if len(candidates) == 0 {
		return false
	}
	candidate := candidates[0]
	units[candidate.unitIndex].requiresCreate = true
	if !candidate.removable {
		// SDK derives a title but preserves a non-removable H1, materializing both.
		units[candidate.unitIndex].blocks = saturatedAdd(units[candidate.unitIndex].blocks, 1)
	}
	return true
}

func sdkMarkdownHeadingStrictlyEmpty(parent gast.Node, source []byte) bool {
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		switch typed := child.(type) {
		case *gast.Text:
			if strings.TrimSpace(string(typed.Value(source))) != "" {
				return false
			}
		case *gast.String:
			if strings.TrimSpace(string(typed.Value)) != "" {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func leadingMarkdownExplicitTitle(source string, units []createBatchUnit) (string, bool) {
	if len(units) == 0 || units[0].tag != "title" || strings.TrimSpace(source[:units[0].start]) != "" {
		return "", false
	}
	end := len(source)
	if len(units) > 1 {
		end = units[1].start
	}
	nodes, err := parseXML(strings.TrimSpace(source[units[0].start:end]))
	if err != nil {
		return "", false
	}
	for _, node := range nodes {
		if node != nil && node.typ == nodeElement && node.tag == "title" {
			return strings.TrimSpace(sdkXMLVisibleText(node)), true
		}
	}
	return "", false
}

func sdkXMLVisibleText(node *Node) string {
	if node == nil {
		return ""
	}
	if node.typ == nodeText {
		return node.text
	}
	var out strings.Builder
	for _, child := range node.children {
		out.WriteString(sdkXMLVisibleText(child))
	}
	return out.String()
}

func collectSDKMarkdownH1s(source string, nodes []gast.Node, starts []int) []sdkMarkdownH1 {
	data := []byte(source)
	var candidates []sdkMarkdownH1
	for i, node := range nodes {
		if _, ok := node.(*gast.Heading); !ok || i >= len(starts) || !isSDKATXH1(data, starts[i]) {
			continue
		}
		heading, localSource, ok := sdkMarkdownUnitHeading(source, nodes, starts, i)
		if !ok || heading.Level != 1 {
			continue
		}
		visible := sdkMarkdownInlineVisibleText(heading, localSource)
		if normalizeSDKTitleText(visible) == "" {
			continue
		}
		candidates = append(candidates, sdkMarkdownH1{
			unitIndex: i,
			visible:   visible,
			removable: sdkMarkdownHeadingRemovable(heading),
		})
	}
	return candidates
}

func sdkMarkdownUnitHeading(source string, nodes []gast.Node, starts []int, index int) (*gast.Heading, []byte, bool) {
	if index < 0 || index >= len(nodes) || index >= len(starts) {
		return nil, nil, false
	}
	if _, ok := nodes[index].(*gast.Heading); !ok {
		return nil, nil, false
	}
	stop := len(source)
	if index+1 < len(starts) {
		stop = starts[index+1]
	}
	if starts[index] < 0 || stop < starts[index] || stop > len(source) {
		return nil, nil, false
	}
	localSource, document := parseSDKMarkdown(source[starts[index]:stop], false)
	heading, ok := document.FirstChild().(*gast.Heading)
	return heading, localSource, ok
}

func isSDKATXH1(source []byte, lineStart int) bool {
	line := markdownLine(source, lineStart)
	indent := 0
	for indent < len(line) && indent < 4 && line[indent] == ' ' {
		indent++
	}
	if indent > 3 || indent >= len(line) || line[indent] != '#' {
		return false
	}
	next := indent + 1
	return next == len(line) || line[next] == ' ' || line[next] == '\t'
}

func sdkMarkdownInlineVisibleText(parent gast.Node, source []byte) string {
	var out strings.Builder
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		out.WriteString(sdkMarkdownNodeVisibleText(child, source))
	}
	return out.String()
}

func sdkMarkdownNodeVisibleText(node gast.Node, source []byte) string {
	switch typed := node.(type) {
	case *gast.Text:
		return string(typed.Value(source))
	case *gast.String:
		return string(typed.Value)
	case *gast.CodeSpan:
		return sdkMarkdownDescendantText(node, source)
	case *gast.AutoLink:
		return string(typed.Label(source))
	}
	if node.Kind() == gast.KindEmphasis || node.Kind() == gast.KindLink || node.Kind() == extast.KindStrikethrough {
		return sdkMarkdownInlineVisibleText(node, source)
	}
	return ""
}

func sdkMarkdownDescendantText(parent gast.Node, source []byte) string {
	var out strings.Builder
	_ = gast.Walk(parent, func(node gast.Node, entering bool) (gast.WalkStatus, error) {
		if !entering || node == parent {
			return gast.WalkContinue, nil
		}
		switch typed := node.(type) {
		case *gast.Text:
			out.Write(typed.Value(source))
		case *gast.String:
			out.Write(typed.Value)
		}
		return gast.WalkContinue, nil
	})
	return out.String()
}

func sdkMarkdownHeadingRemovable(parent gast.Node) bool {
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		if !sdkMarkdownInlineNodeRemovable(child) {
			return false
		}
	}
	return true
}

func sdkMarkdownInlineNodeRemovable(node gast.Node) bool {
	switch node.Kind() {
	case gast.KindText, gast.KindString, gast.KindCodeSpan, gast.KindAutoLink:
		return true
	case gast.KindEmphasis, gast.KindLink:
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			if !sdkMarkdownInlineNodeRemovable(child) {
				return false
			}
		}
		return true
	}
	if node.Kind() == extast.KindStrikethrough {
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			if !sdkMarkdownInlineNodeRemovable(child) {
				return false
			}
		}
		return true
	}
	return false
}

func normalizeSDKTitleText(source string) string {
	source = html.UnescapeString(source)
	source = norm.NFC.String(source)
	return strings.Join(strings.Fields(source), " ")
}
