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
//     lines at the top level and inside content containers (callout,
//     quote-container, lark-td). Code fences are left untouched.
//
//  5. fixCalloutEmoji: replaces named emoji aliases (e.g. emoji="warning") with
//     actual Unicode emoji characters that create-doc understands.
func fixExportedMarkdown(md string) string {
	md = fixBoldSpacing(md)
	md = fixSetextAmbiguity(md)
	md = fixBlockquoteHardBreaks(md)
	md = fixTopLevelSoftbreaks(md)
	md = fixCalloutEmoji(md)
	md = fixCodeBlockTrailingBlanks(md)
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

// calloutEmojiAliases maps named emoji strings that fetch-doc emits to actual
// Unicode emoji characters that create-doc accepts.
var calloutEmojiAliases = map[string]string{
	"warning":      "⚠️",
	"note":         "📝",
	"tip":          "💡",
	"info":         "ℹ️",
	"check":        "✅",
	"success":      "✅",
	"error":        "❌",
	"danger":       "🚨",
	"important":    "❗",
	"caution":      "⚠️",
	"question":     "❓",
	"forbidden":    "🚫",
	"fire":         "🔥",
	"star":         "⭐",
	"pin":          "📌",
	"clock":        "🕐",
	"gift":         "🎁",
	"eyes":         "👀",
	"bulb":         "💡",
	"memo":         "📝",
	"link":         "🔗",
	"key":          "🔑",
	"lock":         "🔒",
	"thumbsup":     "👍",
	"thumbsdown":   "👎",
	"rocket":       "🚀",
	"construction": "🚧",
}

// calloutEmojiRe matches emoji="<name>" in callout opening tags.
var calloutEmojiRe = regexp.MustCompile(`(<callout[^>]*\bemoji=")([^"]+)(")`)

// fixCalloutEmoji replaces named emoji aliases in callout tags with actual
// Unicode emoji characters. fetch-doc sometimes emits emoji="warning" instead
// of emoji="⚠️"; create-doc only accepts Unicode emoji.
func fixCalloutEmoji(md string) string {
	return calloutEmojiRe.ReplaceAllStringFunc(md, func(match string) string {
		parts := calloutEmojiRe.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		name := parts[2]
		if emoji, ok := calloutEmojiAliases[name]; ok {
			return parts[1] + emoji + parts[3]
		}
		return match
	})
}

// fixCodeBlockTrailingBlanks removes blank lines that appear immediately before
// a closing ``` fence inside a fenced code block. fetch-doc / create-doc
// sometimes inserts an extra blank line at the end of code block content:
//
//	```
//	last line
//	           ← spurious blank added by server
//	```
//
// Removing it keeps the round-trip diff clean.
// fixCodeBlockTrailingBlanks removes blank lines that appear immediately before
// a closing ``` fence inside a fenced code block. fetch-doc / create-doc
// sometimes inserts an extra blank line at the end of code block content:
//
//	```
//	last line
//	           ← spurious blank added by server
//	```
//
// Removing it keeps the round-trip diff clean.
// Nested fences (e.g. ```go inside ```markdown) are handled by tracking
// nesting depth so only the outermost closing fence is affected.
func fixCodeBlockTrailingBlanks(md string) string {
	lines := strings.Split(md, "\n")
	out := make([]string, 0, len(lines))
	inCodeBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inCodeBlock {
			// Any ``` line (with or without language id) opens a code block.
			if strings.HasPrefix(trimmed, "```") {
				inCodeBlock = true
				out = append(out, line)
				continue
			}
		} else {
			// Only a plain ``` (no language id) closes a code block.
			// Lines like ```go or ```plaintext inside a code block are content.
			if trimmed == "```" {
				// Drop trailing blank before the closing fence.
				if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
					out = out[:len(out)-1]
				}
				inCodeBlock = false
				out = append(out, line)
				continue
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// isTableStructuralTag returns true for lark-table tags that are structural
// (table/tr/td open/close) and should not themselves trigger blank-line insertion.
func isTableStructuralTag(s string) bool {
	return strings.HasPrefix(s, "<lark-t") ||
		strings.HasPrefix(s, "</lark-t")
}

// contentContainers lists block tags whose interior should have blank lines
// inserted between adjacent content lines (same treatment as lark-td).
var contentContainers = [][2]string{
	{"<lark-td>", "</lark-td>"},
	{"<callout", "</callout>"},
	{"<quote-container>", "</quote-container>"},
}

// fixTopLevelSoftbreaks ensures that adjacent non-empty content lines are
// separated by a blank line in the following contexts:
//  1. Top level (depth == 0): every Lark block becomes its own Markdown paragraph.
//  2. Inside content containers (<lark-td>, <callout>, <quote-container>):
//     multi-line content is preserved as separate paragraphs.
//
// Structural table tags (<lark-table>, <lark-tr>, <lark-td> and their closing
// counterparts) never trigger blank-line insertion themselves. Fenced code
// blocks (``` ... ```) are left completely untouched.
func fixTopLevelSoftbreaks(md string) string {
	lines := strings.Split(md, "\n")
	out := make([]string, 0, len(lines)*2)

	inCodeBlock := false
	// containerDepth > 0 means we are inside a content container.
	containerDepth := 0
	// tableDepth tracks <lark-table> nesting (outer structure, not content).
	tableDepth := 0

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// --- Track fenced code blocks — skip all processing inside. ---
		// Any ``` line opens a block; only plain ``` (no language id) closes it.
		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				if trimmed == "```" {
					inCodeBlock = false
				}
			} else {
				inCodeBlock = true
			}
			out = append(out, line)
			continue
		}

		if !inCodeBlock {
			// --- Track content containers. ---
			for _, cc := range contentContainers {
				if strings.HasPrefix(trimmed, cc[0]) {
					containerDepth++
				}
				if strings.Contains(trimmed, cc[1]) {
					containerDepth--
					if containerDepth < 0 {
						containerDepth = 0
					}
				}
			}

			// --- Track table structure (outer, non-content). ---
			if strings.HasPrefix(trimmed, "<lark-table") {
				tableDepth++
			}
			if strings.Contains(trimmed, "</lark-table>") {
				tableDepth--
				if tableDepth < 0 {
					tableDepth = 0
				}
			}
		}

		// --- Decide whether to insert a blank line before this line. ---
		if !inCodeBlock && trimmed != "" && i > 0 {
			// Skip structural table tags — they are not content lines.
			isStructural := isTableStructuralTag(trimmed)

			// Don't split consecutive blockquote lines ("> ...") — they form
			// one continuous blockquote in the original document.
			isBlockquote := strings.HasPrefix(trimmed, "> ") || trimmed == ">"

			// Container opening/closing tags are structural — skip them.
			isContainerTag := false
			for _, cc := range contentContainers {
				if strings.HasPrefix(trimmed, cc[0]) || strings.HasPrefix(trimmed, "</"+cc[0][1:]) {
					isContainerTag = true
					break
				}
			}

			// Insert blank line when:
			//   - at top level (tableDepth == 0, containerDepth == 0), OR
			//   - inside a content container (containerDepth > 0, not in outer table)
			// AND this line is actual content (not structural/blockquote/container-tag).
			inContent := tableDepth == 0 || containerDepth > 0
			if !isStructural && !isBlockquote && !isContainerTag && inContent {
				prev := ""
				if len(out) > 0 {
					prev = strings.TrimSpace(out[len(out)-1])
				}
				if prev != "" && !isTableStructuralTag(prev) {
					out = append(out, "")
				}
			}
		}

		out = append(out, line)
	}

	return strings.Join(out, "\n")
}
