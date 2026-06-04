// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// cardObj is a convenience alias for generic JSON objects.
type cardObj = map[string]interface{}

// cardMode controls output verbosity.
type cardMode int

const (
	cardModeConcise  cardMode = 0
	cardModeDetailed cardMode = 1
)

// ── Constants ─────────────────────────────────────────────────────────────────

var cardEmojiMap = map[string]string{
	"OK":       "👌",
	"THUMBSUP": "👍",
	"SMILE":    "😊",
	"HEART":    "❤️",
	"CLAP":     "👏",
	"FIRE":     "🔥",
	"PARTY":    "🎉",
	"THINK":    "🤔",
}

var cardChartTypeNames = map[string]string{
	"bar":     "Bar chart",
	"line":    "Line chart",
	"pie":     "Pie chart",
	"area":    "Area chart",
	"radar":   "Radar chart",
	"scatter": "Scatter plot",
}

// ── Entry point ───────────────────────────────────────────────────────────────

type interactiveConverter struct{}

func (interactiveConverter) Convert(ctx *ConvertContext) string {
	return convertCard(ctx.RawContent, ctx.Mentions)
}

// convertCard converts a raw interactive/card message content JSON to human-readable string.
// mentions is the raw mentions array from the API response; pass nil when not available.
func convertCard(raw string, mentions []interface{}) string {
	var parsed cardObj
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return "[interactive card]"
	}

	// raw_card_content format: outer JSON has "json_card" string field
	if jsonCard, ok := parsed["json_card"].(string); ok {
		c := &cardConverter{mode: cardModeConcise}
		switch att := parsed["json_attachment"].(type) {
		case string:
			if att != "" {
				var attObj cardObj
				if json.Unmarshal([]byte(att), &attObj) == nil {
					c.attachment = attObj
				}
			}
		case cardObj:
			c.attachment = att
		}
		if len(mentions) > 0 {
			c.mentionsByKey = buildMentionsByKey(mentions)
		}
		schema := 0
		if s, ok := parsed["card_schema"].(float64); ok {
			schema = int(s)
		}
		result := c.convert(jsonCard, schema)
		if result == "" {
			return "[interactive card]"
		}
		return result
	}

	// Legacy format
	return convertLegacyCard(parsed)
}

// buildMentionsByKey indexes the mentions array by key for O(1) lookup in convertAt.
func buildMentionsByKey(mentions []interface{}) map[string]map[string]interface{} {
	m := make(map[string]map[string]interface{}, len(mentions))
	for _, raw := range mentions {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		key, _ := item["key"].(string)
		if key != "" {
			m[key] = item
		}
	}
	return m
}

// ── Legacy converter ──────────────────────────────────────────────────────────

func convertLegacyCard(parsed cardObj) string {
	var texts []string

	if header, ok := parsed["header"].(cardObj); ok {
		if title, ok := header["title"].(cardObj); ok {
			if content, ok := title["content"].(string); ok && content != "" {
				texts = append(texts, "**"+content+"**")
			}
		}
	}

	body, _ := parsed["body"].(cardObj)
	var elements []interface{}
	if e, ok := parsed["elements"].([]interface{}); ok {
		elements = e
	} else if body != nil {
		if e, ok := body["elements"].([]interface{}); ok {
			elements = e
		}
	}
	legacyExtractTexts(elements, &texts)

	if len(texts) == 0 {
		return "[interactive card]"
	}
	return strings.Join(texts, "\n")
}

func legacyExtractTexts(elements []interface{}, out *[]string) {
	for _, el := range elements {
		elem, ok := el.(cardObj)
		if !ok {
			continue
		}
		tag, _ := elem["tag"].(string)

		if tag == "markdown" {
			if content, ok := elem["content"].(string); ok {
				*out = append(*out, content)
			}
			continue
		}
		if tag == "div" || tag == "plain_text" || tag == "lark_md" {
			if text, ok := elem["text"].(cardObj); ok {
				if content, ok := text["content"].(string); ok && content != "" {
					*out = append(*out, content)
				}
			}
			if content, ok := elem["content"].(string); ok && content != "" {
				*out = append(*out, content)
			}
		}
		if tag == "column_set" {
			if cols, ok := elem["columns"].([]interface{}); ok {
				for _, col := range cols {
					if cm, ok := col.(cardObj); ok {
						if elems, ok := cm["elements"].([]interface{}); ok {
							legacyExtractTexts(elems, out)
						}
					}
				}
			}
		}
		if elems, ok := elem["elements"].([]interface{}); ok {
			legacyExtractTexts(elems, out)
		}
	}
}

// ── CardConverter ─────────────────────────────────────────────────────────────

type cardConverter struct {
	mode          cardMode
	attachment    cardObj
	mentionsByKey map[string]map[string]interface{}
}

func (c *cardConverter) convert(jsonCard string, hintSchema int) string {
	var card cardObj
	if err := json.Unmarshal([]byte(jsonCard), &card); err != nil {
		return "<card>\n[Unable to parse card content]\n</card>"
	}

	header, _ := card["header"].(cardObj)
	title := ""
	subtitle := ""
	headerTags := ""
	if header != nil {
		title = c.extractHeaderTitle(header)
		subtitle = c.extractHeaderSubtitle(header)
		headerTags = c.extractHeaderTags(header)
	}

	bodyContent := ""
	if body, ok := card["body"].(cardObj); ok {
		bodyContent = c.convertBody(body)
	}

	var sb strings.Builder
	if title != "" && subtitle != "" {
		sb.WriteString(fmt.Sprintf("<card title=\"%s\" subtitle=\"%s\">\n", cardEscapeAttr(title), cardEscapeAttr(subtitle)))
	} else if title != "" {
		sb.WriteString(fmt.Sprintf("<card title=\"%s\">\n", cardEscapeAttr(title)))
	} else if subtitle != "" {
		sb.WriteString(fmt.Sprintf("<card subtitle=\"%s\">\n", cardEscapeAttr(subtitle)))
	} else {
		sb.WriteString("<card>\n")
	}
	if headerTags != "" {
		sb.WriteString(headerTags)
		sb.WriteString("\n")
	}
	if bodyContent != "" {
		sb.WriteString(bodyContent)
		sb.WriteString("\n")
	}
	sb.WriteString("</card>")
	return sb.String()
}

