// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
)

// These projections are validation views only. Never replace the submitted body
// with them: GET annotations and fields added by newer services must survive.
type workflowCapabilityStep struct {
	ID   string          `json:"id"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type workflowMessageInput struct {
	ContentType        json.RawMessage `json:"content_type"`
	Content            json.RawMessage `json:"content"`
	RichTextContent    json.RawMessage `json:"rich_text_content"`
	TitleTemplateColor json.RawMessage `json:"title_template_color"`
	Buttons            json.RawMessage `json:"btn_list"`
	ButtonConfig       json.RawMessage `json:"button_config"`
}

type workflowBodyItem struct {
	Type  string          `json:"value_type"`
	Value json.RawMessage `json:"value"`
	Style json.RawMessage `json:"style"`
	Extra json.RawMessage `json:"extra_info"`
}

type workflowBodyExtra struct {
	DisplayType   json.RawMessage `json:"display_type"`
	FieldNames    json.RawMessage `json:"field_name_list"`
	DashboardName json.RawMessage `json:"dashboard_name"`
}

type workflowRichBlock struct {
	Type    string          `json:"type"`
	Value   json.RawMessage `json:"value"`
	Markers json.RawMessage `json:"line_marker_list"`
}

type workflowLineMarker struct {
	Type       string          `json:"type"`
	Level      json.RawMessage `json:"level"`
	Order      json.RawMessage `json:"order"`
	OrderID    json.RawMessage `json:"order_id"`
	StartOrder json.RawMessage `json:"start_order"`
}

// Match the model's public snake/camel spellings without rewriting the wire
// request. Webhook controls are handled separately: Go reads their exact keys.
var workflowCapabilityAliases = map[string]string{
	"contentType": "content_type", "richTextContent": "rich_text_content", "titleTemplateColor": "title_template_color",
	"lineMarkerList": "line_marker_list", "orderId": "order_id", "startOrder": "start_order",
	"valueType": "value_type", "extraInfo": "extra_info", "displayType": "display_type", "fieldNameList": "field_name_list", "dashboardName": "dashboard_name",
	"btnList": "btn_list", "buttonConfig": "button_config", "notRepeatClickMode": "not_repeat_click_mode", "mutualExclusion": "mutual_exclusion",
	"btnAction": "btn_action", "notRepeatClick": "not_repeat_click", "workflowId": "workflow_id", "tableName": "table_name", "recordValues": "record_values",
	"recordType": "record_type", "recordInfo": "record_info", "stepId": "step_id", "fieldName": "field_name", "appName": "app_name", "pageName": "page_name",
}

// Only traverse fields consumed by these validators. Metadata, unknown
// extensions and record field values are opaque; their keys belong to their
// respective owners and must not become Workflow alias conflicts.
var workflowCapabilityShapes = map[string]map[string]string{
	"step": {"data": "data"},
	"data": {
		"content_type": "", "content": "items", "rich_text_content": "blocks", "title_template_color": "",
		"btn_list": "buttons", "button_config": "button_config", "app_name": "", "page_name": "",
	},
	"items":   {"value_type": "", "value": "link", "style": "", "extra_info": "extra"},
	"blocks":  {"type": "", "value": "items", "line_marker_list": "markers"},
	"markers": {"type": "", "level": "", "order": "", "order_id": "", "start_order": ""},
	"link":    {"text": "items", "link": "items"},
	"buttons": {
		"text": "", "btn_action": "", "style": "", "not_repeat_click": "", "link": "items",
		"table_name": "", "record_values": "record_values", "record_type": "", "record_info": "record_info", "workflow_id": "",
	},
	"button_config": {"not_repeat_click_mode": "", "mutual_exclusion": ""},
	"record_values": {"field_name": ""},
	"record_info":   {"step_id": "", "type": ""},
	"extra":         {"display_type": "", "field_name_list": "", "dashboard_name": ""},
}

func workflowCapabilityView(path string, raw interface{}) (interface{}, error) {
	return workflowCapabilityProtocolView(path, raw, "step")
}

func workflowCapabilityProtocolView(path string, raw interface{}, shape string) (interface{}, error) {
	switch value := raw.(type) {
	case []interface{}:
		result := make([]interface{}, len(value))
		for i, child := range value {
			var err error
			result[i], err = workflowCapabilityProtocolView(fmt.Sprintf("%s[%d]", path, i), child, shape)
			if err != nil {
				return nil, err
			}
		}
		return result, nil
	case map[string]interface{}:
		result := make(map[string]interface{}, len(value))
		seen := make(map[string]string, len(value))
		for key, child := range value {
			name := key
			if alias, ok := workflowCapabilityAliases[key]; ok {
				name = alias
			}
			childShape, owned := workflowCapabilityShapes[shape][name]
			if !owned {
				result[key] = child
				continue
			}
			// A text/ref value is not a link object. Record-write values are
			// deliberately opaque as their shape depends on the target field.
			if shape == "items" && name == "value" && value["value_type"] != "link" && value["valueType"] != "link" {
				childShape = ""
			}
			converted := child
			if childShape != "" {
				var err error
				converted, err = workflowCapabilityProtocolView(path+"."+name, child, childShape)
				if err != nil {
					return nil, err
				}
			}
			// Compare canonical children: nested snake/camel spellings may
			// differ in the request while representing identical content.
			if previousKey, exists := seen[name]; exists && !reflect.DeepEqual(result[name], converted) {
				camel := key
				if camel == name {
					camel = previousKey
				}
				return nil, baseFlagErrorf("%s has conflicting %s/%s", path, name, camel)
			}
			result[name] = converted
			seen[name] = key
		}
		return result, nil
	default:
		return raw, nil
	}
}

func workflowDecode[T any](path string, raw json.RawMessage) (T, error) {
	var result T
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &result) != nil {
		return result, baseFlagErrorf("%s has an invalid JSON type", path)
	}
	return result, nil
}

func workflowString(path string, raw json.RawMessage, nonempty bool) (string, error) {
	value, err := workflowDecode[string](path, raw)
	if err != nil {
		return "", err
	}
	if nonempty && strings.TrimSpace(value) == "" {
		return "", baseFlagErrorf("%s must be a non-empty string", path)
	}
	return value, nil
}

func workflowChoice(path string, raw json.RawMessage, choices ...string) (string, error) {
	value, err := workflowString(path, raw, false)
	if err != nil {
		return "", err
	}
	for _, choice := range choices {
		if value == choice {
			return value, nil
		}
	}
	return "", baseFlagErrorf("%s must be one of: %s", path, strings.Join(choices, ", "))
}

// Stable input constraints mirror agent_content.ts, lark_message.ts and the Go
// OpenAPI webhook sanitizer. FG, model/resource availability, reference scope and
// field-specific write types still require the current server context.
func validateWorkflowCapabilitySteps(command string, steps []interface{}) error {
	context, err := indexWorkflowCapabilityContext(steps)
	if err != nil {
		return err
	}
	for index, raw := range steps {
		step, ok := raw.(map[string]interface{})
		if !ok {
			return baseFlagErrorf("--json.steps[%d] must be an object", index)
		}
		typ, _ := step["type"].(string)
		if typ != "LarkMessageAction" && typ != "LarkEmailAction" && typ != "AppScreenshotAction" && typ != "WebhookTrigger" && typ != "AIAgentAction" {
			continue
		}
		path := fmt.Sprintf("--json.steps[%d].data", index)
		view := interface{}(step)
		var err error
		if typ != "WebhookTrigger" && typ != "AIAgentAction" {
			view, err = workflowCapabilityView(fmt.Sprintf("--json.steps[%d]", index), step)
			if err != nil {
				return err
			}
		}
		encoded, err := json.Marshal(view)
		if err != nil {
			return baseFlagErrorf("%s must be JSON", path)
		}
		var input workflowCapabilityStep
		if json.Unmarshal(encoded, &input) != nil {
			return baseFlagErrorf("%s must be an object", path)
		}
		switch typ {
		case "AppScreenshotAction":
			if err = validateWorkflowMediaTrigger(fmt.Sprintf("--json.steps[%d].type AppScreenshotAction", index), context); err != nil {
				return err
			}
			data, err := workflowDecode[struct {
				App  json.RawMessage `json:"app_name"`
				Page json.RawMessage `json:"page_name"`
			}](path, input.Data)
			if err != nil {
				return err
			}
			if _, err = workflowString(path+".app_name", data.App, true); err != nil {
				return err
			}
			if _, err = workflowString(path+".page_name", data.Page, true); err != nil {
				return err
			}
		case "WebhookTrigger":
			if err := validateWorkflowWebhookInput(command, path, input.Data); err != nil {
				return err
			}
		case "AIAgentAction":
			if err := validateWorkflowThinkingMode(path, input.Data); err != nil {
				return err
			}
		default:
			// Preserve incomplete legacy message definitions when no new body
			// configuration is supplied; the server still validates completeness.
			if len(input.Data) == 0 || string(input.Data) == "null" {
				continue
			}
			data, err := workflowDecode[workflowMessageInput](path, input.Data)
			if err != nil {
				return err
			}
			if err = validateWorkflowMessageBody(path, data, typ == "LarkEmailAction", context); err != nil {
				return err
			}
			if err = validateWorkflowDashboardTrigger(path, data, context); err != nil {
				return err
			}
			if typ == "LarkMessageAction" {
				if err = validateWorkflowMessageButtons(path, data, context.stepTypes); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateWorkflowMessageBody(path string, data workflowMessageInput, email bool, context workflowCapabilityContext) error {
	if len(data.TitleTemplateColor) > 0 && !email {
		if _, err := workflowChoice(path+".title_template_color", data.TitleTemplateColor, "none", "blue", "wathet", "turquoise", "green", "yellow", "orange", "red", "carmine", "violet", "purple", "indigo", "grey"); err != nil {
			return err
		}
	}
	mode := "plain_text"
	if len(data.ContentType) > 0 {
		var err error
		mode, err = workflowChoice(path+".content_type", data.ContentType, "plain_text", "rich_text")
		if err != nil {
			return err
		}
	}
	if mode != "rich_text" {
		if len(data.RichTextContent) > 0 {
			return baseFlagErrorf("%s.rich_text_content requires content_type rich_text", path)
		}
		if len(data.Content) > 0 {
			return validateWorkflowBodyItems(path+".content", data.Content, false, false, email, context)
		}
		return nil
	}
	if len(data.Content) > 0 {
		return baseFlagErrorf("%s.content is incompatible with rich_text; remove content", path)
	}
	blocks, err := workflowDecode[[]json.RawMessage](path+".rich_text_content", data.RichTextContent)
	if err != nil {
		return err
	}
	if len(blocks) == 0 {
		return baseFlagErrorf("%s.rich_text_content must be a non-empty block array", path)
	}
	for index, raw := range blocks {
		blockPath := fmt.Sprintf("%s.rich_text_content[%d]", path, index)
		block, err := workflowDecode[workflowRichBlock](blockPath, raw)
		if err != nil {
			return err
		}
		switch block.Type {
		case "horizontalLine":
			properties, err := workflowDecode[map[string]json.RawMessage](blockPath, raw)
			if err != nil {
				return err
			}
			if len(properties) != 1 {
				return baseFlagErrorf("%s horizontalLine accepts type only", blockPath)
			}
		case "paragraph":
			if err = validateWorkflowLineMarkers(blockPath+".line_marker_list", block.Markers); err != nil {
				return err
			}
			if err = validateWorkflowBodyItems(blockPath+".value", block.Value, true, true, email, context); err != nil {
				return err
			}
		default:
			return baseFlagErrorf("%s.type must be paragraph or horizontalLine", blockPath)
		}
	}
	return validateWorkflowListSequences(path+".rich_text_content", blocks)
}

// Keep the same grouping/continuation rules as OrderedListSequences and
// defaultGroupFor in agent_content.ts. Missing order/order_id remains valid.
func validateWorkflowListSequences(path string, blocks []json.RawMessage) error {
	type listItem struct {
		index int
		level float64
		group string
		order float64
	}
	previous := []listItem{}
	counters := map[string]map[float64]float64{}
	for index, raw := range blocks {
		blockPath := fmt.Sprintf("%s[%d]", path, index)
		block, err := workflowDecode[workflowRichBlock](blockPath, raw)
		if err != nil {
			return err
		}
		if len(block.Markers) == 0 {
			continue
		}
		markers, err := workflowDecode[[]workflowLineMarker](blockPath+".line_marker_list", block.Markers)
		if err != nil {
			return err
		}
		for markerIndex, marker := range markers {
			if marker.Type != "orderedList" {
				continue
			}
			level, _ := workflowDecode[float64](path, marker.Level)
			requested, _ := workflowDecode[float64](path, marker.Order)
			name, _ := workflowDecode[string](path, marker.OrderID)
			group := "explicit:" + name
			if len(marker.OrderID) == 0 {
				group = fmt.Sprintf("auto:%d", index)
				if len(previous) > 0 {
					last := previous[len(previous)-1]
					if last.index == index-1 {
						group = last.group
					} else {
						for cursor := len(previous) - 1; cursor >= 0; cursor-- {
							candidate := previous[cursor]
							if candidate.group != last.group {
								break
							}
							if candidate.level < level {
								group = last.group
								break
							}
							if candidate.level == level {
								if len(marker.Order) > 0 && requested == candidate.order+1 {
									group = last.group
								}
								break
							}
						}
						if group != last.group {
							found := false
							for offset, following := range blocks[index+1:] {
								nextPath := fmt.Sprintf("%s[%d]", path, index+1+offset)
								next, err := workflowDecode[workflowRichBlock](nextPath, following)
								if err != nil {
									return err
								}
								if next.Type != "paragraph" || len(next.Markers) == 0 {
									continue
								}
								nextMarkers, err := workflowDecode[[]workflowLineMarker](nextPath+".line_marker_list", next.Markers)
								if err != nil {
									return err
								}
								for _, nextMarker := range nextMarkers {
									if nextMarker.Type == "orderedList" {
										nextName, _ := workflowDecode[string](path, nextMarker.OrderID)
										if len(nextMarker.OrderID) > 0 && "explicit:"+nextName == last.group {
											group = last.group
										}
										found = true
										break
									}
								}
								if found {
									break
								}
							}
						}
					}
				}
			}
			levels := counters[group]
			if levels == nil {
				levels = map[float64]float64{}
				counters[group] = levels
			}
			for oldLevel := range levels {
				if oldLevel > level {
					delete(levels, oldLevel)
				}
			}
			last, continuing := levels[level]
			order := float64(1)
			if continuing {
				order = last + 1
			} else if len(marker.Order) > 0 {
				order = requested
			}
			if continuing && len(marker.Order) > 0 && requested != order {
				return baseFlagErrorf("%s[%d].line_marker_list[%d].order: got %v; expected %v. Remove order or use %v; use another order_id for an independent list", path, index, markerIndex, requested, order, order)
			}
			levels[level] = order
			previous = append(previous, listItem{index: index, level: level, group: group, order: order})
		}
	}
	return nil
}

func validateWorkflowLineMarkers(path string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	markers, err := workflowDecode[[]workflowLineMarker](path, raw)
	if err != nil {
		return err
	}
	headings, lists := 0, 0
	for index, marker := range markers {
		p := fmt.Sprintf("%s[%d]", path, index)
		if len(marker.StartOrder) > 0 {
			return baseFlagErrorf("%s.start_order is not supported; use order", p)
		}
		level, err := workflowPositiveInteger(p+".level", marker.Level)
		if err != nil {
			return err
		}
		switch marker.Type {
		case "heading":
			headings++
			if level > 3 {
				return baseFlagErrorf("%s.level must be 1, 2 or 3", p)
			}
		case "orderedList":
			lists++
			if len(marker.Order) > 0 {
				if _, err = workflowPositiveInteger(p+".order", marker.Order); err != nil {
					return err
				}
			}
			if len(marker.OrderID) > 0 {
				if _, err = workflowString(p+".order_id", marker.OrderID, true); err != nil {
					return err
				}
			}
		case "unorderedList":
			lists++
		default:
			return baseFlagErrorf("%s.type must be heading, unorderedList or orderedList", p)
		}
	}
	if headings > 1 || lists > 1 {
		return baseFlagErrorf("%s supports at most one heading and one list marker", path)
	}
	return nil
}

func workflowPositiveInteger(path string, raw json.RawMessage) (float64, error) {
	value, err := workflowDecode[float64](path, raw)
	if err != nil {
		return 0, err
	}
	if value < 1 || value > 9007199254740991 || math.Trunc(value) != value {
		return 0, baseFlagErrorf("%s must be a positive safe integer", path)
	}
	return value, nil
}

func validateWorkflowBodyItems(path string, raw json.RawMessage, rich, allowEmpty, email bool, context workflowCapabilityContext) error {
	items, err := workflowDecode[[]workflowBodyItem](path, raw)
	if err != nil {
		return err
	}
	if !allowEmpty && len(items) == 0 {
		return baseFlagErrorf("%s must be a non-empty text/ref array", path)
	}
	for index, item := range items {
		p := fmt.Sprintf("%s[%d]", path, index)
		if len(item.Style) > 0 {
			if !rich {
				return baseFlagErrorf("%s.style requires rich_text; apply style to the whole link", p)
			}
			if err = validateWorkflowBodyStyle(p+".style", item.Style); err != nil {
				return err
			}
		}
		switch item.Type {
		case "text", "ref":
			value, err := workflowString(p+".value", item.Value, item.Type == "ref")
			if err != nil {
				return err
			}
			if item.Type == "ref" {
				if err = validateWorkflowBodyRef(p, value, item.Extra, rich, email, context); err != nil {
					return err
				}
			}
		case "link":
			if !rich {
				return baseFlagErrorf("%s.value_type link requires rich_text and cannot be nested", p)
			}
			parts, err := workflowDecode[struct {
				Text json.RawMessage `json:"text"`
				Link json.RawMessage `json:"link"`
			}](p+".value", item.Value)
			if err != nil {
				return err
			}
			for _, part := range []struct {
				name string
				raw  json.RawMessage
			}{{"text", parts.Text}, {"link", parts.Link}} {
				partPath := p + ".value." + part.name
				if err = validateWorkflowBodyItems(partPath, part.raw, false, false, email, context); err != nil {
					return err
				}
				children, _ := workflowDecode[[]workflowBodyItem](partPath, part.raw)
				for _, child := range children {
					value, _ := workflowDecode[string](partPath+".value", child.Value)
					if child.Type == "text" && value == "" {
						return baseFlagErrorf("%s.value link text and address must not be empty", partPath)
					}
					if child.Type == "ref" && (isWorkflowDashboardRef(value) || isWorkflowScreenshotRef(value, context.screenshotIDs)) {
						return baseFlagErrorf("%s cannot contain image or dashboard references", partPath)
					}
				}
			}
		default:
			return baseFlagErrorf("%s.value_type must be text, ref or link", p)
		}
	}
	return nil
}

func validateWorkflowBodyStyle(path string, raw json.RawMessage) error {
	style, err := workflowDecode[map[string]json.RawMessage](path, raw)
	if err != nil {
		return err
	}
	for key, value := range style {
		switch key {
		case "bold", "italic", "strikethrough":
			if _, err = workflowDecode[bool](path+"."+key, value); err != nil {
				return err
			}
		case "color":
			if _, err = workflowChoice(path+".color", value, "gray", "red", "orange", "yellow", "green", "blue", "purple"); err != nil {
				return err
			}
		default:
			return baseFlagErrorf("%s.%s is not a supported style", path, key)
		}
	}
	return nil
}

func isWorkflowDashboardRef(value string) bool {
	return value == "$.dashboard.image" || value == "$.dashboard.aiAnalysis"
}
func isWorkflowScreenshotRef(value string, screenshots map[string]bool) bool {
	parts := strings.Split(value, ".")
	return len(parts) == 3 && parts[0] == "$" && screenshots[parts[1]] && parts[2] == "Image"
}

func validateWorkflowBodyRef(path, value string, raw json.RawMessage, rich, email bool, context workflowCapabilityContext) error {
	if email && isWorkflowScreenshotRef(value, context.screenshotIDs) {
		return baseFlagErrorf("%s AppScreenshotAction.Image is not supported in email content; use attachments", path)
	}
	if email && value == "$.dashboard.aiAnalysis" {
		return baseFlagErrorf("%s dashboard aiAnalysis is not supported in email content", path)
	}
	// Rich text exposes first-record fields rather than a whole FindRecord row.
	// Other step types may publish a scalar field with the same name.
	parts := strings.Split(value, ".")
	if rich && len(parts) == 3 && parts[0] == "$" && context.stepTypes[parts[1]] == "FindRecordAction" && parts[2] == "firstfieldsRecord" {
		return baseFlagErrorf("%s firstfieldsRecord is unavailable in rich_text; choose a specific field or use fieldRecords for whole rows", path)
	}
	if len(raw) == 0 || string(raw) == "null" {
		if isWorkflowDashboardRef(value) {
			return baseFlagErrorf("%s.extra_info.dashboard_name is required", path)
		}
		return nil
	}
	extra, err := workflowDecode[workflowBodyExtra](path+".extra_info", raw)
	if err != nil {
		return err
	}
	if isWorkflowDashboardRef(value) {
		_, err = workflowString(path+".extra_info.dashboard_name", extra.DashboardName, true)
		return err
	}
	if !strings.HasSuffix(value, ".fieldRecords") && !strings.HasSuffix(value, ".firstfieldsRecord") {
		return nil
	}
	if len(extra.DisplayType) > 0 {
		if _, err = workflowChoice(path+".extra_info.display_type", extra.DisplayType, "table", "list"); err != nil {
			return err
		}
	}
	if len(extra.FieldNames) > 0 {
		names, err := workflowDecode[[]string](path+".extra_info.field_name_list", extra.FieldNames)
		if err != nil {
			return err
		}
		if len(names) == 0 {
			return baseFlagErrorf("%s.extra_info.field_name_list must be non-empty", path)
		}
		seen := map[string]bool{}
		for i, name := range names {
			if strings.TrimSpace(name) == "" || seen[name] {
				return baseFlagErrorf("%s.extra_info.field_name_list[%d] must be non-empty and unique", path, i)
			}
			seen[name] = true
		}
	}
	return nil
}
