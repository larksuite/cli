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

// AppsList lists Miaoda apps visible to the calling user (cursor pagination).
//
// Supports name fuzzy match (--keyword), collaborator-dimension filter
// (--scope), and app-type filter (--app-type). See lark-apps SKILL.md for when
// an agent should use this to resolve an app_id from a user-supplied name
// (only when the user named an app and a downstream op needs its app_id — never
// unconditional enumeration).
var AppsList = common.Shortcut{
	Service:     appsService,
	Command:     "+list",
	Description: "List Miaoda apps visible to the calling user (cursor pagination)",
	Risk:        "read",
	Scopes:      []string{"spark:app:read"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "keyword", Desc: "fuzzy match on app name"},
		{Name: "scope", Desc: "collaborator dimension", Enum: []string{"all", "created_by_me", "shared_with_me"}},
		{Name: "app-type", Desc: "app type filter (html or full_stack)", Enum: []string{"html", "full_stack"}},
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
	if kw := strings.TrimSpace(rctx.Str("keyword")); kw != "" {
		params["keyword"] = kw
	}
	if scope := strings.TrimSpace(rctx.Str("scope")); scope != "" {
		params["scope"] = scope
	}
	if at := strings.TrimSpace(rctx.Str("app-type")); at != "" {
		params["app_type"] = at
	}
	return params
}
