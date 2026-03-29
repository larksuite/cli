// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"fmt"
	"unicode/utf8"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	blockTypeTable = 31
	blockTypeText  = 2

	minColumnWidth    = 80
	maxColumnWidth    = 400
	docContainerWidth = 700
	charUnitWidth     = 8 // approximate pixel width per character unit
)

// autoResizeTableColumns fetches all blocks from a document, finds table blocks,
// calculates optimal column widths based on cell content, and updates via API.
// Errors are non-fatal: returns a warning message or empty string on success.
func autoResizeTableColumns(runtime *common.RuntimeContext, documentID string) string {
	blocks, err := fetchAllBlocks(runtime, documentID)
	if err != nil {
		return fmt.Sprintf("table auto-width skipped: %v", err)
	}

	blockMap := make(map[string]map[string]interface{}, len(blocks))
	for _, b := range blocks {
		if m, ok := b.(map[string]interface{}); ok {
			if id, _ := m["block_id"].(string); id != "" {
				blockMap[id] = m
			}
		}
	}

	var warnings []string
	for _, b := range blocks {
		m, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		blockType, _ := m["block_type"].(float64)
		if int(blockType) != blockTypeTable {
			continue
		}
		blockID, _ := m["block_id"].(string)
		if blockID == "" {
			continue
		}
		if warn := resizeOneTable(runtime, documentID, blockID, m, blockMap); warn != "" {
			warnings = append(warnings, warn)
		}
	}

	if len(warnings) > 0 {
		return fmt.Sprintf("table auto-width partial: %v", warnings)
	}
	return ""
}

// fetchAllBlocks retrieves all document blocks with pagination.
func fetchAllBlocks(runtime *common.RuntimeContext, documentID string) ([]interface{}, error) {
	var allItems []interface{}
	var pageToken string
	for {
		params := map[string]interface{}{
			"page_size": 500,
		}
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		data, err := runtime.CallAPI("GET",
			fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks", validate.EncodePathSegment(documentID)),
			params, nil)
		if err != nil {
			return nil, err
		}

		items := common.GetSlice(data, "items")
		allItems = append(allItems, items...)

		if !common.GetBool(data, "has_more") {
			break
		}
		nextToken := common.GetString(data, "page_token")
		if nextToken == "" {
			break
		}
		pageToken = nextToken
	}
	return allItems, nil
}

// resizeOneTable calculates and applies optimal column widths for a single table.
func resizeOneTable(runtime *common.RuntimeContext, documentID, blockID string, tableBlock map[string]interface{}, blockMap map[string]map[string]interface{}) string {
	table := common.GetMap(tableBlock, "table")
	if table == nil {
		return ""
	}
	prop := common.GetMap(table, "property")
	if prop == nil {
		return ""
	}

	colSize := int(common.GetFloat(prop, "column_size"))
	rowSize := int(common.GetFloat(prop, "row_size"))
	if colSize == 0 || rowSize == 0 {
		return ""
	}

	// Get cell block IDs - they are ordered row by row, left to right
	children, _ := tableBlock["children"].([]interface{})
	if len(children) == 0 {
		return ""
	}

	// Calculate max content width for each column
	colMaxWidths := make([]int, colSize)
	for i, childID := range children {
		col := i % colSize
		cellID, _ := childID.(string)
		if cellID == "" {
			continue
		}
		w := cellContentWidth(cellID, blockMap)
		if w > colMaxWidths[col] {
			colMaxWidths[col] = w
		}
	}

	// Convert character widths to pixel widths with constraints
	columnWidths := computePixelWidths(colMaxWidths, colSize)

	// Check if widths actually differ from equal distribution
	equalWidth := docContainerWidth / colSize
	allEqual := true
	for _, w := range columnWidths {
		if w != equalWidth {
			allEqual = false
			break
		}
	}
	if allEqual {
		return ""
	}

	// Update table column widths via batch_update
	requests := []interface{}{
		map[string]interface{}{
			"block_id": blockID,
			"update_table_property": map[string]interface{}{
				"column_width": columnWidths,
			},
		},
	}
	_, err := runtime.CallAPI("PATCH",
		fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/batch_update", validate.EncodePathSegment(documentID)),
		nil, map[string]interface{}{"requests": requests})
	if err != nil {
		return fmt.Sprintf("failed to update table %s: %v", blockID, err)
	}
	return ""
}

