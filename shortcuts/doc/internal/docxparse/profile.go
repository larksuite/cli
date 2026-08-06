// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Profile describes LarkOpenCLI document structure and visible text without
// requiring callers to inspect the full XML.
type Profile struct {
	WordCount  int           `json:"word_count"`
	CharCount  int           `json:"char_count"`
	Breakdown  TextBreakdown `json:"breakdown"`
	BlockCount int           `json:"block_count"`
	Blocks     []BlockShare  `json:"blocks"`
}

// BlockShare reports one LarkOpenCLI block type's count and share. Structural
// and inline-only tags are intentionally excluded.
type BlockShare struct {
	Type  string  `json:"type"`
	Count int     `json:"count"`
	Ratio float64 `json:"ratio"`
}

// TextProfile is the internal result of the LarkOpenCLI semantic counter.
type TextProfile struct {
	WordCount int           `json:"word_count"`
	CharCount int           `json:"char_count"`
	Breakdown TextBreakdown `json:"breakdown"`
}

type TextBreakdown struct {
	HanChars            int `json:"han_chars"`
	EnglishWords        int `json:"english_words"`
	NumberWords         int `json:"number_words"`
	ChinesePunctuations int `json:"chinese_punctuations"`
	EnglishLetters      int `json:"english_letters"`
	Digits              int `json:"digits"`
	EnglishPunctuations int `json:"english_punctuations"`
	SymbolWords         int `json:"symbol_words"`
	SymbolChars         int `json:"symbol_chars"`
}

// Parse checks XML syntax, then builds its structure and visible-text profile.
// XML business semantics are deliberately left to the document writer and
// service.
func Parse(source string, format Format) (ParseResult, error) {
	if format != FormatXML {
		return ParseResult{}, newParseError("unsupported input format %q", format)
	}
	nodes, err := parseXML(source)
	if err != nil {
		return ParseResult{}, err
	}
	if err := validateNestingDepth(nodes); err != nil {
		return ParseResult{}, err
	}
	return ParseResult{
		Format:  format,
		XML:     source,
		Profile: buildProfile(nodes),
	}, nil
}

// ParseCompatibleXML builds a profile after deterministic recovery of common
// malformed XML emitted while authoring a draft.
func ParseCompatibleXML(source string) (Profile, error) {
	if err := validateSource(source); err != nil {
		return Profile{}, err
	}
	trimmed := strings.TrimSpace(strings.TrimPrefix(source, "\uFEFF"))
	if !strings.HasPrefix(trimmed, "<") {
		return Profile{}, newParseError("XML input must begin with '<'")
	}
	nodes, err := parseXMLCompatible(source)
	if err != nil {
		return Profile{}, err
	}
	return buildProfile(nodes), nil
}

func validateNestingDepth(nodes []*Node) error {
	type frame struct {
		node *Node
		exit bool
	}
	frames := make([]frame, 0, len(nodes))
	for i := len(nodes) - 1; i >= 0; i-- {
		frames = append(frames, frame{node: nodes[i]})
	}
	depth := 0
	for len(frames) > 0 {
		current := frames[len(frames)-1]
		frames = frames[:len(frames)-1]
		node := current.node
		if node == nil || node.typ != nodeElement {
			continue
		}
		if current.exit {
			depth--
			continue
		}
		if depth >= MaxNestingDepth {
			return newParseError("document nesting exceeds limit %d at <%s>", MaxNestingDepth, node.tag)
		}
		depth++
		frames = append(frames, frame{node: node, exit: true})
		for i := len(node.children) - 1; i >= 0; i-- {
			frames = append(frames, frame{node: node.children[i]})
		}
	}
	return nil
}

func buildProfile(nodes []*Node) Profile {
	counts := map[string]int{}
	total := 0
	var walk func(*Node)
	walk = func(node *Node) {
		if node == nil || node.typ != nodeElement {
			return
		}
		layout := layoutOf(node.tag)
		isBlock := layout == layoutBlock || layout == layoutDual && node.parent == nil
		if isBlock {
			counts[node.tag]++
			total++
		}
		for _, child := range node.children {
			walk(child)
		}
	}
	for _, node := range nodes {
		walk(node)
	}

	distribution := make([]BlockShare, 0, len(counts))
	for typ, count := range counts {
		ratio := 0.0
		if total > 0 {
			ratio = math.Round(float64(count)/float64(total)*1_000_000) / 1_000_000
		}
		distribution = append(distribution, BlockShare{Type: typ, Count: count, Ratio: ratio})
	}
	sort.Slice(distribution, func(i, j int) bool {
		if distribution[i].Count != distribution[j].Count {
			return distribution[i].Count > distribution[j].Count
		}
		return distribution[i].Type < distribution[j].Type
	})
	segments := extractSegments(nodes)
	stats := newTextCounter().countSegments(segments)
	return Profile{
		WordCount:  stats.WordCount,
		CharCount:  stats.CharCount,
		Breakdown:  stats.Breakdown,
		BlockCount: total,
		Blocks:     distribution,
	}
}

