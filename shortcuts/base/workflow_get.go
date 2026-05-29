// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseWorkflowGet = common.Shortcut{
	Service:     "base",
	Command:     "+workflow-get",
	Description: "Get a single workflow definition (including steps) from a base",
	Risk:        "read",
	Scopes:      []string{"base:workflow:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "base-token", Desc: "base token", Required: true},
		{Name: "workflow-id", Desc: "workflow ID (wkf... prefix)", Required: true},
		{Name: "user-id-type", Desc: "user ID type for creator/updater fields, default open_id", Enum: []string{"open_id", "union_id", "user_id"}},
	},
	Tips: []string{
		"workflow-id must start with wkf; use +workflow-list if the ID is unknown.",
		"steps may be an empty array; that is valid for an unconfigured workflow.",
		"Use +workflow-get before +workflow-update, then edit the returned definition and keep fields you do not intend to change.",
		"Read lark-base-workflow-schema.md when interpreting or reusing returned steps.",
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
		api := common.NewDryRunAPI().
			GET("/open-apis/base/v3/bases/:base_token/workflows/:workflow_id").
			Set("base_token", runtime.Str("base-token")).
			Set("workflow_id", runtime.Str("workflow-id"))
		if t := runtime.Str("user-id-type"); t != "" {
			api = api.Params(map[string]interface{}{"user_id_type": t})
		}
		return api
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		var params map[string]interface{}
		if t := runtime.Str("user-id-type"); t != "" {
			params = map[string]interface{}{"user_id_type": t}
		}
		data, err := baseV3Call(runtime, "GET",
			baseV3Path("bases", runtime.Str("base-token"), "workflows", runtime.Str("workflow-id")),
			params,
			nil,
		)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}
