// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

import (
	"fmt"
	"unicode/utf8"
)

// ContentStatistics is the bounded summary needed to reject a create payload
// before it is split into write operations. Character counts use UTF-16 code
// units, matching DocX RichText validation in docx_xml-go.
type ContentStatistics struct {
	Blocks             int
	MaxBlockCharacters int
	MaxTableCells      int
	MaxTableColumns    int
}

func (s ContentStatistics) merge(other ContentStatistics) ContentStatistics {
	return ContentStatistics{
		Blocks:             saturatedAdd(s.Blocks, other.Blocks),
		MaxBlockCharacters: maxInt(s.MaxBlockCharacters, other.MaxBlockCharacters),
		MaxTableCells:      maxInt(s.MaxTableCells, other.MaxTableCells),
		MaxTableColumns:    maxInt(s.MaxTableColumns, other.MaxTableColumns),
	}
}

// ContentLimits mirrors docx_xml-go's create content limits. It is kept
// separate from batch packing limits so parsing/statistics remain the single
// owner of content semantics while the caller owns write orchestration.
type ContentLimits struct {
	BlockCharacters int
	TableCells      int
	TableColumns    int
}

func DefaultContentLimits() ContentLimits {
	return ContentLimits{
		BlockCharacters: 100_000,
		TableCells:      2_000,
		TableColumns:    100,
	}
}

type ContentLimitKind string

const (
	ContentLimitBlockCharacters ContentLimitKind = "block_char_limit"
	ContentLimitTableCells      ContentLimitKind = "table_cell_limit"
	ContentLimitTableColumns    ContentLimitKind = "table_column_limit"
)

// ContentLimitError reports one violation in the same required order as the
// SDK policy: block characters, table cells, then table columns.
type ContentLimitError struct {
	Kind   ContentLimitKind
	Actual int
	Limit  int
}

func (e *ContentLimitError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("create content %s: actual=%d limit=%d", e.Kind, e.Actual, e.Limit)
}

func validateContentLimits(limits ContentLimits) error {
	if limits.BlockCharacters <= 0 || limits.TableCells <= 0 || limits.TableColumns <= 0 {
		return newParseError("create content limits must be positive")
	}
	return nil
}

func validateContentStatistics(stats ContentStatistics, limits ContentLimits) error {
	if stats.MaxBlockCharacters > limits.BlockCharacters {
		return &ContentLimitError{Kind: ContentLimitBlockCharacters, Actual: stats.MaxBlockCharacters, Limit: limits.BlockCharacters}
	}
	if stats.MaxTableCells > limits.TableCells {
		return &ContentLimitError{Kind: ContentLimitTableCells, Actual: stats.MaxTableCells, Limit: limits.TableCells}
	}
	if stats.MaxTableColumns > limits.TableColumns {
		return &ContentLimitError{Kind: ContentLimitTableColumns, Actual: stats.MaxTableColumns, Limit: limits.TableColumns}
	}
	return nil
}

// ValidateCompatibleXMLContentLimits is the compatibility-path safety net for
// XML that the strict batch planner cannot partition. It intentionally checks
// only content limits; malformed-but-tolerated XML keeps the existing single
// create request behavior.
func ValidateCompatibleXMLContentLimits(source string, limits ContentLimits) error {
	if err := validateContentLimits(limits); err != nil {
		return err
	}
	nodes, err := parseXMLCompatible(source)
	if err != nil {
		return err
	}
	return validateContentStatistics(collectXMLContentStatistics(nodes), limits)
}

type contentStatisticsCollector struct {
	stats ContentStatistics
}

func collectXMLContentStatistics(nodes []*Node) ContentStatistics {
	collector := contentStatisticsCollector{}
	collector.collectNodes(nodes)
	return collector.stats
}

func (c *contentStatisticsCollector) collectNodes(nodes []*Node) {
	for _, node := range nodes {
		c.collectNode(node)
	}
}

func (c *contentStatisticsCollector) collectNode(node *Node) {
	if node == nil || node.typ != nodeElement {
		return
	}
	if node.tag == "table" {
		c.collectTable(node)
		return
	}

	counted := isMaterializedBlock(node)
	if counted {
		c.addBlocks(1)
		c.observeTextBlock(node)
	}

	// A standalone file lowers to View + File. A figure already accounts for
	// the View; a file inside rich text is inline and has no View wrapper.
	if node.tag == "source" && counted && !isFigureParent(node.parent) && !isInlineFileContainer(node.parent) {
		c.addBlocks(1)
	}

	if isOKRRichTextShell(node) {
		return
	}
	c.collectNodes(node.children)
}

func (c *contentStatisticsCollector) addBlocks(count int) {
	c.stats.Blocks = saturatedAdd(c.stats.Blocks, count)
}

