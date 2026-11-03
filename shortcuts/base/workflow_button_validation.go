// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"fmt"
)

type workflowButtonInput struct {
	Text           json.RawMessage `json:"text"`
	Action         json.RawMessage `json:"btn_action"`
	Style          json.RawMessage `json:"style"`
	NotRepeatClick json.RawMessage `json:"not_repeat_click"`
	Link           json.RawMessage `json:"link"`
	TableName      json.RawMessage `json:"table_name"`
	RecordValues   json.RawMessage `json:"record_values"`
	RecordType     json.RawMessage `json:"record_type"`
	RecordInfo     json.RawMessage `json:"record_info"`
	WorkflowID     json.RawMessage `json:"workflow_id"`
}

// Match the model's compatibility rule: irrelevant null/empty fields have no
// content and can appear in a full GET result. Do not reject their presence.
func workflowButtonFieldHasContent(raw json.RawMessage) bool {
	var value interface{}
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return false
	}
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return v != ""
	case []interface{}:
		return len(v) > 0
	case map[string]interface{}:
		return len(v) > 0
	default:
		return true
	}
}

func validateWorkflowMessageButtons(path string, data workflowMessageInput, stepTypes map[string]string) error {
	var buttons []workflowButtonInput
	var err error
	if len(data.Buttons) > 0 && string(data.Buttons) != "null" {
		buttons, err = workflowDecode[[]workflowButtonInput](path+".btn_list", data.Buttons)
		if err != nil {
			return err
		}
	}
	hasRestrictedAction := false
	for index, button := range buttons {
		p := fmt.Sprintf("%s.btn_list[%d]", path, index)
		if _, err = workflowString(p+".text", button.Text, true); err != nil {
			return err
		}
		action, err := workflowChoice(p+".btn_action", button.Action, "openLink", "addRecord", "setRecord", "triggerWorkflow")
		if err != nil {
			return err
		}
		if len(button.Style) > 0 {
			if _, err = workflowChoice(p+".style", button.Style, "default", "primary", "danger"); err != nil {
				return err
			}
		}
		if len(button.NotRepeatClick) > 0 {
			restricted, err := workflowDecode[bool](p+".not_repeat_click", button.NotRepeatClick)
			if err != nil {
				return err
			}
			if restricted && action == "openLink" {
				return baseFlagErrorf("%s.not_repeat_click is not supported by openLink", p)
			}
			hasRestrictedAction = hasRestrictedAction || restricted
		}
		fields := []struct {
			name    string
			raw     json.RawMessage
			allowed bool
		}{
			{"link", button.Link, action == "openLink"},
			{"table_name", button.TableName, action == "addRecord" || action == "setRecord"},
			{"record_values", button.RecordValues, action == "addRecord" || action == "setRecord"},
			{"record_type", button.RecordType, action == "setRecord"},
			{"record_info", button.RecordInfo, action == "setRecord"},
			{"workflow_id", button.WorkflowID, action == "triggerWorkflow"},
		}
		for _, field := range fields {
			if !field.allowed && workflowButtonFieldHasContent(field.raw) {
				return baseFlagErrorf("%s.%s is incompatible with %s", p, field.name, action)
			}
		}
		switch action {
		case "openLink":
			// Button links use mcpTextRefArrayToSegments, which consumes only
			// text/ref values and ignores body styles and annotations.
			items, decodeErr := workflowDecode[[]workflowBodyItem](p+".link", button.Link)
			if decodeErr != nil {
				return decodeErr
			}
			if len(items) == 0 {
				return baseFlagErrorf("%s.link must be a non-empty text/ref array", p)
			}
			for index, item := range items {
				if item.Type != "text" && item.Type != "ref" {
					return baseFlagErrorf("%s.link[%d].value_type must be text or ref", p, index)
				}
				value, valueErr := workflowString(fmt.Sprintf("%s.link[%d].value", p, index), item.Value, item.Type == "ref")
				if valueErr != nil {
					return valueErr
				}
				if isWorkflowDashboardRef(value) {
					return baseFlagErrorf("%s.link cannot contain dashboard references", p)
				}
			}
		case "triggerWorkflow":
			if _, err = workflowString(p+".workflow_id", button.WorkflowID, true); err != nil {
				return err
			}
		default:
			if action == "addRecord" {
				if _, err = workflowString(p+".table_name", button.TableName, true); err != nil {
					return err
				}
			}
			if action == "setRecord" && (len(button.RecordType) > 0 || len(button.RecordInfo) > 0) {
				if _, err = workflowChoice(p+".record_type", button.RecordType, "Ref"); err != nil {
					return err
				}
				info, err := workflowDecode[struct {
					StepID json.RawMessage `json:"step_id"`
					Type   json.RawMessage `json:"type"`
				}](p+".record_info", button.RecordInfo)
				if err != nil {
					return err
				}
				sourceID, err := workflowString(p+".record_info.step_id", info.StepID, true)
				if err != nil {
					return err
				}
				if stepTypes[sourceID] == "FindRecordAction" {
					var kind string
					if json.Unmarshal(info.Type, &kind) != nil || kind != "firstRecord" {
						return baseFlagErrorf("%s.record_info.type must be firstRecord for a FindRecordAction source", p)
					}
				}
			}
			values, err := workflowDecode[[]struct {
				FieldName json.RawMessage `json:"field_name"`
			}](p+".record_values", button.RecordValues)
			if err != nil {
				return err
			}
			if len(values) == 0 {
				return baseFlagErrorf("%s.record_values must be non-empty", p)
			}
			for i, value := range values {
				if _, err = workflowString(fmt.Sprintf("%s.record_values[%d].field_name", p, i), value.FieldName, true); err != nil {
					return err
				}
			}
		}
	}
	if len(data.ButtonConfig) == 0 {
		return nil
	}
	config, err := workflowDecode[struct {
		Mode   json.RawMessage `json:"not_repeat_click_mode"`
		Mutual json.RawMessage `json:"mutual_exclusion"`
	}](path+".button_config", data.ButtonConfig)
	if err != nil {
		return err
	}
	mode := "AllUser"
	// The model uses ?? defaults for these two fields, including explicit null.
	if len(config.Mode) > 0 && string(config.Mode) != "null" {
		mode, err = workflowChoice(path+".button_config.not_repeat_click_mode", config.Mode, "AllUser", "OnlySelf")
		if err != nil {
			return err
		}
	}
	mutual := false
	if len(config.Mutual) > 0 && string(config.Mutual) != "null" {
		mutual, err = workflowDecode[bool](path+".button_config.mutual_exclusion", config.Mutual)
		if err != nil {
			return err
		}
	}
	if mutual {
		if mode == "OnlySelf" {
			return baseFlagErrorf("%s.button_config.not_repeat_click_mode must be AllUser for mutual exclusion", path)
		}
		for index, button := range buttons {
			// openLink false is discarded by the model before message-level
			// coupling checks; it must not invalidate another action button.
			if string(button.NotRepeatClick) == "false" && string(button.Action) != `"openLink"` {
				return baseFlagErrorf("%s.btn_list[%d].not_repeat_click must be true or omitted for mutual exclusion", path, index)
			}
		}
		if !hasRestrictedAction {
			return baseFlagErrorf("%s.button_config mutual exclusion requires an action button with not_repeat_click true", path)
		}
	}
	return nil
}
