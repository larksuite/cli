// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var atMentionRe = regexp.MustCompile(`<at\b([^>]*)>(?:[^<]*</at>)?`)

// Each attr regex captures the value in group 1 (quoted) or group 2 (unquoted).
var atMentionKeyAttrRe = regexp.MustCompile(`\b(?:mention_key|mentions_key)=(?:"([^"]*)"|([^\s">/]+))`)
var atIDAttrRe = regexp.MustCompile(`\bid=(?:"([^"]+)"|([^\s">/]+))`)
var atIDsAttrRe = regexp.MustCompile(`\bids=(?:"([^"]+)"|([^\s">/]+))`)

// attrVal returns the captured attribute value from a quoted-or-unquoted regex match.
func attrVal(m []string) string {
	if len(m) >= 2 && m[1] != "" {
		return m[1]
	}
	if len(m) >= 3 {
		return m[2]
	}
	return ""
}

// buildMentionAtMap builds a mention key → "@name(ou_xxx)" lookup from the message mentions array.
func buildMentionAtMap(mentions []interface{}) map[string]string {
	if len(mentions) == 0 {
		return nil
	}
	m := make(map[string]string, len(mentions))
	for _, raw := range mentions {
		item, _ := raw.(map[string]interface{})
		key, _ := item["key"].(string)
		name, _ := item["name"].(string)
		openID := extractMentionOpenId(item["id"])
		if key == "" {
			continue
		}
		if name != "" && openID != "" {
			m[key] = fmt.Sprintf("@%s(%s)", name, openID)
		} else if name != "" {
			m[key] = "@" + name
		} else if openID != "" {
			m[key] = "@" + openID
		}
	}
	return m
}

// resolveAtMentions replaces <at ...> tags in markdown content with resolved mention strings.
//
// Single mention:  <at mention_key="k" id="ou_x">  → "@name(ou_x)" or "@ou_x" fallback.
// Multi mention:   <at ids="id1,id2,id3" mentions_key="k1,,k3">  → each pair resolved and
//
//	concatenated: "@name1(id1)@id2@name3(id3)".
func resolveAtMentions(content string, mentionAt map[string]string) string {
	if len(mentionAt) == 0 {
		return content
	}
	return atMentionRe.ReplaceAllStringFunc(content, func(match string) string {
		attrs := atMentionRe.FindStringSubmatch(match)
		if len(attrs) < 2 {
			return match
		}
		attrStr := attrs[1]

		// Multi-mention: ids="id1,id2,id3" or ids=id1,id2,id3 with mentions_key
		if idsMatch := atIDsAttrRe.FindStringSubmatch(attrStr); attrVal(idsMatch) != "" {
			ids := strings.Split(attrVal(idsMatch), ",")
			var keys []string
			if mk := atMentionKeyAttrRe.FindStringSubmatch(attrStr); attrVal(mk) != "" {
				keys = strings.Split(attrVal(mk), ",")
			}
			var sb strings.Builder
			for i, id := range ids {
				key := ""
				if i < len(keys) {
					key = keys[i]
				}
				if key != "" {
					if resolved, ok := mentionAt[key]; ok {
						sb.WriteString(resolved)
						continue
					}
				}
				if id != "" {
					sb.WriteString("@" + id)
				}
			}
			return sb.String()
		}

		// Single mention
		key := attrVal(atMentionKeyAttrRe.FindStringSubmatch(attrStr))
		if key != "" {
			if resolved, ok := mentionAt[key]; ok {
				return resolved
			}
		}
		if id := attrVal(atIDAttrRe.FindStringSubmatch(attrStr)); id != "" {
			return "@" + id
		}
		return match
	})
}

// ConvertInteractiveEventContent extracts user_dsl from an interactive event message.content
// and converts it to human-readable text with the same format as cardConverter.
// mentions is the message-level mentions array used to resolve @at references in markdown content.
func ConvertInteractiveEventContent(rawContent string, mentions []interface{}) string {
	var content cardObj
	if err := json.Unmarshal([]byte(rawContent), &content); err != nil {
		return "[interactive card]"
	}
	userDslStr, ok := content["user_dsl"].(string)
	if !ok || userDslStr == "" {
		return "[interactive card]"
	}
	return convertUserDslCard(userDslStr, buildMentionAtMap(mentions))
}

