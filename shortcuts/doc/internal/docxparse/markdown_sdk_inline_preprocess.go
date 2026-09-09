// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

// This file mirrors docx_xml-go/protocol/gmf/emphasis_rehydrate.go for the
// source transformations that affect visible character statistics.

import (
	"strings"
	"unicode"
)

type sdkAdjacentMarkupRule struct {
	delimiter string
	openXML   string
	closeXML  string
}

var sdkAdjacentMarkupRules = []sdkAdjacentMarkupRule{
	{delimiter: "***", openXML: "<em><b>", closeXML: "</b></em>"},
	{delimiter: "~~", openXML: "<del>", closeXML: "</del>"},
	{delimiter: "**", openXML: "<b>", closeXML: "</b>"},
}

type sdkMarkdownFenceState struct {
	in     bool
	marker rune
	length int
}

func preprocessSDKAdjacentInlineMarkup(markdown string) string {
	if !hasSDKAdjacentMarkupCandidate(markdown) {
		return markdown
	}
	var out strings.Builder
	lines := strings.SplitAfter(markdown, "\n")
	var fence sdkMarkdownFenceState
	rawTag := ""
	for _, line := range lines {
		if updateSDKMarkdownFenceState(line, &fence) {
			out.WriteString(line)
			continue
		}
		if fence.in || isSDKMarkdownIndentedCodeLine(line) {
			out.WriteString(line)
			continue
		}
		out.WriteString(rewriteSDKAdjacentMarkupLineSkippingRawTags(line, &rawTag))
	}
	return out.String()
}

func hasSDKAdjacentMarkupCandidate(value string) bool {
	return strings.Contains(value, "**") || strings.Contains(value, "~~")
}

func updateSDKMarkdownFenceState(line string, state *sdkMarkdownFenceState) bool {
	fenceLine := stripSDKMarkdownContainerPrefixesForFence(line)
	trimmed := strings.TrimLeft(fenceLine, " \t")
	if countSDKLeadingIndent(fenceLine) >= 4 {
		return false
	}
	marker, length, rest, ok := parseSDKMarkdownFencePrefix(trimmed)
	if !ok {
		return false
	}
	if state.in {
		if marker == state.marker && length >= state.length && strings.TrimSpace(rest) == "" {
			state.in = false
			state.marker = 0
			state.length = 0
		}
		return true
	}
	state.in = true
	state.marker = marker
	state.length = length
	return true
}

func stripSDKMarkdownContainerPrefixesForFence(line string) string {
	rest := strings.TrimRight(line, "\r\n")
	for {
		next, changed := stripOneSDKMarkdownContainerPrefix(rest)
		if !changed {
			return rest
		}
		rest = next
	}
}

func stripOneSDKMarkdownContainerPrefix(line string) (string, bool) {
	rest := trimSDKUpToThreeSpaces(line)
	if strings.HasPrefix(rest, ">") {
		rest = rest[1:]
		if strings.HasPrefix(rest, " ") || strings.HasPrefix(rest, "\t") {
			rest = rest[1:]
		}
		return rest, true
	}
	if after, ok := stripSDKMarkdownListMarker(rest); ok {
		return after, true
	}
	return line, false
}

func trimSDKUpToThreeSpaces(line string) string {
	rest := line
	for i := 0; i < 3 && strings.HasPrefix(rest, " "); i++ {
		rest = rest[1:]
	}
	return rest
}

func stripSDKMarkdownListMarker(line string) (string, bool) {
	if line == "" {
		return line, false
	}
	runes := []rune(line)
	if len(runes) >= 2 && (runes[0] == '-' || runes[0] == '+' || runes[0] == '*') && isSDKMarkdownWhitespace(runes[1]) {
		return strings.TrimLeft(string(runes[2:]), " \t"), true
	}
	position := 0
	for position < len(runes) && runes[position] >= '0' && runes[position] <= '9' {
		position++
	}
	if position == 0 || position > 9 || position+1 >= len(runes) ||
		(runes[position] != '.' && runes[position] != ')') || !isSDKMarkdownWhitespace(runes[position+1]) {
		return line, false
	}
	return strings.TrimLeft(string(runes[position+2:]), " \t"), true
}

