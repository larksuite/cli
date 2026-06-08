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

// AppsDBTableSchema shows one table's structure.
//
// GET /apps/{app_id}/tables/{table_name}。
//
// `--format` 同时驱动 CLI 渲染和 server 请求形态：
//   - `--format json`（默认）/ table / ndjson / csv：CLI 不传 format query，response 含结构化
//     columns / indexes / constraints / stats，envelope 化输出。
//   - `--format pretty`：CLI 给 server 带 ?format=ddl，response 含 ddl 字符串，stdout 直接打
//     ddl 内容（无 envelope / 无表格包装）。
var AppsDBTableSchema = common.Shortcut{
	Service:     appsService,
	Command:     "+db-table-schema",
	Description: "Show a table's structure, columns, indexes and constraints",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +db-table-schema --app-id <app_id> --table <table>",
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
			return output.ErrValidation("--table is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			GET(appTablePath(appID, strings.TrimSpace(rctx.Str("table")))).
			Desc("Get Miaoda app db table schema").
			Params(buildDBTableSchemaParams(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		path := appTablePath(appID, strings.TrimSpace(rctx.Str("table")))
		data, err := rctx.CallAPITyped("GET", path, buildDBTableSchemaParams(rctx), nil)
		if err != nil {
			return err
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			// pretty 模式：stdout 直接打 ddl 文本（无 trailing newline，由 server 返回的字符串决定）。
			io.WriteString(w, common.GetString(data, "ddl"))
		})
		return nil
	},
}

// buildDBTableSchemaParams 构造 schema 接口的 query。
//
// CLI 检测 rctx.Format == "pretty" 时给 server 带 format=ddl，要求返 CREATE 语句文本；
// 其他 format（含默认 json）不传该参数，让 server 返默认结构化字段。
func buildDBTableSchemaParams(rctx *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{"env": rctx.Str("env")}
	if rctx.Format == "pretty" {
		params["format"] = "ddl"
	}
	return params
}