// cellContentWidth returns the max text width (in character units) of a cell's content.
func cellContentWidth(cellID string, blockMap map[string]map[string]interface{}) int {
	cellBlock, ok := blockMap[cellID]
	if !ok {
		return 0
	}
	children, _ := cellBlock["children"].([]interface{})
	maxWidth := 0
	for _, childID := range children {
		id, _ := childID.(string)
		if id == "" {
			continue
		}
		child, ok := blockMap[id]
		if !ok {
			continue
		}
		w := blockTextWidth(child)
		if w > maxWidth {
			maxWidth = w
		}
	}
	return maxWidth
}

// blockTextWidth calculates the display width of text in a block.
// Chinese/fullwidth characters count as 2 units, ASCII as 1.
func blockTextWidth(block map[string]interface{}) int {
	blockType, _ := block["block_type"].(float64)
	if int(blockType) != blockTypeText {
		return 0
	}
	text := common.GetMap(block, "text")
	if text == nil {
		return 0
	}
	elements, _ := text["elements"].([]interface{})
	totalWidth := 0
	for _, elem := range elements {
		e, ok := elem.(map[string]interface{})
		if !ok {
			continue
		}
		textRun := common.GetMap(e, "text_run")
		if textRun == nil {
			continue
		}
		content, _ := textRun["content"].(string)
		totalWidth += stringDisplayWidth(content)
	}
	return totalWidth
}

// stringDisplayWidth calculates display width: CJK/fullwidth = 2, others = 1.
func stringDisplayWidth(s string) int {
	width := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size <= 1 {
			width++
			i++
			continue
		}
		if isWideChar(r) {
			width += 2
		} else {
			width++
		}
		i += size
	}
	return width
}

// isWideChar returns true for CJK and fullwidth characters.
func isWideChar(r rune) bool {
	return (r >= 0x1100 && r <= 0x115F) || // Hangul Jamo
		(r >= 0x2E80 && r <= 0x303E) || // CJK Radicals, Kangxi, Ideographic
		(r >= 0x3040 && r <= 0x33BF) || // Hiragana, Katakana, Bopomofo, CJK Compatibility
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
		(r >= 0x4E00 && r <= 0xA4CF) || // CJK Unified, Yi
		(r >= 0xA960 && r <= 0xA97C) || // Hangul Jamo Extended-A
		(r >= 0xAC00 && r <= 0xD7FF) || // Hangul Syllables, Hangul Jamo Extended-B
		(r >= 0xF900 && r <= 0xFAFF) || // CJK Compatibility Ideographs
		(r >= 0xFE30 && r <= 0xFE6F) || // CJK Compatibility Forms, Small Form Variants
		(r >= 0xFF01 && r <= 0xFF60) || // Fullwidth Forms
		(r >= 0xFFE0 && r <= 0xFFE6) || // Fullwidth Signs
		(r >= 0x20000 && r <= 0x2FA1F) // CJK Extension B-F, Compatibility Supplement
}

// computePixelWidths converts character-unit widths to pixel widths
// with min/max constraints and total width normalization.
func computePixelWidths(charWidths []int, colSize int) []int {
	pixelWidths := make([]int, colSize)
	for i, cw := range charWidths {
		pw := cw * charUnitWidth
		if pw < minColumnWidth {
			pw = minColumnWidth
		}
		if pw > maxColumnWidth {
			pw = maxColumnWidth
		}
		pixelWidths[i] = pw
	}

	// Normalize to fit within container width
	total := 0
	for _, w := range pixelWidths {
		total += w
	}
	if total > docContainerWidth && total > 0 {
		scale := float64(docContainerWidth) / float64(total)
		newTotal := 0
		for i := range pixelWidths {
			pixelWidths[i] = int(float64(pixelWidths[i]) * scale)
			if pixelWidths[i] < minColumnWidth {
				pixelWidths[i] = minColumnWidth
			}
			newTotal += pixelWidths[i]
		}
	}

	return pixelWidths
}