func (c *cardConverter) extractHeaderTitle(header cardObj) string {
	if prop, ok := header["property"].(cardObj); ok {
		if titleElem, ok := prop["title"]; ok {
			return c.extractTextContent(titleElem)
		}
	}
	if titleElem, ok := header["title"]; ok {
		return c.extractTextContent(titleElem)
	}
	return ""
}

// extractHeaderSubtitle returns the subtitle text of a card header, supporting both
// the property-wrapped and flat element formats.
func (c *cardConverter) extractHeaderSubtitle(header cardObj) string {
	if prop, ok := header["property"].(cardObj); ok {
		if subtitleElem, ok := prop["subtitle"]; ok {
			return c.extractTextContent(subtitleElem)
		}
	}
	if subtitleElem, ok := header["subtitle"]; ok {
		return c.extractTextContent(subtitleElem)
	}
	return ""
}

// extractHeaderTags returns a space-joined string of header tag labels from textTagList,
// supporting both property-wrapped and flat header formats.
func (c *cardConverter) extractHeaderTags(header cardObj) string {
	var prop cardObj
	if p, ok := header["property"].(cardObj); ok {
		prop = p
	} else {
		prop = header
	}
	tagList, ok := prop["textTagList"].([]interface{})
	if !ok || len(tagList) == 0 {
		return ""
	}
	var tags []string
	for _, tag := range tagList {
		tm, ok := tag.(cardObj)
		if !ok {
			continue
		}
		if text := c.convertElement(tm, 0); text != "" {
			tags = append(tags, text)
		}
	}
	if len(tags) == 0 {
		return ""
	}
	return strings.Join(tags, " ")
}

func (c *cardConverter) convertBody(body cardObj) string {
	var elements []interface{}

	if prop, ok := body["property"].(cardObj); ok {
		if e, ok := prop["elements"].([]interface{}); ok && len(e) > 0 {
			elements = e
		}
	}
	if len(elements) == 0 {
		if e, ok := body["elements"].([]interface{}); ok {
			elements = e
		}
	}

	if len(elements) == 0 {
		return ""
	}
	return c.convertElements(elements, 0)
}

func (c *cardConverter) convertElements(elements []interface{}, depth int) string {
	var results []string
	for _, el := range elements {
		elem, ok := el.(cardObj)
		if !ok {
			continue
		}
		if result := c.convertElement(elem, depth); result != "" {
			results = append(results, result)
		}
	}
	return strings.Join(results, "\n")
}

func (c *cardConverter) extractProperty(elem cardObj) cardObj {
	if prop, ok := elem["property"].(cardObj); ok {
		return prop
	}
	return elem
}

func (c *cardConverter) convertElement(elem cardObj, depth int) string {
	tag, _ := elem["tag"].(string)
	id, _ := elem["id"].(string)
	prop := c.extractProperty(elem)

	switch tag {
	case "plain_text", "text":
		return c.convertPlainText(prop)
	case "markdown":
		return c.convertMarkdown(prop)
	case "markdown_v1":
		return c.convertMarkdownV1(elem, prop)
	case "div":
		return c.convertDiv(prop, id)
	case "note":
		return c.convertNote(prop)
	case "hr":
		return "---"
	case "br":
		return "\n"
	case "column_set":
		return c.convertColumnSet(prop, depth)
	case "column":
		return c.convertColumn(prop, depth)
	case "person":
		return c.convertPerson(prop, id)
	case "person_v1":
		return c.convertPersonV1(prop, id)
	case "person_list":
		return c.convertPersonList(prop)
	case "avatar":
		return c.convertAvatar(prop, id)
	case "at":
		return c.convertAt(prop)
	case "at_all":
		return "@everyone"
	case "button":
		return c.convertButton(prop, id)
	case "actions", "action":
		return c.convertActions(prop)
	case "overflow":
		return c.convertOverflow(prop)
	case "select_static", "select_person":
		return c.convertSelect(prop, id, false)
	case "multi_select_static", "multi_select_person":
		return c.convertSelect(prop, id, true)
	case "select_img":
		return c.convertSelectImg(prop, id)
	case "input":
		return c.convertInput(prop, id)
	case "date_picker":
		return c.convertDatePicker(prop, id, "date")
	case "picker_time":
		return c.convertDatePicker(prop, id, "time")
	case "picker_datetime":
		return c.convertDatePicker(prop, id, "datetime")
	case "checker":
		return c.convertChecker(prop, id)
	case "img", "image":
		return c.convertImage(prop, id)
	case "img_combination":
		return c.convertImgCombination(prop)
	case "table":
		return c.convertTable(prop)
	case "chart":
		return c.convertChart(prop, id)
	case "audio":
		return c.convertAudio(prop, id)
	case "video":
		return c.convertVideo(prop, id)
	case "collapsible_panel":
		return c.convertCollapsiblePanel(prop, id)
	case "form":
		return c.convertForm(prop, id)
	case "interactive_container":
		return c.convertInteractiveContainer(prop, id)
	case "text_tag":
		return c.convertTextTag(prop)
	case "number_tag":
		return c.convertNumberTag(prop)
	case "link":
		return c.convertLink(prop)
	case "emoji":
		return c.convertEmoji(prop)
	case "local_datetime":
		return c.convertLocalDatetime(prop)
	case "list":
		return c.convertList(prop)
	case "blockquote":
		return c.convertBlockquote(prop)
	case "code_block":
		return c.convertCodeBlock(prop)
	case "code_span":
		return c.convertCodeSpan(prop)
	case "heading":
		return c.convertHeading(prop)
	case "fallback_text":
		return c.convertFallbackText(prop)
	case "repeat":
		return c.convertRepeat(prop)
	case "card_header", "custom_icon", "standard_icon":
		return ""
	default:
		return c.convertUnknown(prop, tag)
	}
}

// ── Text extraction ───────────────────────────────────────────────────────────

func (c *cardConverter) extractTextContent(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	m, ok := v.(cardObj)
	if !ok {
		return ""
	}
	if prop, ok := m["property"].(cardObj); ok {
		return c.extractTextFromProperty(prop)
	}
	return c.extractTextFromProperty(m)
}

