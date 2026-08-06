// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentread

import (
	"regexp"
	"strings"
)

// Table-cell images arrive as escaped <qa:image> markers. XML decoding turns
// them into plain text, so extract the marker and token with regular expressions.
var (
	qaCellImageRe    = regexp.MustCompile(`(?s)<qa:image>(.*?)</qa>`)
	cellImageTokenRe = regexp.MustCompile(`image_token="([^"]+)"`)
)

// renderCellImages rewrites each <qa:image>…</qa> marker in a cell to a
// markdown image reference (via the shared RenderOneImage) joined on ImageMetaMap.
// A marker without an image_token is dropped.
func (r *anchoredMarkdownRenderer) renderCellImages(s string) string {
	if !strings.Contains(s, "<qa:image>") {
		return s
	}
	return qaCellImageRe.ReplaceAllStringFunc(s, func(marker string) string {
		body := qaCellImageRe.FindStringSubmatch(marker)[1]
		tok := cellImageTokenRe.FindStringSubmatch(body)
		if len(tok) < 2 {
			return ""
		}
		return RenderOneImage(tok[1], r.metas[tok[1]])
	})
}

func gfmCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.TrimSpace(s)
}

// rowsToGFM uses the first row as the header and pads rows to equal width.
func rowsToGFM(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	cols := 0
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	if cols == 0 {
		return ""
	}
	var b strings.Builder
	writeRow := func(cells []string) {
		b.WriteString("|")
		for c := 0; c < cols; c++ {
			v := ""
			if c < len(cells) {
				v = cells[c]
			}
			b.WriteString(" " + v + " |")
		}
		b.WriteString("\n")
	}
	writeRow(rows[0])
	b.WriteString("|")
	for c := 0; c < cols; c++ {
		b.WriteString(" --- |")
	}
	b.WriteString("\n")
	for _, row := range rows[1:] {
		writeRow(row)
	}
	return b.String()
}
