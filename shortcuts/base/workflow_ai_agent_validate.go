// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

const workflowAIAgentType = "AIAgentAction"
const workflowAIMCPType = "AIAgentMCPAction"

var workflowMCPPacks = map[string]bool{
	"search_mcp": true, "link_reader_mcp": true, "feishu_base_mcp": true,
	"openplatform_mcp": true, "doubao_general_search_mcp": true,
}

// Validate the public wire shape and reject settings the current converter would
// silently ignore. Model availability, limits and tool keys belong to the server.
func validateWorkflowAIAgentSteps(steps []interface{}) error {
	stepByID := make(map[string]map[string]interface{})
	triggerType := ""
	for _, raw := range steps {
		step, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := step["id"].(string)
		stepByID[id] = step
		typ, _ := step["type"].(string)
		if strings.HasSuffix(typ, "Trigger") {
			triggerType = typ
		}
	}
	owners := make(map[string]string)
	for i, raw := range steps {
		step, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		typ, _ := step["type"].(string)
		path := fmt.Sprintf("--json.steps[%d]", i)
		if err := validateWorkflowToolInfo(path, step); err != nil {
			return err
		}
		switch typ {
		case "AIAgentLLMAction", "AIAgentMemoryAction":
			return baseFlagErrorf("%s: configure model/memory on AIAgentAction.data; internal child steps are managed by the server", path)
		case "AIAgentCustomMCPAction":
			return baseFlagErrorf("%s: custom MCP is not supported by Workflow CLI", path)
		case workflowAIAgentType:
			if err := validateWorkflowAIAgentData(path, step, triggerType); err != nil {
				return err
			}
			if err := validateWorkflowAIAgentLinks(path, step, stepByID, owners); err != nil {
				return err
			}
		case workflowAIMCPType:
			if err := validateWorkflowMCPData(path, step, triggerType); err != nil {
				return err
			}
		}
	}
	for i, raw := range steps {
		step, ok := raw.(map[string]interface{})
		if !ok || step["type"] != workflowAIMCPType {
			continue
		}
		id, _ := step["id"].(string)
		if _, ok := owners[id]; !ok {
			return baseFlagErrorf("--json.steps[%d]: MCP must be attached to an AIAgentAction with a slot/tool link", i)
		}
	}
	return nil
}

// Tool descriptions are protocol metadata beside the action's business fields.
// Preserve omission, null (clear), and empty text; the server owns enable checks.
func validateWorkflowToolInfo(path string, step map[string]interface{}) error {
	data, ok := step["data"].(map[string]interface{})
	if !ok {
		return nil
	}
	raw, exists := data["tool_info"]
	if !exists || raw == nil {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return baseFlagErrorf("%s.data.tool_info must be a JSON object or null", path)
	}
	var info struct {
		Description *string `json:"description"`
	}
	if err := json.Unmarshal(encoded, &info); err != nil {
		return baseFlagErrorf("%s.data.tool_info must be an object with a string description", path)
	}
	if info.Description == nil {
		return baseFlagErrorf("%s.data.tool_info.description must be a string", path)
	}
	return nil
}

