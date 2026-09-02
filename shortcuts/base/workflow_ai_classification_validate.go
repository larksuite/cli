// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"fmt"
	"strings"
)

const (
	workflowAIClassificationStepType             = "AIClassificationBranch"
	workflowAIClassificationDefaultNoMatchAction = "classifyToOther"
)

func validateWorkflowAIClassificationBranches(body map[string]interface{}) error {
	steps, ok := body["steps"].([]interface{})
	if !ok {
		return nil
	}

	stepIDs := make(map[string]int, len(steps))
	for i, raw := range steps {
		step, _ := raw.(map[string]interface{})
		id, _ := step["id"].(string)
		if strings.TrimSpace(id) != "" {
			stepIDs[id] = i
		}
	}

	for i, raw := range steps {
		step, _ := raw.(map[string]interface{})
		stepType, _ := step["type"].(string)
		if stepType != workflowAIClassificationStepType {
			continue
		}
		if err := validateWorkflowAIClassificationStep(i, step, stepIDs); err != nil {
			return err
		}
	}
	return nil
}

func validateWorkflowAIClassificationStep(index int, step map[string]interface{}, stepIDs map[string]int) error {
	path := fmt.Sprintf("--json steps[%d]", index)
	data, ok := step["data"].(map[string]interface{})
	if !ok || data == nil {
		return baseValidationErrorf("%s.data must be an object for AIClassificationBranch", path)
	}
	classes, err := validateAIClassificationAgentData(path, data, index, stepIDs)
	if err != nil {
		return err
	}
	return validateAIClassificationLinks(path, step, stepIDs, classes, aiClassificationNoMatchAction(data))
}

func validateAIClassificationAgentData(path string, data map[string]interface{}, stepIndex int, stepIDs map[string]int) ([]string, error) {
	if _, ok := data["mode"]; ok {
		return nil, baseValidationErrorf("%s.data.mode is not supported; omit it because AI classification only supports Exclusive mode", path)
	}

	rawClasses, ok := data["classes"].([]interface{})
	if !ok {
		return nil, baseValidationErrorf("%s.data.classes must be an array", path)
	}
	if len(rawClasses) < 2 {
		return nil, baseValidationErrorf("%s.data.classes must contain at least 2 items", path)
	}
	classes := make([]string, 0, len(rawClasses))
	seen := map[string]int{}
	for i, raw := range rawClasses {
		classPath := fmt.Sprintf("%s.data.classes[%d]", path, i)
		item, ok := raw.(map[string]interface{})
		if !ok {
			return nil, baseValidationErrorf("%s must be an object", classPath)
		}
		name, _ := item["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, baseValidationErrorf("%s.name must be a non-empty string", classPath)
		}
		if strings.ContainsAny(name, "\r\n") {
			return nil, baseValidationErrorf("%s.name must not contain newlines", classPath)
		}
		if prev, exists := seen[name]; exists {
			return nil, baseValidationErrorf("%s.name duplicates %s.data.classes[%d].name", classPath, path, prev)
		}
		if _, ok := item["desc"].(string); !ok {
			return nil, baseValidationErrorf("%s.desc must be a string", classPath)
		}
		seen[name] = i
		classes = append(classes, name)
	}

	if err := validateAIClassificationContent(path, data["content"], stepIndex, stepIDs); err != nil {
		return nil, err
	}
	if rule, ok := data["classification_rule"]; ok {
		if _, ok := rule.(string); !ok {
			return nil, baseValidationErrorf("%s.data.classification_rule must be a string when set", path)
		}
	}
	if actionRaw, ok := data["no_match_action"]; ok {
		action, ok := actionRaw.(string)
		if !ok || strings.TrimSpace(action) == "" {
			return nil, baseValidationErrorf("%s.data.no_match_action must be classifyToOther or fail when set", path)
		}
		if action != "classifyToOther" && action != "fail" {
			return nil, baseValidationErrorf("%s.data.no_match_action must be classifyToOther or fail when set", path)
		}
	}
	return classes, nil
}

