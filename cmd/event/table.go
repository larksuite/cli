// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"fmt"
	"io"
)

// tableWidths returns the max cell width per column across headers + rows.
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

// printTableRow renders one padded row; final cell is unpadded to avoid trailing whitespace.
func printTableRow(out io.Writer, widths []int, cells []string, gap string) {
	for i, cell := range cells {
		if i == len(cells)-1 {
			fmt.Fprintln(out, cell)
			return
		}
		fmt.Fprintf(out, "%-*s%s", widths[i], cell, gap)
	}
}