func convertUserDslCard(raw string, mentionAt map[string]string) string {
	var parsed cardObj
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return "[interactive card]"
	}
	c := &userDslConverter{mentionAt: mentionAt}
	result := c.convert(parsed)
	if result == "" {
		return "[interactive card]"
	}
	return result
}

type userDslConverter struct {
	mentionAt map[string]string
}

func (c *userDslConverter) convert(parsed cardObj) string {
	var header cardObj
	var elements []interface{}

	if _, hasSchema := parsed["schema"]; hasSchema {
		// user-2.ts format: header at root, body.elements
		header, _ = parsed["header"].(cardObj)
		if body, ok := parsed["body"].(cardObj); ok {
			elements, _ = body["elements"].([]interface{})
		}
	} else {
		// user-1.ts format: i18n_header.zh_cn or direct header, elements at root
		if i18nHeader, ok := parsed["i18n_header"].(cardObj); ok {
			header, _ = i18nHeader["zh_cn"].(cardObj)
		} else if h, ok := parsed["header"].(cardObj); ok {
			header = h
		}
		elements, _ = parsed["elements"].([]interface{})
	}

	title := c.extractHeaderTitle(header)
	subtitle := c.extractHeaderSubtitle(header)

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

	if len(elements) > 0 {
		body := c.convertElements(elements, 0)
		if body != "" {
			sb.WriteString(body)
			sb.WriteString("\n")
		}
	}
	sb.WriteString("</card>")
	result := sb.String()
	if result == "<card>\n</card>" {
		return "[interactive card]"
	}
	return result
}

func (c *userDslConverter) extractHeaderTitle(header cardObj) string {
	if header == nil {
		return ""
	}
	if titleElem, ok := header["title"].(cardObj); ok {
		return c.extractTextContent(titleElem)
	}
	return ""
}

func (c *userDslConverter) extractHeaderSubtitle(header cardObj) string {
	if header == nil {
		return ""
	}
	if subtitleElem, ok := header["subtitle"].(cardObj); ok {
		return c.extractTextContent(subtitleElem)
	}
	return ""
}

func (c *userDslConverter) extractTextContent(elem cardObj) string {
	if elem == nil {
		return ""
	}
	var content string
	if s, ok := elem["content"].(string); ok {
		content = s
	} else if i18n, ok := elem["i18n"].(cardObj); ok {
		for _, lang := range []string{"zh_cn", "en_us", "ja_jp"} {
			if t, ok := i18n[lang].(string); ok && t != "" {
				content = t
				break
			}
		}
	}
	if tag, _ := elem["tag"].(string); tag == "lark_md" {
		return resolveAtMentions(content, c.mentionAt)
	}
	return content
}

