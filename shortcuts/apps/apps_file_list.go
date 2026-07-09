// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// AppsFileList lists files in a Miaoda app's storage (cursor pagination)。
//
// GET /apps/{app_id}/storage/file_list。过滤器：--name / --path / --type / --size-gt /
// --size-lt / --uploaded-since / --uploaded-until（精确或区间），分页 --page-size/--page-token。
// file 域不分 dev/online，无 --env。
//
// pretty 渲染 5 列：file_name / path / size / type / uploaded_at；空结果打 "No files found."。
// server 字段 created_at → 产品语义 uploaded_at。
var AppsFileList = common.Shortcut{
	Service:     appsService,
	Command:     "+file-list",
	Description: "List files in a Miaoda app's storage (cursor pagination)",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +file-list --app-id <app_id>",
		"Tip: filter fields with --jq, e.g. -q '.data.items[].path'",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "name", Desc: "filter by exact file name"},
		{Name: "path", Desc: "filter by exact remote path"},
		{Name: "type", Desc: "filter by MIME type"},
		{Name: "size-gt", Type: "int", Desc: "filter: size greater than (bytes)"},
		{Name: "size-lt", Type: "int", Desc: "filter: size less than (bytes)"},
		{Name: "uploaded-since", Desc: "filter: uploaded at or after; relative (7d/2h/30s) | date (2026-04-15) | datetime (2026-04-15T10:00:00) | ISO 8601 w/ TZ (bare date/datetime read in local timezone)"},
		{Name: "uploaded-until", Desc: "filter: uploaded at or before; relative (7d/2h/30s) | date (2026-04-15) | datetime (2026-04-15T10:00:00) | ISO 8601 w/ TZ (bare date/datetime read in local timezone)"},
		{Name: "page-size", Type: "int", Default: "20", Desc: "page size"},
		{Name: "page-token", Desc: "pagination cursor from previous response"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		// 设计原则三：<timestamp> 多格式 → 归一化为 RFC3339 UTC，回写到 flag 供 buildFileListParams 透传。
		for _, f := range []string{"uploaded-since", "uploaded-until"} {
			if strings.TrimSpace(rctx.Str(f)) == "" {
				continue
			}
			n, err := normalizeTimestamp(rctx.Str(f))
			if err != nil {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--%s: %v", f, err).WithParam("--" + f)
			}
			_ = rctx.Cmd.Flags().Set(f, n)
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			GET(appFileListPath(appID)).
			Desc("List Miaoda app files").
			Params(buildFileListParams(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("GET", appFileListPath(appID), buildFileListParams(rctx), nil)
		if err != nil {
			return err
		}
		// 白名单投影：server created_at/created_by → uploaded_at/uploaded_by，替换原始 items[]。
		items := projectFileItems(data["items"])
		data["items"] = items
		rctx.OutFormat(data, nil, func(w io.Writer) {
			renderFileListPretty(w, items)
		})
		return nil
	},
}

// projectFileItems 把服务端原始 items 逐项投影为白名单 fileInfo（created_*→uploaded_*）。
func projectFileItems(raw interface{}) []fileInfo {
	arr, _ := raw.([]interface{})
	out := make([]fileInfo, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]interface{}); ok {
			out = append(out, projectFileInfo(m))
		}
	}
	return out
}

// buildFileListParams 组装 file_list 查询参数：page_size 及可选 name/path/type/size_gt/size_lt/uploaded_since/uploaded_until/page_token。
func buildFileListParams(rctx *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{
		"page_size": rctx.Int("page-size"),
	}
	addStr := func(flag, key string) {
		if v := strings.TrimSpace(rctx.Str(flag)); v != "" {
			params[key] = v
		}
	}
	addStr("name", "name")
	addStr("path", "path")
	addStr("type", "type")
	addStr("uploaded-since", "uploaded_since")
	addStr("uploaded-until", "uploaded_until")
	addStr("page-token", "page_token")
	if v := rctx.Int("size-gt"); v > 0 {
		params["size_gt"] = v
	}
	if v := rctx.Int("size-lt"); v > 0 {
		params["size_lt"] = v
	}
	return params
}

// renderFileListPretty 5 列对齐表：file_name / path / size / type / uploaded_at。
func renderFileListPretty(w io.Writer, items []fileInfo) {
	if len(items) == 0 {
		io.WriteString(w, "No files found.\n")
		return
	}
	headers := []string{"file_name", "path", "size", "type", "uploaded_at"}
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, []string{
			dashIfEmpty(it.FileName),
			it.Path,
			humanBytes(it.SizeBytes),
			dashIfEmpty(it.Type),
			dashIfEmpty(it.UploadedAt),
		})
	}
	renderAlignedTable(w, headers, rows)
}
