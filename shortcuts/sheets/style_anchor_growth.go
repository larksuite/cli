// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

// ─── a style above the data is a title row, not an error ──────────────
//
// `--styles for sheet "报价单"[0].range "A1:H1" starts outside the write range
// (its top-left must be at or after A3)` is 30,565 calls in the 09-11..15
// catalog, 29,284 of them on +workbook-create. The shape behind every one is
// the same: the payload anchors its data at A3 and styles A1:H1, because rows
// 1-2 are the title and subtitle the caller is about to put there.
//
// The rejection was a consequence of how styles are applied, not a rule about
// spreadsheets: cell_styles merge INTO the write matrix, the matrix begins at
// the data anchor, and padMatrixForStyles only ever grew it right and down.
// A range above or left of the anchor had nowhere to land.
//
// It can grow the other way too. Prepending rows moves the matrix's origin up
// while the data keeps its absolute position, because the prepended cells are
// `{}` — the same empty-cell form the ragged-row prescription already calls
// "keep those cells unchanged". So the write covers more cells and changes no
// cell it did not already change; the only difference is that the title row
// now carries the style the caller asked for.
//
// Growth is bounded (styleGrowthCellCap). A style at A1 against data at
// A50000 would otherwise materialize 50,000 blank rows to carry one border.

// styleGrowthCellCap bounds the cells the origin shift may materialize. Beyond
// it the range keeps the original rejection, whose message names the anchor
// the caller has to move — the honest answer when the gap is so large that the
// style and the data are not describing one table.
const styleGrowthCellCap = 200_000

// growMatrixOriginForStyles lowers the write anchor to cover cell_styles
// ranges that start above or left of it, prepending `{}` cells so every
// existing cell keeps its absolute position.
//
// Returns the grown matrix and the new anchor. When nothing starts before the
// anchor, or the growth would exceed the cap, it returns its inputs unchanged
// and the caller's existing bounds check reports the range as before.
func growMatrixOriginForStyles(rows [][]interface{}, styles *workbookCreateStylePayload, baseCol, baseRow int) ([][]interface{}, int, int) {
	if styles == nil || len(styles.CellStyles) == 0 {
		return rows, baseCol, baseRow
	}
	minCol, minRow := baseCol, baseRow
	for _, op := range styles.CellStyles {
		startCol, startRow, _, _, err := workbookCreateStyleRangeBounds(op.Range)
		if err != nil {
			continue // the parse error is reported by the merge loop, with context
		}
		if startCol < minCol {
			minCol = startCol
		}
		if startRow < minRow {
			minRow = startRow
		}
	}
	if minCol < 0 {
		minCol = 0
	}
	if minRow < 0 {
		minRow = 0
	}
	addCols, addRows := baseCol-minCol, baseRow-minRow
	if addCols <= 0 && addRows <= 0 {
		return rows, baseCol, baseRow
	}

	width := 0
	if len(rows) > 0 {
		width = len(rows[0])
	}
	// The cap has to be measured against the rectangle that EXISTS when the
	// padding is done, not just the part this function prepends:
	// padMatrixForStyles then grows right and down to every style's end,
	// unbounded. A single data cell at B2 with a style range A1:ZZZ1000 let
	// a few prepended cells through here and materialized ~18.3M cell maps
	// afterwards. int64 throughout — the product of two int extents is
	// exactly where an int32-shaped overflow would hide.
	endCol, endRow := baseCol+width-1, baseRow+len(rows)-1
	for _, op := range styles.CellStyles {
		_, _, opEndCol, opEndRow, err := workbookCreateStyleRangeBounds(op.Range)
		if err != nil {
			continue
		}
		if opEndCol > endCol {
			endCol = opEndCol
		}
		if opEndRow > endRow {
			endRow = opEndRow
		}
	}
	if !styleGrowthFits(int64(minCol), int64(minRow), int64(endCol), int64(endRow)) {
		return rows, baseCol, baseRow
	}

	grown := make([][]interface{}, 0, len(rows)+addRows)
	for i := 0; i < addRows; i++ {
		grown = append(grown, blankCellRow(width+addCols))
	}
	for _, row := range rows {
		if addCols > 0 {
			row = append(blankCellRow(addCols), row...)
		}
		grown = append(grown, row)
	}
	return grown, minCol, minRow
}

// blankCellRow builds a row of empty cell maps. `{}` is the wire's
// "leave this cell alone" — it carries no value, so it cannot overwrite
// content that is already there, and a style merged onto it afterwards
// reaches the cell without touching what it holds.
func blankCellRow(n int) []interface{} {
	row := make([]interface{}, n)
	for i := range row {
		row[i] = map[string]interface{}{}
	}
	return row
}

// styleGrowthFits measures the final bounding rectangle against the cap. A
// degenerate rectangle (no columns, or no rows) is refused rather than scored
// as zero: a style-only payload with no data would otherwise pass any cap at
// all. Division rather than cols*rows keeps the safety check itself from
// overflowing on a syntactically valid extreme A1 coordinate.
func styleGrowthFits(minCol, minRow, endCol, endRow int64) bool {
	cols, rows := endCol-minCol+1, endRow-minRow+1
	return rectangleFitsCellLimit(cols, rows, int64(styleGrowthCellCap))
}

func rectangleFitsCellLimit(cols, rows, limit int64) bool {
	if cols <= 0 || rows <= 0 || limit < 0 {
		return false
	}
	return cols <= limit/rows
}

