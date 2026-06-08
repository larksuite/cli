// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseWorkflowEnable = common.Shortcut{
	Service:     "base",
	Command:     "+workflow-enable",
	Description: "Enable a workflow in a base",
	Risk:        "write",
	Scopes:      []string{"base:workflow:update"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "base-token", Desc: "base token", Required: true},
		{Name: "workflow-id", Desc: "workflow ID (wkf... prefix)", Required: true},
	},
	Tips: []string{
		"workflow-id must start with wkf; do not pass a tbl table ID from the same URL.",
		"Enable only changes workflow state; it does not modify steps.",
		"New workflows are created disabled; enable after creation only when the user wants it active.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("base-token")) == "" {
			return baseFlagErrorf("--base-token must not be blank")
		}
		if strings.TrimSpace(runtime.Str("workflow-id")) == "" {
			return baseFlagErrorf("--workflow-id must not be blank")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			PATCH("/open-apis/base/v3/bases/:base_token/workflows/:workflow_id/enable").
			Set("base_token", runtime.Str("base-token")).
			Set("workflow_id", runtime.Str("workflow-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		data, err := baseV3Call(runtime, "PATCH",
			baseV3Path("bases", runtime.Str("base-token"), "workflows", runtime.Str("workflow-id"), "enable"),
			nil,
			map[string]interface{}{},
		)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}
