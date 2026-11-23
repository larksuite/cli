// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

import (
	"regexp"
	"strings"
)

var (
	compatibleBlockIDSelfClosing = regexp.MustCompile(`^<block_id="([^"]+)"\s*/>`)
	compatibleBlockIDWithClosing = regexp.MustCompile(`^<block_id="([^"]+)"\s*>\s*</block_id>`)
	compatibleBlockIDOpen        = regexp.MustCompile(`^<block_id="([^"]+)"\s*>`)
)

// normalizeCompatibleXMLInput applies deterministic legacy-shape rewrites
// before the tolerant parser builds its in-memory tree. The repaired XML is
// intentionally not exposed or written back by docs +script parse.
func normalizeCompatibleXMLInput(source string) string {
	var out strings.Builder
	out.Grow(len(source))
	for offset := 0; offset < len(source); {
		relative := strings.IndexByte(source[offset:], '<')
		if relative < 0 {
			out.WriteString(source[offset:])
			break
		}
		start := offset + relative
		out.WriteString(source[offset:start])

		_, end, state := scanXMLToken(source, start)
		if state == tokenComment || state == tokenCDATA || state == tokenProcessingInstruction {
			out.WriteString(source[start:end])
			offset = end
			continue
		}

		if replacement, consumed, ok := rewriteCompatibleBlockID(source[start:]); ok {
			out.WriteString(replacement)
			offset = start + consumed
			continue
		}
		if end > start+1 && (state == tokenOK || state == tokenInvalid) {
			out.WriteString(source[start:end])
			offset = end
			continue
		}

		out.WriteByte(source[start])
		offset = start + 1
	}
	return out.String()
}

func rewriteCompatibleBlockID(source string) (string, int, bool) {
	for _, expression := range []*regexp.Regexp{
		compatibleBlockIDSelfClosing,
		compatibleBlockIDWithClosing,
		compatibleBlockIDOpen,
	} {
		match := expression.FindStringSubmatchIndex(source)
		if len(match) < 4 || match[0] != 0 {
			continue
		}
		return `<block_id>` + source[match[2]:match[3]] + `</block_id>`, match[1], true
	}
	return "", 0, false
}
