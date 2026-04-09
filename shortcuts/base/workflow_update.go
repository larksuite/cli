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
		{Name: "json", Desc: `workflow body JSON, e.g. {"title":"New Title","steps":[...]}; or @path/to/file.json for large definitions`, Required: true},
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
