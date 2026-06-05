// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"io"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFormCreate = common.Shortcut{
	Service:     "base",
	Command:     "+form-create",
	Description: "Create a form in a Base table",
	Risk:        "write",
	Scopes:      []string{"base:form:create"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "base-token", Desc: "Base token (base_token)", Required: true},
		{Name: "table-id", Desc: "table ID", Required: true},
		{Name: "name", Desc: "form name", Required: true},
		{Name: "description", Desc: `form description (plain text or markdown link like [text](https://example.com))`},
	},
	Tips: []string{
		"Record the returned form_id; form question create/list/update/delete commands need it.",
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/forms").
			Set("base_token", runtime.Str("base-token")).
			Set("table_id", runtime.Str("table-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		baseToken := runtime.Str("base-token")
		tableId := runtime.Str("table-id")
		name := runtime.Str("name")
		description := runtime.Str("description")

		body := map[string]interface{}{"name": name}
		if description != "" {
			body["description"] = description
		}

		data, err := baseV3Call(runtime, "POST",
			baseV3Path("bases", baseToken, "tables", tableId, "forms"), nil, body)
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
