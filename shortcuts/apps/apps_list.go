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

// AppsList lists apps visible to the calling user (cursor pagination).
//
// Supports name fuzzy match (--keyword), ownership-dimension filter
// (--ownership: all / mine / shared), and app-type filter (--app-type). See
// lark-apps SKILL.md for when an agent should use this to resolve an app_id
// from a user-supplied name (only when the user named an app and a downstream
// op needs its app_id — never unconditional enumeration).
var AppsList = common.Shortcut{
	Service:     appsService,
	Command:     "+list",
	Description: "List apps visible to the calling user (cursor pagination)",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +list",
		"Example: lark-cli apps +list --keyword <keyword>",
		"Tip: filter fields with --jq, e.g. -q '.data.items[].app_id'",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "keyword", Desc: "fuzzy match on app name"},
		{Name: "ownership", Desc: "ownership filter: all (created by me + shared with me) | mine | shared", Enum: []string{"all", "mine", "shared"}},
		{Name: "app-type", Desc: "app type filter (html or full_stack)", Enum: []string{"html", "full_stack"}},
		{Name: "page-size", Type: "int", Default: "20", Desc: "page size"},
		{Name: "page-token", Desc: "pagination cursor from previous response"},
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			GET(apiBasePath + "/apps").
			Desc("List apps").
			Params(buildAppsListParams(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		data, err := rctx.CallAPITyped("GET", apiBasePath+"/apps", buildAppsListParams(rctx), nil)
		if err != nil {
			return err
		}
		// Project away icon_url (an image URL agents can't render) and created_at
		// (redundant with updated_at) from every item BEFORE OutFormat, so json /
		// table / pretty are all lean. Every other field (description, etc.) is kept.
		rawItems, _ := data["items"].([]interface{})
		items := make([]interface{}, 0, len(rawItems))
		for _, item := range rawItems {
			m, ok := item.(map[string]interface{})
			if !ok {
				items = append(items, item)
				continue
			}
			out := make(map[string]interface{}, len(m))
			for k, v := range m {
				if k == "icon_url" || k == "created_at" {
					continue
				}
				out[k] = v
			}
			items = append(items, out)
		}
		data["items"] = items
		rctx.OutFormat(data, nil, func(w io.Writer) {
			// Curated pretty view (--format pretty) shows the columns most useful
			// for visual scanning: app_id (to copy-paste downstream), name (to match
			// what the user sees in the UI), is_published / online_url (publish state
			// and post-publish access link — the actionable fields after a deploy),
			// and updated_at (to pick the most recent variant). online_url can be long
			// but is the key value once published; the renderer clamps column width.
			// Unpublished apps carry no online_url, so that cell renders empty.
			// description stays in the underlying data (--format json / table) but
			// would make the curated view too wide. icon_url / created_at are trimmed
			// from the data entirely above (not useful to an agent).
			rows := make([]map[string]interface{}, 0, len(items))
			for _, item := range items {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				rows = append(rows, map[string]interface{}{
					"app_id":       m["app_id"],
					"name":         m["name"],
					"is_published": m["is_published"],
					"online_url":   m["online_url"],
					"updated_at":   m["updated_at"],
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
	if ownership := strings.TrimSpace(rctx.Str("ownership")); ownership != "" {
		params["ownership"] = ownership
	}
	if at := strings.TrimSpace(rctx.Str("app-type")); at != "" {
		params["app_type"] = at
	}
	return params
}
