// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentread

import (
	"fmt"
	"regexp"
	"strings"
)

const truncateHintFmt = "> 还有 %d 行"

// TruncateHintFor returns an entity-specific table truncation notice.
func TruncateHintFor(fetchType string) string {
	switch fetchType {
	case "sheet":
		return "> 还有 %d 行(用 sheets +cells-get 取全量)"
	case "bitable":
		return "> 还有 %d 行(用 base +record-list 取全量)"
	default:
		return truncateHintFmt
	}
}

var gfmDelimiterRe = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)

// TruncateGFMTables limits each GFM table independently and skips code fences.
// Non-positive maxRows disables truncation; an empty hintFmt uses the default.
func TruncateGFMTables(md string, maxRows int, hintFmt string) string {
	if maxRows <= 0 {
		return md
	}
	if strings.TrimSpace(hintFmt) == "" {
		hintFmt = truncateHintFmt
	}
	lines := strings.Split(md, "\n")
	out := make([]string, 0, len(lines))
	inFence := false
	for i := 0; i < len(lines); {
		line := lines[i]
		if isFenceLine(line) {
			inFence = !inFence
			out = append(out, line)
			i++
			continue
		}
		if !inFence && i+1 < len(lines) && strings.Contains(line, "|") && gfmDelimiterRe.MatchString(lines[i+1]) {
			out = append(out, lines[i], lines[i+1])
			j := i + 2
			kept, dropped := 0, 0
			for j < len(lines) && isTableRow(lines[j]) {
				if kept < maxRows {
					out = append(out, lines[j])
					kept++
				} else {
					dropped++
				}
				j++
			}
			if dropped > 0 {
				out = append(out, "", fmt.Sprintf(hintFmt, dropped))
			}
			i = j
			continue
		}
		out = append(out, line)
		i++
	}
	return strings.Join(out, "\n")
}

func isFenceLine(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")
}

func isTableRow(line string) bool {
	if isFenceLine(line) {
		return false
	}
	return strings.TrimSpace(line) != "" && strings.Contains(line, "|")
}
