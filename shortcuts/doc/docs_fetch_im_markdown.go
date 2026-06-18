// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
)

type imMarkdownContext struct {
	baseURL string
}

type imMarkdownHandleFunc func(segment, inner string, attrs map[string]string, imCtx imMarkdownContext) string

type imMarkdownTagHandler struct {
	closeRE *regexp.Regexp
	handle  imMarkdownHandleFunc
}

func registerIMMarkdownHandler(tag string, handle imMarkdownHandleFunc) {
	imMarkdownHandlers[tag] = imMarkdownTagHandler{
		closeRE: regexp.MustCompile(`(?is)<(/?)` + regexp.QuoteMeta(tag) + `(?:\s[^<>]*?)?\s*/?>`),
		handle:  handle,
	}
}

var (
	imMarkdownTagStartRE  = regexp.MustCompile(`(?s)<([A-Za-z][A-Za-z0-9:_-]*)(?:\s[^<>]*?)?\s*/?>`)
	imMarkdownAttrRE      = regexp.MustCompile(`([A-Za-z_:][A-Za-z0-9_:.-]*)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	imMarkdownRowsRE      = regexp.MustCompile(`(?is)<tr\b[^>]*>(.*?)</tr>`)
	imMarkdownCellsRE     = regexp.MustCompile(`(?is)<t[dh]\b[^>]*>(.*?)</t[dh]>`)
	imMarkdownCellBreakRE = regexp.MustCompile(`(?i)<br\s*/?>`)
	imMarkdownAnyTagRE    = regexp.MustCompile(`(?s)</?([A-Za-z][A-Za-z0-9:_-]*)(?:\s[^<>]*?)?>`)
	imMarkdownLinkRE      = regexp.MustCompile(`(?is)<a\b[^>]*\bhref=(?:"([^"]*)"|'([^']*)')[^>]*>(.*?)</a>`)
)

var imMarkdownHandlers = map[string]imMarkdownTagHandler{}

func init() {
	registerIMMarkdownHandler("title", handleIMMarkdownTitle)
	registerIMMarkdownHandler("callout", handleIMMarkdownCallout)
	registerIMMarkdownHandler("grid", handleIMMarkdownPassthroughContainer)
	registerIMMarkdownHandler("column", handleIMMarkdownColumn)
	registerIMMarkdownHandler("table", handleIMMarkdownTable)
	registerIMMarkdownHandler("figure", handleIMMarkdownDiscard)
	registerIMMarkdownHandler("source", handleIMMarkdownDiscard)
	registerIMMarkdownHandler("button", handleIMMarkdownDiscard)
	registerIMMarkdownHandler("time", handleIMMarkdownDiscard)
	registerIMMarkdownHandler("whiteboard", handleIMMarkdownInlineCode)
	registerIMMarkdownHandler("sheet", handleIMMarkdownSheet)
	registerIMMarkdownHandler("bookmark", handleIMMarkdownBookmark)
	registerIMMarkdownHandler("cite", handleIMMarkdownCite)
}

func isIMMarkdownFetch(runtime interface{ Str(string) string }) bool {
	return strings.TrimSpace(runtime.Str("doc-format")) == "im-markdown"
}

func applyFetchIMMarkdown(data map[string]interface{}, docInput string) {
	doc, ok := data["document"].(map[string]interface{})
	if !ok {
		return
	}
	content, ok := doc["content"].(string)
	if !ok {
		return
	}
	doc["content"] = convertToIMMarkdown(content, newIMMarkdownContext(docInput))
}

func newIMMarkdownContext(docInput string) imMarkdownContext {
	base := "https://larkoffice.com"
	raw := strings.TrimSpace(docInput)
	if extracted, ok := imMarkdownBaseURLFromInput(raw); ok {
		base = extracted
	}
	return imMarkdownContext{baseURL: base}
}

