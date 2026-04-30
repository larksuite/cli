// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"io"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFormGet = common.Shortcut{
	Service:     "base",
	Command:     "+form-get",
	Description: "Get a form in a Base table",
	Risk:        "read",
	Scopes:      []string{"base:form:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "table-id", Desc: "table ID", Required: true},
		{Name: "form-id", Desc: "form ID", Required: true},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/forms/:form_id").
			Set("base_token", runtime.Str("base-token")).
			Set("table_id", runtime.Str("table-id")).
			Set("form_id", runtime.Str("form-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		baseToken := runtime.Str("base-token")
		tableId := runtime.Str("table-id")
		formId := runtime.Str("form-id")

		data, err := baseV3Call(runtime, "GET",
			baseV3Path("bases", baseToken, "tables", tableId, "forms", formId), nil, nil)
		if err != nil {
			return err
		}

		runtime.OutFormat(data, nil, func(w io.Writer) {
			output.PrintTable(w, []map[string]interface{}{
				{
					"id":          data["id"],
					"name":        data["name"],
					"description": data["description"],
				},
			})
		})
		return nil
	},
}
