// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseWorkflowDisable = common.Shortcut{
	Service:     "base",
	Command:     "+workflow-disable",
	Description: "Disable a workflow in a base",
	Risk:        "write",
	Scopes:      []string{"base:workflow:update"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "base-token", Desc: "base token", Required: true},
		{Name: "workflow-id", Desc: "workflow ID (wkf... prefix)", Required: true},
	},
	Tips: []string{
		"workflow-id must start with wkf; do not pass a tbl table ID from the same URL.",
		"Disable only changes workflow state; it does not delete the workflow or its steps.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("base-token")) == "" {
			return common.FlagErrorf("--base-token must not be blank")
		}
		if strings.TrimSpace(runtime.Str("workflow-id")) == "" {
			return common.FlagErrorf("--workflow-id must not be blank")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			PATCH("/open-apis/base/v3/bases/:base_token/workflows/:workflow_id/disable").
			Set("base_token", runtime.Str("base-token")).
			Set("workflow_id", runtime.Str("workflow-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		data, err := baseV3Call(runtime, "PATCH",
			baseV3Path("bases", runtime.Str("base-token"), "workflows", runtime.Str("workflow-id"), "disable"),
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