func (c *cardConverter) extractTextFromProperty(prop cardObj) string {
	// i18n content
	if i18n, ok := prop["i18nContent"].(cardObj); ok {
		for _, lang := range []string{"zh_cn", "en_us", "ja_jp"} {
			if t, ok := i18n[lang].(string); ok && t != "" {
				return t
			}
		}
	}
	if content, ok := prop["content"].(string); ok {
		return content
	}
	if elements, ok := prop["elements"].([]interface{}); ok && len(elements) > 0 {
		var texts []string
		for _, el := range elements {
			if t := c.extractTextContent(el); t != "" {
				texts = append(texts, t)
			}
		}
		return strings.Join(texts, "")
	}
	if text, ok := prop["text"].(string); ok {
		return text
	}
	return ""
}

// ── Element converters ────────────────────────────────────────────────────────

func (c *cardConverter) convertPlainText(prop cardObj) string {
	content, _ := prop["content"].(string)
	if content == "" {
		return ""
	}
	return c.applyTextStyle(content, prop)
}

func (c *cardConverter) convertMarkdown(prop cardObj) string {
	if elements, ok := prop["elements"].([]interface{}); ok && len(elements) > 0 {
		return c.convertMarkdownElements(elements)
	}
	if content, ok := prop["content"].(string); ok {
		return content
	}
	return ""
}

func (c *cardConverter) convertMarkdownV1(elem, prop cardObj) string {
	if elements, ok := prop["elements"].([]interface{}); ok && len(elements) > 0 {
		return c.convertMarkdownElements(elements)
	}
	if fallback, ok := elem["fallback"].(cardObj); ok {
		return c.convertElement(fallback, 0)
	}
	if content, ok := prop["content"].(string); ok {
		return content
	}
	return ""
}

func (c *cardConverter) convertMarkdownElements(elements []interface{}) string {
	var parts []string
	for _, el := range elements {
		elem, ok := el.(cardObj)
		if !ok {
			continue
		}
		if result := c.convertElement(elem, 0); result != "" {
			parts = append(parts, result)
		}
	}
	return strings.Join(parts, "")
}

func (c *cardConverter) convertDiv(prop cardObj, _ string) string {
	var results []string

	if textElem, ok := prop["text"].(cardObj); ok {
		if text := c.convertElement(textElem, 0); text != "" {
			textProp := c.extractProperty(textElem)
			if textStyle, ok := textProp["textStyle"].(cardObj); ok {
				if size, _ := textStyle["size"].(string); size == "notation" {
					text = "📝 " + text
				}
			}
			results = append(results, text)
		}
	}

	if fields, ok := prop["fields"].([]interface{}); ok {
		var fieldTexts []string
		for _, field := range fields {
			fm, ok := field.(cardObj)
			if !ok {
				continue
			}
			if te, ok := fm["text"].(cardObj); ok {
				if ft := c.convertElement(te, 0); ft != "" {
					fieldTexts = append(fieldTexts, ft)
				}
			}
		}
		if len(fieldTexts) > 0 {
			results = append(results, strings.Join(fieldTexts, "\n"))
		}
	}

	if extraElem, ok := prop["extra"].(cardObj); ok {
		if extra := c.convertElement(extraElem, 0); extra != "" {
			results = append(results, extra)
		}
	}

	return strings.Join(results, "\n")
}

func (c *cardConverter) convertNote(prop cardObj) string {
	elements, _ := prop["elements"].([]interface{})
	if len(elements) == 0 {
		return ""
	}
	var texts []string
	for _, el := range elements {
		elem, ok := el.(cardObj)
		if !ok {
			continue
		}
		if text := c.convertElement(elem, 0); text != "" {
			texts = append(texts, text)
		}
	}
	if len(texts) == 0 {
		return ""
	}
	return "📝 " + strings.Join(texts, " ")
}

func (c *cardConverter) convertLink(prop cardObj) string {
	content, _ := prop["content"].(string)
	if content == "" {
		content = "Link"
	}
	urlStr := ""
	if urlObj, ok := prop["url"].(cardObj); ok {
		urlStr, _ = urlObj["url"].(string)
	}
	if urlStr != "" {
		return fmt.Sprintf("[%s](%s)", escapeMDLinkText(content), urlStr)
	}
	return content
}

func (c *cardConverter) convertEmoji(prop cardObj) string {
	key, _ := prop["key"].(string)
	if emoji, ok := cardEmojiMap[key]; ok {
		return emoji
	}
	return ":" + key + ":"
}

func (c *cardConverter) convertLocalDatetime(prop cardObj) string {
	var ms string
	switch v := prop["milliseconds"].(type) {
	case string:
		ms = v
	case float64:
		ms = strconv.FormatInt(int64(v), 10)
	}
	if ms != "" {
		if formatted := cardFormatMillisToISO8601(ms); formatted != "" {
			return formatted
		}
	}
	fallback, _ := prop["fallbackText"].(string)
	return fallback
}

func (c *cardConverter) convertList(prop cardObj) string {
	items, _ := prop["items"].([]interface{})
	if len(items) == 0 {
		return ""
	}
	var lines []string
	for _, item := range items {
		im, ok := item.(cardObj)
		if !ok {
			continue
		}
		level := 0
		if l, ok := im["level"].(float64); ok {
			level = int(l)
		}
		listType, _ := im["type"].(string)
		order := 0
		if o, ok := im["order"].(float64); ok {
			order = int(math.Floor(float64(o)))
		}
		indent := strings.Repeat("  ", level)
		marker := "-"
		if listType == "ol" {
			marker = fmt.Sprintf("%d.", order)
		}
		if elements, ok := im["elements"].([]interface{}); ok {
			content := c.convertMarkdownElements(elements)
			lines = append(lines, fmt.Sprintf("%s%s %s", indent, marker, content))
		}
	}
	return strings.Join(lines, "\n")
}

func (c *cardConverter) convertBlockquote(prop cardObj) string {
	content := ""
	if s, ok := prop["content"].(string); ok {
		content = s
	} else if elements, ok := prop["elements"].([]interface{}); ok {
		content = c.convertMarkdownElements(elements)
	}
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = "> " + line
	}
	return strings.Join(lines, "\n")
}

func (c *cardConverter) convertCodeBlock(prop cardObj) string {
	language, _ := prop["language"].(string)
	if language == "" {
		language = "plaintext"
	}
	var code strings.Builder
	if contents, ok := prop["contents"].([]interface{}); ok {
		for _, line := range contents {
			lm, ok := line.(cardObj)
			if !ok {
				continue
			}
			if lineContents, ok := lm["contents"].([]interface{}); ok {
				for _, lc := range lineContents {
					cm, ok := lc.(cardObj)
					if !ok {
						continue
					}
					if s, ok := cm["content"].(string); ok {
						code.WriteString(s)
					}
				}
			}
		}
	}
	return fmt.Sprintf("```%s\n%s```", language, code.String())
}