func validateWorkflowAIAgentData(path string, step map[string]interface{}, trigger string) error {
	raw, exists := step["data"]
	if !exists {
		return nil
	}
	data, ok := raw.(map[string]interface{})
	if !ok {
		return baseFlagErrorf("%s.data must be an object", path)
	}
	for _, key := range []string{"prompt", "system_message"} {
		value, exists := data[key]
		if !exists {
			continue
		}
		items, ok := value.([]interface{})
		if !ok {
			return baseFlagErrorf("%s.data.%s must be a text/ref array", path, key)
		}
		for j, rawItem := range items {
			item, ok := rawItem.(map[string]interface{})
			if !ok {
				return baseFlagErrorf("%s.data.%s[%d] must be a text/ref object", path, key, j)
			}
			var aliases struct {
				Snake json.RawMessage `json:"value_type"`
				Camel json.RawMessage `json:"valueType"`
			}
			encoded, err := json.Marshal(item)
			if err != nil {
				return baseFlagErrorf("%s.data.%s[%d] must be a JSON object", path, key, j)
			}
			if err := json.Unmarshal(encoded, &aliases); err != nil {
				return baseFlagErrorf("%s.data.%s[%d] must be a JSON object", path, key, j)
			}
			typ, err := workflowValueType(fmt.Sprintf("%s.data.%s[%d]", path, key, j), aliases.Snake, aliases.Camel)
			if err != nil {
				return err
			}
			text, ok := item["value"].(string)
			if !ok || (typ != "text" && typ != "ref") || (typ == "ref" && !strings.HasPrefix(text, "$.")) {
				return baseFlagErrorf("%s.data.%s[%d] requires text or a workflow reference starting with $.", path, key, j)
			}
		}
	}
	if value, exists := data["max_iterations"]; exists {
		if err := workflowAgentPositiveInteger(path+".data.max_iterations", value); err != nil {
			return err
		}
	}
	if value, exists := data["enable_streaming_reply"]; exists {
		if _, ok := value.(bool); !ok {
			return baseFlagErrorf("%s.data.enable_streaming_reply must be boolean", path)
		}
	}
	if rawModel, exists := data["model"]; exists {
		model, ok := rawModel.(map[string]interface{})
		if !ok {
			return baseFlagErrorf("%s.data.model must be an object", path)
		}
		name, ok := model["name"].(string)
		if !ok || strings.TrimSpace(name) == "" {
			return baseFlagErrorf("%s.data.model.name must be a model pack ID", path)
		}
		if value, exists := model["temperature"]; exists {
			if _, ok := value.(float64); !ok {
				return baseFlagErrorf("%s.data.model.temperature must be a number", path)
			}
		}
		for _, key := range []string{"max_token", "timeout"} {
			if value, exists := model[key]; exists {
				if err := workflowAgentPositiveInteger(path+".data.model."+key, value); err != nil {
					return err
				}
			}
		}
	}
	if value, exists := data["memory"]; exists {
		if trigger != "LarkMessageTrigger" {
			return baseFlagErrorf("%s.data.memory is effective only with LarkMessageTrigger", path)
		}
		memory, ok := value.(map[string]interface{})
		if !ok {
			return baseFlagErrorf("%s.data.memory must be an object", path)
		}
		if rounds, exists := memory["history_rounds"]; exists {
			if err := workflowAgentPositiveInteger(path+".data.memory.history_rounds", rounds); err != nil {
				return err
			}
		}
	}
	format, hasFormat := data["output_format_type"]
	if hasFormat && format != "text" && format != "json" && format != "form" {
		return baseFlagErrorf("%s.data.output_format_type must be text, json or form", path)
	}
	if rawJSON, exists := data["json_output"]; exists || format == "json" {
		example, ok := rawJSON.(string)
		if !ok || !json.Valid([]byte(example)) {
			return baseFlagErrorf("%s.data.json_output must be a string containing a valid JSON output example", path)
		}
	}
	if rawForm, exists := data["form_output"]; exists || format == "form" {
		fields, ok := rawForm.([]interface{})
		if !ok || len(fields) == 0 {
			return baseFlagErrorf("%s.data.form_output must be a non-empty array", path)
		}
		for j, rawField := range fields {
			field, ok := rawField.(map[string]interface{})
			if !ok {
				return baseFlagErrorf("%s.data.form_output[%d] must be an object", path, j)
			}
			name, _ := field["name"].(string)
			typ, _ := field["type"].(string)
			if strings.TrimSpace(name) == "" || (typ != "string" && typ != "number" && typ != "boolean" && typ != "timestamp" && typ != "fileInfo") {
				return baseFlagErrorf("%s.data.form_output[%d] requires a name and a supported field type", path, j)
			}
		}
	}
	return nil
}

func workflowAgentPositiveInteger(path string, value interface{}) error {
	n, ok := value.(float64)
	if !ok || n <= 0 || math.Trunc(n) != n {
		return baseFlagErrorf("%s must be a positive integer", path)
	}
	return nil
}

