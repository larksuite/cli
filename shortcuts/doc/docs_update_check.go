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
// Both checks ignore fenced code blocks (```…``` and ~~~…~~~, with up
// to 3 leading spaces per CommonMark §4.5), inline code spans, and
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
//  2. Lark does not round-trip bold+italic. Six shapes are detected:
//     ***text***   ___text___
//     **_text_**   __*text*__
//     _**text**_   *__text__*
//     Lark stores only one of the two emphases (usually italic), silently
//     dropping the other. The user wanted both; they will get one.
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

// combinedEmphasisPatterns holds the six documented combined-emphasis shapes
// that Lark downgrades to a single emphasis. Each entry pairs a regex with a
// short shape label for the warning message. The two forms per shape (with
// and without `[^…]*?`) are there because the lazy quantifier needs at least
// one non-delimiter character to match; single-rune payloads (e.g. `***X***`)
// take the second alternation.
var combinedEmphasisPatterns = []struct {
	shape string
	re    *regexp.Regexp
}{
	// Bold+italic with a single delimiter char.
	{"***text***", regexp.MustCompile(`\*\*\*\S[^*]*?\S\*\*\*|\*\*\*\S\*\*\*`)},
	{"___text___", regexp.MustCompile(`___\S[^_]*?\S___|___\S___`)},

	// Bold wrapping italic (asterisk outside).
	{"**_text_**", regexp.MustCompile(`\*\*_\S[^_*]*?\S_\*\*|\*\*_\S_\*\*`)},
	{"__*text*__", regexp.MustCompile(`__\*\S[^_*]*?\S\*__|__\*\S\*__`)},

	// Italic wrapping bold (asterisk inside).
	{"_**text**_", regexp.MustCompile(`_\*\*\S[^_*]*?\S\*\*_|_\*\*\S\*\*_`)},
	{"*__text__*", regexp.MustCompile(`\*__\S[^_*]*?\S__\*|\*__\S__\*`)},
}

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
	for _, p := range combinedEmphasisPatterns {
		if p.re.MatchString(sanitized) {
			return "Lark does not support combined bold+italic markers " +
				"(e.g. ***text***, ___text___, **_text_**, _**text**_, __*text*__, *__text__*); " +
				"the emphasis will be downgraded to either bold or italic. " +
				"Split into two separate emphases or drop one of them."
		}
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
		if inFence {
			if isCodeFenceClose(line, fenceMarker) {
				inFence = false
				fenceMarker = ""
			}
			continue
		}
		if marker := codeFenceOpenMarker(line); marker != "" {
			inFence = true
			fenceMarker = marker
			continue
		}
		if strings.TrimSpace(line) == "" && i > 0 && i+1 < len(lines) {
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
		if inFence {
			if isCodeFenceClose(line, fenceMarker) {
				inFence = false
				fenceMarker = ""
			}
			lines[i] = ""
			continue
		}
		if marker := codeFenceOpenMarker(line); marker != "" {
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
//
// Known limitation: a doubled backslash escape ("\\" followed by a real
// emphasis marker, e.g. `\\***text***`) renders as a literal backslash
// followed by genuine combined emphasis, but this strip is not a proper
// parser and will instead consume the second backslash as the opener for
// another escape. That hides the real emphasis from the check, producing
// a false negative. Practical impact is small (this shape is rare in the
// kind of AI-Agent prompts we target) and the alternative — a full
// CommonMark escape parser — is not worth the code surface here.
func stripEscapedEmphasisMarkers(s string) string {
	s = strings.ReplaceAll(s, `\*`, "")
	s = strings.ReplaceAll(s, `\_`, "")
	return s
}

// codeFenceOpenMarker returns the fence marker (e.g. "```" or "~~~~") if
// line opens a fenced code block, otherwise "". Applies CommonMark §4.5
// rules: up to 3 leading spaces are tolerated; 4+ leading spaces (or any
// leading tab, which expands to 4 columns) make the line an indented code
// block rather than a fence.
func codeFenceOpenMarker(line string) string {
	body, ok := fenceIndentOK(line)
	if !ok {
		return ""
	}
	switch {
	case strings.HasPrefix(body, "```"):
		return leadingRun(body, '`')
	case strings.HasPrefix(body, "~~~"):
		return leadingRun(body, '~')
	}
	return ""
}

// isCodeFenceClose reports whether line closes a fence opened with marker.
// Per CommonMark §4.5 the closer must use the same fence character, be at
// least as long as the opener, sit within 0..3 leading spaces, and carry
// no info-string text.
func isCodeFenceClose(line, marker string) bool {
	if marker == "" {
		return false
	}
	body, ok := fenceIndentOK(line)
	if !ok {
		return false
	}
	fenceChar := marker[0]
	run := leadingRun(body, fenceChar)
	if len(run) < len(marker) {
		return false
	}
	return strings.TrimSpace(body[len(run):]) == ""
}

// fenceIndentOK returns (bodyWithoutLeadingSpaces, true) when line has
// 0..3 leading spaces and no leading tab — i.e. the indentation is
// permissible for a CommonMark fence. Returns ("", false) otherwise
// (4+ leading spaces or any tab), meaning the line must be treated as
// indented code block content rather than a fence boundary.
func fenceIndentOK(line string) (string, bool) {
	for i := 0; i < len(line) && i < 4; i++ {
		switch line[i] {
		case ' ':
			continue
		case '\t':
			return "", false
		default:
			return line[i:], true
		}
	}
	// Reached index 4 without hitting a non-space character: too indented.
	if len(line) >= 4 {
		return "", false
	}
	// Line shorter than 4 chars and all spaces — still valid (empty content).
	return "", true
}

// leadingRun returns the longest prefix of s made up of the byte c.
func leadingRun(s string, c byte) string {
	i := 0
	for i < len(s) && s[i] == c {
		i++
	}
	return s[:i]
}
