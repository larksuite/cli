// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"fmt"

	"github.com/larksuite/cli/errs"
)

func validateWorkflowMessageActions(body map[string]interface{}) error {
	rawSteps, ok := body["steps"]
	if !ok || rawSteps == nil {
		return nil
	}

	steps, ok := rawSteps.([]interface{})
	if !ok {
		// The API owns validation for the workflow definition as a whole.
		// This preflight only validates recognized message action shapes.
		return nil
	}

	for index, rawStep := range steps {
		step, ok := rawStep.(map[string]interface{})
		if !ok || step["type"] != "LarkMessageAction" {
			continue
		}

		dataPath := fmt.Sprintf("--json.steps[%d].data", index)
		data, ok := step["data"].(map[string]interface{})
		if !ok || data == nil {
			return workflowMessageActionShapeError(
				dataPath,
				"a JSON object",
				fmt.Sprintf("Set %s to a JSON object.", dataPath),
			)
		}

		receiverPath := dataPath + ".receiver"
		if receiver, ok := data["receiver"].([]interface{}); !ok || len(receiver) == 0 {
			return workflowMessageActionShapeError(
				receiverPath,
				"a non-empty JSON array",
				fmt.Sprintf("Set %s to a non-empty JSON array.", receiverPath),
			)
		}

		contentPath := dataPath + ".content"
		if content, ok := data["content"].([]interface{}); !ok || len(content) == 0 {
			return workflowMessageActionShapeError(
				contentPath,
				"a non-empty JSON array",
				fmt.Sprintf("Set %s to a non-empty JSON array.", contentPath),
			)
		}

		if sendToEveryone, exists := data["send_to_everyone"]; exists {
			if _, ok := sendToEveryone.(bool); !ok {
				path := dataPath + ".send_to_everyone"
				return workflowMessageActionShapeError(
					path,
					"a JSON boolean when provided",
					fmt.Sprintf("Omit %s when unused, or set it to true or false.", path),
				)
			}
		}

		if buttonList, exists := data["btn_list"]; exists {
			if _, ok := buttonList.([]interface{}); !ok {
				path := dataPath + ".btn_list"
				return workflowMessageActionShapeError(
					path,
					"a JSON array when provided",
					fmt.Sprintf("Omit %s when unused, or provide a JSON array; an empty array is valid.", path),
				)
			}
		}
	}

	return nil
}

func workflowMessageActionShapeError(path, expected, hint string) error {
	return errs.NewValidationError(
		errs.SubtypeInvalidArgument,
		"%s for LarkMessageAction must be %s",
		path,
		expected,
	).WithParam("--json").WithHint(hint)
}
