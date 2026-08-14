// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

var BaseWorkflowUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+workflow-update",
	Description: "Replace a workflow's full definition (title and/or steps) in a base",
	Risk:        "write",
	Scopes:      []string{"base:workflow:update"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "base-token", Desc: "base token", Required: true},
		{Name: "workflow-id", Desc: "workflow ID (wkf... prefix)", Required: true},
		{Name: "json", Desc: "workflow body JSON; read lark-base-workflow-guide.md and lark-base-workflow-schema.md before replacing steps", Required: true},
	},
	Tips: []string{
		"lark-cli base +workflow-update --base-token <base_token> --workflow-id <workflow_id> --json @workflow.json",
		"PUT uses full replacement semantics; omitting steps clears the existing workflow steps.",
		"Use +workflow-get first, then build the update body from the returned title and steps; preserve every step field you do not intend to change.",
		"workflow-id must start with wkf; do not pass a tbl table ID.",
		"Step ids must be unique, and every next/children link must reference an existing step id.",
		"Updating does not enable or disable a workflow; call +workflow-enable or +workflow-disable separately.",
		"Use lark-base-workflow-guide.md as the entry guide and lark-base-workflow-schema.md as the steps JSON SSOT; do not invent steps[].type/data/next/children from natural language.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("base-token")) == "" {
			return baseFlagErrorf("--base-token must not be blank")
		}
		if strings.TrimSpace(runtime.Str("workflow-id")) == "" {
			return baseFlagErrorf("--workflow-id must not be blank")
		}
		pc := newParseCtx(runtime)
		body, err := parseJSONObject(pc, runtime.Str("json"), "json")
		if err != nil {
			return err
		}
		return validateWorkflowDefinition(body)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		pc := newParseCtx(runtime)
		var body map[string]interface{}
		body, _ = parseJSONObject(pc, runtime.Str("json"), "json")
		return common.NewDryRunAPI().
			PUT("/open-apis/base/v3/bases/:base_token/workflows/:workflow_id").
			Body(body).
			Set("base_token", runtime.Str("base-token")).
			Set("workflow_id", runtime.Str("workflow-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		pc := newParseCtx(runtime)
		body, err := parseJSONObject(pc, runtime.Str("json"), "json")
		if err != nil {
			return err
		}
		data, err := baseV3Call(runtime, "PUT",
			baseV3Path("bases", runtime.Str("base-token"), "workflows", runtime.Str("workflow-id")),
			nil,
			body,
		)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

// validateWorkflowDefinition enforces the workflow contracts shared by create
// and update that the CLI can determine without replacing the server schema.
func validateWorkflowDefinition(body map[string]interface{}) error {
	steps, ok := body["steps"].([]interface{})
	if !ok {
		return nil
	}

	for index, rawStep := range steps {
		step, ok := rawStep.(map[string]interface{})
		if !ok {
			continue
		}
		stepType, _ := step["type"].(string)
		if stepType != "LarkMessageAction" {
			continue
		}
		if err := validateWorkflowMessageAction(index, step["data"]); err != nil {
			return err
		}
	}
	return nil
}

func validateWorkflowMessageAction(index int, rawData interface{}) error {
	data, ok := rawData.(map[string]interface{})
	if !ok {
		return workflowFieldError("--json.steps[%d].data for LarkMessageAction must be a JSON object", index)
	}
	if receiver, ok := data["receiver"].([]interface{}); !ok || len(receiver) == 0 {
		return workflowFieldError("--json.steps[%d].data.receiver for LarkMessageAction must be a non-empty array", index)
	}
	if content, ok := data["content"].([]interface{}); !ok || len(content) == 0 {
		return workflowFieldError("--json.steps[%d].data.content for LarkMessageAction must be a non-empty array", index)
	}
	if sendToEveryone, provided := data["send_to_everyone"]; provided {
		if _, ok := sendToEveryone.(bool); !ok {
			return workflowFieldError("--json.steps[%d].data.send_to_everyone for LarkMessageAction must be a boolean when provided", index)
		}
	}
	if buttonList, provided := data["btn_list"]; provided {
		if _, ok := buttonList.([]interface{}); !ok {
			return workflowFieldError("--json.steps[%d].data.btn_list for LarkMessageAction must be an array when provided", index)
		}
	}
	return nil
}

func workflowFieldError(format string, args ...any) error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, format, args...).
		WithParam("--json").
		WithHint("Fix the reported field without inferring values or rewriting unrelated workflow data.")
}
