// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"fmt"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFieldManageSelectOptions = common.Shortcut{
	Service:     "base",
	Command:     "+manage-select-options",
	Description: "Add, rename, or delete static select options in order",
	Risk:        "write",
	Scopes:      []string{"base:field:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		fieldRefFlag(true),
		{Name: "json", Desc: "JSON object with an operations array", Required: true},
		{Name: "yes", Type: "bool", Hidden: true, Desc: "accepted for compatibility; has no effect"},
	},
	Tips: []string{
		`Example: lark-cli base +manage-select-options --base-token <base_token> --table-id <table_id> --field-id "Status" --json '{"operations":[{"operation":"add","name":"Done"},{"operation":"rename","name":"Todo","new_name":"Doing"},{"operation":"delete","name":"Old"}]}'`,
		"Static single-select and multi-select only; for dynamic options, update the source field. Rename and delete change existing cell values.",
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		_, err := parseJSONObject(newParseCtx(runtime), runtime.Str("json"), "json")
		return err
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, err := parseJSONObject(newParseCtx(runtime), runtime.Str("json"), "json")
		if err != nil {
			return common.NewDryRunAPI().Desc(fmt.Sprintf("dry-run validation failed: %v", err))
		}
		return common.NewDryRunAPI().
			POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id/options").
			Body(body).
			Set("base_token", runtime.Str("base-token")).
			Set("table_id", baseTableID(runtime)).
			Set("field_id", runtime.Str("field-id"))
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		body, err := parseJSONObject(newParseCtx(runtime), runtime.Str("json"), "json")
		if err != nil {
			return err
		}
		data, err := baseV3Call(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "fields", runtime.Str("field-id"), "options"), nil, body)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}