func parseSDKMarkdownFencePrefix(trimmed string) (rune, int, string, bool) {
	runes := []rune(trimmed)
	if len(runes) < 3 || runes[0] != '`' && runes[0] != '~' {
		return 0, 0, "", false
	}
	marker := runes[0]
	length := 0
	for length < len(runes) && runes[length] == marker {
		length++
	}
	if length < 3 {
		return 0, 0, "", false
	}
	return marker, length, string(runes[length:]), true
}

func countSDKLeadingIndent(line string) int {
	count := 0
	for _, character := range line {
		switch character {
		case ' ':
			count++
		case '\t':
			count += 4
		default:
			return count
		}
	}
	return count
}

func isSDKMarkdownIndentedCodeLine(line string) bool {
	return countSDKLeadingIndent(line) >= 4
}

func rewriteSDKAdjacentMarkupLineSkippingRawTags(line string, rawTag *string) string {
	if *rawTag == "" && !hasSDKAdjacentMarkupCandidate(line) {
		return line
	}
	var out strings.Builder
	rest := line
	for rest != "" {
		if *rawTag != "" {
			closeIndex := findSDKRawTagClose(rest, *rawTag)
			if closeIndex < 0 {
				out.WriteString(rest)
				return out.String()
			}
			closeEnd := closeIndex + len("</"+*rawTag+">")
			out.WriteString(rest[:closeEnd])
			rest = rest[closeEnd:]
			*rawTag = ""
			continue
		}

		openIndex, tag := findSDKRawTagOpen(rest)
		if openIndex < 0 {
			out.WriteString(rewriteSDKAdjacentMarkupLine(rest))
			break
		}
		if openIndex > 0 {
			out.WriteString(rewriteSDKAdjacentMarkupLine(rest[:openIndex]))
		}
		openRest := rest[openIndex:]
		openEnd := strings.IndexByte(openRest, '>')
		if openEnd < 0 {
			out.WriteString(openRest)
			break
		}
		openEnd++
		if isSDKSelfClosingXMLTagOpen(openRest[:openEnd]) {
			out.WriteString(openRest[:openEnd])
			rest = openRest[openEnd:]
			continue
		}
		closeIndex := findSDKRawTagClose(openRest[openEnd:], tag)
		if closeIndex < 0 {
			out.WriteString(openRest)
			*rawTag = tag
			break
		}
		rawEnd := openEnd + closeIndex + len("</"+tag+">")
		out.WriteString(openRest[:rawEnd])
		rest = openRest[rawEnd:]
	}
	return out.String()
}

func findSDKRawTagOpen(value string) (int, string) {
	for search := 0; search < len(value); search++ {
		index := strings.IndexByte(value[search:], '<')
		if index < 0 {
			break
		}
		index += search
		tag, ok := sdkRawTagNameAt(value, index)
		tag = strings.ToLower(tag)
		if ok && shouldSkipSDKAdjacentMarkupInRawTag(tag) {
			return index, tag
		}
		search = index + 1
	}
	return -1, ""
}

func sdkRawTagNameAt(value string, openIndex int) (string, bool) {
	if openIndex+1 >= len(value) {
		return "", false
	}
	next := value[openIndex+1]
	if next == '/' || next == '!' || next == '?' {
		return "", false
	}
	start := openIndex + 1
	end := start
	for end < len(value) && isSDKXMLTagNameByte(value[end]) {
		end++
	}
	if end == start || end < len(value) && !isSDKXMLTagNameBoundaryByte(value[end]) {
		return "", false
	}
	return value[start:end], true
}

func isSDKXMLTagNameByte(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' ||
		value >= '0' && value <= '9' || value == '-' || value == '_' || value == ':'
}

func isSDKXMLTagNameBoundaryByte(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r' || value == '>' || value == '/'
}

func shouldSkipSDKAdjacentMarkupInRawTag(tag string) bool {
	switch tag {
	case "pre", "code", "whiteboard":
		return true
	}
	return isSDKAllowedTag(tag) && !sdkMarkdownNativeInlineTags[tag] && !sdkContainerBlockParserTags[tag]
}

