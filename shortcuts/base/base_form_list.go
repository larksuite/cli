// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"fmt"
	"io"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFormsList = common.Shortcut{
	Service:     "base",
	Command:     "+form-list",
	Description: "List all forms in a Base table (auto-paginated)",
	Risk:        "read",
	Scopes:      []string{"base:form:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "base-token", Desc: "Base token (base_token)", Required: true},
		{Name: "table-id", Desc: "table ID", Required: true},
		{Name: "page-size", Type: "int", Default: "100", Desc: "page size per request, range 1-100"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := common.ValidatePageSizeTyped(runtime, "page-size", 100, 1, 100)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/forms").
			Params(map[string]interface{}{"page_size": runtime.Int("page-size")}).
			Set("base_token", runtime.Str("base-token")).
			Set("table_id", runtime.Str("table-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		baseToken := runtime.Str("base-token")
		tableId := runtime.Str("table-id")

		var allForms []interface{}
		pageToken := ""
		for {
			params := map[string]interface{}{
				"page_size": runtime.Int("page-size"),
			}
			if pageToken != "" {
				params["page_token"] = pageToken
			}

			data, err := baseV3Call(runtime, "GET",
				baseV3Path("bases", baseToken, "tables", tableId, "forms"), params, nil)
			if err != nil {
				return err
			}

			forms, _ := data["forms"].([]interface{})
			allForms = append(allForms, forms...)

			hasMore, _ := data["has_more"].(bool)
			if !hasMore {
				break
			}
			nextToken, _ := data["page_token"].(string)
			if nextToken == "" {
				break
			}
			pageToken = nextToken
		}

		outData := map[string]interface{}{
			"forms": allForms,
			"total": len(allForms),
		}
		runtime.OutFormat(outData, nil, func(w io.Writer) {
			if len(allForms) == 0 {
				fmt.Fprintln(w, "No forms found.")
				return
			}
			var rows []map[string]interface{}
			for _, item := range allForms {
				m, _ := item.(map[string]interface{})
				rows = append(rows, map[string]interface{}{
					"id":          m["id"],
					"name":        m["name"],
					"description": m["description"],
				})
			}
			output.PrintTable(w, rows)
			fmt.Fprintf(w, "\n%d form(s) total\n", len(allForms))
		})
		return nil
	},
}
