// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/larksuite/cli/internal/output"
)

const SafeParagraphLimit = 10000

var ErrUnsafeMarkdown = errors.New("oversized markdown contains complex structural elements (fenced code blocks, tables, blockquotes, or HTML) that cannot be safely split. Please manually split the content below 10,000 characters before uploading")

func isTableAlignmentRow(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" || !strings.Contains(line, "-") {
		return false
	}
	for _, r := range line {
		if r != '|' && r != '-' && r != ':' && r != ' ' && r != '\t' {
			return false
		}
	}
	return true
}

func containsUnsafeMarkdown(md string) bool {
	lines := strings.Split(md, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			return true
		}

		if strings.HasPrefix(trimmed, ">") {
			return true
		}

		temp := trimmed
		for {
			idx := strings.IndexByte(temp, '<')
			if idx == -1 || idx+1 >= len(temp) {
				break
			}
			next := temp[idx+1]
			if (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') || next == '/' || next == '!' {
				return true
			}
			temp = temp[idx+1:]
		}

		if isTableAlignmentRow(trimmed) {
			return true
		}
		if strings.Contains(trimmed, "|") && i+1 < len(lines) {
			if isTableAlignmentRow(strings.TrimSpace(lines[i+1])) {
				return true
			}
		}
	}
	return false
}

func splitPlainParagraphs(md string) []string {
	var paragraphs []string
	var current strings.Builder
	hasContent := false

	for _, line := range strings.Split(md, "\n") {
		if strings.TrimSpace(line) == "" {
			if hasContent {
				paragraphs = append(paragraphs, current.String())
				current.Reset()
				hasContent = false
			}
			continue
		}
		if hasContent {
			current.WriteByte('\n')
		}
		current.WriteString(line)
		hasContent = true
	}
	if hasContent {
		paragraphs = append(paragraphs, current.String())
	}

	return paragraphs
}

func splitOversizedParagraph(para string) []string {
	if utf8.RuneCountInString(para) <= SafeParagraphLimit {
		return []string{para}
	}

	var chunks []string
	pos := 0

	for pos < len(para) {
		remaining := para[pos:]
		if utf8.RuneCountInString(remaining) <= SafeParagraphLimit {
			chunks = append(chunks, remaining)
			break
		}

		limitByte := runeOffset(remaining, SafeParagraphLimit)
		splitAt := findSplitPoint(remaining, limitByte)

		chunks = append(chunks, remaining[:splitAt])
		pos += splitAt
	}

	return chunks
}

func runeOffset(s string, n int) int {
	offset := 0
	for i := 0; i < n && offset < len(s); i++ {
		_, size := utf8.DecodeRuneInString(s[offset:])
		offset += size
	}
	return offset
}

func findSplitPoint(s string, limitByte int) int {
	minByte := limitByte * 3 / 4

	for i := limitByte - 1; i >= minByte; i-- {
		if s[i] == '\n' {
			return i + 1
		}
	}

	for i := limitByte - 1; i >= minByte; i-- {
		if s[i] == ' ' {
			return i + 1
		}
	}

	return limitByte
}

func chunkMarkdownForUpload(md string) (string, error) {
	if md == "" {
		return "", nil
	}

	if utf8.RuneCountInString(md) <= SafeParagraphLimit {
		return md, nil
	}

	if containsUnsafeMarkdown(md) {
		return "", ErrUnsafeMarkdown
	}

	paragraphs := splitPlainParagraphs(md)
	var chunks []string

	for _, para := range paragraphs {
		chunks = append(chunks, splitOversizedParagraph(para)...)
	}

	return strings.Join(chunks, "\n\n"), nil
}

func applyChunkingToBody(body map[string]interface{}, contentKey, docFormat string) error {
	if docFormat != "markdown" {
		return nil
	}
	content, _ := body[contentKey].(string)
	if content == "" {
		return nil
	}
	chunked, err := chunkMarkdownForUpload(content)
	if err != nil {
		return output.Errorf(output.ExitAPI, "chunk_error", "%v", err)
	}
	body[contentKey] = chunked
	return nil
}