func (c *userDslConverter) convertElements(elements []interface{}, depth int) string {
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

func (c *userDslConverter) convertElement(elem cardObj, depth int) string {
	tag, _ := elem["tag"].(string)
	switch tag {
	case "plain_text", "lark_md":
		return c.extractTextContent(elem)
	case "markdown":
		content, _ := elem["content"].(string)
		return resolveAtMentions(content, c.mentionAt)
	case "div":
		return c.convertDiv(elem)
	case "note":
		return c.convertNote(elem)
	case "hr":
		return "---"
	case "br":
		return "\n"
	case "img", "image":
		return c.convertImage(elem)
	case "img_combination":
		return c.convertImgCombination(elem)
	case "column_set":
		return c.convertColumnSet(elem, depth)
	case "column":
		return c.convertColumn(elem, depth)
	case "button":
		return c.convertButton(elem)
	case "actions", "action":
		return c.convertActions(elem)
	case "overflow":
		return c.convertOverflow(elem)
	case "select_static", "select_person":
		return c.convertSelect(elem, false)
	case "multi_select_static", "multi_select_person":
		return c.convertSelect(elem, true)
	case "input":
		return c.convertInput(elem)
	case "date_picker", "picker_date":
		return c.convertDatePicker(elem, "date")
	case "picker_time":
		return c.convertDatePicker(elem, "time")
	case "picker_datetime":
		return c.convertDatePicker(elem, "datetime")
	case "checker":
		return c.convertChecker(elem)
	case "table":
		return c.convertTable(elem)
	case "chart":
		return c.convertChart(elem)
	case "form":
		return c.convertForm(elem)
	case "collapsible_panel":
		return c.convertCollapsiblePanel(elem)
	case "interactive_container":
		return c.convertInteractiveContainer(elem)
	case "person":
		return c.convertPerson(elem)
	case "person_list":
		return c.convertPersonList(elem)
	case "text_tag":
		return c.convertTextTag(elem)
	case "avatar":
		return c.convertAvatar(elem)
	case "select_img":
		return c.convertSelectImg(elem)
	case "repeat":
		return c.convertRepeat(elem)
	case "audio":
		return c.convertAudio(elem)
	case "video":
		return c.convertVideo(elem)
	case "custom_icon", "standard_icon":
		return ""
	case "fallback_text":
		if textElem, ok := elem["text"].(cardObj); ok {
			return c.extractTextContent(textElem)
		}
		return ""
	default:
		if content, ok := elem["content"].(string); ok && content != "" {
			return content
		}
		if textElem, ok := elem["text"].(cardObj); ok {
			return c.extractTextContent(textElem)
		}
		if elems, ok := elem["elements"].([]interface{}); ok && len(elems) > 0 {
			return c.convertElements(elems, depth)
		}
		return ""
	}
}

func (c *userDslConverter) convertDiv(elem cardObj) string {
	var results []string
	if textElem, ok := elem["text"].(cardObj); ok {
		if text := c.convertElement(textElem, 0); text != "" {
			if size, _ := textElem["text_size"].(string); size == "notation" {
				text = "📝 " + text
			}
			results = append(results, text)
		}
	}
	if fields, ok := elem["fields"].([]interface{}); ok {
		var fieldTexts []string
		for _, field := range fields {
			fm, ok := field.(cardObj)
			if !ok {
				continue
			}
			if te, ok := fm["text"].(cardObj); ok {
				if ft := c.extractTextContent(te); ft != "" {
					fieldTexts = append(fieldTexts, ft)
				}
			}
		}
		if len(fieldTexts) > 0 {
			results = append(results, strings.Join(fieldTexts, "\n"))
		}
	}
	if extraElem, ok := elem["extra"].(cardObj); ok {
		if extra := c.convertElement(extraElem, 0); extra != "" {
			results = append(results, extra)
		}
	}
	return strings.Join(results, "\n")
}

func (c *userDslConverter) convertNote(elem cardObj) string {
	elements, _ := elem["elements"].([]interface{})
	if len(elements) == 0 {
		return ""
	}
	var texts []string
	for _, el := range elements {
		e, ok := el.(cardObj)
		if !ok {
			continue
		}
		if text := c.convertElement(e, 0); text != "" {
			texts = append(texts, text)
		}
	}
	if len(texts) == 0 {
		return ""
	}
	return "📝 " + strings.Join(texts, " ")
}

func (c *userDslConverter) convertImage(elem cardObj) string {
	alt := "Image"
	if altElem, ok := elem["alt"].(cardObj); ok {
		if altText := c.extractTextContent(altElem); altText != "" {
			alt = altText
		}
	}
	if titleElem, ok := elem["title"].(cardObj); ok {
		if titleText := c.extractTextContent(titleElem); titleText != "" {
			alt = titleText
		}
	}
	result := "🖼️ " + alt
	if imgKey, ok := elem["img_key"].(string); ok && imgKey != "" {
		result += "(img_key:" + imgKey + ")"
	}
	return result
}

func (c *userDslConverter) convertImgCombination(elem cardObj) string {
	imgList, _ := elem["img_list"].([]interface{})
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
		if imgKey, ok := im["img_key"].(string); ok && imgKey != "" {
			keys = append(keys, imgKey)
		}
	}
	if len(keys) > 0 {
		result += "(keys:" + strings.Join(keys, ",") + ")"
	}
	return result
}

func (c *userDslConverter) convertColumnSet(elem cardObj, depth int) string {
	columns, _ := elem["columns"].([]interface{})
	if len(columns) == 0 {
		return ""
	}
	var results []string
	for _, col := range columns {
		colElem, ok := col.(cardObj)
		if !ok {
			continue
		}
		if result := c.convertElement(colElem, depth+1); result != "" {
			results = append(results, result)
		}
	}
	sep := "\n\n"
	if allColumnsAreButtons(results) {
		sep = " "
	}
	return strings.Join(results, sep)
}

func (c *userDslConverter) convertColumn(elem cardObj, depth int) string {
	elements, _ := elem["elements"].([]interface{})
	if len(elements) == 0 {
		return ""
	}
	return c.convertElements(elements, depth)
}