func findSDKRawTagClose(value, tag string) int {
	return indexASCIIFold(value, "</"+tag+">")
}

func isSDKSelfClosingXMLTagOpen(openTag string) bool {
	return strings.HasSuffix(strings.TrimSpace(openTag), "/>")
}

func rewriteSDKAdjacentMarkupLine(line string) string {
	if !hasSDKAdjacentMarkupCandidate(line) {
		return line
	}
	runes := []rune(line)
	var out strings.Builder
	for index := 0; index < len(runes); {
		if runes[index] == '`' && !isSDKRuneEscaped(runes, index) {
			if end := findSDKCodeSpanEnd(runes, index); end > index {
				out.WriteString(string(runes[index:end]))
				index = end
				continue
			}
		}
		for _, rule := range sdkAdjacentMarkupRules {
			delimiter := []rune(rule.delimiter)
			if !isSDKExactDelimiterRunAt(runes, index, delimiter) || isSDKRuneEscaped(runes, index) ||
				!sdkDelimiterCanOpen(runes, index, delimiter) {
				continue
			}
			if closeAt := findSDKDelimiterCloser(runes, index+len(delimiter), delimiter); closeAt > 0 {
				content := runes[index+len(delimiter) : closeAt]
				before := sdkRuneAt(runes, index-1)
				after := sdkRuneAt(runes, closeAt+len(delimiter))
				if shouldPreprocessSDKAdjacentMarkup(before, content, after) {
					out.WriteString(rule.openXML)
					out.WriteString(escapeSDKXMLText(stripSDKBackslashEscapes(string(content))))
					out.WriteString(rule.closeXML)
					index = closeAt + len(delimiter)
					goto next
				}
			}
		}
		out.WriteRune(runes[index])
		index++
	next:
	}
	return out.String()
}

func findSDKCodeSpanEnd(runes []rune, open int) int {
	ticks := 0
	for open+ticks < len(runes) && runes[open+ticks] == '`' {
		ticks++
	}
	for index := open + ticks; index < len(runes); index++ {
		if runes[index] != '`' || isSDKRuneEscaped(runes, index) {
			continue
		}
		end := index
		for end < len(runes) && runes[end] == '`' {
			end++
		}
		if end-index == ticks {
			return end
		}
		index = end - 1
	}
	return -1
}

func hasSDKDelimiterAt(runes []rune, index int, delimiter []rune) bool {
	if index+len(delimiter) > len(runes) {
		return false
	}
	for offset, character := range delimiter {
		if runes[index+offset] != character {
			return false
		}
	}
	return true
}

func isSDKExactDelimiterRunAt(runes []rune, index int, delimiter []rune) bool {
	if len(delimiter) == 0 || !hasSDKDelimiterAt(runes, index, delimiter) {
		return false
	}
	marker := delimiter[0]
	if index > 0 && runes[index-1] == marker {
		return false
	}
	return index+len(delimiter) >= len(runes) || runes[index+len(delimiter)] != marker
}

func sdkDelimiterCanOpen(runes []rune, index int, delimiter []rune) bool {
	before := sdkRuneAt(runes, index-1)
	after := sdkRuneAt(runes, index+len(delimiter))
	return isSDKLeftFlankingDelimiterRun(before, after) || isSDKCompatibleOpeningDelimiterRun(before, after)
}

func isSDKLeftFlankingDelimiterRun(before, after rune) bool {
	if isSDKMarkdownWhitespace(after) {
		return false
	}
	return !isSDKPunctuationRune(after) || isSDKMarkdownWhitespace(before) || isSDKPunctuationRune(before)
}

func isSDKCompatibleOpeningDelimiterRun(before, after rune) bool {
	return isSDKCJKLetter(before) && isSDKOpeningPunctuationRune(after)
}

func isSDKMarkdownWhitespace(character rune) bool {
	return character == 0 || unicode.IsSpace(character)
}

