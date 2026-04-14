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
//     and strips redundant ** from ATX headings. Applied only outside fenced
//     code blocks, and skips inline code spans.
//
//  2. fixSetextAmbiguity: inserts a blank line before any "---" that immediately
//     follows a non-empty line, preventing it from being parsed as a Setext H2.
//     Applied only outside fenced code blocks.
//
//  3. fixBlockquoteHardBreaks: inserts a blank blockquote line (">") between
//     consecutive blockquote content lines so create-doc preserves line breaks.
//     Applied only outside fenced code blocks.
//
//  4. fixTopLevelSoftbreaks: inserts a blank line between adjacent non-empty
//     lines at the top level and inside content containers (callout,
//     quote-container, lark-td). Code fences are left untouched, and
//     consecutive list items / continuations are not separated.
//
//  5. fixCalloutEmoji: replaces named emoji aliases (e.g. emoji="warning") with
//     actual Unicode emoji characters that create-doc understands. Applied only
//     outside fenced code blocks.
//
// prepareMarkdownForCreate applies fixes that should run on any Markdown before
// it is sent to the create-doc / update-doc MCP tools, regardless of whether
// the content originated from a Lark export or was written by hand.
func prepareMarkdownForCreate(md string) string {
	return applyOutsideCodeFences(md, fixCalloutType)
}

func fixExportedMarkdown(md string) string {
	md = applyOutsideCodeFences(md, fixBoldSpacing)
	md = applyOutsideCodeFences(md, fixSetextAmbiguity)
	md = applyOutsideCodeFences(md, fixBlockquoteHardBreaks)
	md = fixTopLevelSoftbreaks(md)
	md = applyOutsideCodeFences(md, fixCalloutEmoji)
	md = applyOutsideCodeFences(md, fixCalloutType)
	// Collapse runs of 3+ consecutive newlines into exactly 2 (one blank line),
	// but only outside fenced code blocks to preserve intentional blank lines in code.
	md = applyOutsideCodeFences(md, func(s string) string {
		for strings.Contains(s, "\n\n\n") {
			s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
		}
		return s
	})
	md = strings.TrimRight(md, "\n") + "\n"
	return md
}

// applyOutsideCodeFences applies fn only to content outside fenced code blocks.
// Lines inside fenced code blocks (``` ... ```) are passed through unchanged,
// preventing transforms from corrupting literal code content.
func applyOutsideCodeFences(md string, fn func(string) string) string {
	lines := strings.Split(md, "\n")
	var out []string
	var chunk []string
	inCode := false

	flush := func() {
		if len(chunk) == 0 {
			return
		}
		out = append(out, strings.Split(fn(strings.Join(chunk, "\n")), "\n")...)
		chunk = chunk[:0]
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if !inCode {
				flush()
				inCode = true
			} else if trimmed == "```" {
				inCode = false
			}
			out = append(out, line)
			continue
		}
		if inCode {
			out = append(out, line)
		} else {
			chunk = append(chunk, line)
		}
	}
	flush()
	return strings.Join(out, "\n")
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
//
// Both fixes skip inline code spans to avoid modifying literal code content.
var (
	boldTrailingSpaceRe   = regexp.MustCompile(`(\*\*\S[^*]*?)\s+(\*\*)`)
	italicTrailingSpaceRe = regexp.MustCompile(`(\*\S[^*]*?)\s+(\*)`)
	// headingBoldRe uses [^*]+ (no asterisks) to avoid mismatching headings
	// that contain multiple disjoint bold spans such as "# **foo** and **bar**".
	headingBoldRe = regexp.MustCompile(`(?m)^(#{1,6})\s+\*\*([^*]+)\*\*\s*$`)
)

func fixBoldSpacing(md string) string {
	lines := strings.Split(md, "\n")
	for i, line := range lines {
		lines[i] = fixBoldSpacingLine(line)
	}
	md = strings.Join(lines, "\n")
	md = headingBoldRe.ReplaceAllString(md, "$1 $2")
	return md
}

// atxHeadingRe matches ATX heading lines (# ... through ###### ...).
var atxHeadingRe = regexp.MustCompile(`^#{1,6}\s`)

// scanInlineCodeSpans returns the byte ranges [start, end) of all inline code
// spans in line. It handles multi-backtick delimiters (e.g. “ `foo` “) by
// finding the opening run of N backticks and searching for the next identical
// run to close the span, per CommonMark spec §6.1.
func scanInlineCodeSpans(line string) [][2]int {
	var spans [][2]int
	i := 0
	for i < len(line) {
		if line[i] != '`' {
			i++
			continue
		}
		// Count the opening backtick run.
		start := i
		for i < len(line) && line[i] == '`' {
			i++
		}
		delim := line[start:i] // e.g. "`" or "``" or "```"
		// Search for the closing run of the same length.
		j := i
		for j <= len(line)-len(delim) {
			if line[j] == '`' {
				k := j
				for k < len(line) && line[k] == '`' {
					k++
				}
				if k-j == len(delim) {
					spans = append(spans, [2]int{start, k})
					i = k
					break
				}
				j = k // skip this backtick run and keep searching
			} else {
				j++
			}
		}
		// No closing delimiter found — not a code span, continue.
	}
	return spans
}

