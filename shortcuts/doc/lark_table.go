// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"regexp"
	"strings"
)

var (
	larkTableRE  = regexp.MustCompile(`(?is)<lark-table\b[^>]*>(.*?)</lark-table>`)
	larkTrRE     = regexp.MustCompile(`(?is)<lark-tr\b[^>]*>(.*?)</lark-tr>`)
	larkTdRE     = regexp.MustCompile(`(?is)<lark-td\b[^>]*>(.*?)</lark-td>`)
	gridRE       = regexp.MustCompile(`(?is)<grid\b[^>]*>(.*?)</grid>`)
	columnRE     = regexp.MustCompile(`(?is)<column\b[^>]*>(.*?)</column>`)
	textTagRE    = regexp.MustCompile(`(?is)<text\b[^>]*>(.*?)</text>`)
	larkTagRE    = regexp.MustCompile(`(?i)</?lark-[^>]+>`)
	brTagRE      = regexp.MustCompile(`(?i)<br\s*/?>`)
	headerAttrRE = regexp.MustCompile(`(?i)header-row="false"`)
)

// NormalizeLarkTables converts Lark-formatted table XML-like structures (<lark-table>)
// into standard GitHub-flavored Markdown (GFM) pipe tables.
// If any cell contains a fenced code block (```), it returns the original markup
// to avoid breaking multi-line code rendering in pipe tables.
func NormalizeLarkTables(md string) string {
	return larkTableRE.ReplaceAllStringFunc(md, func(block string) string {
		m := larkTableRE.FindStringSubmatch(block)
		if len(m) < 2 {
			return block
		}
		inner := m[1]
		headerRow := !headerAttrRE.MatchString(block)

		rowMatches := larkTrRE.FindAllStringSubmatch(inner, -1)
		if len(rowMatches) == 0 {
			return block
		}

		var rows [][]string
		maxCols := 0
		for _, rm := range rowMatches {
			cellMatches := larkTdRE.FindAllStringSubmatch(rm[1], -1)
			var cells []string
			for _, cm := range cellMatches {
				rawCell := cm[1]
				// CRITICAL: GFM pipe tables cannot contain multi-line fenced code blocks.
				// Check raw content before any tag stripping/cleaning to avoid brittleness.
				if strings.Contains(rawCell, "```") {
					return block
				}
				cells = append(cells, cleanCell(rawCell))
			}
			if len(cells) > maxCols {
				maxCols = len(cells)
			}
			rows = append(rows, cells)
		}

		if maxCols == 0 || len(rows) == 0 {
			return block
		}

		// Row padding: Ensure all rows have the same number of columns
		for i := range rows {
			for len(rows[i]) < maxCols {
				rows[i] = append(rows[i], "")
			}
		}

		var lines []string
		startBody := 0

		if headerRow && len(rows) > 0 {
			lines = append(lines, "| "+joinCells(rows[0])+" |")
			seps := make([]string, maxCols)
			for i := range seps {
				seps[i] = "---"
			}
			lines = append(lines, "| "+strings.Join(seps, " | ")+" |")
			startBody = 1
		}

		for i := startBody; i < len(rows); i++ {
			lines = append(lines, "| "+joinCells(rows[i])+" |")
		}

		return strings.Join(lines, "\n")
	})
}

// joinCells joins a slice of strings with the GFM pipe separator, escaping existing pipes.
func joinCells(cells []string) string {
	escaped := make([]string, len(cells))
	for i, c := range cells {
		c = strings.ReplaceAll(c, "\n", "<br>")
		c = strings.ReplaceAll(c, "|", "\\|")
		escaped[i] = c
	}
	return strings.Join(escaped, " | ")
}

// cleanCell strips Lark-specific tags and flattens nested layout structures.
func cleanCell(s string) string {
	s = strings.TrimSpace(s)
	s = flattenGrid(s)
	s = stripTextTags(s)
	s = larkTagRE.ReplaceAllString(s, "")
	s = brTagRE.ReplaceAllString(s, "\n")
	return strings.TrimSpace(s)
}

// flattenGrid handles <grid><column>...</column></grid> layouts by joining columns with newlines.
func flattenGrid(s string) string {
	for {
		n := gridRE.ReplaceAllStringFunc(s, func(g string) string {
			m := gridRE.FindStringSubmatch(g)
			if len(m) < 2 {
				return g
			}
			cols := columnRE.FindAllStringSubmatch(m[1], -1)
			if len(cols) == 0 {
				return strings.TrimSpace(m[1])
			}
			var parts []string
			for _, c := range cols {
				parts = append(parts, cleanCell(c[1]))
			}
			return strings.Join(parts, "\n")
		})
		if n == s {
			break
		}
		s = n
	}
	return s
}

// stripTextTags removes <text color="..."> wrappers but preserves their content.
func stripTextTags(s string) string {
	for {
		n := textTagRE.ReplaceAllString(s, "$1")
		if n == s {
			break
		}
		s = n
	}
	return s
}
