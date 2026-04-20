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
// Warnings emitted (current):
//
//  1. replace_* modes do not split blocks. A Markdown payload containing
//     a blank line (\n\n) implies the caller expects multiple paragraphs,
//     but replace_range / replace_all only swap in-block text. The
//     resulting block will contain the blank line as literal text and
//     appear as a single paragraph in the UI.
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
// blank-line paragraph break under a replace_* mode. Returns an empty
// string when the combination is fine.
func checkDocsUpdateReplaceMultilineMarkdown(mode, markdown string) string {
	if mode != "replace_range" && mode != "replace_all" {
		return ""
	}
	// A CR/LF-robust check: both "\n\n" and "\r\n\r\n" count as paragraph
	// separators. We normalize line endings once before the substring match.
	normalized := strings.ReplaceAll(markdown, "\r\n", "\n")
	if !strings.Contains(normalized, "\n\n") {
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
// combine bold and italic in a way Lark cannot represent.
func checkDocsUpdateBoldItalic(markdown string) string {
	if markdown == "" {
		return ""
	}
	if reBoldItalicTriple.MatchString(markdown) ||
		reBoldItalicUnderscoreInside.MatchString(markdown) ||
		reBoldItalicUnderscoreOutside.MatchString(markdown) {
		return "Lark does not support combined bold+italic markers (***text***, **_text_**, _**text**_); " +
			"the emphasis will be downgraded to either bold or italic. " +
			"Split into two separate emphases or drop one of them."
	}
	return ""
}
