// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFieldGroupList = common.Shortcut{
	Service:     "base",
	Command:     "+field-group-list",
	Description: "List all field groups and their members in a table",
	Risk:        "read",
	Scopes:      []string{"base:field:read"},
	AuthTypes:   authTypes(),
	Flags:       []common.Flag{baseTokenFlag(true), tableRefFlag(true)},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/field_groups").
			Set("base_token", runtime.Str("base-token")).
			Set("table_id", baseTableID(runtime))
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		data, err := baseV3Call(runtime, "GET",
			baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "field_groups"),
			nil, nil,
		)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var BaseFieldGroupManage = common.Shortcut{
	Service:     "base",
	Command:     "+field-group-manage",
	Description: "Apply a batch of field group operations in a table",
	Risk:        "write",
	Scopes:      []string{"base:field:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "json", Desc: "field group batch JSON object with operations", Required: true, Input: []string{common.File, common.Stdin}},
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		_, err := parseJSONObject(newParseCtx(runtime), runtime.Str("json"), "json")
		return err
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, _ := parseJSONObject(newParseCtx(runtime), runtime.Str("json"), "json")
		return common.NewDryRunAPI().
			POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/field_groups").
			Body(body).
			Set("base_token", runtime.Str("base-token")).
			Set("table_id", baseTableID(runtime))
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		body, err := parseJSONObject(newParseCtx(runtime), runtime.Str("json"), "json")
		if err != nil {
			return err
		}
		data, err := baseV3Call(runtime, "POST",
			baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "field_groups"),
			nil, body,
		)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}