func (c *cardConverter) convertCodeSpan(prop cardObj) string {
	content, _ := prop["content"].(string)
	return "`" + content + "`"
}

func (c *cardConverter) convertHeading(prop cardObj) string {
	level := 1
	if l, ok := prop["level"].(float64); ok {
		level = int(l)
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
	}
	content := ""
	if s, ok := prop["content"].(string); ok {
		content = s
	} else if elements, ok := prop["elements"].([]interface{}); ok {
		content = c.convertMarkdownElements(elements)
	}
	return strings.Repeat("#", level) + " " + content
}

func (c *cardConverter) convertFallbackText(prop cardObj) string {
	if textElem, ok := prop["text"].(cardObj); ok {
		return c.extractTextContent(textElem)
	}
	if elements, ok := prop["elements"].([]interface{}); ok {
		return c.convertMarkdownElements(elements)
	}
	return ""
}

func (c *cardConverter) convertTextTag(prop cardObj) string {
	textElem := prop["text"]
	text := c.extractTextContent(textElem)
	if text == "" {
		return ""
	}
	return "「" + text + "」"
}

func (c *cardConverter) convertNumberTag(prop cardObj) string {
	textElem := prop["text"]
	text := c.extractTextContent(textElem)
	if text == "" {
		return ""
	}
	if urlObj, ok := prop["url"].(cardObj); ok {
		if urlStr, ok := urlObj["url"].(string); ok && urlStr != "" {
			return fmt.Sprintf("[%s](%s)", escapeMDLinkText(text), urlStr)
		}
	}
	return text
}

func (c *cardConverter) convertUnknown(prop cardObj, tag string) string {
	if prop != nil {
		for _, path := range []string{"content", "text", "title", "label", "placeholder"} {
			if v, ok := prop[path]; ok {
				text := c.extractTextContent(v)
				if text != "" {
					return text
				}
			}
		}
		if elements, ok := prop["elements"].([]interface{}); ok && len(elements) > 0 {
			return c.convertElements(elements, 0)
		}
	}
	if c.mode == cardModeDetailed {
		return fmt.Sprintf("[Unknown content](tag:%s)", tag)
	}
	return "[Unknown content]"
}

func (c *cardConverter) convertColumnSet(prop cardObj, depth int) string {
	columns, _ := prop["columns"].([]interface{})
	if len(columns) == 0 {
		return ""
	}
	var results []string
	for _, col := range columns {
		elem, ok := col.(cardObj)
		if !ok {
			continue
		}
		if result := c.convertElement(elem, depth+1); result != "" {
			results = append(results, result)
		}
	}
	sep := "\n\n"
	if allColumnsAreButtons(results) {
		sep = " "
	}
	return strings.Join(results, sep)
}

// allColumnsAreButtons reports whether every result looks like a button token
// (e.g. "[Text]", "[Text](url)", "[Text ✗]"). Used to decide whether
// column_set columns should be space-joined (button row) or newline-joined.
func allColumnsAreButtons(results []string) bool {
	if len(results) == 0 {
		return false
	}
	for _, r := range results {
		if !strings.HasPrefix(r, "[") || strings.Contains(r, "\n") {
			return false
		}
	}
	return true
}

func (c *cardConverter) convertColumn(prop cardObj, depth int) string {
	elements, _ := prop["elements"].([]interface{})
	if len(elements) == 0 {
		return ""
	}
	return c.convertElements(elements, depth)
}

func (c *cardConverter) convertForm(prop cardObj, _ string) string {
	var sb strings.Builder
	sb.WriteString("<form>\n")
	if elements, ok := prop["elements"].([]interface{}); ok {
		sb.WriteString(c.convertElements(elements, 0))
	}
	sb.WriteString("\n</form>")
	return sb.String()
}

func (c *cardConverter) convertCollapsiblePanel(prop cardObj, _ string) string {
	expanded, _ := prop["expanded"].(bool)
	title := "Details"
	if header, ok := prop["header"].(cardObj); ok {
		if titleElem, ok := header["title"]; ok {
			if t := c.extractTextContent(titleElem); t != "" {
				title = t
			}
		}
	}

	indicator := "▶"
	if expanded {
		indicator = "▼"
	}
	var sb strings.Builder
	sb.WriteString(indicator + " " + title + "\n")
	if elements, ok := prop["elements"].([]interface{}); ok {
		content := c.convertElements(elements, 1)
		for _, line := range strings.Split(content, "\n") {
			if line != "" {
				sb.WriteString("    " + line + "\n")
			}
		}
	}
	sb.WriteString("▲")
	return sb.String()
}

