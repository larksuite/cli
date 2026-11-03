// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"fmt"
)

type workflowCapabilityContext struct {
	stepTypes     map[string]string
	triggerType   string
	screenshotIDs map[string]bool
}

// Index identities, never array positions: a predecessor can appear later in
// the public steps array. Execution order and loop visibility stay server-owned.
func indexWorkflowCapabilityContext(steps []interface{}) (workflowCapabilityContext, error) {
	context := workflowCapabilityContext{stepTypes: map[string]string{}, screenshotIDs: map[string]bool{}}
	if len(steps) == 0 {
		return context, nil
	}
	raw, err := json.Marshal(steps)
	if err != nil {
		return context, baseFlagErrorf("--json.steps must contain JSON step objects")
	}
	identities, err := workflowDecode[[]workflowCapabilityStep]("--json.steps", raw)
	if err != nil {
		return context, err
	}
	for _, step := range identities {
		context.stepTypes[step.ID] = step.Type
		if step.Type == "AppScreenshotAction" {
			context.screenshotIDs[step.ID] = true
		}
		if _, known := workflowMediaTriggerSupport(step.Type); known {
			context.triggerType = step.Type
		}
	}
	return context, nil
}

// Match TriggerType and supportSendDashboardTriggerTypes in the current model.
// Unknown future trigger types are delegated, so an older CLI cannot block a
// capability introduced by a newer service solely because of its type name.
func workflowMediaTriggerSupport(typ string) (allowed, known bool) {
	switch typ {
	case "ReminderTrigger", "TimerTrigger", "ButtonTrigger":
		return true, true
	case "AddRecordTrigger", "SetRecordTrigger", "DeleteRecordTrigger", "ChangeRecordTrigger", "CommentTrigger", "WebhookTrigger", "LarkMessageTrigger", "CustomTrigger":
		return false, true
	default:
		return false, false
	}
}

func validateWorkflowMediaTrigger(path string, context workflowCapabilityContext) error {
	if allowed, known := workflowMediaTriggerSupport(context.triggerType); known && !allowed {
		return baseFlagErrorf("%s requires ReminderTrigger, TimerTrigger or ButtonTrigger; got %s", path, context.triggerType)
	}
	return nil
}

// Inspect only the message/email body already validated by the body parser.
// Titles, attachments and opaque extensions do not acquire body capabilities.
func validateWorkflowDashboardTrigger(path string, data workflowMessageInput, context workflowCapabilityContext) error {
	if allowed, known := workflowMediaTriggerSupport(context.triggerType); allowed || !known {
		return nil
	}
	check := func(path string, raw json.RawMessage) error {
		if len(raw) == 0 {
			return nil
		}
		items, err := workflowDecode[[]workflowBodyItem](path, raw)
		if err != nil {
			return err
		}
		for index, item := range items {
			if item.Type != "ref" {
				continue
			}
			valuePath := fmt.Sprintf("%s[%d].value", path, index)
			value, err := workflowString(valuePath, item.Value, true)
			if err != nil {
				return err
			}
			if isWorkflowDashboardRef(value) {
				return validateWorkflowMediaTrigger(fmt.Sprintf("%s[%d].value", path, index), context)
			}
		}
		return nil
	}
	if err := check(path+".content", data.Content); err != nil {
		return err
	}
	if len(data.RichTextContent) == 0 {
		return nil
	}
	blocks, err := workflowDecode[[]workflowRichBlock](path+".rich_text_content", data.RichTextContent)
	if err != nil {
		return err
	}
	for index, block := range blocks {
		if block.Type == "paragraph" {
			if err := check(fmt.Sprintf("%s.rich_text_content[%d].value", path, index), block.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

// Only explicit values are checked here. Omission keeps the existing server
// default/modify semantics; the selected model's supported subset remains TCC-owned.
func validateWorkflowThinkingMode(path string, data json.RawMessage) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	var input struct {
		Model json.RawMessage `json:"model"`
	}
	if err := json.Unmarshal(data, &input); err != nil {
		return baseFlagErrorf("%s must be an object", path)
	}
	if len(input.Model) == 0 || string(input.Model) == "null" {
		return nil
	}
	var model struct {
		Snake json.RawMessage `json:"thinking_mode"`
		Camel json.RawMessage `json:"thinkingMode"`
	}
	if err := json.Unmarshal(input.Model, &model); err != nil {
		return baseFlagErrorf("%s.model must be an object", path)
	}
	raw := model.Snake
	if len(raw) == 0 {
		raw = model.Camel
	}
	if len(raw) == 0 {
		return nil
	}
	var mode string
	if json.Unmarshal(raw, &mode) == nil && (mode == "auto" || mode == "quick" || mode == "deep") {
		return nil
	}
	return baseFlagErrorf("%s.model.thinking_mode must be auto, quick or deep; omit thinking_mode to use the server default", path)
}
