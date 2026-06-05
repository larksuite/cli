// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseWorkflowCreate = common.Shortcut{
	Service:     "base",
	Command:     "+workflow-create",
	Description: "Create a new workflow in a base",
	Risk:        "write",
	Scopes:      []string{"base:workflow:create"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "base-token", Desc: "base token", Required: true},
		{Name: "json", Desc: "workflow body JSON; read lark-base-workflow-guide.md and lark-base-workflow-schema.md before constructing steps", Required: true},
	},
	Tips: []string{
		"lark-cli base +workflow-create --base-token <base_token> --json @workflow.json",
		"client_token is required and should be unique per create request.",
		"New workflows are created disabled; call +workflow-enable after creation when the user wants it active.",
		"Before constructing steps, use +table-list and +field-list to confirm real table and field names.",
		"Step ids must be unique, and every next/children link must reference an existing step id.",
		"Use lark-base-workflow-guide.md as the entry guide and lark-base-workflow-schema.md as the steps JSON SSOT; do not invent steps[].type/data/next/children from natural language.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("base-token")) == "" {
			return baseFlagErrorf("--base-token must not be blank")
		}
		pc := newParseCtx(runtime)
		raw, err := loadJSONInput(pc, runtime.Str("json"), "json")
		if err != nil {
			return err
		}
		if _, err := parseJSONObject(pc, raw, "json"); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		pc := newParseCtx(runtime)
		var body map[string]interface{}
		if raw, err := loadJSONInput(pc, runtime.Str("json"), "json"); err == nil {
			body, _ = parseJSONObject(pc, raw, "json")
		}
		return common.NewDryRunAPI().
			POST("/open-apis/base/v3/bases/:base_token/workflows").
			Body(body).
			Set("base_token", runtime.Str("base-token"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		pc := newParseCtx(runtime)
		raw, err := loadJSONInput(pc, runtime.Str("json"), "json")
		if err != nil {
			return err
		}
		body, err := parseJSONObject(pc, raw, "json")
		if err != nil {
			return err
		}
		data, err := baseV3Call(runtime, "POST",
			baseV3Path("bases", runtime.Str("base-token"), "workflows"),
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
