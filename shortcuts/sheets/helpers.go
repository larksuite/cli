// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var (
	singleCellRangePattern = regexp.MustCompile(`^[A-Za-z]+[1-9][0-9]*$`)
	cellSpanRangePattern   = regexp.MustCompile(`^[A-Za-z]+[1-9][0-9]*:[A-Za-z]+[1-9][0-9]*$`)
	cellToColRangePattern  = regexp.MustCompile(`^[A-Za-z]+[1-9][0-9]*:[A-Za-z]+$`)
	colSpanRangePattern    = regexp.MustCompile(`^[A-Za-z]+:[A-Za-z]+$`)
	rowSpanRangePattern    = regexp.MustCompile(`^[1-9][0-9]*:[1-9][0-9]*$`)
	cellRefPattern         = regexp.MustCompile(`^([A-Za-z]+)([1-9][0-9]*)$`)
)

var sheetRangeSeparatorReplacer = strings.NewReplacer(`\！`, "!", `\!`, "!", "！", "!")

// getFirstSheetID queries the spreadsheet and returns the first sheet's ID.
func getFirstSheetID(runtime *common.RuntimeContext, spreadsheetToken string) (string, error) {
	data, err := runtime.CallAPI("GET", fmt.Sprintf("/open-apis/sheets/v3/spreadsheets/%s/sheets/query", validate.EncodePathSegment(spreadsheetToken)), nil, nil)
	if err != nil {
		return "", err
	}
	sheets, _ := data["sheets"].([]interface{})
	if len(sheets) > 0 {
		sheet, _ := sheets[0].(map[string]interface{})
		if id, ok := sheet["sheet_id"].(string); ok && id != "" {
			return id, nil
		}
	}
	return "", output.Errorf(output.ExitAPI, "not_found", "no sheets found in this spreadsheet")
}

// extractSpreadsheetToken extracts spreadsheet token from URL.
func extractSpreadsheetToken(input string) string {
	input = strings.TrimSpace(input)
	prefixes := []string{"/sheets/", "/spreadsheets/"}
	for _, prefix := range prefixes {
		if idx := strings.Index(input, prefix); idx >= 0 {
			token := input[idx+len(prefix):]
			if idx2 := strings.IndexAny(token, "/?#"); idx2 >= 0 {
				token = token[:idx2]
			}
			return token
		}
	}
	return input
}

func normalizeSheetRange(sheetID, input string) string {
	input = normalizeSheetRangeSeparators(input)
	if input == "" || strings.Contains(input, "!") || sheetID == "" {
		return input
	}
	if looksLikeRelativeRange(input) {
		return sheetID + "!" + input
	}
	return input
}

func normalizePointRange(sheetID, input string) string {
	input = normalizeSheetRange(sheetID, input)
	if input == "" {
		return input
	}
	rangeSheetID, subRange, ok := splitSheetRange(input)
	if !ok || !singleCellRangePattern.MatchString(subRange) {
		return input
	}
	return rangeSheetID + "!" + subRange + ":" + subRange
}

func normalizeWriteRange(sheetID, input string, values interface{}) string {
	rows, cols := matrixDimensions(values)
	input = normalizeSheetRangeSeparators(input)
	if input == "" {
		return buildRectRange(sheetID, "A1", rows, cols)
	}

	input = normalizeSheetRange(sheetID, input)
	rangeSheetID, subRange, ok := splitSheetRange(input)
	if !ok {
		return buildRectRange(input, "A1", rows, cols)
	}
	if singleCellRangePattern.MatchString(subRange) {
		return buildRectRange(rangeSheetID, subRange, rows, cols)
	}
	return input
}

func validateSheetRangeInput(sheetID, input string) error {
	input = normalizeSheetRangeSeparators(input)
	if input == "" || strings.Contains(input, "!") || sheetID != "" {
		return nil
	}
	if looksLikeRelativeRange(input) {
		return common.FlagErrorf("--range %q requires --sheet-id or a <sheetId>! prefix", input)
	}
	return nil
}

// validateSingleCellRange rejects multi-cell spans (e.g. "A1:B2") that are
// invalid for single-cell operations like write-image. Empty and single-cell
// values pass through.
func validateSingleCellRange(input string) error {
	input = normalizeSheetRangeSeparators(input)
	if input == "" {
		return nil
	}
	// Extract the sub-range after the sheet ID prefix, if present.
	subRange := input
	if _, sr, ok := splitSheetRange(input); ok {
		subRange = sr
	}
	if cellSpanRangePattern.MatchString(subRange) {
		parts := strings.SplitN(subRange, ":", 2)
		if strings.EqualFold(parts[0], parts[1]) {
			return nil
		}
		return common.FlagErrorf("--range %q must be a single cell (e.g. A1 or A1:A1), got a multi-cell span", input)
	}
	return nil
}

func looksLikeRelativeRange(input string) bool {
	input = normalizeSheetRangeSeparators(input)
	if input == "" {
		return false
	}
	return singleCellRangePattern.MatchString(input) ||
		cellSpanRangePattern.MatchString(input) ||
		cellToColRangePattern.MatchString(input) ||
		colSpanRangePattern.MatchString(input) ||
		rowSpanRangePattern.MatchString(input)
}

func splitSheetRange(input string) (sheetID, subRange string, ok bool) {
	parts := strings.SplitN(normalizeSheetRangeSeparators(input), "!", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func normalizeSheetRangeSeparators(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return input
	}
	return sheetRangeSeparatorReplacer.Replace(input)
}

func buildRectRange(sheetID, anchor string, rows, cols int) string {
	if sheetID == "" {
		return ""
	}
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}
	endCell, err := offsetCell(anchor, rows-1, cols-1)
	if err != nil {
		return sheetID
	}
	return sheetID + "!" + anchor + ":" + endCell
}

func matrixDimensions(values interface{}) (rows, cols int) {
	rowList, ok := values.([]interface{})
	if !ok || len(rowList) == 0 {
		return 1, 1
	}
	rows = len(rowList)
	for _, row := range rowList {
		if cells, ok := row.([]interface{}); ok && len(cells) > cols {
			cols = len(cells)
		}
	}
	if cols == 0 {
		cols = 1
	}
	return rows, cols
}

func offsetCell(cell string, rowOffset, colOffset int) (string, error) {
	matches := cellRefPattern.FindStringSubmatch(strings.TrimSpace(cell))
	if len(matches) != 3 {
		return "", fmt.Errorf("invalid cell reference: %s", cell)
	}
	colIndex := columnNameToIndex(matches[1])
	if colIndex < 1 {
		return "", fmt.Errorf("invalid column: %s", matches[1])
	}
	rowIndex, err := strconv.Atoi(matches[2])
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%d", columnIndexToName(colIndex+colOffset), rowIndex+rowOffset), nil
}

func columnNameToIndex(name string) int {
	name = strings.ToUpper(strings.TrimSpace(name))
	if name == "" {
		return 0
	}
	index := 0
	for _, r := range name {
		if r < 'A' || r > 'Z' {
			return 0
		}
		index = index*26 + int(r-'A'+1)
	}
	return index
}

func columnIndexToName(index int) string {
	if index < 1 {
		return ""
	}
	var out []byte
	for index > 0 {
		index--
		out = append([]byte{byte('A' + index%26)}, out...)
		index /= 26
	}
	return string(out)
}
