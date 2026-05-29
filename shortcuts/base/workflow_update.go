// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

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
		"Use +workflow-get first, then edit the returned definition and keep title/status/steps fields you do not intend to change.",
		"workflow-id must start with wkf; do not pass a tbl table ID.",
		"Step ids must be unique, and every next/children link must reference an existing step id.",
		"Updating does not enable or disable a workflow; call +workflow-enable or +workflow-disable separately.",
		"Use lark-base-workflow-guide.md as the entry guide and lark-base-workflow-schema.md as the steps JSON SSOT; do not invent steps[].type/data/next/children from natural language.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("base-token")) == "" {
			return common.FlagErrorf("--base-token must not be blank")
		}
		if strings.TrimSpace(runtime.Str("workflow-id")) == "" {
			return common.FlagErrorf("--workflow-id must not be blank")
		}
		pc := newParseCtx(runtime)
		if _, err := parseJSONObject(pc, runtime.Str("json"), "json"); err != nil {
			return err
		}
		return nil
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
