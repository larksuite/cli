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
//
// Hidden from --help / tab completion (Hidden: true) so agents do not discover it
// as a way to enumerate / search applications. Direct invocation still works for
// humans who know the command. When agents need an existing app_id, they should
// ask the user to provide either the Miaoda app URL (extract app_id from the
// path segment after /app/) or the app_id string directly; see lark-apps SKILL.md.
var AppsList = common.Shortcut{
	Service:     appsService,
	Command:     "+list",
	Description: "List Miaoda apps owned by the calling user (cursor pagination)",
	Risk:        "read",
	Scopes:      []string{"spark:app:read"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Hidden:      true,
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
			// Table view (--format table) intentionally shows only the columns
			// most useful for visual scanning: app_id (to copy-paste downstream),
			// name (to match what the user sees in the UI), and updated_at (to
			// pick the most recent variant). description / icon_url / created_at
			// stay in the underlying JSON (--format json) but would make the
			// table too wide for a terminal.
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
