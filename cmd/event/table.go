// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"fmt"
	"io"
)

// tableWidths returns the max cell width for each column, considering
// both the header labels and every row. Rows may be shorter or longer
// than headers; extra columns are ignored, missing ones keep the header
// width. The returned slice has len(headers) entries.
func tableWidths(headers []string, rows [][]string) []int {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				break
			}
			if l := len(cell); l > widths[i] {
				widths[i] = l
			}
		}
	}
	return widths
}

// printTableRow renders one row of cells padded to widths and separated
// by gap. The final cell is not padded, so no trailing whitespace leaks
// into the output.
func printTableRow(out io.Writer, widths []int, cells []string, gap string) {
	for i, cell := range cells {
		if i == len(cells)-1 {
			fmt.Fprintln(out, cell)
			return
		}
		fmt.Fprintf(out, "%-*s%s", widths[i], cell, gap)
	}
}
