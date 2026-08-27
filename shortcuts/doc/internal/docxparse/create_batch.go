// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

import (
	"fmt"
	"strconv"
	"strings"
)

type CreateBatchPlan struct {
	Batches     []string
	BatchBlocks []int
	TotalBlocks int
}

type CreateBatchLimits struct {
	TargetBlocks    int
	OperationBlocks int
	TotalBlocks     int
	Content         ContentLimits
}

type CreateBatchPlanErrorKind string

const (
	CreateBatchTotalLimit       CreateBatchPlanErrorKind = "total_limit"
	CreateBatchSubtreeLimit     CreateBatchPlanErrorKind = "subtree_limit"
	CreateBatchInitialCapacity  CreateBatchPlanErrorKind = "initial_capacity"
	CreateBatchTitleAfterCreate CreateBatchPlanErrorKind = "title_after_create"
)

type CreateBatchPlanError struct {
	Kind   CreateBatchPlanErrorKind
	Tag    string
	Blocks int
	Limit  int
}

func (e *CreateBatchPlanError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("create batch plan %s: tag=%s blocks=%d limit=%d", e.Kind, e.Tag, e.Blocks, e.Limit)
}

// PlanCreateBatches validates strict XML and splits only between top-level
// elements, preserving every source byte. A parse error is returned to the
// caller so existing compatibility-capable create behavior can remain intact.
func PlanCreateBatches(source string, batchLimit, totalLimit int) (CreateBatchPlan, error) {
	return PlanCreateBatchesWithLimits(source, CreateBatchLimits{
		TargetBlocks:    batchLimit,
		OperationBlocks: batchLimit,
		TotalBlocks:     totalLimit,
		Content:         DefaultContentLimits(),
	})
}

func PlanCreateBatchesWithLimits(source string, limits CreateBatchLimits) (CreateBatchPlan, error) {
	if err := validateCreateBatchLimits(limits); err != nil {
		return CreateBatchPlan{}, err
	}
	nodes, err := parseXML(source)
	if err != nil {
		return CreateBatchPlan{}, err
	}
	elements := topLevelElements(nodes)
	spans, err := topLevelElementSpans(source)
	if err != nil {
		return CreateBatchPlan{}, err
	}
	if len(elements) != len(spans) {
		return CreateBatchPlan{}, newParseError("top-level XML element count mismatch")
	}

	hasTitle := false
	units := make([]createBatchUnit, len(elements))
	statistics := ContentStatistics{}
	for i, node := range elements {
		if node.tag == "title" {
			hasTitle = true
		}
		unitStatistics := collectXMLContentStatistics([]*Node{node})
		statistics = statistics.merge(unitStatistics)
		units[i] = createBatchUnit{
			start:          spans[i].start,
			tag:            node.tag,
			blocks:         unitStatistics.Blocks,
			requiresCreate: node.tag == "title",
		}
	}
	if err := validateContentStatistics(statistics, limits.Content); err != nil {
		return CreateBatchPlan{}, err
	}
	return packCreateUnits(source, units, hasTitle, limits)
}

func validateCreateBatchLimits(limits CreateBatchLimits) error {
	if limits.TargetBlocks <= 0 || limits.OperationBlocks <= 0 || limits.TotalBlocks <= 0 {
		return newParseError("create batch limits must be positive")
	}
	if limits.TargetBlocks > limits.OperationBlocks {
		return newParseError("create batch target %d exceeds operation limit %d", limits.TargetBlocks, limits.OperationBlocks)
	}
	return validateContentLimits(limits.Content)
}

type createBatchUnit struct {
	start          int
	tag            string
	blocks         int
	requiresCreate bool
}

