// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"fmt"

	"github.com/larksuite/cli/shortcuts/common"
)

const aiAnalysisActionType = "AIAnalysisAction"

var aiAnalysisIdentityTypes = map[string]bool{
	"maker":           true,
	"triggerPersonal": true,
}

func parseWorkflowBodyJSON(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	pc := newParseCtx(runtime)
	body, err := parseJSONObject(pc, runtime.Str("json"), "json")
	if err != nil {
		return nil, err
	}
	if err := validateWorkflowBodyForCLI(body); err != nil {
		return nil, err
	}
	return body, nil
}

func parseWorkflowUpdateBodyJSON(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	body, err := parseWorkflowBodyJSON(runtime)
	if err != nil {
		return nil, err
	}
	if _, ok := body["steps"]; !ok {
		body["steps"] = []interface{}{}
	}
	return body, nil
}

func validateWorkflowBodyForCLI(body map[string]interface{}) error {
	steps, ok := body["steps"].([]interface{})
	if !ok {
		return nil
	}

	for stepIndex, rawStep := range steps {
		step, ok := rawStep.(map[string]interface{})
		if !ok {
			continue
		}
		stepType, _ := step["type"].(string)
		if stepType != aiAnalysisActionType {
			continue
		}
		data, ok := step["data"].(map[string]interface{})
		if !ok {
			continue
		}
		if value, exists := data["analysis_table_names"]; exists {
			items, ok := value.([]interface{})
			if !ok {
				return baseFlagErrorf("%s must be a string array", workflowJSONPath(stepIndex, "analysis_table_names"))
			}
			for itemIndex, item := range items {
				if _, ok := item.(string); !ok {
					return baseFlagErrorf("%s[%d] must be a string", workflowJSONPath(stepIndex, "analysis_table_names"), itemIndex)
				}
			}
		}
		if value, exists := data["identity_type"]; exists {
			identityType, ok := value.(string)
			if !ok {
				return baseFlagErrorf("%s must be one of: maker, triggerPersonal", workflowJSONPath(stepIndex, "identity_type"))
			}
			if !aiAnalysisIdentityTypes[identityType] {
				return baseFlagErrorf("%s must be one of: maker, triggerPersonal", workflowJSONPath(stepIndex, "identity_type"))
			}
		}
	}

	return nil
}

func workflowJSONPath(stepIndex int, suffix string) string {
	return fmt.Sprintf("--json.steps[%d].data.%s", stepIndex, suffix)
}