func validateWorkflowAIAgentLinks(path string, step map[string]interface{}, steps map[string]map[string]interface{}, owners map[string]string) error {
	raw, exists := step["children"]
	if !exists || raw == nil {
		return nil
	}
	children, ok := raw.(map[string]interface{})
	if !ok {
		return baseFlagErrorf("%s.children must be an object", path)
	}
	links, ok := children["links"].([]interface{})
	if !ok {
		return baseFlagErrorf("%s.children.links must be an array", path)
	}
	for i, rawLink := range links {
		link, ok := rawLink.(map[string]interface{})
		if !ok {
			return baseFlagErrorf("%s.children.links[%d] must be an object", path, i)
		}
		label, _ := link["label"].(string)
		to, isString := link["to"].(string)
		if link["kind"] != "slot" || !isString || (label != "tool" && label != "llm" && label != "memory") {
			return baseFlagErrorf("%s.children.links[%d] must be a slot link with tool/llm/memory label", path, i)
		}
		if to == "" {
			continue
		} // Readback contains empty internal slots.
		if label != "tool" {
			return baseFlagErrorf("%s: llm/memory links must be empty; configure data.model/data.memory instead", path)
		}
		target, exists := steps[to]
		if !exists {
			return baseFlagErrorf("%s: tool link target %q does not exist", path, to)
		}
		if _, exists := owners[to]; exists {
			return baseFlagErrorf("%s: tool %q is attached more than once", path, to)
		}
		typ, _ := target["type"].(string)
		switch typ {
		case "LarkMessageAction", "LarkEmailAction", "AddRecordAction", "SetRecordAction", "FindRecordAction", "HTTPClientAction", "GenerateAiTextAction", "AIAnalysisAction", "APIHubAction", workflowAIMCPType:
		default:
			return baseFlagErrorf("%s: %s is not a supported AI Agent tool type", path, typ)
		}
		if next, exists := target["next"]; exists && next != nil && next != "" {
			return baseFlagErrorf("%s: tool %q must not have next; use the parent Agent's next", path, to)
		}
		owners[to], _ = step["id"].(string)
	}
	return nil
}

func validateWorkflowMCPData(path string, step map[string]interface{}, trigger string) error {
	meta, _ := step["meta"].(map[string]interface{})
	pack, _ := meta["component_id"].(string)
	if !workflowMCPPacks[pack] {
		packs := make([]string, 0, len(workflowMCPPacks))
		for name := range workflowMCPPacks {
			packs = append(packs, name)
		}
		sort.Strings(packs)
		return baseFlagErrorf("%s.meta.component_id is not in this CLI version's supported Workflow MCP package list; supported packages: %s", path, strings.Join(packs, ", "))
	}
	raw, exists := step["data"]
	if !exists {
		return nil
	}
	data, ok := raw.(map[string]interface{})
	if !ok {
		return baseFlagErrorf("%s.data must be an object", path)
	}
	if rawList, exists := data["tool_list"]; exists {
		list, ok := rawList.([]interface{})
		if !ok {
			return baseFlagErrorf("%s.data.tool_list must be a string array", path)
		}
		for _, item := range list {
			key, ok := item.(string)
			if !ok || strings.TrimSpace(key) == "" {
				return baseFlagErrorf("%s.data.tool_list must contain non-empty tool keys", path)
			}
		}
	}
	if identity, exists := data["identity_type"]; exists {
		if identity != "maker" && identity != "triggerPersonal" {
			return baseFlagErrorf("%s.data.identity_type must be maker or triggerPersonal", path)
		}
		if identity == "triggerPersonal" && trigger != "ButtonTrigger" && trigger != "LarkMessageTrigger" {
			return baseFlagErrorf("%s: triggerPersonal requires ButtonTrigger or LarkMessageTrigger", path)
		}
	}
	if rawEnv, exists := data["env"]; exists {
		env, ok := rawEnv.(map[string]interface{})
		if !ok {
			return baseFlagErrorf("%s.data.env must be a string map", path)
		}
		for _, value := range env {
			if _, ok := value.(string); !ok {
				return baseFlagErrorf("%s.data.env values must be strings", path)
			}
		}
	}
	return nil
}
