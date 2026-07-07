// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// AppsAutomationList lists an app's automation triggers (all 4 types).
var AppsAutomationList = common.Shortcut{
	Service:     appsService,
	Command:     "+automation-list",
	Description: "List a Miaoda app's automation triggers (cron/record-change/webhook/feishu-approval)",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +automation-list --app-id <app_id>",
		"Example: lark-cli apps +automation-list --app-id <app_id> --trigger-type webhook",
		"Example: lark-cli apps +automation-list --app-id <app_id> --all   # aggregate all pages",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "trigger-type", Desc: "filter by type: cron | record-change | webhook | feishu-approval"},
		{Name: "page-size", Type: "int", Desc: "page size (server default 50, max 100)"},
		{Name: "page-token", Desc: "pagination cursor from previous response"},
		{Name: "all", Type: "bool", Desc: "auto-aggregate all pages until has_more=false"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if tt := strings.TrimSpace(rctx.Str("trigger-type")); tt != "" {
			if _, err := mapTriggerType(tt); err != nil {
				return err
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			GET(automationListPath(appID)).
			Desc("List automation triggers").
			Params(buildAutomationListParams(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		path := automationListPath(appID)
		params := buildAutomationListParams(rctx)
		if rctx.Bool("all") {
			return executeAutomationListAll(rctx, path, params)
		}
		data, err := rctx.CallAPITyped("GET", path, params, nil)
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		return outputAutomationList(rctx, data)
	},
}

// buildAutomationListParams 组装 list 查询参数。--trigger-type kebab→snake 下推给后端。
func buildAutomationListParams(rctx *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{}
	if tt := strings.TrimSpace(rctx.Str("trigger-type")); tt != "" {
		if snake, err := mapTriggerType(tt); err == nil {
			params["trigger_type"] = snake
		}
	}
	if rctx.Changed("page-size") {
		params["page_size"] = rctx.Int("page-size")
	}
	if pt := strings.TrimSpace(rctx.Str("page-token")); pt != "" {
		params["page_token"] = pt
	}
	return params
}

// executeAutomationListAll 循环翻页聚合到 has_more=false（spec Rule-1-1，禁止静默漏项）。
func executeAutomationListAll(rctx *common.RuntimeContext, path string, params map[string]interface{}) error {
	all := make([]interface{}, 0, 16)
	token := ""
	for {
		p := make(map[string]interface{}, len(params)+1)
		for k, v := range params {
			p[k] = v
		}
		if token != "" {
			p["page_token"] = token
		}
		data, err := rctx.CallAPITyped("GET", path, p, nil)
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		all = append(all, common.GetSlice(data, "items")...)
		hasMore, next := common.PaginationMeta(data)
		if !hasMore || next == "" {
			break
		}
		token = next
	}
	out := map[string]interface{}{"items": all, "has_more": false}
	return outputAutomationList(rctx, out)
}

// outputAutomationList 输出 items + 分页提示。
func outputAutomationList(rctx *common.RuntimeContext, data map[string]interface{}) error {
	items := common.GetSlice(data, "items")
	rctx.OutFormat(data, nil, func(w io.Writer) {
		fmt.Fprintf(w, "%d trigger(s)\n", len(items))
		for _, it := range items {
			if m, ok := it.(map[string]interface{}); ok {
				fmt.Fprintf(w, "- %v  [%v]  %v\n", m["name"], m["trigger_type"], m["status"])
			}
		}
		fmt.Fprint(w, common.PaginationHint(data, len(items)))
	})
	return nil
}