// imMarkdownBaseURLFromInput keeps the tenant host from --doc when it is a URL
// so generated doc/sheet links point back to the same tenant. parseDocumentRef
// intentionally strips host information, so it cannot serve this formatting path.
func imMarkdownBaseURLFromInput(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	if u, err := url.Parse(raw); err == nil && u.Scheme != "" && u.Host != "" {
		return u.Scheme + "://" + u.Host, true
	}
	for _, marker := range []string{"/docx/", "/wiki/", "/doc/"} {
		idx := strings.Index(raw, marker)
		if idx <= 0 {
			continue
		}
		candidate := strings.Trim(raw[:idx], "/")
		if candidate == "" {
			continue
		}
		if u, err := url.Parse(candidate); err == nil && u.Scheme != "" && u.Host != "" {
			return u.Scheme + "://" + u.Host, true
		}
		if u, err := url.Parse("https://" + candidate); err == nil && u.Host != "" && strings.Contains(u.Host, ".") {
			return "https://" + u.Host, true
		}
	}
	return "", false
}

func convertToIMMarkdown(content string, imCtx imMarkdownContext) string {
	var out strings.Builder
	for offset := 0; offset < len(content); {
		// Scan only to the next XML-like opening tag. Plain Markdown text between
		// registered tags is copied unchanged, so ordinary Markdown is not re-parsed.
		loc := imMarkdownTagStartRE.FindStringSubmatchIndex(content[offset:])
		if loc == nil {
			out.WriteString(content[offset:])
			break
		}
		start := offset + loc[0]
		openEnd := offset + loc[1]
		tag := strings.ToLower(content[offset+loc[2] : offset+loc[3]])
		handler, ok := imMarkdownHandlers[tag]
		if !ok {
			// Unknown tags are left intact. im-markdown only downgrades tags with
			// explicit handlers so future server output does not get guessed at.
			out.WriteString(content[offset:openEnd])
			offset = openEnd
			continue
		}

		out.WriteString(content[offset:start])
		opening := content[start:openEnd]
		attrs := parseIMMarkdownAttrs(opening)
		if isSelfClosingIMMarkdownTag(opening) {
			out.WriteString(handler.handle(opening, "", attrs, imCtx))
			offset = openEnd
			continue
		}

		// Use the handler's precompiled close regexp to find the matching end tag.
		// Depth tracking keeps nested same-name containers paired correctly.
		closeStart, closeEnd, found := findIMMarkdownClosingTag(content, openEnd, handler)
		if !found {
			// Malformed or truncated fragments are preserved as-is from the opening
			// tag onward; do not drop content when the XML-ish structure is incomplete.
			out.WriteString(content[start:openEnd])
			offset = openEnd
			continue
		}
		segment := content[start:closeEnd]
		inner := content[openEnd:closeStart]
		out.WriteString(handler.handle(segment, inner, attrs, imCtx))
		offset = closeEnd
	}
	return out.String()
}

func findIMMarkdownClosingTag(content string, from int, handler imMarkdownTagHandler) (int, int, bool) {
	depth := 1
	for _, loc := range handler.closeRE.FindAllStringSubmatchIndex(content[from:], -1) {
		start := from + loc[0]
		end := from + loc[1]
		token := content[start:end]
		if loc[2] >= 0 && content[from+loc[2]:from+loc[3]] == "/" {
			depth--
			if depth == 0 {
				return start, end, true
			}
			continue
		}
		if !isSelfClosingIMMarkdownTag(token) {
			depth++
		}
	}
	return 0, 0, false
}

func parseIMMarkdownAttrs(opening string) map[string]string {
	attrs := map[string]string{}
	for _, match := range imMarkdownAttrRE.FindAllStringSubmatch(opening, -1) {
		value := match[2]
		if value == "" {
			value = match[3]
		}
		attrs[strings.ToLower(match[1])] = html.UnescapeString(value)
	}
	return attrs
}

func isSelfClosingIMMarkdownTag(tag string) bool {
	return strings.HasSuffix(strings.TrimSpace(tag), "/>")
}

func handleIMMarkdownTitle(_ string, inner string, _ map[string]string, _ imMarkdownContext) string {
	text := strings.TrimSpace(inner)
	if text == "" {
		return ""
	}
	return "# " + text
}

