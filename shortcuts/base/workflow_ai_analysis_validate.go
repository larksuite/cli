// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import "fmt"

const aiAnalysisActionType = "AIAnalysisAction"

var aiAnalysisIdentityTypes = map[string]bool{
	"maker":           true,
	"triggerPersonal": true,
}

func validateWorkflowAIAnalysisAction(stepIndex int, step map[string]interface{}) error {
	data, ok := step["data"].(map[string]interface{})
	if !ok {
		return nil
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
	return nil
}

func workflowJSONPath(stepIndex int, suffix string) string {
	return fmt.Sprintf("--json.steps[%d].data.%s", stepIndex, suffix)
}