func (c *contentStatisticsCollector) observeTextBlock(node *Node) {
	if characters, ok := materializedTextCharacters(node); ok {
		c.stats.MaxBlockCharacters = maxInt(c.stats.MaxBlockCharacters, characters)
	}
	if characters, ok := captionCharacters(node); ok {
		c.stats.MaxBlockCharacters = maxInt(c.stats.MaxBlockCharacters, characters)
	}
}

func (c *contentStatisticsCollector) collectTable(table *Node) {
	rows := tableRowsForBatch(table)
	rowCount, columnCount, visibleCells := tableDimensionsForBatch(rows)
	rowCount = maxInt(rowCount, 1)
	columnCount = maxInt(columnCount, 1)
	physicalCells := saturatedMultiply(rowCount, columnCount)

	c.addBlocks(1)
	c.addBlocks(physicalCells)
	c.stats.MaxTableCells = maxInt(c.stats.MaxTableCells, physicalCells)
	c.stats.MaxTableColumns = maxInt(c.stats.MaxTableColumns, columnCount)
	if characters, ok := captionCharacters(table); ok {
		c.stats.MaxBlockCharacters = maxInt(c.stats.MaxBlockCharacters, characters)
	}

	// Every physical cell starts with one empty text child. Explicit visible
	// cells replace it; merged/missing placeholders retain the default child.
	placeholderCells := physicalCells - len(visibleCells)
	if placeholderCells > 0 {
		c.addBlocks(placeholderCells)
	}
	for _, cell := range visibleCells {
		before := c.stats.Blocks
		c.collectNodes(cell.children)
		segments, maxCharacters := tableCellInlineSegments(cell)
		if segments > 0 {
			c.addBlocks(segments)
			c.stats.MaxBlockCharacters = maxInt(c.stats.MaxBlockCharacters, maxCharacters)
		} else if c.stats.Blocks == before {
			c.addBlocks(1)
		}
	}
}

func tableCellInlineSegments(cell *Node) (count, maxCharacters int) {
	currentCharacters := 0
	active := false
	flush := func() {
		if !active {
			return
		}
		count++
		maxCharacters = maxInt(maxCharacters, currentCharacters)
		currentCharacters = 0
		active = false
	}
	for _, child := range cell.children {
		if isTableCellDispatchedBlock(child) {
			flush()
			continue
		}
		characters, meaningful := inlineNodeCharacters(child)
		if !meaningful {
			continue
		}
		active = true
		currentCharacters = saturatedAdd(currentCharacters, characters)
	}
	flush()
	return count, maxCharacters
}

func inlineNodeCharacters(node *Node) (int, bool) {
	if node == nil {
		return 0, false
	}
	if node.typ == nodeText {
		characters := utf16CodeUnits(node.text)
		return characters, characters > 0
	}
	if node.tag == "br" || isInlinePlaceholder(node) {
		return 1, true
	}
	characters := inlineCharacters(node)
	return characters, characters > 0
}

func materializedTextCharacters(node *Node) (int, bool) {
	switch node.tag {
	case "title", "h1", "h2", "h3", "h4", "h5", "h6", "h7", "h8", "h9",
		"p", "li", "checkbox", "code", "latex", "okr-objective", "okr-key-result", "pre":
		return inlineCharacters(node), true
	default:
		return 0, false
	}
}

func captionCharacters(node *Node) (int, bool) {
	switch node.tag {
	case "table", "img", "whiteboard", "pre", "code":
		caption := node.attrs["caption"]
		if caption != "" {
			return utf16CodeUnits(caption), true
		}
	}
	return 0, false
}

func inlineCharacters(node *Node) int {
	if node == nil {
		return 0
	}
	count := 0
	var walk func(*Node, bool)
	walk = func(current *Node, root bool) {
		if current == nil {
			return
		}
		if current.typ == nodeText {
			count = saturatedAdd(count, utf16CodeUnits(current.text))
			return
		}
		if !root {
			if current.tag == "br" {
				count = saturatedAdd(count, 1)
				return
			}
			if isInlinePlaceholder(current) {
				count = saturatedAdd(count, 1)
				return
			}
			if isMaterializedBlock(current) {
				return
			}
		}
		for _, child := range current.children {
			walk(child, false)
		}
	}
	walk(node, true)
	return count
}

func isInlinePlaceholder(node *Node) bool {
	if node == nil {
		return false
	}
	switch node.tag {
	case "cite", "time", "mention-date", "inline-file", "button":
		return true
	case "source":
		return isInlineFileContainer(node.parent)
	case "a":
		return node.attrs["type"] == "url-preview"
	default:
		return false
	}
}

func utf16CodeUnits(value string) int {
	count := 0
	for len(value) > 0 {
		r, size := utf8.DecodeRuneInString(value)
		if size == 0 {
			break
		}
		value = value[size:]
		count = saturatedAdd(count, utf16Units(r))
	}
	return count
}
