// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"regexp"
	"strings"
)

// docsUpdateWarnings returns a list of human-readable warnings for a
// `docs +update` invocation based on static analysis of the mode and
// Markdown payload. The warnings describe CLI/MCP contract edges that
// commonly surprise users; the update is still executed — callers
// decide whether to stop at a warning.
//
// Both checks ignore fenced code blocks, inline code spans, and
// backslash-escaped emphasis markers so that literal Markdown content
// embedded in code samples or escaped prose does not produce false
// positives.
//
// Warnings emitted (current):
//
//  1. replace_* modes do not split blocks. A Markdown payload containing
//     a blank line (\n\n) in prose implies the caller expects multiple
//     paragraphs, but replace_range / replace_all only swap in-block
//     text. The resulting block will contain the blank line as literal
//     text and appear as a single paragraph in the UI.
//
//  2. Lark does not round-trip bold+italic. Markdown like ***text*** or
//     **_text_** / _**text**_ is stored as only one of the two emphases
//     (usually italic), silently dropping the other. The user wanted
//     both; they will get one.
func docsUpdateWarnings(mode, markdown string) []string {
	var warnings []string
	if w := checkDocsUpdateReplaceMultilineMarkdown(mode, markdown); w != "" {
		warnings = append(warnings, w)
	}
	if w := checkDocsUpdateBoldItalic(markdown); w != "" {
		warnings = append(warnings, w)
	}
	return warnings
}

// checkDocsUpdateReplaceMultilineMarkdown flags markdown that contains a
// blank-line paragraph break outside fenced code blocks under a replace_*
// mode. Blank lines inside code fences are literal content and don't
// imply paragraph semantics, so they are deliberately ignored.
func checkDocsUpdateReplaceMultilineMarkdown(mode, markdown string) string {
	if mode != "replace_range" && mode != "replace_all" {
		return ""
	}
	// A CR/LF-robust check: both "\n\n" and "\r\n\r\n" count as paragraph
	// separators. We normalize line endings once before detection.
	normalized := strings.ReplaceAll(markdown, "\r\n", "\n")
	if !proseHasBlankLine(normalized) {
		return ""
	}
	return "--mode=" + mode + " does not split a block into multiple paragraphs; " +
		"the blank line in --markdown will render as literal text. " +
		"For multiple paragraphs, use --mode=delete_range followed by --mode=insert_before."
}

// reBoldItalicTriple matches ***text*** with non-whitespace text between.
var reBoldItalicTriple = regexp.MustCompile(`\*\*\*\S[^*]*?\S\*\*\*|\*\*\*\S\*\*\*`)

// reBoldItalicUnderscoreInside matches **_text_** — bold wrapping an
// underscore italic. Same downgrade issue in Lark.
var reBoldItalicUnderscoreInside = regexp.MustCompile(`\*\*_\S[^_*]*?\S_\*\*|\*\*_\S_\*\*`)

// reBoldItalicUnderscoreOutside matches _**text**_ — underscore italic
// wrapping a bold.
var reBoldItalicUnderscoreOutside = regexp.MustCompile(`_\*\*\S[^_*]*?\S\*\*_|_\*\*\S\*\*_`)

// checkDocsUpdateBoldItalic flags Markdown emphases that attempt to
// combine bold and italic in a way Lark cannot represent. Fenced code
// blocks, inline code spans, and backslash-escaped emphasis markers are
// stripped first so that literal markdown examples ("here is a
// `***keyword***` to flag") do not trigger the warning.
func checkDocsUpdateBoldItalic(markdown string) string {
	if markdown == "" {
		return ""
	}
	sanitized := stripEscapedEmphasisMarkers(stripMarkdownCodeRegions(markdown))
	if reBoldItalicTriple.MatchString(sanitized) ||
		reBoldItalicUnderscoreInside.MatchString(sanitized) ||
		reBoldItalicUnderscoreOutside.MatchString(sanitized) {
		return "Lark does not support combined bold+italic markers (***text***, **_text_**, _**text**_); " +
			"the emphasis will be downgraded to either bold or italic. " +
			"Split into two separate emphases or drop one of them."
	}
	return ""
}