func validateAIClassificationContent(path string, raw interface{}, stepIndex int, stepIDs map[string]int) error {
	items, ok := raw.([]interface{})
	if !ok {
		return baseValidationErrorf("%s.data.content must be an array", path)
	}
	hasContent := false
	for i, rawItem := range items {
		itemPath := fmt.Sprintf("%s.data.content[%d]", path, i)
		item, ok := rawItem.(map[string]interface{})
		if !ok {
			return baseValidationErrorf("%s must be an object", itemPath)
		}
		valueType, _ := item["value_type"].(string)
		value, _ := item["value"].(string)
		switch valueType {
		case "text":
			if strings.TrimSpace(value) != "" {
				hasContent = true
			}
		case "ref":
			refStep, ok := workflowRefStepID(value)
			if !ok {
				return baseValidationErrorf("%s.value must be a workflow ref path starting with $.", itemPath)
			}
			refIndex, exists := stepIDs[refStep]
			if !exists {
				return baseValidationErrorf("%s.value references unknown step id %q", itemPath, refStep)
			}
			if refIndex >= stepIndex {
				return baseValidationErrorf("%s.value must reference a previous step, got %q", itemPath, refStep)
			}
			hasContent = true
		default:
			return baseValidationErrorf("%s.value_type must be text or ref", itemPath)
		}
	}
	if !hasContent {
		return baseValidationErrorf("%s.data.content must contain non-empty text or ref", path)
	}
	return nil
}

func validateAIClassificationLinks(path string, step map[string]interface{}, stepIDs map[string]int, classes []string, noMatchAction string) error {
	children, _ := step["children"].(map[string]interface{})
	links, _ := children["links"].([]interface{})
	if len(links) == 0 {
		return baseValidationErrorf("%s.children.links must contain one non-empty case link for each class", path)
	}

	ordinaryLinks := 0
	defaultLinks := 0
	seenTargets := map[string]int{}
	for i, raw := range links {
		linkPath := fmt.Sprintf("%s.children.links[%d]", path, i)
		link, ok := raw.(map[string]interface{})
		if !ok {
			return baseValidationErrorf("%s must be an object", linkPath)
		}
		kind, _ := link["kind"].(string)
		if kind != "case" {
			return baseValidationErrorf("%s.kind must be case for AIClassificationBranch", linkPath)
		}
		label, _ := link["label"].(string)
		if label == "other" {
			return baseValidationErrorf("%s.label must be default for AIClassificationBranch default branch, not other", linkPath)
		}
		to, _ := link["to"].(string)
		to = strings.TrimSpace(to)
		if to == "" {
			return baseValidationErrorf("%s.to must not be blank", linkPath)
		}
		if _, ok := stepIDs[to]; !ok {
			return baseValidationErrorf("%s.to references unknown step id %q", linkPath, to)
		}
		if prev, exists := seenTargets[to]; exists {
			return baseValidationErrorf("%s.to duplicates %s.children.links[%d].to", linkPath, path, prev)
		}
		seenTargets[to] = i

		if label == "default" {
			defaultLinks++
			continue
		}
		wantLabel := fmt.Sprintf("branch_%d", ordinaryLinks+1)
		if label != wantLabel {
			return baseValidationErrorf("%s.label must be %s for AIClassificationBranch class link", linkPath, wantLabel)
		}
		desc, _ := link["desc"].(string)
		if ordinaryLinks < len(classes) && desc != classes[ordinaryLinks] {
			return baseValidationErrorf("%s.desc must equal --json steps data.classes[%d].name", linkPath, ordinaryLinks)
		}
		ordinaryLinks++
	}
	if ordinaryLinks != len(classes) {
		return baseValidationErrorf("%s.children.links must contain one non-empty case link for each class", path)
	}
	if noMatchAction == "classifyToOther" && defaultLinks != 1 {
		return baseValidationErrorf("%s.children.links must contain exactly one default link when no_match_action is classifyToOther", path)
	}
	if noMatchAction == "fail" && defaultLinks != 0 {
		return baseValidationErrorf("%s.children.links must not contain a default link when no_match_action is fail", path)
	}
	return nil
}

func aiClassificationNoMatchAction(data map[string]interface{}) string {
	action, _ := data["no_match_action"].(string)
	action = strings.TrimSpace(action)
	if action == "" {
		return workflowAIClassificationDefaultNoMatchAction
	}
	return action
}

func workflowRefStepID(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "$.") {
		return "", false
	}
	value = strings.TrimPrefix(value, "$.")
	if value == "" {
		return "", false
	}
	if idx := strings.Index(value, "."); idx >= 0 {
		value = value[:idx]
	}
	return value, value != ""
}
