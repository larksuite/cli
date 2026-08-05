// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

type imMarkdownContext struct {
	baseURL         string
	blockquoteDepth int
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
	imMarkdownRowTagRE    = regexp.MustCompile(`(?is)<(/?)tr\b[^>]*?\s*/?>`)
	imMarkdownCellTagRE   = regexp.MustCompile(`(?is)<(/?)t[dh]\b[^>]*?\s*/?>`)
	imMarkdownCellBreakRE = regexp.MustCompile(`(?i)<br\s*/?>`)
	imMarkdownAnyTagRE    = regexp.MustCompile(`(?s)</?([A-Za-z][A-Za-z0-9:_-]*)(?:\s[^<>]*?)?>`)
	imMarkdownLinkRE      = regexp.MustCompile(`(?is)<a\b[^>]*\bhref=(?:"([^"]*)"|'([^']*)')[^>]*>(.*?)</a>`)
	imMarkdownCodeBlockRE = regexp.MustCompile(`(?is)^\s*<code(?:\s[^<>]*?)?>(.*?)</code>\s*$`)
	imMarkdownLiOpenRE    = regexp.MustCompile(`(?is)<li(?:\s[^<>]*?)?>`)
	imMarkdownLiCloseRE   = regexp.MustCompile(`(?is)<(/?)li(?:\s[^<>]*?)?\s*/?>`)
)

var imMarkdownHandlers = map[string]imMarkdownTagHandler{}

