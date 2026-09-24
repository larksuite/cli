// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"

	"github.com/larksuite/cli/shortcuts/common"
)

func parseWorkflowBodyJSON(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	pc := newParseCtx(runtime)
	body, err := parseJSONObject(pc, runtime.Str("json"), "json")
	if err != nil {
		return nil, err
	}
	if !runtime.Bool("no-validate") {
		if err := validateWorkflowBodyForCLI(body); err != nil {
			return nil, err
		}
	}
	return body, nil
}

func validateWorkflowBodyForCLI(body map[string]interface{}) error {
	steps, ok := body["steps"].([]interface{})
	if !ok {
		return nil
	}

	var stepIDs map[string]int
	for stepIndex, rawStep := range steps {
		step, ok := rawStep.(map[string]interface{})
		if !ok {
			continue
		}

		stepType, _ := step["type"].(string)
		switch stepType {
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
		}
	}

	return validateWorkflowAIAgentSteps(steps)
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
