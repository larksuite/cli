// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const workflowAPIHubActionType = "APIHubAction"

// These rules describe the supported wire contract, not resource permissions or
// execution semantics. The workflow service remains authoritative for those.
type workflowAPIHubParamRule struct {
	key      string
	required bool
	types    string
}

var workflowAPIHubRules = map[string][]workflowAPIHubParamRule{
	"Automation_Create_Task":         {{"rich_summary", true, "text ref"}, {"collaborator_ids", false, "user ref"}, {"endTime", false, "date ref"}, {"rich_description", false, "text ref"}, {"follower_ids", false, "user ref"}},
	"Automation_Complete_Task":       {{"task_id", true, "text ref"}},
	"Automation_Uncomplete_Task":     {{"task_id", true, "text ref"}},
	"Automation_Delete_Task":         {{"task_id", true, "text ref"}},
	"Automation_Create_Schedule":     {{"summary", false, "text ref"}, {"startTime", true, "date ref"}, {"endTime", true, "date ref"}, {"attendees", false, "user group ref"}, {"calendarId", true, "option"}, {"description", false, "text ref"}, {"location", false, "text ref"}, {"needNotification", false, "boolean ref"}},
	"Automation_Delete_Schedule":     {{"calendarId", true, "text ref"}, {"eventId", true, "text ref"}, {"needNotification", false, "boolean ref"}},
	"Automation_Create_Group":        {{"name", true, "text ref"}, {"userList", true, "user ref"}, {"owner", true, "user ref"}, {"description", false, "text ref"}, {"topBase", false, "boolean ref"}},
	"Automation_Add_Group_Member":    {{"group", true, "group ref"}, {"userList", true, "user ref"}},
	"Automation_Delete_Group_Member": {{"group", true, "group ref"}, {"userList", true, "user ref"}},
}

type workflowAPIHubStep struct {
	Meta *struct {
		ComponentID string `json:"component_id"`
	} `json:"meta"`
	Data *struct {
		Params []workflowAPIHubParam `json:"params"`
	} `json:"data"`
}

type workflowAPIHubParam struct {
	Key   string                `json:"key"`
	Value []workflowAPIHubValue `json:"value"`
}

type workflowAPIHubValue struct {
	Type      json.RawMessage `json:"value_type"`
	CamelType json.RawMessage `json:"valueType"`
	Value     json.RawMessage `json:"value"`
}

func validateWorkflowAPIHubAction(index int, raw map[string]interface{}) error {
	path := fmt.Sprintf("--json.steps[%d]", index)
	encoded, err := json.Marshal(raw)
	if err != nil {
		return baseFlagErrorf("%s must be a JSON object", path)
	}
	var step workflowAPIHubStep
	if err := json.Unmarshal(encoded, &step); err != nil {
		return baseFlagErrorf("%s APIHubAction requires meta.component_id and data.params with key/value arrays", path)
	}
	if step.Meta == nil || step.Meta.ComponentID == "" {
		return baseFlagErrorf("%s.meta.component_id is required for APIHubAction", path)
	}
	rules, ok := workflowAPIHubRules[step.Meta.ComponentID]
	if !ok {
		components := make([]string, 0, len(workflowAPIHubRules))
		for component := range workflowAPIHubRules {
			components = append(components, component)
		}
		slices.Sort(components)
		return baseFlagErrorf("%s.meta.component_id is not in this CLI version's supported Workflow APIHub component list; supported components: %s", path, strings.Join(components, ", "))
	}
	if step.Data == nil || step.Data.Params == nil {
		return baseFlagErrorf("%s.data.params must be an array", path)
	}
	seen := make(map[string]bool, len(step.Data.Params))
	for i, param := range step.Data.Params {
		paramPath := fmt.Sprintf("%s.data.params[%d]", path, i)
		var rule *workflowAPIHubParamRule
		for j := range rules {
			if rules[j].key == param.Key {
				rule = &rules[j]
				break
			}
		}
		if rule == nil {
			return baseFlagErrorf("%s.key is not supported by %s; parameter keys are case-sensitive", paramPath, step.Meta.ComponentID)
		}
		if seen[param.Key] {
			return baseFlagErrorf("%s.key duplicates an earlier parameter", paramPath)
		}
		seen[param.Key] = true
		if param.Value == nil || (rule.required && len(param.Value) == 0) {
			return baseFlagErrorf("%s.value must be an array%s", paramPath, workflowAPIHubRequiredSuffix(rule.required))
		}
		for j, value := range param.Value {
			valuePath := fmt.Sprintf("%s.value[%d]", paramPath, j)
			if err := validateWorkflowAPIHubValue(valuePath, value, *rule); err != nil {
				return err
			}
		}
	}
	for _, rule := range rules {
		if rule.required && !seen[rule.key] {
			return baseFlagErrorf("%s.data.params requires parameter %s", path, rule.key)
		}
	}
	return nil
}

func workflowAPIHubRequiredSuffix(required bool) string {
	if required {
		return " containing a non-empty value"
	}
	return ""
}

func validateWorkflowAPIHubValue(path string, item workflowAPIHubValue, rule workflowAPIHubParamRule) error {
	valueType, err := workflowValueType(path, item.Type, item.CamelType)
	if err != nil {
		return err
	}
	if valueType == "" || !slices.Contains(strings.Fields(rule.types), valueType) {
		return baseFlagErrorf("%s.value_type must be one of: %s", path, strings.Join(strings.Fields(rule.types), ", "))
	}
	if len(item.Value) == 0 || string(item.Value) == "null" {
		return baseFlagErrorf("%s.value must not be null", path)
	}
	switch valueType {
	case "text", "date", "ref":
		var value string
		if json.Unmarshal(item.Value, &value) != nil {
			return baseFlagErrorf("%s.value must be a string", path)
		}
		if (rule.required || valueType != "text") && strings.TrimSpace(value) == "" {
			return baseFlagErrorf("%s.value must not be empty", path)
		}
		if valueType == "ref" && !strings.HasPrefix(value, "$.") {
			return baseFlagErrorf("%s.value must be a workflow reference starting with $.", path)
		}
	case "boolean":
		var value bool
		if json.Unmarshal(item.Value, &value) != nil {
			return baseFlagErrorf("%s.value must be a boolean", path)
		}
	case "user", "group", "option":
		var value struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if json.Unmarshal(item.Value, &value) != nil || (strings.TrimSpace(value.ID) == "" && strings.TrimSpace(value.Name) == "") {
			return baseFlagErrorf("%s.value must be an object containing id or name", path)
		}
	}
	return nil
}
