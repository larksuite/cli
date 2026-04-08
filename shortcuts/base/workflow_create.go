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
		{Name: "json", Desc: `workflow body JSON, e.g. {"title":"My Workflow","steps":[...]}; or @path/to/file.json for large definitions`, Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("base-token")) == "" {
			return common.FlagErrorf("--base-token must not be blank")
		}
		raw, err := loadJSONInput(runtime.FileIO(), runtime.Str("json"), "json")
		if err != nil {
			return err
		}
		if _, err := parseJSONObject(runtime.FileIO(), raw, "json"); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		var body map[string]interface{}
		if raw, err := loadJSONInput(runtime.FileIO(), runtime.Str("json"), "json"); err == nil {
			body, _ = parseJSONObject(runtime.FileIO(), raw, "json")
		}
		return common.NewDryRunAPI().
			POST("/open-apis/base/v3/bases/:base_token/workflows").
			Body(body).
			Set("base_token", runtime.Str("base-token"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		raw, err := loadJSONInput(runtime.FileIO(), runtime.Str("json"), "json")
		if err != nil {
			return err
		}
		body, err := parseJSONObject(runtime.FileIO(), raw, "json")
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