func findSDKDelimiterCloser(runes []rune, from int, delimiter []rune) int {
	for index := from; index+len(delimiter) <= len(runes); index++ {
		if runes[index] == '\n' {
			return -1
		}
		if isSDKExactDelimiterRunAt(runes, index, delimiter) && !isSDKRuneEscaped(runes, index) {
			return index
		}
	}
	return -1
}

func shouldPreprocessSDKAdjacentMarkup(before rune, content []rune, after rune) bool {
	if len(content) == 0 || unicode.IsSpace(content[0]) || unicode.IsSpace(content[len(content)-1]) ||
		containsSDKMarkdownNestedSyntax(content) || !isSDKPunctuationRune(content[len(content)-1]) ||
		!hasSDKCJKWritingContext(content) {
		return false
	}
	if isSDKCJKLetter(after) || isSDKASCIIAlphaNumeric(after) {
		return true
	}
	return isSDKOpeningPunctuationRune(content[0]) && isSDKCJKLetter(before)
}

func containsSDKMarkdownNestedSyntax(content []rune) bool {
	for _, character := range content {
		switch character {
		case '`', '[', ']', '<', '>':
			return true
		}
	}
	return false
}

func hasSDKCJKWritingContext(content []rune) bool {
	for _, character := range content {
		if isSDKCJKLetter(character) || character > 0x7f && isSDKPunctuationRune(character) {
			return true
		}
	}
	return false
}

func isSDKASCIIAlphaNumeric(character rune) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9'
}

func isSDKRuneEscaped(runes []rune, index int) bool {
	count := 0
	for position := index - 1; position >= 0 && runes[position] == '\\'; position-- {
		count++
	}
	return count%2 == 1
}

func sdkRuneAt(runes []rune, index int) rune {
	if index < 0 || index >= len(runes) {
		return 0
	}
	return runes[index]
}

func isSDKPunctuationRune(character rune) bool {
	if character == 0 {
		return false
	}
	return unicode.IsPunct(character) || unicode.IsSymbol(character) || character < 0x80 && isSDKASCIIPunctuation(byte(character))
}

func isSDKOpeningPunctuationRune(character rune) bool {
	if character == 0 {
		return false
	}
	if unicode.Is(unicode.Ps, character) || unicode.Is(unicode.Pi, character) {
		return true
	}
	switch character {
	case '(', '[', '{', '<', '"', '\'', '“', '‘', '（', '［', '｛', '《', '〈', '「', '『', '【':
		return true
	default:
		return false
	}
}

func isSDKCJKLetter(character rune) bool {
	return character != 0 && unicode.In(character, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

// normalizeSDKRenderedTextForCharacters mirrors the SDK's post-parse CJK
// bold rehydration but returns visible text instead of XML markup.
func normalizeSDKRenderedTextForCharacters(raw string) string {
	if !strings.Contains(raw, "**") {
		return stripSDKBackslashEscapes(raw)
	}
	runes := []rune(raw)
	var out strings.Builder
	delimiter := []rune("**")
	for index := 0; index < len(runes); {
		if isSDKExactDelimiterRunAt(runes, index, delimiter) && !isSDKRuneEscaped(runes, index) &&
			sdkDelimiterCanOpen(runes, index, delimiter) {
			if closeAt := findSDKDelimiterCloser(runes, index+len(delimiter), delimiter); closeAt > 0 {
				content := runes[index+len(delimiter) : closeAt]
				if shouldRehydrateSDKBold(sdkRuneAt(runes, index-1), content, sdkRuneAt(runes, closeAt+len(delimiter))) {
					out.WriteString(string(content))
					index = closeAt + len(delimiter)
					continue
				}
			}
		}
		out.WriteRune(runes[index])
		index++
	}
	return stripSDKBackslashEscapes(out.String())
}

func shouldRehydrateSDKBold(before rune, content []rune, after rune) bool {
	if len(content) == 0 || unicode.IsSpace(content[0]) || unicode.IsSpace(content[len(content)-1]) {
		return false
	}
	return isSDKPunctuationRune(content[len(content)-1]) && isSDKCJKLetter(after) ||
		isSDKOpeningPunctuationRune(content[0]) && isSDKCJKLetter(before)
}