// fixBoldSpacingLine applies bold/italic trailing-space fixes to a single line,
// skipping content inside inline code spans to avoid corrupting literal code.
// ATX heading lines are also skipped here because headingBoldRe in fixBoldSpacing
// handles them separately and boldTrailingSpaceRe can misfire on headings with
// multiple disjoint bold spans (e.g. "# **foo** and **bar**").
func fixBoldSpacingLine(line string) string {
	if atxHeadingRe.MatchString(line) {
		return line
	}
	spans := scanInlineCodeSpans(line)
	if len(spans) == 0 {
		line = boldTrailingSpaceRe.ReplaceAllString(line, "$1$2")
		line = italicTrailingSpaceRe.ReplaceAllString(line, "$1$2")
		return line
	}
	var sb strings.Builder
	pos := 0
	for _, loc := range spans {
		// Process the non-code segment before this inline code span.
		seg := line[pos:loc[0]]
		seg = boldTrailingSpaceRe.ReplaceAllString(seg, "$1$2")
		seg = italicTrailingSpaceRe.ReplaceAllString(seg, "$1$2")
		sb.WriteString(seg)
		// Preserve inline code span as-is.
		sb.WriteString(line[loc[0]:loc[1]])
		pos = loc[1]
	}
	// Remaining non-code segment after the last code span.
	seg := line[pos:]
	seg = boldTrailingSpaceRe.ReplaceAllString(seg, "$1$2")
	seg = italicTrailingSpaceRe.ReplaceAllString(seg, "$1$2")
	sb.WriteString(seg)
	return sb.String()
}

var setextRe = regexp.MustCompile(`(?m)^([^\n]+)\n(-{3,}\s*$)`)

func fixSetextAmbiguity(md string) string {
	return setextRe.ReplaceAllString(md, "$1\n\n$2")
}

// calloutTypeColors maps callout type="<name>" to a [background-color, border-color] pair.
// When a callout tag has type= but no background-color=, these defaults are applied.
var calloutTypeColors = map[string][2]string{
	"warning":   {"light-yellow", "yellow"},
	"caution":   {"light-orange", "orange"},
	"note":      {"light-blue", "blue"},
	"info":      {"light-blue", "blue"},
	"tip":       {"light-green", "green"},
	"success":   {"light-green", "green"},
	"check":     {"light-green", "green"},
	"error":     {"light-red", "red"},
	"danger":    {"light-red", "red"},
	"important": {"light-purple", "purple"},
}

// calloutTypeRe matches a <callout …> opening tag so individual attributes can
// be inspected and patched.
var calloutTypeRe = regexp.MustCompile(`<callout(\s[^>]*)?>`)

// fixCalloutType expands the semantic type="<name>" shorthand on callout tags
// into explicit background-color= (and border-color= when absent).  When a
// background-color is already present the tag is left unchanged so that an
// explicit color always wins.
func fixCalloutType(md string) string {
	return calloutTypeRe.ReplaceAllStringFunc(md, func(tag string) string {
		attrs := ""
		if m := calloutTypeRe.FindStringSubmatch(tag); len(m) == 2 {
			attrs = m[1]
		}
		// Extract type value.
		typeRe := regexp.MustCompile(`\btype="([^"]*)"`)
		typeParts := typeRe.FindStringSubmatch(attrs)
		if len(typeParts) != 2 {
			return tag // no type= attribute
		}
		typeName := typeParts[1]
		colors, ok := calloutTypeColors[typeName]
		if !ok {
			return tag // unknown type — leave as-is
		}
		// Only inject background-color when it is absent.
		if strings.Contains(attrs, "background-color=") {
			return tag
		}
		// Inject background-color (and border-color when absent) before the
		// closing >.
		extra := ` background-color="` + colors[0] + `"`
		if !strings.Contains(attrs, "border-color=") {
			extra += ` border-color="` + colors[1] + `"`
		}
		return tag[:len(tag)-1] + extra + ">"
	})
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

// listItemRe matches unordered and ordered list item markers, including
// indented (nested) items.
var listItemRe = regexp.MustCompile(`^[ \t]*([-*+]|\d+[.)]) `)

// isListItemOrContinuation returns true for lines that are part of a list:
// either a list item marker line or an indented continuation of a list item.
// This is used to prevent blank lines being inserted between tight list lines,
// which would turn a tight list into a loose list and change rendering.
func isListItemOrContinuation(line string) bool {
	if listItemRe.MatchString(line) {
		return true
	}
	// Continuation lines are indented by at least 2 spaces or 1 tab.
	return strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t")
}

// fixTopLevelSoftbreaks ensures that adjacent non-empty content lines are
// separated by a blank line in the following contexts:
//  1. Top level (depth == 0): every Lark block becomes its own Markdown paragraph.
//  2. Inside content containers (<lark-td>, <callout>, <quote-container>):
//     multi-line content is preserved as separate paragraphs.
//
// Structural table tags (<lark-table>, <lark-tr>, <lark-td> and their closing
// counterparts) never trigger blank-line insertion themselves. Fenced code
// blocks (``` ... ```) are left completely untouched. Consecutive list items
// and list continuations are not separated (to preserve tight lists).
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

			// Only closing container tags suppress blank-line insertion.
			// Opening container tags may still receive a blank line before them
			// (e.g. two consecutive <callout> blocks need a blank between them).
			isContainerTag := false
			for _, cc := range contentContainers {
				closingTag := "</" + cc[0][1:]
				if strings.HasPrefix(trimmed, closingTag) {
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
				// Don't split consecutive list items / continuations — inserting a
				// blank line between them turns a tight list into a loose list.
				isListRelated := isListItemOrContinuation(line)
				prevIsListRelated := len(out) > 0 && isListItemOrContinuation(out[len(out)-1])
				if !(isListRelated && prevIsListRelated) {
					prev := ""
					if len(out) > 0 {
						prev = strings.TrimSpace(out[len(out)-1])
					}
					if prev != "" && !isTableStructuralTag(prev) {
						out = append(out, "")
					}
				}
			}
		}

		out = append(out, line)
	}

	return strings.Join(out, "\n")
}