// styleBoundingRectForSize measures a concrete matrix with dataCols × dataRows
// cells at the supplied anchor after applying every cell_style range.
func styleBoundingRectForSize(dataCols, dataRows int, sp *workbookCreateStylePayload, baseCol, baseRow int) (cols, rows int64) {
	minCol, minRow := int64(baseCol), int64(baseRow)
	endCol, endRow := minCol-1, minRow-1
	hasCells := dataCols > 0 && dataRows > 0
	if hasCells {
		endCol = minCol + int64(dataCols) - 1
		endRow = minRow + int64(dataRows) - 1
	}
	hasStyles := false
	if sp != nil {
		for _, op := range sp.CellStyles {
			startCol, startRow, opEndCol, opEndRow, err := workbookCreateStyleRangeBounds(op.Range)
			if err != nil {
				continue
			}
			minCol = minInt64(minCol, int64(startCol))
			minRow = minInt64(minRow, int64(startRow))
			endCol = maxInt64(endCol, int64(opEndCol))
			endRow = maxInt64(endRow, int64(opEndRow))
			hasStyles = true
		}
	}
	if !hasCells && !hasStyles {
		return 0, 0
	}
	if !hasCells {
		// growMatrixOriginForStyles prepends base-min blank rows. A leftward gap
		// is materialized only when there are such rows; with no rows to prepend,
		// moving the empty origin left costs nothing and padding creates only the
		// style width. Down/right padding always includes each style endpoint.
		gapRows := int64(baseRow) - minRow
		styleCols := endCol - minCol + 1
		styleRows := endRow - minRow + 1
		gapCols := int64(0)
		if gapRows > 0 {
			gapCols = int64(baseCol) - minCol
		}
		return maxInt64(gapCols, styleCols), maxInt64(gapRows, styleRows)
	}
	return endCol - minCol + 1, endRow - minRow + 1
}

// styleBoundingRect is the rectangle a sheet's write actually covers once its
// cell_styles are applied: the data matrix, grown up and left to every style's
// start and right and down to every style's end. Returned in int64 so the two
// extents can be multiplied without an overflow hiding the result.
//
// The single source for both the per-sheet growth cap and the payload-wide
// cell budget. They measured different things before, and a payload sized to
// pass one sailed past the other.
func styleBoundingRect(s *tableSheetSpec, sp *workbookCreateStylePayload, baseCol, baseRow int) (cols, rows int64) {
	// headerRowCount, not headerOn: append to an EMPTY target sheet writes a
	// header even though headerOn reports false for append. It is the
	// function that already carries the execute path's semantics, so the
	// estimate matches the matrix instead of approximating it — an
	// unconditional +1 refused a legitimate header:false payload sitting
	// exactly on the cap.
	return styleBoundingRectForSize(len(s.Columns), len(s.Rows)+headerRowCount(s), sp, baseCol, baseRow)
}

// styleAnchorGrowthFits reports whether growing the origin to cover this
// sheet's cell_styles stays inside styleGrowthCellCap. It runs at Validate
// time, before any matrix exists, against the same rectangle the write phase
// materializes.
func styleAnchorGrowthFits(s *tableSheetSpec, sp *workbookCreateStylePayload, baseCol, baseRow int) bool {
	if sp == nil || len(sp.CellStyles) == 0 {
		return true
	}
	cols, rows := styleBoundingRect(s, sp, baseCol, baseRow)
	return rectangleFitsCellLimit(cols, rows, int64(styleGrowthCellCap))
}

// appendStyleMinimumRect returns a lower bound on the rectangle an existing-
// sheet append can occupy before its live row is known. The data column is
// static, and separate style ranges have a fixed span; only the gap between the
// data rows and those styles is dynamic. Rejecting this lower bound is safe,
// while using start_cell's ignored row can reject a tiny live placement.
func appendStyleMinimumRect(s *tableSheetSpec, sp *workbookCreateStylePayload, baseCol int) (cols, rows int64) {
	dataRows := int64(len(s.Rows))
	if s.Header != nil && *s.Header {
		dataRows++
	}
	if sp == nil || len(sp.CellStyles) == 0 {
		return int64(len(s.Columns)), dataRows
	}
	minCol, endCol := int64(0), int64(-1)
	hasCols := dataRows > 0 && len(s.Columns) > 0
	if hasCols {
		minCol = int64(baseCol)
		endCol = minCol + int64(len(s.Columns)) - 1
	}
	var minStyleRow, endStyleRow int64
	hasStyleRow := false
	for _, op := range sp.CellStyles {
		startCol, startRow, opEndCol, opEndRow, err := workbookCreateStyleRangeBounds(op.Range)
		if err != nil {
			continue
		}
		if !hasCols {
			minCol, endCol = int64(startCol), int64(opEndCol)
			hasCols = true
		} else {
			minCol = minInt64(minCol, int64(startCol))
			endCol = maxInt64(endCol, int64(opEndCol))
		}
		if !hasStyleRow {
			minStyleRow, endStyleRow = int64(startRow), int64(opEndRow)
			hasStyleRow = true
		} else {
			minStyleRow = minInt64(minStyleRow, int64(startRow))
			endStyleRow = maxInt64(endStyleRow, int64(opEndRow))
		}
	}
	styleRows := int64(0)
	if hasStyleRow {
		styleRows = endStyleRow - minStyleRow + 1
	}
	return endCol - minCol + 1, maxInt64(styleRows, maxInt64(dataRows, 1))
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