func handleIMMarkdownCallout(_ string, inner string, attrs map[string]string, imCtx imMarkdownContext) string {
	body := strings.TrimSpace(convertToIMMarkdown(inner, imCtx))
	emoji := strings.TrimSpace(attrs["emoji"])
	if emoji != "" {
		if body == "" {
			body = emoji
		} else {
			body = emoji + " " + body
		}
	}
	if body == "" {
		return "---\n---"
	}
	return fmt.Sprintf("---\n%s\n---", body)
}

func handleIMMarkdownPassthroughContainer(_ string, inner string, _ map[string]string, imCtx imMarkdownContext) string {
	return strings.TrimSpace(convertToIMMarkdown(inner, imCtx))
}

func handleIMMarkdownColumn(_ string, inner string, _ map[string]string, imCtx imMarkdownContext) string {
	body := strings.TrimSpace(convertToIMMarkdown(inner, imCtx))
	if body == "" {
		return ""
	}
	return body + "\n"
}

func handleIMMarkdownDiscard(_ string, _ string, _ map[string]string, _ imMarkdownContext) string {
	return ""
}

func handleIMMarkdownInlineCode(segment string, _ string, _ map[string]string, _ imMarkdownContext) string {
	return imMarkdownInlineCode(segment)
}

func handleIMMarkdownSheet(segment string, _ string, attrs map[string]string, imCtx imMarkdownContext) string {
	token := strings.TrimSpace(attrs["token"])
	if token == "" {
		return imMarkdownInlineCode(segment)
	}
	label := "sheet"
	if sheetID := strings.TrimSpace(attrs["sheet-id"]); sheetID != "" {
		label = "sheet " + sheetID
	}
	return fmt.Sprintf("[%s](%s/sheets/%s)", escapeMarkdownLinkText(label), strings.TrimRight(imCtx.baseURL, "/"), token)
}

func handleIMMarkdownBookmark(segment string, inner string, attrs map[string]string, imCtx imMarkdownContext) string {
	href := strings.TrimSpace(attrs["href"])
	name := firstNonEmpty(attrs["name"], attrs["title"], markdownPlainText(convertToIMMarkdown(inner, imCtx)), href)
	if href == "" {
		return name
	}
	return markdownLink(name, href)
}

func handleIMMarkdownCite(segment string, inner string, attrs map[string]string, imCtx imMarkdownContext) string {
	switch strings.ToLower(strings.TrimSpace(attrs["type"])) {
	case "user":
		userID := firstNonEmpty(attrs["user-id"], attrs["open-id"], attrs["id"])
		name := firstNonEmpty(attrs["user-name"], attrs["name"], markdownPlainText(inner), userID)
		if userID == "" {
			return name
		}
		return fmt.Sprintf(`<at user_id="%s">%s</at>`, html.EscapeString(userID), html.EscapeString(name))
	case "doc":
		title := firstNonEmpty(attrs["title"], attrs["name"], attrs["doc-id"], "document")
		if href := firstNonEmpty(attrs["href"], attrs["url"]); href != "" {
			return markdownLink(title, href)
		}
		docID := firstNonEmpty(attrs["doc-id"], attrs["token"])
		if docID == "" {
			return imMarkdownInlineCode(segment)
		}
		fileType := strings.Trim(strings.ToLower(firstNonEmpty(attrs["file-type"], "docx")), "/")
		return markdownLink(title, strings.TrimRight(imCtx.baseURL, "/")+"/"+fileType+"/"+docID)
	case "citation":
		if text, href, ok := extractIMMarkdownInnerLink(inner); ok {
			return markdownLink(text, href)
		}
		if href := firstNonEmpty(attrs["href"], attrs["url"]); href != "" {
			return markdownLink(firstNonEmpty(attrs["title"], attrs["name"], href), href)
		}
		return markdownPlainText(convertToIMMarkdown(inner, imCtx))
	default:
		return imMarkdownInlineCode(segment)
	}
}

