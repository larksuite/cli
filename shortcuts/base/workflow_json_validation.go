// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/larksuite/cli/shortcuts/common"
)

func parseWorkflowBodyJSON(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	pc := newParseCtx(runtime)
	body, err := parseJSONObject(pc, runtime.Str("json"), "json")
	if err != nil {
		return nil, err
	}
	if !runtime.Bool("no-validate") {
		if err := validateWorkflowBodyForCLI(runtime.Command(), body); err != nil {
			return nil, err
		}
	}
	return body, nil
}

// Check stable public inputs locally; context-dependent rules stay on the server.
// Validation must leave the original request unchanged.
func validateWorkflowBodyForCLI(command string, body map[string]interface{}) error {
	if raw, exists := body["steps"]; exists {
		steps, ok := raw.([]interface{})
		if !ok {
			return baseFlagErrorf("--json.steps must be an array")
		}
		for index, rawStep := range steps {
			path := fmt.Sprintf("--json.steps[%d]", index)
			step, ok := rawStep.(map[string]interface{})
			if !ok {
				return baseFlagErrorf("%s must be an object", path)
			}
			for _, key := range []string{"id", "type", "next"} {
				value, exists := step[key]
				if !exists {
					continue
				}
				switch value.(type) {
				case string:
					continue
				case nil:
					if key == "next" {
						continue
					}
				}
				return baseFlagErrorf("%s.%s must be a string", path, key)
			}
			for _, key := range []string{"data", "meta", "children"} {
				if value, exists := step[key]; exists {
					switch value.(type) {
					case nil, map[string]interface{}:
					default:
						return baseFlagErrorf("%s.%s must be an object or null", path, key)
					}
				}
			}
		}
	}
	if err := validateWorkflowValueAliases("--json", body); err != nil {
		return err
	}
	steps, _ := body["steps"].([]interface{})
	if err := validateWorkflowCapabilitySteps(command, steps); err != nil {
		return err
	}
	var stepIDs map[string]int
	for stepIndex, rawStep := range steps {
		step, ok := rawStep.(map[string]interface{})
		if !ok {
			return baseFlagErrorf("--json.steps[%d] must be an object", stepIndex)
		}
		switch step["type"] {
		case workflowAPIHubActionType:
			if err := validateWorkflowAPIHubAction(stepIndex, step); err != nil {
				return err
			}
		case aiAnalysisActionType:
			if err := validateWorkflowAIAnalysisAction(stepIndex, step); err != nil {
				return err
			}
		case workflowAIClassificationStepType:
			if stepIDs == nil {
				stepIDs = indexWorkflowStepIDs(steps)
			}
			if err := validateWorkflowAIClassificationStep(stepIndex, step, stepIDs); err != nil {
				return err
			}
		default:
			continue
		}
	}
	return validateWorkflowAIAgentSteps(steps)
}

// Both spellings reach one service field. Accept new discriminator values, but
// reject conflicting aliases rather than letting key iteration choose a value.
func validateWorkflowValueAliases(path string, raw interface{}) error {
	switch value := raw.(type) {
	case []interface{}:
		for index, child := range value {
			if err := validateWorkflowValueAliases(fmt.Sprintf("%s[%d]", path, index), child); err != nil {
				return err
			}
		}
	case map[string]interface{}:
		if value["type"] == "AIAgentAction" {
			data, _ := value["data"].(map[string]interface{})
			model, _ := data["model"].(map[string]interface{})
			snakeMode, hasSnakeMode := model["thinking_mode"]
			camelMode, hasCamelMode := model["thinkingMode"]
			// The service normalizes both keys to thinkingMode. Reject a collision
			// before map iteration can choose which value reaches model validation.
			if hasSnakeMode && hasCamelMode && !reflect.DeepEqual(snakeMode, camelMode) {
				return baseFlagErrorf("%s.data.model has conflicting thinking_mode/thinkingMode", path)
			}
		}
		snake, hasSnake := value["value_type"]
		camel, hasCamel := value["valueType"]
		for _, key := range []string{"value_type", "valueType"} {
			if field, exists := value[key]; exists {
				if _, ok := field.(string); !ok {
					return baseFlagErrorf("%s.%s must be a string", path, key)
				}
			}
		}
		if hasSnake && hasCamel && snake != camel {
			return baseFlagErrorf("%s has conflicting value_type/valueType", path)
		}
		for key, child := range value {
			if key == "data" && value["type"] == "AIAgentMCPAction" {
				if data, ok := child.(map[string]interface{}); ok {
					// MCP env is a tool-defined dictionary, not Workflow fields.
					// Inspect a shallow protocol view; never rewrite the request.
					protocol := make(map[string]interface{}, len(data))
					for field, item := range data {
						if field != "env" {
							protocol[field] = item
						}
					}
					child = protocol
				}
			}
			if err := validateWorkflowValueAliases(path+"."+key, child); err != nil {
				return err
			}
		}
	}
	return nil
}

// Both aliases reach the same server key, so unequal values must not be forwarded.
func workflowValueType(path string, snake, camel json.RawMessage) (string, error) {
	var values [2]string
	for i, raw := range []json.RawMessage{snake, camel} {
		if len(raw) == 0 {
			continue
		}
		if string(raw) == "null" || json.Unmarshal(raw, &values[i]) != nil {
			return "", baseFlagErrorf("%s.%s must be a string", path, []string{"value_type", "valueType"}[i])
		}
	}
	if len(snake) > 0 && len(camel) > 0 && values[0] != values[1] {
		return "", baseFlagErrorf("%s has conflicting value_type/valueType", path)
	}
	if len(snake) > 0 {
		return values[0], nil
	}
	return values[1], nil
}
