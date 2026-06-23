// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"io"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFormUpdate = common.Shortcut{
	Service:           "base",
	Command:           "+form-update",
	Description:       "Update a form in a Base table",
	Risk:              "write",
	ConditionalScopes: []string{"wiki:node:retrieve"},
	Scopes:            []string{"base:form:update"},
	AuthTypes:         []string{"user", "bot"},
	HasFormat:         true,
	Flags: []common.Flag{
		{Name: "base-token", Desc: "Base token (base_token)", Required: true},
		{Name: "table-id", Desc: "table ID", Required: true},
		{Name: "form-id", Desc: "form ID", Required: true},
		{Name: "name", Desc: "new form name"},
		{Name: "description", Desc: "new form description (plain text or markdown link like [text](https://example.com))"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			PATCH("/open-apis/base/v3/bases/:base_token/tables/:table_id/forms/:form_id").
			Set("base_token", baseTokenOrRaw(runtime)).
			Set("table_id", runtime.Str("table-id")).
			Set("form_id", runtime.Str("form-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		baseToken := baseTokenOrRaw(runtime)
		tableId := runtime.Str("table-id")
		formId := runtime.Str("form-id")
		name := runtime.Str("name")
		description := runtime.Str("description")

		body := map[string]interface{}{}
		if name != "" {
			body["name"] = name
		}
		if description != "" {
			body["description"] = description
		}

		data, err := baseV3Call(runtime, "PATCH",
			baseV3Path("bases", baseToken, "tables", tableId, "forms", formId), nil, body)
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
