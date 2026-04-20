// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import "strings"

type markdownFence struct {
	char byte
	size int
}

func parseMarkdownFence(line string) (markdownFence, bool) {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 3 {
		return markdownFence{}, false
	}
	char := trimmed[0]
	if char != '`' && char != '~' {
		return markdownFence{}, false
	}
	size := 0
	for size < len(trimmed) && trimmed[size] == char {
		size++
	}
	if size < 3 {
		return markdownFence{}, false
	}
	return markdownFence{char: char, size: size}, true
}

func isMarkdownFenceClose(line string, fence markdownFence) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < fence.size {
		return false
	}
	size := 0
	for size < len(trimmed) && trimmed[size] == fence.char {
		size++
	}
	return size >= fence.size && strings.TrimSpace(trimmed[size:]) == ""
}

func isMarkdownBlankLine(line string) bool {
	return strings.TrimSpace(strings.TrimRight(line, "\r\n")) == ""
}

// TrimMarkdownCodeBlockTrailingBlanks removes blank lines immediately before
// fenced code block closing markers. Lark's document Markdown round-trip can
// append one such blank line per fetch/update cycle, causing code blocks to grow.
func TrimMarkdownCodeBlockTrailingBlanks(markdown string) string {
	if markdown == "" {
		return markdown
	}

	lines := strings.SplitAfter(markdown, "\n")
	out := make([]string, 0, len(lines))
	pendingBlanks := make([]string, 0, 2)
	var fence markdownFence
	inCodeBlock := false

	for _, line := range lines {
		if !inCodeBlock {
			out = append(out, line)
			if parsed, ok := parseMarkdownFence(line); ok {
				fence = parsed
				inCodeBlock = true
			}
			continue
		}

		if isMarkdownFenceClose(line, fence) {
			pendingBlanks = pendingBlanks[:0]
			out = append(out, line)
			inCodeBlock = false
			continue
		}

		if isMarkdownBlankLine(line) {
			pendingBlanks = append(pendingBlanks, line)
			continue
		}

		if len(pendingBlanks) > 0 {
			out = append(out, pendingBlanks...)
			pendingBlanks = pendingBlanks[:0]
		}
		out = append(out, line)
	}

	if len(pendingBlanks) > 0 {
		out = append(out, pendingBlanks...)
	}
	return strings.Join(out, "")
}
