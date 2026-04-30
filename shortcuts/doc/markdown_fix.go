// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// fixExportedMarkdown applies post-processing to Lark-exported Markdown to
// improve round-trip fidelity on re-import:
//
//  1. fixBoldSpacing: removes trailing whitespace before closing ** / *,
//     and strips redundant ** from ATX headings. Applied only outside fenced
//     code blocks, and skips inline code spans.
//
//  2. normalizeNestedListIndentation: rewrites space-pair-indented nested list
//     markers to tab-indented markers. This avoids nested ordered list items
//     being flattened or interpreted as plain text/code on re-import.
//
//  3. fixSetextAmbiguity: inserts a blank line before any "---" that immediately
//     follows a non-empty line, preventing it from being parsed as a Setext H2.
//     Applied only outside fenced code blocks.
//
//  4. fixBlockquoteHardBreaks: inserts a blank blockquote line (">") between
//     consecutive blockquote content lines so create-doc preserves line breaks.
//     Applied only outside fenced code blocks.
//
//  5. fixTopLevelSoftbreaks: inserts a blank line between adjacent non-empty
//     lines at the top level and inside content containers (callout,
//     quote-container, lark-td). Code fences are left untouched, and
//     consecutive list items / continuations are not separated.
//
//  6. fixCalloutEmoji: replaces named emoji aliases (e.g. emoji="warning") with
//     actual Unicode emoji characters that create-doc understands. Applied only
//     outside fenced code blocks.
func fixExportedMarkdown(md string) string {
	md = applyOutsideCodeFences(md, fixBoldSpacing)
	md = applyOutsideCodeFences(md, normalizeNestedListIndentation)
	md = applyOutsideCodeFences(md, fixSetextAmbiguity)
	md = applyOutsideCodeFences(md, fixBlockquoteHardBreaks)
	md = fixTopLevelSoftbreaks(md)
	md = applyOutsideCodeFences(md, fixCalloutEmoji)
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

// fixBoldSpacing normalizes emphasis markers exported by Lark while preserving
// inline code spans:
//
//  1. Removes leading whitespace after opening ** and * delimiters:
//     "** text**" → "**text**", "* text*" → "*text*"
//
//  2. Removes trailing whitespace before closing ** and * delimiters:
//     "**text **" → "**text**", "*text *" → "*text*"
//
//  3. Removes redundant bold around an entire ATX heading:
//     "# **text**" → "# text"
//
// The bold and italic spacing fixes only run on non-code segments so literal
// code content is left unchanged.
var (
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
// handles them separately, keeping heading-only normalization isolated from the
// inline emphasis spacing scanner below.
func fixBoldSpacingLine(line string) string {
	if atxHeadingRe.MatchString(line) {
		return line
	}
	spans := scanInlineCodeSpans(line)
	if len(spans) == 0 {
		return fixEmphasisSpacingSegment(line)
	}
	var sb strings.Builder
	pos := 0
	for _, loc := range spans {
		// Process the non-code segment before this inline code span.
		seg := line[pos:loc[0]]
		sb.WriteString(fixEmphasisSpacingSegment(seg))
		// Preserve inline code span as-is.
		sb.WriteString(line[loc[0]:loc[1]])
		pos = loc[1]
	}
	// Remaining non-code segment after the last code span.
	sb.WriteString(fixEmphasisSpacingSegment(line[pos:]))
	return sb.String()
}

// fixEmphasisSpacingSegment trims only the whitespace immediately inside simple
// *...* and **...** spans. It deliberately ignores runs of 3+ asterisks and
// any candidate whose payload contains another asterisk so nested emphasis-like
// text remains untouched. When both inner sides contain whitespace, single-rune
// payloads are preserved as literal text (for example "* x *" and "** x **").
func fixEmphasisSpacingSegment(seg string) string {
	if !strings.Contains(seg, "*") {
		return seg
	}

	var sb strings.Builder
	pos := 0
	for pos < len(seg) {
		openStart, openEnd, ok := nextAsteriskRun(seg, pos)
		if !ok {
			sb.WriteString(seg[pos:])
			break
		}

		sb.WriteString(seg[pos:openStart])

		markerLen := openEnd - openStart
		if markerLen != 1 && markerLen != 2 {
			sb.WriteString(seg[openStart:openEnd])
			pos = openEnd
			continue
		}

		closeStart, closeEnd, ok := nextAsteriskRun(seg, openEnd)
		if !ok || closeEnd-closeStart != markerLen {
			sb.WriteString(seg[openStart:openEnd])
			pos = openEnd
			continue
		}

		payload := seg[openEnd:closeStart]
		normalized, shouldNormalize := normalizeEmphasisPayload(payload)
		if !shouldNormalize {
			sb.WriteString(seg[openStart:closeEnd])
			pos = closeEnd
			continue
		}

		marker := seg[openStart:openEnd]
		sb.WriteString(marker)
		sb.WriteString(normalized)
		sb.WriteString(marker)
		pos = closeEnd
	}
	return sb.String()
}

func nextAsteriskRun(s string, start int) (runStart, runEnd int, ok bool) {
	for i := start; i < len(s); i++ {
		if s[i] != '*' {
			continue
		}
		j := i
		for j < len(s) && s[j] == '*' {
			j++
		}
		return i, j, true
	}
	return 0, 0, false
}

func normalizeEmphasisPayload(payload string) (string, bool) {
	trimmedLeft := strings.TrimLeftFunc(payload, unicode.IsSpace)
	trimmed := strings.TrimRightFunc(trimmedLeft, unicode.IsSpace)
	if trimmed == "" {
		return payload, false
	}

	hasLeadingSpace := len(trimmedLeft) != len(payload)
	hasTrailingSpace := len(trimmed) != len(trimmedLeft)
	if !hasLeadingSpace && !hasTrailingSpace {
		return payload, true
	}

	if hasLeadingSpace && hasTrailingSpace && utf8.RuneCountInString(trimmed) == 1 {
		return payload, false
	}
	return trimmed, true
}

var setextRe = regexp.MustCompile(`(?m)^([^\n]+)\n(-{3,}\s*$)`)

func fixSetextAmbiguity(md string) string {
	return setextRe.ReplaceAllString(md, "$1\n\n$2")
}

// calloutTypeColors maps the semantic type= shorthand to a recommended
// [background-color, border-color] pair for Feishu callout blocks.
// Used only for hint messages — the Markdown itself is never rewritten.
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

// calloutOpenTagRe matches a <callout …> opening tag.
var calloutOpenTagRe = regexp.MustCompile(`<callout(\s[^>]*)?>`)

// calloutTypeAttrRe extracts the value of a type= attribute (single or
// double quoted) from a callout opening tag's attribute string. The
// (?:^|\s) anchor instead of \b is intentional: \b sits at any
// word/non-word boundary, and `-` is a non-word character, so
// `\btype=` would also match the suffix of `data-type=` and yield a
// bogus type lookup. Anchoring on start-of-string-or-whitespace
// requires a real attribute separator before the name.
var calloutTypeAttrRe = regexp.MustCompile(`(?:^|\s)type=(?:"([^"]*)"|'([^']*)')`)

// calloutBackgroundColorAttrRe matches a background-color= attribute
// name with optional whitespace around the equals sign, so forms like
// `background-color="..."` and `background-color = "..."` are both
// accepted. Same (?:^|\s) anchor as calloutTypeAttrRe, for the same
// reason: `data-background-color="..."` must not look like a present
// background-color and silently suppress the hint.
var calloutBackgroundColorAttrRe = regexp.MustCompile(`(?:^|\s)background-color\s*=`)

// WarnCalloutType scans md for callout tags that carry a type= attribute but
// no background-color= attribute, then writes a hint line to w for each one
// suggesting the explicit Feishu color attributes to use instead.
//
// Callout tags inside fenced code blocks (``` or ~~~) are skipped — they
// are documentation samples, not real callouts the user wants Feishu to
// render. Fence detection uses the shared codeFenceOpenMarker /
// isCodeFenceClose helpers so both backtick and tilde fences are handled
// (matching CommonMark §4.5).
//
// The Markdown is not modified — the caller is responsible for acting on
// the hints or ignoring them. This keeps the create/update path
// transparent: user input reaches create-doc exactly as written.
func WarnCalloutType(md string, w io.Writer) {
	fenceMarker := ""
	for _, line := range strings.Split(md, "\n") {
		if fenceMarker != "" {
			// Inside a fenced block — skip everything until the matching
			// closer. Code samples that show literal <callout type=...>
			// must not produce a phantom hint.
			if isCodeFenceClose(line, fenceMarker) {
				fenceMarker = ""
			}
			continue
		}
		if marker := codeFenceOpenMarker(line); marker != "" {
			fenceMarker = marker
			continue
		}
		scanCalloutTagsForWarning(line, w)
	}
}

// scanCalloutTagsForWarning emits a hint to w for every <callout type="...">
// tag in s that lacks an explicit background-color= attribute. Pulled out
// of WarnCalloutType so the line walker only handles fence state and the
// per-tag scan is its own readable unit.
//
// The previous implementation routed the tag iteration through
// calloutOpenTagRe.ReplaceAllStringFunc with a callback that always
// returned the original tag and threw the rebuilt string away — using a
// rewrite primitive purely for its iteration side-effect, plus a second
// regex execution to recover the capture groups inside the callback.
// FindAllStringSubmatch hands us both the iteration and the groups in one
// pass, no allocation thrown away.
func scanCalloutTagsForWarning(s string, w io.Writer) {
	for _, m := range calloutOpenTagRe.FindAllStringSubmatch(s, -1) {
		attrs := m[1]
		// Skip tags that already carry an explicit background-color.
		if calloutBackgroundColorAttrRe.MatchString(attrs) {
			continue
		}
		parts := calloutTypeAttrRe.FindStringSubmatch(attrs)
		if len(parts) < 3 {
			continue // no type= attribute
		}
		// parts[1] is the double-quoted capture, parts[2] is single-quoted.
		typeName := parts[1]
		if typeName == "" {
			typeName = parts[2]
		}
		colors, ok := calloutTypeColors[typeName]
		if !ok {
			continue // unknown type — no hint to give
		}
		fmt.Fprintf(w,
			"hint: callout type=%q has no background-color; consider: background-color=%q border-color=%q\n",
			typeName, colors[0], colors[1])
	}
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

// nestedListIndentRe matches nested list item markers indented with pairs of
// spaces. We rewrite those space pairs to tabs because some downstream
// round-trip paths treat multi-space indented ordered items as flat items or
// literal text, while tab indentation remains nested and avoids 4-space code
// block ambiguity.
var nestedListIndentRe = regexp.MustCompile(`^( {2,})([-*+]|\d+[.)]) `)

func normalizeNestedListIndentation(md string) string {
	lines := strings.Split(md, "\n")
	for i, line := range lines {
		matches := nestedListIndentRe.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}
		if !hasPreviousNonBlankListItem(lines, i) {
			continue
		}
		indent := matches[1]
		if len(indent)%2 != 0 {
			continue
		}
		tabs := strings.Repeat("\t", len(indent)/2)
		lines[i] = tabs + line[len(indent):]
	}
	return strings.Join(lines, "\n")
}

func hasPreviousNonBlankListItem(lines []string, index int) bool {
	for i := index - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			return false
		}
		return listItemRe.MatchString(lines[i])
	}
	return false
}

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