func init() {
	registerIMMarkdownHandler("title", handleIMMarkdownTitle)
	for level := 1; level <= 9; level++ {
		registerIMMarkdownHandler(fmt.Sprintf("h%d", level), handleIMMarkdownHeading(level))
	}
	registerIMMarkdownHandler("p", handleIMMarkdownParagraph)
	registerIMMarkdownHandler("ul", handleIMMarkdownUnorderedList)
	registerIMMarkdownHandler("ol", handleIMMarkdownOrderedList)
	registerIMMarkdownHandler("li", handleIMMarkdownListItem)
	registerIMMarkdownHandler("callout", handleIMMarkdownCallout)
	registerIMMarkdownHandler("blockquote", handleIMMarkdownBlockquote)
	registerIMMarkdownHandler("grid", handleIMMarkdownPassthroughContainer)
	registerIMMarkdownHandler("column", handleIMMarkdownColumn)
	registerIMMarkdownHandler("table", handleIMMarkdownTable)
	registerIMMarkdownHandler("colgroup", handleIMMarkdownDiscard)
	registerIMMarkdownHandler("col", handleIMMarkdownDiscard)
	registerIMMarkdownHandler("pre", handleIMMarkdownPre)
	registerIMMarkdownHandler("code", handleIMMarkdownCode)
	registerIMMarkdownHandler("latex", handleIMMarkdownLatex)
	registerIMMarkdownHandler("hr", handleIMMarkdownHorizontalRule)
	registerIMMarkdownHandler("img", handleIMMarkdownImage)
	registerIMMarkdownHandler("figure", handleIMMarkdownDiscard)
	registerIMMarkdownHandler("source", handleIMMarkdownSource)
	registerIMMarkdownHandler("button", handleIMMarkdownDiscard)
	registerIMMarkdownHandler("time", handleIMMarkdownDiscard)
	registerIMMarkdownHandler("whiteboard", handleIMMarkdownInlineCode)
	registerIMMarkdownHandler("sheet", handleIMMarkdownSheet)
	registerIMMarkdownHandler("task", handleIMMarkdownConditionalResourceLabel("Task", "task-id", "guid", "token", "id"))
	registerIMMarkdownHandler("chat_card", handleIMMarkdownConditionalResourceLabel("Chat card", "chat-id", "chat_id", "id"))
	registerIMMarkdownHandler("bitable", handleIMMarkdownResourceLabel("Base"))
	registerIMMarkdownHandler("base_refer", handleIMMarkdownResourceLabel("Base"))
	registerIMMarkdownHandler("okr", handleIMMarkdownResourceLabel("OKR"))
	registerIMMarkdownHandler("poll", handleIMMarkdownDiscard)
	registerIMMarkdownHandler("agenda", handleIMMarkdownDiscard)
	registerIMMarkdownHandler("folder_manager", handleIMMarkdownDiscard)
	registerIMMarkdownHandler("wiki_catalog", handleIMMarkdownDiscard)
	registerIMMarkdownHandler("wiki_recent_update", handleIMMarkdownDiscard)
	registerIMMarkdownHandler("chart_refer_host_perm", handleIMMarkdownDiscard)
	registerIMMarkdownHandler("synced_reference", handleIMMarkdownDiscard)
	registerIMMarkdownHandler("synced-source", handleIMMarkdownDiscard)
	registerIMMarkdownHandler("mindnote", handleIMMarkdownDiscard)
	registerIMMarkdownHandler("bookmark", handleIMMarkdownBookmark)
	registerIMMarkdownHandler("cite", handleIMMarkdownCite)
	registerIMMarkdownHandler("b", handleIMMarkdownStrong)
	registerIMMarkdownHandler("em", handleIMMarkdownEmphasis)
	registerIMMarkdownHandler("del", handleIMMarkdownDelete)
	registerIMMarkdownHandler("u", handleIMMarkdownPlainInline)
	registerIMMarkdownHandler("span", handleIMMarkdownPlainInline)
	registerIMMarkdownHandler("a", handleIMMarkdownAnchor)
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

func (c imMarkdownContext) withBlockquote() imMarkdownContext {
	c.blockquoteDepth++
	return c
}

func (c imMarkdownContext) inBlockquote() bool {
	return c.blockquoteDepth > 0
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
			out.WriteString(content[start:])
			break
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

func handleIMMarkdownTitle(_ string, inner string, _ map[string]string, imCtx imMarkdownContext) string {
	text := strings.TrimSpace(convertToIMMarkdown(inner, imCtx))
	if text == "" {
		return ""
	}
	return "# " + text
}

func handleIMMarkdownHeading(level int) imMarkdownHandleFunc {
	return func(_ string, inner string, _ map[string]string, imCtx imMarkdownContext) string {
		text := strings.TrimSpace(convertToIMMarkdown(inner, imCtx))
		if text == "" {
			return ""
		}
		markdownLevel := level
		if markdownLevel > 6 {
			markdownLevel = 6
		}
		return strings.Repeat("#", markdownLevel) + " " + text
	}
}

func handleIMMarkdownParagraph(_ string, inner string, _ map[string]string, imCtx imMarkdownContext) string {
	body := strings.TrimSpace(convertToIMMarkdown(inner, imCtx))
	if body == "" {
		return ""
	}
	if imCtx.inBlockquote() {
		return body + "\n"
	}
	return body
}

func handleIMMarkdownUnorderedList(_ string, inner string, _ map[string]string, imCtx imMarkdownContext) string {
	return convertIMMarkdownListItems(inner, false, imCtx)
}

func handleIMMarkdownOrderedList(_ string, inner string, _ map[string]string, imCtx imMarkdownContext) string {
	return convertIMMarkdownListItems(inner, true, imCtx)
}

func handleIMMarkdownListItem(_ string, inner string, attrs map[string]string, imCtx imMarkdownContext) string {
	prefix := "-"
	if seq := strings.TrimSpace(attrs["seq"]); seq != "" && seq != "auto" {
		prefix = strings.TrimSuffix(seq, ".") + "."
	}
	body := strings.TrimSpace(convertToIMMarkdown(inner, imCtx))
	if body == "" {
		return ""
	}
	return prefix + " " + indentIMMarkdownListContinuation(body) + "\n"
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

func handleIMMarkdownBlockquote(_ string, inner string, _ map[string]string, imCtx imMarkdownContext) string {
	body := strings.TrimSpace(convertToIMMarkdown(inner, imCtx.withBlockquote()))
	if body == "" {
		return ""
	}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ">"
			continue
		}
		lines[i] = "> " + line
	}
	return strings.Join(lines, "\n")
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

func handleIMMarkdownPre(_ string, inner string, attrs map[string]string, _ imMarkdownContext) string {
	lang := strings.TrimSpace(attrs["lang"])
	code := strings.TrimSpace(inner)
	if match := imMarkdownCodeBlockRE.FindStringSubmatch(code); match != nil {
		code = match[1]
	}
	return imMarkdownFencedCode(html.UnescapeString(code), lang)
}

func handleIMMarkdownCode(_ string, inner string, _ map[string]string, _ imMarkdownContext) string {
	return imMarkdownInlineCode(markdownPlainText(inner))
}

func handleIMMarkdownLatex(_ string, inner string, _ map[string]string, _ imMarkdownContext) string {
	expr := strings.TrimSpace(markdownPlainText(inner))
	if expr == "" {
		return ""
	}
	return "$" + strings.ReplaceAll(expr, "$", `\$`) + "$"
}

func handleIMMarkdownHorizontalRule(_ string, _ string, _ map[string]string, _ imMarkdownContext) string {
	return "---"
}

func handleIMMarkdownImage(_ string, _ string, attrs map[string]string, _ imMarkdownContext) string {
	href := firstNonEmpty(attrs["href"], attrs["src"], attrs["url"])
	if href == "" {
		return ""
	}
	alt := firstNonEmpty(attrs["alt"], attrs["name"], attrs["title"])
	return fmt.Sprintf("![%s](%s)", escapeMarkdownLinkText(alt), escapeMarkdownLinkDestination(href))
}

func handleIMMarkdownSource(_ string, _ string, attrs map[string]string, _ imMarkdownContext) string {
	name := strings.TrimSpace(attrs["name"])
	if name == "" {
		return ""
	}
	return imMarkdownInlineCode(name)
}

func handleIMMarkdownResourceLabel(label string) imMarkdownHandleFunc {
	return func(_ string, _ string, _ map[string]string, _ imMarkdownContext) string {
		return imMarkdownInlineCode(label)
	}
}

func handleIMMarkdownConditionalResourceLabel(label string, attrNames ...string) imMarkdownHandleFunc {
	return func(_ string, _ string, attrs map[string]string, _ imMarkdownContext) string {
		for _, attrName := range attrNames {
			if strings.TrimSpace(attrs[attrName]) != "" {
				return imMarkdownInlineCode(label)
			}
		}
		return ""
	}
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
	return markdownLink(label, strings.TrimRight(imCtx.baseURL, "/")+"/sheets/"+token)
}

func handleIMMarkdownBookmark(segment string, inner string, attrs map[string]string, imCtx imMarkdownContext) string {
	href := strings.TrimSpace(attrs["href"])
	name := firstNonEmpty(attrs["name"], attrs["title"], markdownLinkLabelText(convertToIMMarkdown(inner, imCtx)), href)
	if href == "" {
		return name
	}
	return markdownLink(name, href)
}

func handleIMMarkdownStrong(_ string, inner string, _ map[string]string, imCtx imMarkdownContext) string {
	body := strings.TrimSpace(convertToIMMarkdown(inner, imCtx))
	if body == "" {
		return ""
	}
	return "**" + body + "**"
}

func handleIMMarkdownEmphasis(_ string, inner string, _ map[string]string, imCtx imMarkdownContext) string {
	body := strings.TrimSpace(convertToIMMarkdown(inner, imCtx))
	if body == "" {
		return ""
	}
	return "*" + body + "*"
}

func handleIMMarkdownDelete(_ string, inner string, _ map[string]string, imCtx imMarkdownContext) string {
	body := strings.TrimSpace(convertToIMMarkdown(inner, imCtx))
	if body == "" {
		return ""
	}
	return "~~" + body + "~~"
}

func handleIMMarkdownPlainInline(_ string, inner string, _ map[string]string, imCtx imMarkdownContext) string {
	return strings.TrimSpace(convertToIMMarkdown(inner, imCtx))
}

func handleIMMarkdownAnchor(_ string, inner string, attrs map[string]string, imCtx imMarkdownContext) string {
	href := strings.TrimSpace(attrs["href"])
	text := firstNonEmpty(markdownLinkLabelText(convertToIMMarkdown(inner, imCtx)), attrs["name"], attrs["title"], href)
	if href == "" {
		return text
	}
	return markdownLink(text, href)
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
	// Rows and cells are matched with tag-depth tracking instead of non-greedy
	// regex captures. A table nested inside a cell can contain its own </tr> and
	// </td>; treating those as the outer row/cell boundary corrupts the table.
	rowBodies := extractIMMarkdownElementBodies(inner, imMarkdownRowTagRE)
	if len(rowBodies) == 0 {
		return imMarkdownInlineCode(segment)
	}

	rows := make([][]string, 0, len(rowBodies))
	for _, rowBody := range rowBodies {
		cellBodies := extractIMMarkdownElementBodies(rowBody, imMarkdownCellTagRE)
		if len(cellBodies) == 0 {
			continue
		}
		row := make([]string, 0, len(cellBodies))
		for _, cellBody := range cellBodies {
			row = append(row, normalizeIMMarkdownTableCell(convertToIMMarkdown(cellBody, imCtx)))
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

// extractIMMarkdownElementBodies returns the inner content of each top-level
// element matched by tagRE. tagRE must expose the optional closing slash as its
// first capture group, matching the row/cell regexes above.
func extractIMMarkdownElementBodies(content string, tagRE *regexp.Regexp) []string {
	var bodies []string
	for offset := 0; offset < len(content); {
		loc := tagRE.FindStringSubmatchIndex(content[offset:])
		if loc == nil {
			break
		}
		openStart := offset + loc[0]
		openEnd := offset + loc[1]
		opening := content[openStart:openEnd]
		if loc[2] >= 0 && content[offset+loc[2]:offset+loc[3]] == "/" {
			offset = openEnd
			continue
		}
		if isSelfClosingIMMarkdownTag(opening) {
			bodies = append(bodies, "")
			offset = openEnd
			continue
		}
		closeStart, closeEnd, found := findIMMarkdownElementClosingTag(content, openEnd, tagRE)
		if !found {
			break
		}
		bodies = append(bodies, content[openEnd:closeStart])
		offset = closeEnd
	}
	return bodies
}

func findIMMarkdownElementClosingTag(content string, from int, tagRE *regexp.Regexp) (int, int, bool) {
	depth := 1
	for _, loc := range tagRE.FindAllStringSubmatchIndex(content[from:], -1) {
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

func convertIMMarkdownListItems(inner string, ordered bool, imCtx imMarkdownContext) string {
	var out strings.Builder
	for offset, index := 0, 1; offset < len(inner); {
		loc := imMarkdownLiOpenRE.FindStringIndex(inner[offset:])
		if loc == nil {
			break
		}
		openStart := offset + loc[0]
		openEnd := offset + loc[1]
		opening := inner[openStart:openEnd]
		closeStart, closeEnd, found := findIMMarkdownListItemClosingTag(inner, openEnd)
		if !found {
			break
		}
		body := strings.TrimSpace(convertToIMMarkdown(inner[openEnd:closeStart], imCtx))
		if body != "" {
			prefix := "-"
			if ordered {
				attrs := parseIMMarkdownAttrs(opening)
				if seq := strings.TrimSpace(attrs["seq"]); seq != "" && seq != "auto" {
					prefix = strings.TrimSuffix(seq, ".") + "."
				} else {
					prefix = fmt.Sprintf("%d.", index)
				}
				index++
			}
			out.WriteString(prefix)
			out.WriteString(" ")
			out.WriteString(indentIMMarkdownListContinuation(body))
			out.WriteString("\n")
		}
		offset = closeEnd
	}
	return strings.TrimRight(out.String(), "\n")
}

func findIMMarkdownListItemClosingTag(content string, from int) (int, int, bool) {
	depth := 1
	for _, loc := range imMarkdownLiCloseRE.FindAllStringSubmatchIndex(content[from:], -1) {
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

func indentIMMarkdownListContinuation(body string) string {
	return strings.ReplaceAll(body, "\n", "\n  ")
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

func markdownLinkLabelText(s string) string {
	text := markdownPlainText(s)
	if !strings.Contains(text, "---") {
		return text
	}
	lines := strings.Split(text, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.TrimSpace(line) == "---" {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func markdownLink(text, href string) string {
	cleanHref := strings.TrimSpace(href)
	return fmt.Sprintf("[%s](%s)", escapeMarkdownLinkText(firstNonEmpty(text, cleanHref)), escapeMarkdownLinkDestination(cleanHref))
}

func escapeMarkdownLinkText(text string) string {
	text = strings.ReplaceAll(text, `\`, `\\`)
	text = strings.ReplaceAll(text, `[`, `\[`)
	text = strings.ReplaceAll(text, `]`, `\]`)
	return text
}

func escapeMarkdownLinkDestination(href string) string {
	// Lark/Feishu IM Markdown does not reliably parse raw spaces or parentheses
	// inside (...). Keep URL delimiters like :/?#&= intact, but percent-encode
	// characters that can terminate or split the Markdown link destination.
	var out strings.Builder
	out.Grow(len(href))
	for i := 0; i < len(href); {
		if href[i] == '%' {
			if i+2 < len(href) && isHexDigit(href[i+1]) && isHexDigit(href[i+2]) {
				out.WriteString(href[i : i+3])
				i += 3
			} else {
				writePercentEncodedByte(&out, href[i])
				i++
			}
			continue
		}
		if href[i] < utf8.RuneSelf {
			if shouldPercentEncodeIMMarkdownURLByte(href[i]) {
				writePercentEncodedByte(&out, href[i])
			} else {
				out.WriteByte(href[i])
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(href[i:])
		if r == utf8.RuneError && size == 1 {
			writePercentEncodedByte(&out, href[i])
			i++
			continue
		}
		for _, b := range []byte(href[i : i+size]) {
			writePercentEncodedByte(&out, b)
		}
		i += size
	}
	return out.String()
}

func shouldPercentEncodeIMMarkdownURLByte(b byte) bool {
	if b <= ' ' || b >= 0x7f {
		return true
	}
	switch b {
	case '(', ')', '<', '>', '"', '\\', '^', '`', '{', '|', '}':
		return true
	default:
		return false
	}
}

func writePercentEncodedByte(out *strings.Builder, b byte) {
	const hex = "0123456789ABCDEF"
	out.WriteByte('%')
	out.WriteByte(hex[b>>4])
	out.WriteByte(hex[b&0x0f])
}

func isHexDigit(b byte) bool {
	return ('0' <= b && b <= '9') || ('a' <= b && b <= 'f') || ('A' <= b && b <= 'F')
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
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		return fence + " " + s + " " + fence
	}
	return fence + s + fence
}

func imMarkdownFencedCode(code, lang string) string {
	maxRun := 0
	for _, line := range strings.Split(code, "\n") {
		if run := leadingBacktickRun(line); run > maxRun {
			maxRun = run
		}
	}
	fenceLen := maxRun + 1
	if fenceLen < 3 {
		fenceLen = 3
	}
	fence := strings.Repeat("`", fenceLen)
	return fence + strings.TrimSpace(lang) + "\n" + strings.Trim(code, "\n") + "\n" + fence
}

func leadingBacktickRun(s string) int {
	run := 0
	for _, r := range s {
		if r != '`' {
			break
		}
		run++
	}
	return run
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
