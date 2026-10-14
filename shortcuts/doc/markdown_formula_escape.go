// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import "strings"

func protectMarkdownFormulaEscapes(content string) string {
	if content == "" || !strings.Contains(content, `\|`) || !strings.Contains(content, "$") {
		return content
	}

	var (
		out       strings.Builder
		segment   strings.Builder
		fenceChar byte
		fenceLen  int
	)
	flush := func() {
		out.WriteString(protectMarkdownFormulaSegment(segment.String()))
		segment.Reset()
	}

	for _, line := range strings.SplitAfter(content, "\n") {
		char, run, isFence := markdownFence(line)
		if fenceChar == 0 && isFence {
			flush()
			fenceChar, fenceLen = char, run
			out.WriteString(line)
			continue
		}
		if fenceChar != 0 {
			out.WriteString(line)
			if isFence && char == fenceChar && run >= fenceLen && markdownFenceCloses(line, char, run) {
				fenceChar, fenceLen = 0, 0
			}
			continue
		}
		segment.WriteString(line)
	}
	flush()
	return out.String()
}

func protectMarkdownFormulaSegment(segment string) string {
	var out strings.Builder
	for i := 0; i < len(segment); {
		if segment[i] == '`' {
			run := countByteRun(segment, i, '`')
			end := findMatchingBacktickRun(segment, i+run, run)
			if end < 0 {
				out.WriteString(segment[i:])
				break
			}
			out.WriteString(segment[i:end])
			i = end
			continue
		}
		if segment[i] != '$' || isEscapedMarkdownByte(segment, i) {
			out.WriteByte(segment[i])
			i++
			continue
		}

		delimiterLen := countByteRun(segment, i, '$')
		if delimiterLen != 1 && delimiterLen != 2 {
			out.WriteString(segment[i : i+delimiterLen])
			i += delimiterLen
			continue
		}
		close := findMarkdownFormulaClose(segment, i+delimiterLen, delimiterLen)
		if close < 0 {
			out.WriteString(segment[i:])
			break
		}
		out.WriteString(segment[i : i+delimiterLen])
		out.WriteString(protectLatexPipeEscapes(segment[i+delimiterLen : close]))
		out.WriteString(segment[close : close+delimiterLen])
		i = close + delimiterLen
	}
	return out.String()
}

func findMarkdownFormulaClose(content string, start int, delimiterLen int) int {
	for i := start; i < len(content); {
		if delimiterLen == 1 && (content[i] == '\n' || content[i] == '\r') {
			return -1
		}
		if content[i] != '$' {
			i++
			continue
		}
		run := countByteRun(content, i, '$')
		if run == delimiterLen && !isEscapedMarkdownByte(content, i) {
			return i
		}
		i += run
	}
	return -1
}

func protectLatexPipeEscapes(formula string) string {
	var out strings.Builder
	for i := 0; i < len(formula); {
		if formula[i] != '\\' {
			out.WriteByte(formula[i])
			i++
			continue
		}
		run := countByteRun(formula, i, '\\')
		if run == 1 && i+run < len(formula) && formula[i+run] == '|' {
			out.WriteString(`\\`)
		} else {
			out.WriteString(formula[i : i+run])
		}
		i += run
	}
	return out.String()
}