func handleIMMarkdownTable(segment string, inner string, _ map[string]string, imCtx imMarkdownContext) string {
	rowMatches := imMarkdownRowsRE.FindAllStringSubmatch(inner, -1)
	if len(rowMatches) == 0 {
		return imMarkdownInlineCode(segment)
	}

	rows := make([][]string, 0, len(rowMatches))
	for _, rowMatch := range rowMatches {
		cellMatches := imMarkdownCellsRE.FindAllStringSubmatch(rowMatch[1], -1)
		if len(cellMatches) == 0 {
			continue
		}
		row := make([]string, 0, len(cellMatches))
		for _, cellMatch := range cellMatches {
			row = append(row, normalizeIMMarkdownTableCell(convertToIMMarkdown(cellMatch[1], imCtx)))
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return imMarkdownInlineCode(segment)
	}

	cols := 0
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	var out strings.Builder
	writeIMMarkdownTableRow(&out, padIMMarkdownTableRow(rows[0], cols))
	separator := make([]string, cols)
	for i := range separator {
		separator[i] = "-"
	}
	writeIMMarkdownTableRow(&out, separator)
	for _, row := range rows[1:] {
		writeIMMarkdownTableRow(&out, padIMMarkdownTableRow(row, cols))
	}
	return strings.TrimRight(out.String(), "\n")
}

func normalizeIMMarkdownTableCell(cell string) string {
	const brPlaceholder = "\x00BR\x00"
	cell = imMarkdownCellBreakRE.ReplaceAllString(cell, brPlaceholder)
	cell = imMarkdownAnyTagRE.ReplaceAllStringFunc(cell, func(tag string) string {
		name := strings.ToLower(strings.TrimPrefix(imMarkdownAnyTagRE.FindStringSubmatch(tag)[1], "/"))
		if name == "at" {
			return tag
		}
		return ""
	})
	cell = html.UnescapeString(cell)
	cell = strings.ReplaceAll(cell, brPlaceholder, "<br>")
	cell = strings.ReplaceAll(cell, "  \n", "<br>")
	cell = strings.ReplaceAll(cell, "\n", "<br>")
	cell = strings.ReplaceAll(cell, "|", `\|`)
	lines := strings.Fields(cell)
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, " ")
}

func writeIMMarkdownTableRow(out *strings.Builder, row []string) {
	out.WriteString("| ")
	out.WriteString(strings.Join(row, " | "))
	out.WriteString(" |\n")
}

func padIMMarkdownTableRow(row []string, cols int) []string {
	if len(row) >= cols {
		return row
	}
	padded := make([]string, cols)
	copy(padded, row)
	return padded
}

func extractIMMarkdownInnerLink(inner string) (string, string, bool) {
	match := imMarkdownLinkRE.FindStringSubmatch(inner)
	if match == nil {
		return "", "", false
	}
	href := match[1]
	if href == "" {
		href = match[2]
	}
	text := strings.TrimSpace(markdownPlainText(match[3]))
	if text == "" {
		text = href
	}
	return text, html.UnescapeString(href), true
}

func markdownPlainText(s string) string {
	s = imMarkdownCellBreakRE.ReplaceAllString(s, "\n")
	s = imMarkdownAnyTagRE.ReplaceAllString(s, "")
	return strings.TrimSpace(html.UnescapeString(s))
}

func markdownLink(text, href string) string {
	return fmt.Sprintf("[%s](%s)", escapeMarkdownLinkText(firstNonEmpty(text, href)), strings.TrimSpace(href))
}

func escapeMarkdownLinkText(text string) string {
	text = strings.ReplaceAll(text, `\`, `\\`)
	text = strings.ReplaceAll(text, `[`, `\[`)
	text = strings.ReplaceAll(text, `]`, `\]`)
	return text
}

func imMarkdownInlineCode(s string) string {
	maxRun := 0
	run := 0
	for _, r := range s {
		if r == '`' {
			run++
			if run > maxRun {
				maxRun = run
			}
			continue
		}
		run = 0
	}
	fence := strings.Repeat("`", maxRun+1)
	return fence + s + fence
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
