// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"regexp"
	"strings"
)

// fixExportedMarkdown applies post-processing to Lark-exported Markdown to
// improve round-trip fidelity on re-import:
//
//  1. fixBoldSpacing: removes trailing whitespace before closing ** / *,
//     and strips redundant ** from ATX headings.
//
//  2. fixSetextAmbiguity: inserts a blank line before any "---" that immediately
//     follows a non-empty line, preventing it from being parsed as a Setext H2.
//
//  3. fixBlockquoteHardBreaks: inserts a blank blockquote line (">") between
//     consecutive blockquote content lines so create-doc preserves line breaks.
//
//  4. fixTopLevelSoftbreaks: inserts a blank line between adjacent non-empty
//     lines at the top level (outside tables, callouts, code blocks, etc.).
//     Lark exports each block element on its own line with only \n between them;
//     standard Markdown parsers collapse those into a single paragraph on
//     re-import, losing the original block structure entirely.
func fixExportedMarkdown(md string) string {
	md = fixBoldSpacing(md)
	md = fixSetextAmbiguity(md)
	md = fixBlockquoteHardBreaks(md)
	md = fixTopLevelSoftbreaks(md)
	// Collapse runs of 3+ consecutive newlines into exactly 2 (one blank line).
	for strings.Contains(md, "\n\n\n") {
		md = strings.ReplaceAll(md, "\n\n\n", "\n\n")
	}
	md = strings.TrimRight(md, "\n") + "\n"
	return md
}

// fixBlockquoteHardBreaks inserts a blank blockquote line (">") between
// consecutive blockquote content lines. This forces each line into its own
// paragraph within the blockquote, so MCP create-doc preserves line breaks
// instead of collapsing them into a single paragraph.
//
// Before: "> line1\n> line2"  →  After: "> line1\n>\n> line2"
func fixBlockquoteHardBreaks(md string) string {
	lines := strings.Split(md, "\n")
	out := make([]string, 0, len(lines)*2)
	for i, line := range lines {
		out = append(out, line)
		if strings.HasPrefix(line, "> ") && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "> ") {
			out = append(out, ">")
		}
	}
	return strings.Join(out, "\n")
}

// fixBoldSpacing fixes two issues with bold markers exported by Lark:
//
//  1. Trailing whitespace before closing **: "**text **" → "**text**"
//     CommonMark requires no space before a closing delimiter; otherwise the
//     ** is rendered as literal text.
//
//  2. Redundant bold in ATX headings: "# **text**" → "# text"
//     Headings are already bold, so the inner ** is visually redundant and
//     some renderers display the markers literally.
var (
	boldTrailingSpaceRe   = regexp.MustCompile(`(\*\*\S[^*]*?)\s+(\*\*)`)
	italicTrailingSpaceRe = regexp.MustCompile(`(\*\S[^*]*?)\s+(\*)`)
	headingBoldRe         = regexp.MustCompile(`(?m)^(#{1,6})\s+\*\*(.+?)\*\*\s*$`)
)

func fixBoldSpacing(md string) string {
	// Process line-by-line to avoid cross-line mismatches where ** from
	// different bold spans on different lines confuse the regex engine.
	lines := strings.Split(md, "\n")
	for i, line := range lines {
		lines[i] = boldTrailingSpaceRe.ReplaceAllString(line, "$1$2")
		lines[i] = italicTrailingSpaceRe.ReplaceAllString(lines[i], "$1$2")
	}
	md = strings.Join(lines, "\n")
	md = headingBoldRe.ReplaceAllString(md, "$1 $2")
	return md
}

var setextRe = regexp.MustCompile(`(?m)^([^\n]+)\n(-{3,}\s*$)`)

func fixSetextAmbiguity(md string) string {
	return setextRe.ReplaceAllString(md, "$1\n\n$2")
}

// opaqueBlocks are block elements whose interior must never be modified.
var opaqueBlocks = [][2]string{
	{"<callout", "</callout>"},
	{"<quote-container>", "</quote-container>"},
	{"```", "```"},
}

// isTableStructuralTag returns true for lark-table tags that are structural
// (table/tr/td open/close) and should not themselves trigger blank-line insertion.
func isTableStructuralTag(s string) bool {
	return strings.HasPrefix(s, "<lark-t") ||
		strings.HasPrefix(s, "</lark-t")
}

// fixTopLevelSoftbreaks ensures that adjacent non-empty content lines are
// separated by a blank line in two contexts:
//  1. Top level (depth == 0): every Lark block becomes its own Markdown paragraph.
//  2. Inside <lark-td>: multi-line cell content is preserved as separate paragraphs.
//
// Structural table tags (<lark-table>, <lark-tr>, <lark-td> and their closing
// counterparts) never trigger blank-line insertion themselves. Opaque blocks
// (callout, quote-container, code fences) are left untouched.
func fixTopLevelSoftbreaks(md string) string {
	lines := strings.Split(md, "\n")
	out := make([]string, 0, len(lines)*2)

	// opaqueDepth tracks nesting inside opaque blocks (callout, quote, code).
	opaqueDepth := 0
	inCodeBlock := false
	// inTableCell is true when we are between <lark-td> and </lark-td>.
	inTableCell := false
	// tableDepth tracks <lark-table> nesting (for the outer structure).
	tableDepth := 0

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// --- Track fenced code blocks (``` toggles). ---
		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				inCodeBlock = false
				opaqueDepth--
			} else {
				inCodeBlock = true
				opaqueDepth++
			}
			out = append(out, line)
			continue
		}

		if !inCodeBlock {
			// --- Track opaque blocks (other than ```). ---
			for _, bd := range opaqueBlocks {
				if bd[0] == "```" {
					continue
				}
				if strings.HasPrefix(trimmed, bd[0]) {
					opaqueDepth++
				}
				if strings.Contains(trimmed, bd[1]) {
					opaqueDepth--
					if opaqueDepth < 0 {
						opaqueDepth = 0
					}
				}
			}

			// --- Track table structure. ---
			if strings.HasPrefix(trimmed, "<lark-table") {
				tableDepth++
			}
			if strings.Contains(trimmed, "</lark-table>") {
				tableDepth--
				if tableDepth < 0 {
					tableDepth = 0
				}
			}
			if strings.HasPrefix(trimmed, "<lark-td>") {
				inTableCell = true
			}
			if strings.Contains(trimmed, "</lark-td>") {
				inTableCell = false
			}
		}

		// --- Decide whether to insert a blank line before this line. ---
		// Skip if inside an opaque block.
		if opaqueDepth == 0 && trimmed != "" && i > 0 {
			// Skip structural table tags — they are not content lines.
			isStructural := isTableStructuralTag(trimmed)

			// Don't split consecutive blockquote lines ("> ...") — they form
			// one continuous blockquote in the original document.
			isBlockquote := strings.HasPrefix(trimmed, "> ") || trimmed == ">"

			// Insert blank line if: (a) top level, or (b) inside a table cell,
			// AND this line is a content line, AND the previous output is non-empty.
			if !isStructural && !isBlockquote && (tableDepth == 0 || inTableCell) {
				prev := ""
				if len(out) > 0 {
					prev = strings.TrimSpace(out[len(out)-1])
				}
				// Don't insert blank line after a structural tag either.
				if prev != "" && !isTableStructuralTag(prev) {
					out = append(out, "")
				}
			}
		}

		out = append(out, line)
	}

	return strings.Join(out, "\n")
}
