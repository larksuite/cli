// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"io"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

const dbTableGetHint = "verify --app-id and --table are correct; list tables with `lark-cli apps +db-table-list --app-id <app_id>`; if targeting --env dev, create it first with `lark-cli apps +db-env-create --app-id <app_id> --env dev`"

// AppsDBTableGet gets one table's structure (动词对齐 +db-table-list)。
//
// GET /apps/{app_id}/tables/{table_name}。
//
// `--format` 同时驱动 CLI 渲染和 server 请求形态：
//   - `--format json`（默认）/ table / ndjson / csv：CLI 不传 format query，response 含结构化
//     columns / indexes / constraints / stats，envelope 化输出。
//   - `--format pretty`：CLI 给 server 带 ?format=ddl，response 含 ddl 字符串，stdout 直接打
//     ddl 内容（无 envelope / 无表格包装）。
var AppsDBTableGet = common.Shortcut{
	Service:     appsService,
	Command:     "+db-table-get",
	Description: "Get a table's structure: columns, indexes and constraints",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +db-table-get --app-id <app_id> --table <table>",
		"Tip: filter fields with --jq (json format), e.g. -q '.data.columns[].name'",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "table", Desc: "table name", Required: true},
		{Name: "env", Default: "online", Enum: []string{"dev", "online"}, Desc: "target db environment"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if strings.TrimSpace(rctx.Str("table")) == "" {
			return appsValidationParamError("--table", "--table is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			GET(appTablePath(appID, strings.TrimSpace(rctx.Str("table")))).
			Desc("Get Miaoda app db table schema").
			Params(buildDBTableGetParams(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		path := appTablePath(appID, strings.TrimSpace(rctx.Str("table")))
		data, err := rctx.CallAPITyped("GET", path, buildDBTableGetParams(rctx), nil)
		if err != nil {
			return withAppsHint(err, dbTableGetHint)
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			// pretty 模式：stdout 直接打 ddl 文本（无 trailing newline，由 server 返回的字符串决定）。
			io.WriteString(w, common.GetString(data, "ddl"))
		})
		return nil
	},
}

// buildDBTableGetParams 构造 schema 接口的 query。
//
// CLI 检测 rctx.Format == "pretty" 时给 server 带 format=ddl，要求返 CREATE 语句文本；
// 其他 format（含默认 json）不传该参数，让 server 返默认结构化字段。
func buildDBTableGetParams(rctx *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{"env": rctx.Str("env")}
	if rctx.Format == "pretty" {
		params["format"] = "ddl"
	}
	return params
}
