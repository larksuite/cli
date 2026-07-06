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

const dbChangelogHint = "verify --app-id is correct; if targeting --environment dev, create it first with `lark-cli apps +db-env-create --app-id <app_id> --environment dev`"

// AppsDBChangelogList 列出应用数据库的 DDL 变更记录（建表/改表/索引等结构变更追溯）。
//
// GET /apps/{app_id}/db/changelog_list（cursor 分页）。过滤：--table、--since/--until（多格式时间）。
// --change-id 精确查单条（命中返单条、否则空）。operator 后端以 JSON 字符串透传 {id,name}，
// json 还原成对象、pretty 只展示 name。
var AppsDBChangelogList = common.Shortcut{
	Service:     appsService,
	Command:     "+db-changelog-list",
	Description: "List a Miaoda app database's DDL change history (cursor pagination)",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +db-changelog-list --app-id <app_id>",
		"Pin a single change with --change-id; filter time with --since 7d / --until 2026-04-15.",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: append([]common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "table", Desc: "filter by target table"},
		{Name: "change-id", Desc: "look up a single change by id (returns that one record only)"},
		{Name: "since", Desc: "filter: changed at or after; relative (7d/2h) | date | datetime | ISO 8601 w/ TZ (bare date/datetime read in local timezone)"},
		{Name: "until", Desc: "filter: changed at or before; same formats as --since"},
		{Name: "page-size", Type: "int", Default: "20", Desc: "page size"},
		{Name: "page-token", Desc: "pagination cursor from previous response"},
	}, dbEnvFlags("", []string{"dev", "online"}, "target db environment; leave unset to auto-select (multi-env app uses dev, single-env uses online), or pass dev/online")...),
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if err := rejectLegacyEnvFlag(rctx); err != nil {
			return err
		}
		return normalizeTimeFlags(rctx, "since", "until")
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			GET(appChangelogListPath(appID)).
			Desc("List Miaoda app DDL changelog").
			Params(buildChangelogParams(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("GET", appChangelogListPath(appID), buildChangelogParams(rctx), nil)
		if err != nil {
			return withAppsHint(err, dbChangelogHint)
		}
		items := projectChangelogItems(data["items"])
		data["items"] = items
		changeID := strings.TrimSpace(rctx.Str("change-id"))
		rctx.OutFormat(data, nil, func(w io.Writer) {
			renderChangelogPretty(w, items, changeID)
		})
		return nil
	},
}

// buildChangelogParams 组装 changelog_list 查询参数：env / page_size 及可选 table/change_id/since/until/page_token。
func buildChangelogParams(rctx *common.RuntimeContext) map[string]interface{} {
	params := dbEnvParams(rctx, map[string]interface{}{
		"page_size": rctx.Int("page-size"),
	})
	addStr := func(flag, key string) {
		if v := strings.TrimSpace(rctx.Str(flag)); v != "" {
			params[key] = v
		}
	}
	addStr("table", "table")
	addStr("change-id", "change_id")
	addStr("since", "since")
	addStr("until", "until")
	addStr("page-token", "page_token")
	return params
}

type changelogItem struct {
	ChangeID    string       `json:"change_id"`
	ChangedAt   string       `json:"changed_at"`
	Operator    *operatorRef `json:"operator,omitempty"`
	TargetTable string       `json:"target_table"`
	ChangeType  string       `json:"change_type"`
	Summary     string       `json:"summary"`
	Statement   string       `json:"statement,omitempty"`
}

// projectChangelogItems 把服务端原始 DDL 变更记录投影为白名单 changelogItem（operator 解析成对象）。
func projectChangelogItems(raw interface{}) []changelogItem {
	arr, _ := raw.([]interface{})
	out := make([]changelogItem, 0, len(arr))
	for _, it := range arr {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		out = append(out, changelogItem{
			ChangeID:    common.GetString(m, "change_id"),
			ChangedAt:   common.GetString(m, "changed_at"),
			Operator:    parseOperator(common.GetString(m, "operator")),
			TargetTable: common.GetString(m, "target_table"),
			ChangeType:  common.GetString(m, "change_type"),
			Summary:     common.GetString(m, "summary"),
			Statement:   common.GetString(m, "statement"),
		})
	}
	return out
}

// renderChangelogPretty 6 列：change_id / changed_at / operator(name) / target_table / change_type / summary。
func renderChangelogPretty(w io.Writer, items []changelogItem, changeID string) {
	if len(items) == 0 {
		if changeID != "" {
			fmt.Fprintf(w, "No DDL change with id=%s found.\n", changeID)
		} else {
			io.WriteString(w, "No DDL changes found.\n")
		}
		return
	}
	headers := []string{"change_id", "changed_at", "operator", "target_table", "change_type", "summary"}
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, []string{
			it.ChangeID,
			dashIfEmpty(it.ChangedAt),
			operatorName(it.Operator),
			dashIfEmpty(it.TargetTable),
			it.ChangeType,
			dashIfEmpty(it.Summary),
		})
	}
	renderAlignedTable(w, headers, rows)
}
