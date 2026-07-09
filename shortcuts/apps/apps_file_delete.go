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

// AppsFileDelete batch-deletes files by remote path（high-risk-write，框架自动注入 --yes 确认）。
//
// POST /apps/{app_id}/storage/file_batch_remove，body {paths:[...]}。网关把该路由注册为 POST
// （DELETE-with-body 不被网关支持，实测 DELETE→404 / POST→200）。后端 results[] 与请求 paths
// 顺序一一对应：成功项带 file，失败项带 error_code（CLI 据下标回填 path）。
// 部分失败整体仍 ok:true —— 失败项落在 data.results[].error，不翻成非 0 退出码（lark-cli 信封语义）。
var AppsFileDelete = common.Shortcut{
	Service:     appsService,
	Command:     "+file-delete",
	Description: "Delete one or more files by remote path (batch)",
	Risk:        "high-risk-write",
	Tips: []string{
		"Example: lark-cli apps +file-delete --app-id <app_id> --path /1858537546760216.png --yes",
		"Repeat --path for batch delete.",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "path", Type: "string_slice", Desc: "remote file path to delete (repeatable)", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if len(cleanDeletePaths(rctx)) == 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--path is required (at least one remote path)").WithParam("--path")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			POST(appFileBatchRemovePath(appID)).
			Desc("Batch delete Miaoda app files").
			Body(map[string]interface{}{"paths": cleanDeletePaths(rctx)})
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		paths := cleanDeletePaths(rctx)
		data, err := rctx.CallAPITyped("POST", appFileBatchRemovePath(appID), nil, map[string]interface{}{"paths": paths})
		if err != nil {
			return err
		}
		results := projectDeleteResults(data["results"], paths)
		out := map[string]interface{}{"results": results}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			renderFileDeletePretty(w, results)
		})
		return nil
	},
}

// cleanDeletePaths 取 --path 切片，trim 去空。
func cleanDeletePaths(rctx *common.RuntimeContext) []string {
	out := make([]string, 0)
	for _, p := range rctx.StrSlice("path") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// projectDeleteResults 把后端 results[] 按下标 zip 回请求 paths，回填 path，
// 失败项把 error_code 包成 {code,message} 便于消费。
func projectDeleteResults(raw interface{}, inputs []string) []map[string]interface{} {
	arr, _ := raw.([]interface{})
	out := make([]map[string]interface{}, 0, len(inputs))
	for i, input := range inputs {
		var r map[string]interface{}
		if i < len(arr) {
			r, _ = arr[i].(map[string]interface{})
		}
		status := "ok"
		if r != nil && common.GetString(r, "status") != "" {
			status = common.GetString(r, "status")
		}
		item := map[string]interface{}{"status": status, "path": input}
		if status == "ok" {
			if r != nil {
				if f, ok := r["file"].(map[string]interface{}); ok {
					item["file_name"] = common.GetString(f, "file_name")
				}
			}
		} else {
			code := ""
			if r != nil {
				code = common.GetString(r, "error_code")
			}
			if code == "" {
				code = "DELETE_FAILED"
			}
			item["error"] = map[string]interface{}{
				"code":    code,
				"message": deleteErrorMessage(code, input),
			}
		}
		out = append(out, item)
	}
	return out
}

// deleteErrorMessage 据 error_code 生成删除失败文案：FILE_NOT_FOUND 提示文件不存在，其余统一删除失败。
func deleteErrorMessage(code, path string) string {
	if code == "FILE_NOT_FOUND" {
		return fmt.Sprintf("File '%s' does not exist", path)
	}
	return fmt.Sprintf("Failed to delete '%s'", path)
}

// renderFileDeletePretty 逐项打 ✓ / ✗，末行汇总 deleted 计数。
func renderFileDeletePretty(w io.Writer, results []map[string]interface{}) {
	okCount := 0
	for _, r := range results {
		path := common.GetString(r, "path")
		if common.GetString(r, "status") == "ok" {
			fmt.Fprintf(w, "✓ %s\n", path)
			okCount++
			continue
		}
		code := ""
		if e, ok := r["error"].(map[string]interface{}); ok {
			code = common.GetString(e, "code")
		}
		fmt.Fprintf(w, "✗ %s (%s)\n", path, code)
	}
	fmt.Fprintf(w, "\n%d/%d deleted\n", okCount, len(results))
}
