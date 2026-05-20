// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// AppsList lists Miaoda apps owned by the calling user (cursor pagination).
var AppsList = common.Shortcut{
	Service:     appsService,
	Command:     "+list",
	Description: "List Miaoda apps owned by the calling user (cursor pagination)",
	Risk:        "read",
	Scopes:      []string{"spark:app:read"}, // 对齐 BOE 后端 scope 命名 (spark 命名空间)
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "page-size", Type: "int", Default: "20", Desc: "page size"},
		{Name: "page-token", Desc: "pagination cursor from previous response"},
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			GET(apiBasePath + "/apps").
			Desc("List Miaoda apps").
			Params(buildAppsListParams(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		data, err := rctx.CallAPI("GET", apiBasePath+"/apps", buildAppsListParams(rctx), nil)
		if err != nil {
			return err
		}
		items, _ := data["items"].([]interface{})
		rctx.OutFormat(data, nil, func(w io.Writer) {
			rows := make([]map[string]interface{}, 0, len(items))
			for _, item := range items {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				rows = append(rows, map[string]interface{}{
					"app_id":     m["app_id"],
					"name":       m["name"],
					"updated_at": m["updated_at"],
				})
			}
			output.PrintTable(w, rows)
		})
		return nil
	},
}

func buildAppsListParams(rctx *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{
		"page_size": rctx.Int("page-size"),
	}
	if token := strings.TrimSpace(rctx.Str("page-token")); token != "" {
		params["page_token"] = token
	}
	return params
}