func packCreateUnits(source string, units []createBatchUnit, hasTitle bool, limits CreateBatchLimits) (CreateBatchPlan, error) {
	totalBlocks := 0
	for _, unit := range units {
		totalBlocks = saturatedAdd(totalBlocks, unit.blocks)
	}
	if !hasTitle {
		totalBlocks = saturatedAdd(totalBlocks, 1) // implicit page/title block
	}
	if totalBlocks > limits.TotalBlocks {
		return CreateBatchPlan{}, &CreateBatchPlanError{
			Kind:   CreateBatchTotalLimit,
			Blocks: totalBlocks,
			Limit:  limits.TotalBlocks,
		}
	}
	if totalBlocks <= limits.TargetBlocks || len(units) == 0 {
		return CreateBatchPlan{
			Batches:     []string{source},
			BatchBlocks: []int{totalBlocks},
			TotalBlocks: totalBlocks,
		}, nil
	}

	plan := CreateBatchPlan{TotalBlocks: totalBlocks}
	batchStartByte := 0
	batchStartElement := 0
	batchBlocks := 0
	if !hasTitle {
		batchBlocks = 1
	}
	for i, unit := range units {
		blocks := unit.blocks
		if blocks > limits.OperationBlocks {
			return CreateBatchPlan{}, &CreateBatchPlanError{
				Kind:   CreateBatchSubtreeLimit,
				Tag:    unit.tag,
				Blocks: blocks,
				Limit:  limits.OperationBlocks,
			}
		}
		if saturatedAdd(batchBlocks, blocks) > limits.TargetBlocks {
			if i == batchStartElement {
				if saturatedAdd(batchBlocks, blocks) > limits.OperationBlocks {
					return CreateBatchPlan{}, &CreateBatchPlanError{
						Kind:   CreateBatchInitialCapacity,
						Tag:    unit.tag,
						Blocks: blocks,
						Limit:  limits.OperationBlocks - batchBlocks,
					}
				}
			} else {
				plan.Batches = append(plan.Batches, source[batchStartByte:unit.start])
				plan.BatchBlocks = append(plan.BatchBlocks, batchBlocks)
				batchStartByte = unit.start
				batchStartElement = i
				batchBlocks = 0
			}
		}
		if unit.requiresCreate && len(plan.Batches) > 0 {
			return CreateBatchPlan{}, &CreateBatchPlanError{
				Kind:   CreateBatchTitleAfterCreate,
				Tag:    unit.tag,
				Blocks: blocks,
				Limit:  limits.OperationBlocks,
			}
		}
		batchBlocks = saturatedAdd(batchBlocks, blocks)
	}
	plan.Batches = append(plan.Batches, source[batchStartByte:])
	plan.BatchBlocks = append(plan.BatchBlocks, batchBlocks)
	return plan, nil
}

type sourceSpan struct {
	start int
}

func topLevelElements(nodes []*Node) []*Node {
	elements := make([]*Node, 0, len(nodes))
	for _, node := range nodes {
		if node != nil && node.typ == nodeElement {
			elements = append(elements, node)
		}
	}
	return elements
}

func topLevelElementSpans(source string) ([]sourceSpan, error) {
	var spans []sourceSpan
	depth := 0
	currentStart := -1
	for offset := 0; offset < len(source); {
		relative := strings.IndexByte(source[offset:], '<')
		if relative < 0 {
			break
		}
		start := offset + relative
		token, end, state := scanXMLToken(source, start)
		switch state {
		case tokenComment, tokenCDATA, tokenProcessingInstruction:
			offset = end
			continue
		case tokenOK:
		default:
			return nil, newParseError("invalid XML token at byte %d", start)
		}

		if token.closing {
			depth--
			if depth == 0 && currentStart >= 0 {
				spans = append(spans, sourceSpan{start: currentStart})
				currentStart = -1
			}
			offset = end
			continue
		}
		if depth == 0 {
			currentStart = start
		}
		if token.selfClosing || isVoidTag(token.name) {
			if depth == 0 {
				spans = append(spans, sourceSpan{start: start})
				currentStart = -1
			}
		} else {
			depth++
		}
		offset = end
	}
	return spans, nil
}

func materializedBlockCount(node *Node) int {
	return collectXMLContentStatistics([]*Node{node}).Blocks
}

func isMaterializedBlock(node *Node) bool {
	if node == nil || node.typ != nodeElement {
		return false
	}
	switch node.tag {
	case "td", "th":
		return true
	case "code":
		return node.parent == nil || node.parent.tag != "pre" && !isInlineCodeContainer(node.parent)
	case "latex":
		return node.parent == nil || !isRichTextContainer(node.parent)
	case "ul", "ol", "thead", "tbody", "tfoot", "tr", "div", "append":
		return false
	default:
		return layoutOf(node.tag) == layoutBlock && !isOKRRichTextShell(node)
	}
}

