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

// AppsSessionList lists sessions under a Miaoda app (cursor pagination, single page).
var AppsSessionList = common.Shortcut{
	Service:     appsService,
	Command:     "+session-list",
	Description: "List sessions under a Miaoda app (cursor pagination)",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +session-list --app-id <app_id>",
		"Tip: filter fields with --jq, e.g. -q '.data.sessions[].session_id'",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "page-size", Type: "int", Default: "20", Desc: "page size (max 50)"},
		{Name: "page-token", Desc: "pagination cursor from previous response"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			GET(sessionsPath(rctx.Str("app-id"))).
			Desc("List sessions under a Miaoda app").
			Params(buildSessionListParams(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		data, err := rctx.CallAPITyped("GET", sessionsPath(rctx.Str("app-id")), buildSessionListParams(rctx), nil)
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		sessions, _ := data["sessions"].([]interface{})
		rctx.OutFormat(data, nil, func(w io.Writer) {
			rows := make([]map[string]interface{}, 0, len(sessions))
			for _, item := range sessions {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				rows = append(rows, map[string]interface{}{
					"session_id": m["session_id"],
					"name":       m["name"],
					"is_active":  m["is_active"],
					"updated_at": m["updated_at"],
				})
			}
			output.PrintTable(w, rows)
		})
		return nil
	},
}

func buildSessionListParams(rctx *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{
		"page_size": rctx.Int("page-size"),
	}
	if token := strings.TrimSpace(rctx.Str("page-token")); token != "" {
		params["page_token"] = token
	}
	return params
}