func (c *cardConverter) convertInteractiveContainer(prop cardObj, id string) string {
	urlStr := ""
	if actions, ok := prop["actions"].([]interface{}); ok && len(actions) > 0 {
		if action, ok := actions[0].(cardObj); ok {
			if actionType, _ := action["type"].(string); actionType == "open_url" {
				if actionData, ok := action["action"].(cardObj); ok {
					urlStr, _ = actionData["url"].(string)
				}
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("<clickable")
	if urlStr != "" {
		sb.WriteString(fmt.Sprintf(" url=\"%s\"", cardEscapeAttr(urlStr)))
	}
	if c.mode == cardModeDetailed && id != "" {
		sb.WriteString(fmt.Sprintf(" id=\"%s\"", id))
	}
	sb.WriteString(">\n")
	if elements, ok := prop["elements"].([]interface{}); ok {
		sb.WriteString(c.convertElements(elements, 0))
	}
	sb.WriteString("\n</clickable>")
	return sb.String()
}

func (c *cardConverter) convertRepeat(prop cardObj) string {
	if elements, ok := prop["elements"].([]interface{}); ok {
		return c.convertElements(elements, 0)
	}
	return ""
}

func (c *cardConverter) convertButton(prop cardObj, _ string) string {
	buttonText := ""
	if textElem, ok := prop["text"].(cardObj); ok {
		buttonText = c.extractTextContent(textElem)
	}
	if buttonText == "" {
		buttonText = "Button"
	}

	disabled, _ := prop["disabled"].(bool)
	if disabled {
		result := fmt.Sprintf("[%s ✗]", buttonText)
		if tips, ok := prop["disabledTips"].(cardObj); ok {
			if tipsText := c.extractTextContent(tips); tipsText != "" {
				result += fmt.Sprintf("(tips:\"%s\")", tipsText)
			}
		}
		return result
	}

	result := fmt.Sprintf("[%s]", buttonText)
	if actions, ok := prop["actions"].([]interface{}); ok {
		for _, action := range actions {
			am, ok := action.(cardObj)
			if !ok {
				continue
			}
			if am["type"] == "open_url" {
				if ad, ok := am["action"].(cardObj); ok {
					if urlStr, ok := ad["url"].(string); ok && urlStr != "" {
						result = fmt.Sprintf("[%s](%s)", escapeMDLinkText(buttonText), urlStr)
						break
					}
				}
			}
		}
	}

	if confirmObj, ok := prop["confirm"].(cardObj); ok {
		var parts []string
		if titleElem, ok := confirmObj["title"]; ok {
			if t := c.extractTextContent(titleElem); t != "" {
				parts = append(parts, t)
			}
		}
		if textElem, ok := confirmObj["text"]; ok {
			if t := c.extractTextContent(textElem); t != "" {
				parts = append(parts, t)
			}
		}
		if len(parts) > 0 {
			result += fmt.Sprintf("(confirm:\"%s\")", strings.Join(parts, ": "))
		}
	}

	return result
}

func (c *cardConverter) convertActions(prop cardObj) string {
	actions, _ := prop["actions"].([]interface{})
	if len(actions) == 0 {
		return ""
	}
	var results []string
	for _, action := range actions {
		elem, ok := action.(cardObj)
		if !ok {
			continue
		}
		if result := c.convertElement(elem, 0); result != "" {
			results = append(results, result)
		}
	}
	return strings.Join(results, " ")
}

func (c *cardConverter) convertOverflow(prop cardObj) string {
	options, _ := prop["options"].([]interface{})
	if len(options) == 0 {
		return ""
	}
	var optTexts []string
	for _, opt := range options {
		om, ok := opt.(cardObj)
		if !ok {
			continue
		}
		text := ""
		if textElem, ok := om["text"].(cardObj); ok {
			text = c.extractTextContent(textElem)
		}
		if text == "" {
			continue
		}
		urlStr := ""
		if actions, ok := om["actions"].([]interface{}); ok {
			for _, a := range actions {
				am, ok := a.(cardObj)
				if !ok {
					continue
				}
				if am["type"] == "open_url" {
					if ad, ok := am["action"].(cardObj); ok {
						urlStr, _ = ad["url"].(string)
					}
				}
			}
		}
		if urlStr != "" {
			text = fmt.Sprintf("[%s](%s)", escapeMDLinkText(text), urlStr)
		} else if value, _ := om["value"].(string); value != "" {
			text += "(" + value + ")"
		}
		optTexts = append(optTexts, text)
	}
	return "⋮ " + strings.Join(optTexts, ", ")
}

func (c *cardConverter) convertSelect(prop cardObj, id string, isMulti bool) string {
	options, _ := prop["options"].([]interface{})

	selectedValues := map[string]bool{}
	if isMulti {
		if vals, ok := prop["selectedValues"].([]interface{}); ok {
			for _, v := range vals {
				if s, ok := v.(string); ok {
					selectedValues[s] = true
				}
			}
		}
	} else {
		if init, ok := prop["initialOption"].(string); ok {
			selectedValues[init] = true
		}
		if idx, ok := prop["initialIndex"].(float64); ok {
			i := int(idx)
			if i >= 0 && i < len(options) {
				if opt, ok := options[i].(cardObj); ok {
					if val, ok := opt["value"].(string); ok {
						selectedValues[val] = true
					}
				}
			}
		}
	}

	var optionTexts []string
	hasSelected := false
	for _, opt := range options {
		om, ok := opt.(cardObj)
		if !ok {
			continue
		}
		value, _ := om["value"].(string)
		optText := ""
		if textElem, ok := om["text"].(cardObj); ok {
			optText = c.extractTextContent(textElem)
		}
		if optText == "" {
			optText = c.lookupOptionUserName(value)
		}
		if optText == "" {
			optText = value
		}
		if optText == "" {
			continue
		}
		if selectedValues[value] {
			optText = "✓" + optText
			hasSelected = true
		}
		optionTexts = append(optionTexts, optText)
	}

	if len(optionTexts) == 0 {
		placeholder := "Please select"
		if phElem, ok := prop["placeholder"].(cardObj); ok {
			if ph := c.extractTextContent(phElem); ph != "" {
				placeholder = ph
			}
		}
		optionTexts = append(optionTexts, placeholder+" ▼")
	} else if !hasSelected {
		optionTexts[len(optionTexts)-1] += " ▼"
	}

	result := "{" + strings.Join(optionTexts, " / ") + "}"
	var attrs []string
	if isMulti {
		attrs = append(attrs, "multi")
	}
	if c.mode == cardModeDetailed && strings.Contains(id, "person") {
		attrs = append(attrs, "type:person")
	}
	if len(attrs) > 0 {
		result += "(" + strings.Join(attrs, " ") + ")"
	}
	return result
}

func (c *cardConverter) convertSelectImg(prop cardObj, _ string) string {
	options, _ := prop["options"].([]interface{})
	if len(options) == 0 {
		return ""
	}
	selectedValues := map[string]bool{}
	if vals, ok := prop["selectedValues"].([]interface{}); ok {
		for _, v := range vals {
			if s, ok := v.(string); ok {
				selectedValues[s] = true
			}
		}
	}
	var optTexts []string
	for i, opt := range options {
		om, ok := opt.(cardObj)
		if !ok {
			continue
		}
		value, _ := om["value"].(string)
		text := fmt.Sprintf("🖼️ Image %d", i+1)
		if value != "" {
			text += "(" + value + ")"
		}
		if imageID, ok := om["imageID"].(string); ok && imageID != "" {
			originKey, imgToken := c.getImageKeyAndToken(imageID)
			if originKey != "" {
				text += "(img_key:" + originKey + ")"
			} else if imgToken != "" {
				text += "(img_token:" + imgToken + ")"
			}
		}
		if selectedValues[value] {
			text = "✓" + text
		}
		optTexts = append(optTexts, text)
	}
	return "{" + strings.Join(optTexts, " / ") + "}"
}

func (c *cardConverter) convertInput(prop cardObj, _ string) string {
	label := ""
	if labelElem, ok := prop["label"].(cardObj); ok {
		label = c.extractTextContent(labelElem)
	}

	defaultValue, _ := prop["defaultValue"].(string)
	placeholder := ""
	if phElem, ok := prop["placeholder"].(cardObj); ok {
		placeholder = c.extractTextContent(phElem)
	}

	var result string
	switch {
	case defaultValue != "":
		result = defaultValue + "___"
	case placeholder != "":
		result = placeholder + "_____"
	default:
		result = "_____"
	}

	if label != "" {
		result = label + ": " + result
	}

	if inputType, _ := prop["inputType"].(string); inputType == "multiline_text" {
		result = strings.ReplaceAll(result, "_____", "...")
	}
	return result
}

func (c *cardConverter) convertDatePicker(prop cardObj, _ string, pickerType string) string {
	var emoji, value string
	switch pickerType {
	case "date":
		emoji = "📅"
		value, _ = prop["initialDate"].(string)
	case "time":
		emoji = "🕐"
		value, _ = prop["initialTime"].(string)
	case "datetime":
		emoji = "📅"
		value, _ = prop["initialDatetime"].(string)
	default:
		emoji = "📅"
	}

	if value != "" {
		value = cardNormalizeTimeFormat(value)
	}
	if value == "" {
		placeholder := "Select"
		if phElem, ok := prop["placeholder"].(cardObj); ok {
			if ph := c.extractTextContent(phElem); ph != "" {
				placeholder = ph
			}
		}
		value = placeholder
	}
	return emoji + " " + value
}

func (c *cardConverter) convertChecker(prop cardObj, id string) string {
	checked, _ := prop["checked"].(bool)
	checkMark := "[ ]"
	if checked {
		checkMark = "[x]"
	}
	text := ""
	if textElem, ok := prop["text"].(cardObj); ok {
		text = c.extractTextContent(textElem)
	}
	result := checkMark + " " + text
	if c.mode == cardModeDetailed && id != "" {
		result += "(id:" + id + ")"
	}
	return result
}

func (c *cardConverter) convertImage(prop cardObj, _ string) string {
	alt := "Image"
	if altElem, ok := prop["alt"].(cardObj); ok {
		if altText := c.extractTextContent(altElem); altText != "" {
			alt = altText
		}
	}
	if titleElem, ok := prop["title"].(cardObj); ok {
		if titleText := c.extractTextContent(titleElem); titleText != "" {
			alt = titleText
		}
	}

	result := "🖼️ " + alt
	if imageID, ok := prop["imageID"].(string); ok && imageID != "" {
		originKey, imgToken := c.getImageKeyAndToken(imageID)
		if originKey != "" {
			result += "(img_key:" + originKey + ")"
		} else if imgToken != "" {
			result += "(img_token:" + imgToken + ")"
		} else {
			result += "(img_key:" + imageID + ")"
		}
	}
	return result
}

func (c *cardConverter) convertImgCombination(prop cardObj) string {
	imgList, _ := prop["imgList"].([]interface{})
	if len(imgList) == 0 {
		return ""
	}
	result := fmt.Sprintf("🖼️ %d image(s)", len(imgList))
	var keys []string
	for _, img := range imgList {
		im, ok := img.(cardObj)
		if !ok {
			continue
		}
		if imageID, ok := im["imageID"].(string); ok && imageID != "" {
			originKey, imgToken := c.getImageKeyAndToken(imageID)
			if originKey != "" {
				keys = append(keys, originKey)
			} else if imgToken != "" {
				keys = append(keys, imgToken)
			} else {
				keys = append(keys, imageID)
			}
		}
	}
	if len(keys) > 0 {
		result += "(keys:" + strings.Join(keys, ",") + ")"
	}
	return result
}

func (c *cardConverter) convertChart(prop cardObj, _ string) string {
	title := "Chart"
	chartType := ""

	if chartSpec, ok := prop["chartSpec"].(cardObj); ok {
		if titleObj, ok := chartSpec["title"].(cardObj); ok {
			if text, ok := titleObj["text"].(string); ok && text != "" {
				title = text
			}
		}
		if ct, ok := chartSpec["type"].(string); ok && ct != "" {
			chartType = ct
			if typeName, ok := cardChartTypeNames[ct]; ok {
				if title != "Chart" {
					title += " (" + typeName + ")"
				} else {
					title = typeName
				}
			}
		}
	}

	summary := c.extractChartSummary(prop, chartType)
	result := "📊 " + title
	if summary != "" {
		result += "\nSummary: " + summary
	}
	return result
}

func (c *cardConverter) extractChartSummary(prop cardObj, chartType string) string {
	chartSpec, ok := prop["chartSpec"].(cardObj)
	if !ok {
		return ""
	}

	// VChart spec: data is an array of series objects ([{"id":"...","values":[...]}]).
	// Older/object format: data is a map with a "values" key directly.
	var values []interface{}
	switch d := chartSpec["data"].(type) {
	case cardObj:
		if v, ok := d["values"].([]interface{}); ok {
			values = v
		}
	case []interface{}:
		for _, series := range d {
			if sm, ok := series.(cardObj); ok {
				if v, ok := sm["values"].([]interface{}); ok {
					values = append(values, v...)
				}
			}
		}
	}
	if len(values) == 0 {
		return ""
	}

	switch chartType {
	case "line", "bar", "area":
		xField, _ := chartSpec["xField"].(string)
		yField, _ := chartSpec["yField"].(string)
		if xField == "" || yField == "" {
			return fmt.Sprintf("%d data point(s)", len(values))
		}
		var parts []string
		for _, v := range values {
			vm, ok := v.(cardObj)
			if !ok {
				continue
			}
			parts = append(parts, fmt.Sprintf("%v:%v", vm[xField], vm[yField]))
		}
		if len(parts) > 0 {
			return strings.Join(parts, ", ")
		}
	case "pie":
		catField, _ := chartSpec["categoryField"].(string)
		valField, _ := chartSpec["valueField"].(string)
		if catField == "" || valField == "" {
			return fmt.Sprintf("%d data point(s)", len(values))
		}
		var parts []string
		for _, v := range values {
			vm, ok := v.(cardObj)
			if !ok {
				continue
			}
			parts = append(parts, fmt.Sprintf("%v:%v", vm[catField], vm[valField]))
		}
		if len(parts) > 0 {
			return strings.Join(parts, ", ")
		}
	}
	return fmt.Sprintf("%d data point(s)", len(values))
}

func (c *cardConverter) convertAudio(prop cardObj, _ string) string {
	result := "🎵 Audio"
	fileID, _ := prop["fileID"].(string)
	if fileID == "" {
		fileID, _ = prop["audioID"].(string)
	}
	if fileID != "" {
		result += "(key:" + fileID + ")"
	}
	return result
}

func (c *cardConverter) convertVideo(prop cardObj, _ string) string {
	result := "🎬 Video"
	fileID, _ := prop["fileID"].(string)
	if fileID == "" {
		fileID, _ = prop["videoID"].(string)
	}
	if fileID != "" {
		result += "(key:" + fileID + ")"
	}
	return result
}

func (c *cardConverter) convertTable(prop cardObj) string {
	columns, _ := prop["columns"].([]interface{})
	if len(columns) == 0 {
		return ""
	}
	rows, _ := prop["rows"].([]interface{})

	var colNames, colKeys []string
	for _, col := range columns {
		cm, ok := col.(cardObj)
		if !ok {
			continue
		}
		displayName, _ := cm["displayName"].(string)
		name, _ := cm["name"].(string)
		if displayName == "" {
			displayName = name
		}
		colNames = append(colNames, displayName)
		colKeys = append(colKeys, name)
	}

	var lines []string
	lines = append(lines, "| "+strings.Join(colNames, " | ")+" |")
	separator := "|"
	for range colNames {
		separator += "------|"
	}
	lines = append(lines, separator)

	for _, row := range rows {
		rm, ok := row.(cardObj)
		if !ok {
			continue
		}
		var cells []string
		for _, key := range colKeys {
			cellValue := ""
			if cellData, ok := rm[key].(cardObj); ok {
				if cellData["data"] != nil {
					cellValue = c.extractTableCellValue(cellData["data"])
				}
			}
			cells = append(cells, cellValue)
		}
		lines = append(lines, "| "+strings.Join(cells, " | ")+" |")
	}
	return strings.Join(lines, "\n")
}

func (c *cardConverter) extractTableCellValue(data interface{}) string {
	switch v := data.(type) {
	case string:
		// Lark API serialises array-type cell data as a Go-format string like
		// "[map[text:VIP] map[text:Premium]]". Detect and extract text values.
		if texts := goMapArrayTexts(v); len(texts) > 0 {
			return strings.Join(texts, ", ")
		}
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case []interface{}:
		var texts []string
		for _, item := range v {
			im, ok := item.(cardObj)
			if !ok {
				continue
			}
			if text, ok := im["text"].(string); ok {
				texts = append(texts, "「"+text+"」")
			}
		}
		return strings.Join(texts, " ")
	default:
		if m, ok := data.(cardObj); ok {
			return c.extractTextContent(m)
		}
		return ""
	}
}

// goMapNextKey matches the start of the next key in a Go fmt map literal (space + identifier + colon).
var goMapNextKey = regexp.MustCompile(` [a-zA-Z_][a-zA-Z0-9_]*:`)

// goMapArrayTexts extracts "text" values from a Go-format slice-of-maps string,
// e.g. "[map[text:VIP] map[text:Premium]]" → ["VIP", "Premium"].
// Values may contain spaces; they are delimited by the next map key or by "]".
// Returns nil if the string doesn't look like this format.
func goMapArrayTexts(s string) []string {
	if !strings.HasPrefix(s, "[") || !strings.Contains(s, "map[") {
		return nil
	}
	const key = "text:"
	var texts []string
	rest := s
	for {
		idx := strings.Index(rest, key)
		if idx < 0 {
			break
		}
		after := rest[idx+len(key):]
		bracketEnd := strings.Index(after, "]")
		nextKey := goMapNextKey.FindStringIndex(after)
		var end int
		if nextKey != nil && (bracketEnd < 0 || nextKey[0] < bracketEnd) {
			end = nextKey[0]
		} else if bracketEnd >= 0 {
			end = bracketEnd
		} else {
			if after != "" {
				texts = append(texts, after)
			}
			break
		}
		if val := after[:end]; val != "" {
			texts = append(texts, val)
		}
		rest = after[end:]
	}
	return texts
}

func (c *cardConverter) convertPerson(prop cardObj, _ string) string {
	userID, _ := prop["userID"].(string)
	if userID == "" {
		return ""
	}
	personName := c.lookupPersonName(userID)
	if personName == "" {
		if notation, ok := prop["notation"].(cardObj); ok {
			personName = c.extractTextContent(notation)
		}
	}
	if personName != "" {
		if c.mode == cardModeDetailed {
			return fmt.Sprintf("%s(open_id:%s)", personName, userID)
		}
		return personName
	}
	if c.mode == cardModeDetailed {
		return fmt.Sprintf("user(open_id:%s)", userID)
	}
	return userID
}

// convertPersonV1 handles the v1 card schema person element.
// [#20] NOTE: this function duplicates ~20 lines from convertPerson with the only difference
// being the absence of the `notation` fallback block. Ideally it should delegate to
// convertPerson, but doing so would introduce the notation fallback for v1 schema elements
// (subtle behavior change). Not merged to preserve identical output behavior.
func (c *cardConverter) convertPersonV1(prop cardObj, _ string) string {
	userID, _ := prop["userID"].(string)
	if userID == "" {
		return ""
	}
	personName := c.lookupPersonName(userID)
	if personName != "" {
		if c.mode == cardModeDetailed {
			return fmt.Sprintf("%s(open_id:%s)", personName, userID)
		}
		return personName
	}
	if c.mode == cardModeDetailed {
		return fmt.Sprintf("user(open_id:%s)", userID)
	}
	return userID
}

func (c *cardConverter) convertPersonList(prop cardObj) string {
	persons, _ := prop["persons"].([]interface{})
	if len(persons) == 0 {
		return ""
	}
	var names []string
	for _, person := range persons {
		pm, ok := person.(cardObj)
		if !ok {
			continue
		}
		personID, _ := pm["id"].(string)
		personName := c.lookupPersonName(personID)
		if personName != "" {
			if c.mode == cardModeDetailed {
				names = append(names, fmt.Sprintf("%s(open_id:%s)", personName, personID))
			} else {
				names = append(names, personName)
			}
		} else if personID != "" {
			if c.mode == cardModeDetailed {
				names = append(names, fmt.Sprintf("user(id:%s)", personID))
			} else {
				names = append(names, personID)
			}
		} else {
			names = append(names, "user")
		}
	}
	return strings.Join(names, ", ")
}

func (c *cardConverter) convertAvatar(prop cardObj, _ string) string {
	userID, _ := prop["userID"].(string)
	personName := c.lookupPersonName(userID)
	if personName != "" {
		if c.mode == cardModeDetailed {
			return fmt.Sprintf("👤 %s(open_id:%s)", personName, userID)
		}
		return "👤 " + personName
	}
	result := "👤"
	if userID != "" {
		result += "(id:" + userID + ")"
	}
	return result
}

func (c *cardConverter) convertAt(prop cardObj) string {
	userID, _ := prop["userID"].(string)
	if userID == "" {
		return ""
	}
	userName := ""
	actualUserID := ""
	fromMentions := false
	if c.attachment != nil {
		if atUsers, ok := c.attachment["at_users"].(cardObj); ok {
			if userInfo, ok := atUsers[userID].(cardObj); ok {
				userName, _ = userInfo["content"].(string)
				actualUserID, _ = userInfo["user_id"].(string)
				// When the backend populates mention_key (raw_card_content path), use
				// mentions[] for the canonical name and the reading-app open_id, which is
				// more accurate than the origKey-stored user_id in at_users.
				if mentionKey, _ := userInfo["mention_key"].(string); mentionKey != "" {
					if mention, ok := c.mentionsByKey[mentionKey]; ok {
						if name, _ := mention["name"].(string); name != "" {
							userName = name
						}
						if id := extractMentionOpenId(mention["id"]); id != "" {
							actualUserID = id
							fromMentions = true
						}
					}
				}
			}
		}
	}
	if userName != "" {
		if c.mode == cardModeDetailed {
			if actualUserID != "" {
				label := "user_id"
				if fromMentions {
					label = "open_id"
				}
				return fmt.Sprintf("@%s(%s:%s)", userName, label, actualUserID)
			}
			return fmt.Sprintf("@%s(open_id:%s)", userName, userID)
		}
		if fromMentions && actualUserID != "" {
			return fmt.Sprintf("@%s(%s)", userName, actualUserID)
		}
		return fmt.Sprintf("@%s(%s)", userName, userID)
	}
	if c.mode == cardModeDetailed {
		if actualUserID != "" {
			label := "user_id"
			if fromMentions {
				label = "open_id"
			}
			return fmt.Sprintf("@user(%s:%s)", label, actualUserID)
		}
		return fmt.Sprintf("@user(open_id:%s)", userID)
	}
	return "@" + userID
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (c *cardConverter) lookupPersonName(userID string) string {
	if c.attachment == nil {
		return ""
	}
	if persons, ok := c.attachment["persons"].(cardObj); ok {
		if person, ok := persons[userID].(cardObj); ok {
			if content, ok := person["content"].(string); ok {
				return content
			}
		}
	}
	return ""
}

// lookupOptionUserName resolves a user display name from the attachment's option_users map,
// used for person-selector option labels.
func (c *cardConverter) lookupOptionUserName(userID string) string {
	if c.attachment == nil {
		return ""
	}
	if optUsers, ok := c.attachment["option_users"].(cardObj); ok {
		if userInfo, ok := optUsers[userID].(cardObj); ok {
			if content, ok := userInfo["content"].(string); ok {
				return content
			}
		}
	}
	return ""
}

// getImageKeyAndToken returns the origin_key and token for an image ID from the attachment map.
// origin_key takes priority over token as the display-ready image reference.
func (c *cardConverter) getImageKeyAndToken(imageID string) (originKey, token string) {
	if c.attachment == nil {
		return "", ""
	}
	if images, ok := c.attachment["images"].(cardObj); ok {
		if imageInfo, ok := images[imageID].(cardObj); ok {
			originKey, _ = imageInfo["origin_key"].(string)
			token, _ = imageInfo["token"].(string)
		}
	}
	return originKey, token
}

type cardTextStyle struct {
	bold          bool
	italic        bool
	strikethrough bool
}

func (c *cardConverter) extractTextStyle(prop cardObj) cardTextStyle {
	style := cardTextStyle{}
	textStyle, ok := prop["textStyle"].(cardObj)
	if !ok {
		return style
	}
	attrs, _ := textStyle["attributes"].([]interface{})
	for _, attr := range attrs {
		s, ok := attr.(string)
		if !ok {
			continue
		}
		switch s {
		case "bold":
			style.bold = true
		case "italic":
			style.italic = true
		case "strikethrough":
			style.strikethrough = true
		}
	}
	return style
}

func (c *cardConverter) applyTextStyle(content string, prop cardObj) string {
	if content == "" {
		return content
	}
	style := c.extractTextStyle(prop)
	if style.strikethrough {
		content = "~~" + content + "~~"
	}
	if style.italic {
		content = "*" + content + "*"
	}
	if style.bold {
		content = "**" + content + "**"
	}
	return content
}

// ── Utility functions ─────────────────────────────────────────────────────────

func cardEscapeAttr(s string) string {
	return cardAttrEscaper.Replace(s)
}

var cardAttrEscaper = strings.NewReplacer(
	`\`, `\\`,
	`"`, `\"`,
	"\n", `\n`,
	"\r", `\r`,
	"\t", `\t`,
)

func cardFormatMillisToISO8601(ms string) string {
	n, err := strconv.ParseInt(ms, 10, 64)
	if err != nil {
		return ""
	}
	t := time.Unix(n/1000, (n%1000)*int64(time.Millisecond)).UTC()
	return t.Format(time.RFC3339)
}

func cardNormalizeTimeFormat(input string) string {
	if input == "" {
		return ""
	}
	n, err := strconv.ParseInt(input, 10, 64)
	if err == nil {
		if len(input) >= 13 {
			t := time.Unix(n/1000, (n%1000)*int64(time.Millisecond)).UTC()
			return t.Format(time.RFC3339)
		} else if len(input) >= 10 {
			t := time.Unix(n, 0).UTC()
			return t.Format(time.RFC3339)
		}
	}
	// Already ISO8601 or date/time string
	return input
}