func (c *userDslConverter) extractButtonURL(elem cardObj) string {
	if urlStr, ok := elem["url"].(string); ok && urlStr != "" {
		return urlStr
	}
	if multiURL, ok := elem["multi_url"].(cardObj); ok {
		if urlStr, ok := multiURL["url"].(string); ok && urlStr != "" {
			return urlStr
		}
	}
	if behaviors, ok := elem["behaviors"].([]interface{}); ok {
		for _, b := range behaviors {
			bm, ok := b.(cardObj)
			if !ok {
				continue
			}
			if bm["type"] == "open_url" {
				if urlStr, ok := bm["default_url"].(string); ok && urlStr != "" {
					return urlStr
				}
			}
		}
	}
	return ""
}

func (c *userDslConverter) convertButton(elem cardObj) string {
	buttonText := ""
	if textElem, ok := elem["text"].(cardObj); ok {
		buttonText = c.extractTextContent(textElem)
	}
	if buttonText == "" {
		buttonText = "Button"
	}
	disabled, _ := elem["disabled"].(bool)
	if disabled {
		result := fmt.Sprintf("[%s ✗]", buttonText)
		if tipsElem, ok := elem["disabled_tips"].(cardObj); ok {
			if tipsText := c.extractTextContent(tipsElem); tipsText != "" {
				result += fmt.Sprintf("(tips:\"%s\")", tipsText)
			}
		}
		return result
	}
	result := fmt.Sprintf("[%s]", buttonText)
	if urlStr := c.extractButtonURL(elem); urlStr != "" {
		result = fmt.Sprintf("[%s](%s)", escapeMDLinkText(buttonText), urlStr)
	}
	if confirmObj, ok := elem["confirm"].(cardObj); ok {
		var parts []string
		if titleElem, ok := confirmObj["title"].(cardObj); ok {
			if t := c.extractTextContent(titleElem); t != "" {
				parts = append(parts, t)
			}
		}
		if textElem, ok := confirmObj["text"].(cardObj); ok {
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

func (c *userDslConverter) convertActions(elem cardObj) string {
	actions, _ := elem["actions"].([]interface{})
	if len(actions) == 0 {
		return ""
	}
	var results []string
	for _, action := range actions {
		ae, ok := action.(cardObj)
		if !ok {
			continue
		}
		if result := c.convertElement(ae, 0); result != "" {
			results = append(results, result)
		}
	}
	return strings.Join(results, " ")
}

func (c *userDslConverter) convertOverflow(elem cardObj) string {
	options, _ := elem["options"].([]interface{})
	if len(options) == 0 {
		return ""
	}
	var optTexts []string
	for _, opt := range options {
		om, ok := opt.(cardObj)
		if !ok {
			continue
		}
		if textElem, ok := om["text"].(cardObj); ok {
			if text := c.extractTextContent(textElem); text != "" {
				urlStr := ""
				if u, ok := om["url"].(string); ok && u != "" {
					urlStr = u
				} else if multiURL, ok := om["multi_url"].(cardObj); ok {
					urlStr, _ = multiURL["url"].(string)
				}
				if urlStr != "" {
					text = fmt.Sprintf("[%s](%s)", escapeMDLinkText(text), urlStr)
				} else if value, _ := om["value"].(string); value != "" {
					text += "(" + value + ")"
				}
				optTexts = append(optTexts, text)
			}
		}
	}
	return "⋮ " + strings.Join(optTexts, ", ")
}

func (c *userDslConverter) convertSelect(elem cardObj, isMulti bool) string {
	options, _ := elem["options"].([]interface{})
	tag, _ := elem["tag"].(string)
	isPerson := tag == "select_person" || tag == "multi_select_person"

	selectedValues := map[string]bool{}
	var selectedOrder []string // preserve order for synthetic person entries
	if isMulti {
		if vals, ok := elem["selected_values"].([]interface{}); ok {
			for _, v := range vals {
				if s, ok := v.(string); ok {
					selectedValues[s] = true
					selectedOrder = append(selectedOrder, s)
				}
			}
		}
	} else {
		if init, ok := elem["initial_option"].(string); ok {
			selectedValues[init] = true
			selectedOrder = append(selectedOrder, init)
		}
		if idx, ok := elem["initial_index"].(float64); ok {
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

	// Person selectors have no static options in the DSL; synthesize from selected IDs.
	if isPerson && len(options) == 0 && len(selectedOrder) > 0 {
		for _, id := range selectedOrder {
			optionTexts = append(optionTexts, "✓"+id)
		}
		hasSelected = true
	}

	if len(optionTexts) == 0 {
		placeholder := "Please select"
		if phElem, ok := elem["placeholder"].(cardObj); ok {
			if ph := c.extractTextContent(phElem); ph != "" {
				placeholder = ph
			}
		}
		optionTexts = append(optionTexts, placeholder+" ▼")
	} else if !hasSelected {
		optionTexts[len(optionTexts)-1] += " ▼"
	}
	result := "{" + strings.Join(optionTexts, " / ") + "}"
	if isMulti {
		result += "(multi)"
	}
	return result
}

func (c *userDslConverter) convertInput(elem cardObj) string {
	label := ""
	if labelElem, ok := elem["label"].(cardObj); ok {
		label = c.extractTextContent(labelElem)
	}
	defaultValue, _ := elem["default_value"].(string)
	placeholder := ""
	if phElem, ok := elem["placeholder"].(cardObj); ok {
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
	if inputType, _ := elem["input_type"].(string); inputType == "multiline_text" {
		result = strings.ReplaceAll(result, "_____", "...")
	}
	return result
}

func (c *userDslConverter) convertDatePicker(elem cardObj, pickerType string) string {
	var emoji, value string
	switch pickerType {
	case "date":
		emoji = "📅"
		value, _ = elem["initial_date"].(string)
	case "time":
		emoji = "🕐"
		value, _ = elem["initial_time"].(string)
	case "datetime":
		emoji = "📅"
		value, _ = elem["initial_datetime"].(string)
	default:
		emoji = "📅"
	}
	if value != "" {
		value = cardNormalizeTimeFormat(value)
	}
	if value == "" {
		placeholder := "Select"
		if phElem, ok := elem["placeholder"].(cardObj); ok {
			if ph := c.extractTextContent(phElem); ph != "" {
				placeholder = ph
			}
		}
		value = placeholder
	}
	return emoji + " " + value
}

func (c *userDslConverter) convertChecker(elem cardObj) string {
	checked, _ := elem["checked"].(bool)
	checkMark := "[ ]"
	if checked {
		checkMark = "[x]"
	}
	text := ""
	if textElem, ok := elem["text"].(cardObj); ok {
		text = c.extractTextContent(textElem)
	}
	return checkMark + " " + text
}

func (c *userDslConverter) convertTable(elem cardObj) string {
	columns, _ := elem["columns"].([]interface{})
	if len(columns) == 0 {
		return ""
	}
	rows, _ := elem["rows"].([]interface{})

	var colNames, colKeys []string
	for _, col := range columns {
		cm, ok := col.(cardObj)
		if !ok {
			continue
		}
		displayName, _ := cm["display_name"].(string)
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
			cells = append(cells, c.extractUserDslTableCellValue(rm[key]))
		}
		lines = append(lines, "| "+strings.Join(cells, " | ")+" |")
	}
	return strings.Join(lines, "\n")
}

func (c *userDslConverter) extractUserDslTableCellValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case []interface{}:
		var texts []string
		for _, item := range val {
			im, ok := item.(cardObj)
			if !ok {
				continue
			}
			if text, ok := im["text"].(string); ok && text != "" {
				texts = append(texts, "「"+text+"」")
			}
		}
		return strings.Join(texts, " ")
	default:
		if m, ok := v.(cardObj); ok {
			if content, ok := m["content"].(string); ok {
				return content
			}
		}
		return fmt.Sprintf("%v", v)
	}
}

func (c *userDslConverter) convertChart(elem cardObj) string {
	chartSpec, ok := elem["chart_spec"].(cardObj)
	if !ok {
		return "📊 Chart"
	}
	title := "Chart"
	chartType := ""
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
	summary := c.extractChartSummary(chartSpec, chartType)
	result := "📊 " + title
	if summary != "" {
		result += "\nSummary: " + summary
	}
	return result
}

func (c *userDslConverter) extractChartSummary(chartSpec cardObj, chartType string) string {
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
		if xField == "" {
			if arr, ok := chartSpec["xField"].([]interface{}); ok && len(arr) > 0 {
				xField, _ = arr[0].(string)
			}
		}
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

func (c *userDslConverter) convertForm(elem cardObj) string {
	var sb strings.Builder
	sb.WriteString("<form>\n")
	if elements, ok := elem["elements"].([]interface{}); ok {
		sb.WriteString(c.convertElements(elements, 0))
	}
	sb.WriteString("\n</form>")
	return sb.String()
}

func (c *userDslConverter) convertCollapsiblePanel(elem cardObj) string {
	expanded, _ := elem["expanded"].(bool)
	title := "Details"
	if header, ok := elem["header"].(cardObj); ok {
		if titleRaw, ok := header["title"]; ok {
			if titleElem, ok := titleRaw.(cardObj); ok {
				if t := c.convertElement(titleElem, 0); t != "" {
					title = t
				}
			}
		}
	}
	indicator := "▶"
	if expanded {
		indicator = "▼"
	}
	var sb strings.Builder
	sb.WriteString(indicator + " " + title + "\n")
	if elements, ok := elem["elements"].([]interface{}); ok {
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

func (c *userDslConverter) convertInteractiveContainer(elem cardObj) string {
	urlStr := ""
	if behaviors, ok := elem["behaviors"].([]interface{}); ok {
		for _, b := range behaviors {
			bm, ok := b.(cardObj)
			if !ok {
				continue
			}
			if bm["type"] == "open_url" {
				if u, ok := bm["default_url"].(string); ok && u != "" {
					urlStr = u
					break
				}
			}
		}
	}
	if urlStr == "" {
		if action, ok := elem["action"].(cardObj); ok {
			if u, ok := action["url"].(string); ok && u != "" {
				urlStr = u
			} else if multiURL, ok := action["multi_url"].(cardObj); ok {
				urlStr, _ = multiURL["url"].(string)
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("<clickable")
	if urlStr != "" {
		sb.WriteString(fmt.Sprintf(" url=\"%s\"", cardEscapeAttr(urlStr)))
	}
	sb.WriteString(">\n")
	if elements, ok := elem["elements"].([]interface{}); ok {
		sb.WriteString(c.convertElements(elements, 0))
	}
	sb.WriteString("\n</clickable>")
	return sb.String()
}

func (c *userDslConverter) convertPerson(elem cardObj) string {
	userID, _ := elem["user_id"].(string)
	if userID == "" {
		return ""
	}
	return userID
}

func (c *userDslConverter) convertPersonList(elem cardObj) string {
	persons, _ := elem["persons"].([]interface{})
	if len(persons) == 0 {
		return ""
	}
	var ids []string
	for _, p := range persons {
		pm, ok := p.(cardObj)
		if !ok {
			continue
		}
		if id, ok := pm["id"].(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return strings.Join(ids, ", ")
}

func (c *userDslConverter) convertTextTag(elem cardObj) string {
	textElem, ok := elem["text"].(cardObj)
	if !ok {
		return ""
	}
	text := c.extractTextContent(textElem)
	if text == "" {
		return ""
	}
	return "「" + text + "」"
}

func (c *userDslConverter) convertAvatar(elem cardObj) string {
	userID, _ := elem["user_id"].(string)
	result := "👤"
	if userID != "" {
		result += "(id:" + userID + ")"
	}
	return result
}

func (c *userDslConverter) convertSelectImg(elem cardObj) string {
	options, _ := elem["options"].([]interface{})
	if len(options) == 0 {
		return ""
	}
	selectedValues := map[string]bool{}
	if vals, ok := elem["selected_values"].([]interface{}); ok {
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
		if imgKey, ok := om["img_key"].(string); ok && imgKey != "" {
			text += "(img_key:" + imgKey + ")"
		}
		if selectedValues[value] {
			text = "✓" + text
		}
		optTexts = append(optTexts, text)
	}
	return "{" + strings.Join(optTexts, " / ") + "}"
}

func (c *userDslConverter) convertRepeat(elem cardObj) string {
	if elements, ok := elem["elements"].([]interface{}); ok {
		return c.convertElements(elements, 0)
	}
	return ""
}

func (c *userDslConverter) convertAudio(elem cardObj) string {
	result := "🎵 Audio"
	fileKey, _ := elem["file_key"].(string)
	if fileKey == "" {
		fileKey, _ = elem["audio_id"].(string)
	}
	if fileKey != "" {
		result += "(key:" + fileKey + ")"
	}
	return result
}

func (c *userDslConverter) convertVideo(elem cardObj) string {
	result := "🎬 Video"
	fileKey, _ := elem["file_key"].(string)
	if fileKey != "" {
		result += "(key:" + fileKey + ")"
	}
	return result
}