// proseHasBlankLine reports whether markdown contains a blank line outside
// of fenced code blocks. Blank lines inside ```...``` or ~~~...~~~ fences
// are code content, not paragraph separators, and must not trip the
// "replace_* cannot split paragraphs" warning.
//
// A blank line counts only when it sits between two non-blank boundaries
// (other prose, or a fence open/close). A trailing empty line at EOF is
// not treated as "\n\n".
func proseHasBlankLine(markdown string) bool {
	lines := strings.Split(markdown, "\n")
	inFence := false
	var fenceMarker string
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if inFence {
			if isCodeFenceClose(trimmed, fenceMarker) {
				inFence = false
				fenceMarker = ""
			}
			continue
		}
		if marker := codeFenceOpenMarker(trimmed); marker != "" {
			inFence = true
			fenceMarker = marker
			continue
		}
		if trimmed == "" && i > 0 && i+1 < len(lines) {
			return true
		}
	}
	return false
}

// stripMarkdownCodeRegions returns markdown with fenced code blocks blanked
// out and inline code spans replaced by whitespace of equivalent length.
// Byte offsets outside the masked regions are preserved, so follow-on
// regex matches still point at real prose positions.
func stripMarkdownCodeRegions(markdown string) string {
	lines := strings.Split(markdown, "\n")
	inFence := false
	var fenceMarker string
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if inFence {
			if isCodeFenceClose(trimmed, fenceMarker) {
				inFence = false
				fenceMarker = ""
			}
			lines[i] = ""
			continue
		}
		if marker := codeFenceOpenMarker(trimmed); marker != "" {
			inFence = true
			fenceMarker = marker
			lines[i] = ""
			continue
		}
		lines[i] = maskInlineCodeSpans(line)
	}
	return strings.Join(lines, "\n")
}

// maskInlineCodeSpans replaces the byte ranges of any inline code spans in
// line with space characters of equal length. Uses scanInlineCodeSpans from
// markdown_fix.go, which implements the CommonMark §6.1 matching-backtick-run
// rule (so “ `a`b` “ is a single span).
func maskInlineCodeSpans(line string) string {
	spans := scanInlineCodeSpans(line)
	if len(spans) == 0 {
		return line
	}
	var sb strings.Builder
	pos := 0
	for _, loc := range spans {
		sb.WriteString(line[pos:loc[0]])
		sb.WriteString(strings.Repeat(" ", loc[1]-loc[0]))
		pos = loc[1]
	}
	sb.WriteString(line[pos:])
	return sb.String()
}

// stripEscapedEmphasisMarkers removes backslash-escaped '*' and '_' so the
// bold/italic regexes don't treat literal sequences like `\***text***` as
// real combined emphasis. CommonMark renders "\*" as a literal "*" with no
// emphasis semantics; dropping the escape + its target from the detection
// input keeps the heuristic aligned with what the renderer actually does.
func stripEscapedEmphasisMarkers(s string) string {
	s = strings.ReplaceAll(s, `\*`, "")
	s = strings.ReplaceAll(s, `\_`, "")
	return s
}

// codeFenceOpenMarker returns the exact fence marker (e.g. "```" or "~~~~")
// if trimmed opens a fenced code block, otherwise "". Supports any fence of
// length ≥ 3 per CommonMark §4.5.
func codeFenceOpenMarker(trimmed string) string {
	switch {
	case strings.HasPrefix(trimmed, "```"):
		return leadingRun(trimmed, '`')
	case strings.HasPrefix(trimmed, "~~~"):
		return leadingRun(trimmed, '~')
	}
	return ""
}

// isCodeFenceClose reports whether trimmed closes a fence opened with
// marker. Per CommonMark, the closer must use the same fence character,
// be at least as long as the opener, and contain no info-string text.
func isCodeFenceClose(trimmed, marker string) bool {
	if marker == "" || !strings.HasPrefix(trimmed, marker) {
		return false
	}
	return strings.TrimSpace(trimmed[len(marker):]) == ""
}

// leadingRun returns the longest prefix of s made up of the byte c.
func leadingRun(s string, c byte) string {
	i := 0
	for i < len(s) && s[i] == c {
		i++
	}
	return s[:i]
}