func tableRowsForBatch(table *Node) []*Node {
	var rows []*Node
	var visit func([]*Node)
	visit = func(nodes []*Node) {
		for _, node := range nodes {
			if node == nil || node.typ != nodeElement {
				continue
			}
			switch node.tag {
			case "tr":
				rows = append(rows, node)
			case "thead", "tbody", "tfoot":
				visit(node.children)
			}
		}
	}
	visit(table.children)
	return rows
}

type tablePosition struct {
	row int
	col int
}

func tableDimensionsForBatch(rows []*Node) (rowCount, columnCount int, visibleCells []*Node) {
	occupied := make(map[tablePosition]struct{})
	for rowIndex, row := range rows {
		rowCount = maxInt(rowCount, rowIndex+1)
		columnIndex := 0
		for _, cell := range row.children {
			if cell == nil || cell.typ != nodeElement || cell.tag != "td" && cell.tag != "th" {
				continue
			}
			for isBatchTablePositionOccupied(occupied, rowIndex, columnIndex) {
				columnIndex++
			}
			rowspan := positiveSpan(cell.attrs["rowspan"])
			colspan := positiveSpan(cell.attrs["colspan"])
			visibleCells = append(visibleCells, cell)
			for rowOffset := 0; rowOffset < minInt(rowspan, 2_001); rowOffset++ {
				for columnOffset := 0; columnOffset < minInt(colspan, 101); columnOffset++ {
					occupied[tablePosition{
						row: saturatedAdd(rowIndex, rowOffset),
						col: saturatedAdd(columnIndex, columnOffset),
					}] = struct{}{}
				}
			}
			rowCount = maxInt(rowCount, saturatedAdd(rowIndex, rowspan))
			columnCount = maxInt(columnCount, saturatedAdd(columnIndex, colspan))
			columnIndex = saturatedAdd(columnIndex, colspan)
		}
	}
	return rowCount, columnCount, visibleCells
}

func isBatchTablePositionOccupied(occupied map[tablePosition]struct{}, row, column int) bool {
	_, ok := occupied[tablePosition{row: row, col: column}]
	return ok
}

func isTableCellDispatchedBlock(node *Node) bool {
	if node == nil || node.typ != nodeElement {
		return false
	}
	switch node.tag {
	case "b", "em", "u", "del", "code", "a", "span", "cite", "time", "source", "button", "br":
		return false
	default:
		return isMaterializedBlock(node)
	}
}

func isRichTextContainer(node *Node) bool {
	if node == nil {
		return false
	}
	switch node.tag {
	case "title", "h1", "h2", "h3", "h4", "h5", "h6", "h7", "h8", "h9",
		"p", "li", "checkbox", "code", "latex", "okr-objective", "okr-key-result":
		return true
	default:
		return false
	}
}

func isInlineFileContainer(node *Node) bool {
	return isRichTextContainer(node) || isTableCellNode(node)
}

func isInlineCodeContainer(node *Node) bool {
	return isRichTextContainer(node) || isTableCellNode(node)
}

func isTableCellNode(node *Node) bool {
	return node != nil && (node.tag == "td" || node.tag == "th")
}

func isFigureParent(node *Node) bool {
	return node != nil && node.tag == "figure"
}

func isOKRRichTextShell(node *Node) bool {
	return node != nil && node.tag == "p" && node.parent != nil &&
		(node.parent.tag == "okr-objective" || node.parent.tag == "okr-key-result") &&
		node.attrs["id"] == ""
}

func positiveSpan(raw string) int {
	span, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || span <= 0 {
		return 1
	}
	return span
}

func saturatedAdd(left, right int) int {
	max := int(^uint(0) >> 1)
	if right > 0 && left > max-right {
		return max
	}
	return left + right
}

func saturatedMultiply(left, right int) int {
	max := int(^uint(0) >> 1)
	if left <= 0 || right <= 0 {
		return 0
	}
	if left > max/right {
		return max
	}
	return left * right
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
