// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFormDelete = common.Shortcut{
	Service:     "base",
	Command:     "+form-delete",
	Description: "Delete a form in a Base table",
	Risk:        "high-risk-write",
	Scopes:      []string{"base:form:delete"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "table-id", Desc: "table ID", Required: true},
		{Name: "form-id", Desc: "form ID", Required: true},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			DELETE("/open-apis/base/v3/bases/:base_token/tables/:table_id/forms/:form_id").
			Set("base_token", runtime.Str("base-token")).
			Set("table_id", runtime.Str("table-id")).
			Set("form_id", runtime.Str("form-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		baseToken := runtime.Str("base-token")
		tableId := runtime.Str("table-id")
		formId := runtime.Str("form-id")

		_, err := baseV3Call(runtime, "DELETE",
			baseV3Path("bases", baseToken, "tables", tableId, "forms", formId), nil, nil)
		if err != nil {
			return err
		}

		runtime.Out(map[string]interface{}{"deleted": true, "form_id": formId}, nil)
		return nil
	},
}
