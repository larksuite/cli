// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
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

// executeAutomationListAll 循环翻页聚合到 has_more=false（禁止静默漏项）。
// 用页数上限 + 已见 token 检测防止后端非收敛响应导致无限循环。
const automationListAllMaxPages = 100

func executeAutomationListAll(rctx *common.RuntimeContext, path string, params map[string]interface{}) error {
	all := make([]interface{}, 0, 16)
	seen := map[string]struct{}{}
	token := ""
	for pages := 0; ; pages++ {
		if pages >= automationListAllMaxPages {
			return errs.NewInternalError(errs.SubtypeInvalidResponse,
				"pagination did not converge after %d pages", automationListAllMaxPages)
		}
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
		if _, ok := seen[next]; ok {
			return errs.NewInternalError(errs.SubtypeInvalidResponse,
				"pagination did not converge: page_token %q repeated", next)
		}
		seen[next] = struct{}{}
		token = next
	}
	out := map[string]interface{}{"items": all, "has_more": false}
	return outputAutomationList(rctx, out)
}

// outputAutomationList 输出 items + 分页提示。逐条对 items 套 redactWebhookToken，
// 抹掉 trigger_condition.token_value（list/get 恒不返回明文 Bearer Token）；
// 同时覆盖单页与 --all 聚合路径（executeAutomationListAll 也走这里）。
func outputAutomationList(rctx *common.RuntimeContext, data map[string]interface{}) error {
	items := common.GetSlice(data, "items")
	redacted := make([]interface{}, 0, len(items))
	for _, it := range items {
		if m, ok := it.(map[string]interface{}); ok {
			redacted = append(redacted, redactWebhookToken(m))
		} else {
			redacted = append(redacted, it)
		}
	}
	// 保留分页字段供 PaginationHint/PaginationMeta 读取（读的是同一个 map）。
	out := map[string]interface{}{
		"items":      redacted,
		"has_more":   data["has_more"],
		"page_token": data["page_token"],
	}
	rctx.OutFormat(out, nil, func(w io.Writer) {
		fmt.Fprintf(w, "%d trigger(s)\n", len(redacted))
		for _, it := range redacted {
			if m, ok := it.(map[string]interface{}); ok {
				fmt.Fprintf(w, "- %v  [%v]  %v\n", m["name"], m["trigger_type"], m["status"])
			}
		}
		fmt.Fprint(w, common.PaginationHint(out, len(redacted)))
	})
	return nil
}