type segmentKind uint8

const (
	segmentText segmentKind = iota
	segmentMarker
	segmentCode
)

type textSegment struct {
	text string
	kind segmentKind
}

var ignoredResourceTags = map[string]bool{
	"whiteboard": true, "sheet": true, "source": true, "chat_card": true,
	"base_refer": true, "bitable": true, "synced_reference": true,
	"poll": true, "isv": true, "mindnote": true, "sub-page-list": true,
	"okr": true, "html5-block": true,
}

var ignoredInlineTags = map[string]bool{
	"button": true, "cite": true, "latex": true, "bookmark": true,
}

func extractSegments(nodes []*Node) []textSegment {
	var segments []textSegment
	for _, node := range nodes {
		extractNodeSegments(node, &segments)
	}
	return segments
}

func extractNodeSegments(node *Node, segments *[]textSegment) {
	if node == nil {
		return
	}
	if node.typ == nodeText {
		if strings.TrimSpace(node.text) != "" {
			*segments = append(*segments, textSegment{text: node.text})
		}
		return
	}
	if ignoredInlineTags[node.tag] || ignoredResourceTags[node.tag] {
		return
	}
	if node.tag == "task" {
		return
	}
	if node.tag == "synced-source" && len(node.children) == 0 {
		return
	}

	switch node.tag {
	case "ul", "ol":
		sequence := 1
		for _, child := range node.children {
			if child.typ == nodeElement && child.tag == "li" {
				if node.tag == "ul" {
					*segments = append(*segments, textSegment{text: "•", kind: segmentMarker})
				} else {
					marker := sequence
					if raw := child.attrs["seq"]; raw != "" {
						if _, err := fmt.Sscanf(raw, "%d", &marker); err == nil {
							sequence = marker
						}
					}
					*segments = append(*segments, textSegment{text: fmt.Sprintf("%d.", marker), kind: segmentMarker})
					sequence++
				}
			}
			extractNodeSegments(child, segments)
		}
		return
	case "checkbox":
		marker := "☐"
		if node.attrs["done"] == "true" {
			marker = "☑"
		}
		*segments = append(*segments, textSegment{text: marker, kind: segmentMarker})
	}

	kind := segmentText
	if node.tag == "pre" || node.tag == "code" && (node.parent == nil || node.parent.tag != "p") {
		kind = segmentCode
	}
	text := visibleInlineText(node)
	if strings.TrimSpace(text) == "" && !hasBlockChildren(node) {
		if node.tag == "img" {
			text = node.attrs["caption"]
		} else {
			text = firstNonEmpty(node.attrs["text"], node.attrs["name"], node.attrs["title"], node.attrs["alt"], node.attrs["caption"])
		}
	}
	if strings.TrimSpace(text) != "" {
		*segments = append(*segments, textSegment{text: text, kind: kind})
	}

	for _, child := range node.children {
		if child.typ != nodeElement || isInlineForExtraction(child.tag) {
			continue
		}
		extractNodeSegments(child, segments)
	}
}

func visibleInlineText(node *Node) string {
	var out strings.Builder
	var walk func(*Node)
	walk = func(current *Node) {
		if current.typ == nodeText {
			out.WriteString(current.text)
			return
		}
		if current != node && !isInlineForExtraction(current.tag) {
			return
		}
		if ignoredInlineTags[current.tag] {
			return
		}
		if current.tag == "br" {
			out.WriteByte('\n')
			return
		}
		before := out.Len()
		for _, child := range current.children {
			walk(child)
		}
		if current != node && out.Len() == before {
			if display := firstNonEmpty(current.attrs["text"], current.attrs["name"], current.attrs["title"], current.attrs["alt"]); display != "" {
				out.WriteString(display)
			}
		}
	}
	for _, child := range node.children {
		walk(child)
	}
	return out.String()
}

func hasBlockChildren(node *Node) bool {
	for _, child := range node.children {
		if child.typ == nodeElement && !isInlineForExtraction(child.tag) {
			return true
		}
	}
	return false
}

func isInlineForExtraction(tag string) bool {
	layout := layoutOf(tag)
	return layout == layoutInline || layout == layoutDual
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
