// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"io"

	"github.com/larksuite/cli/shortcuts/common"
)

// AppsFileGet gets one file's metadata by exact remote path（动词对齐 +file-list）。
//
// GET /apps/{app_id}/storage/file?path=<path>。file 仅按 path 精确寻址，无按名寻址。
// pretty 渲染 key/value：file_name / path / size(含 bytes) / type / uploaded_by(只 name) / uploaded_at /
// download_url（条件出现）。server created_at/created_by → uploaded_at/uploaded_by。
var AppsFileGet = common.Shortcut{
	Service:     appsService,
	Command:     "+file-get",
	Description: "Get a single file's metadata by path",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +file-get --app-id <app_id> --path /1858537546760216.png",
		"Tip: extract a single field with --jq, e.g. -q '.size_bytes' or -q '.download_url'",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "path", Desc: "remote file path", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		_, err := requireFilePath(rctx.Str("path"))
		return err
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			GET(appFileGetPath(appID)).
			Desc("Get Miaoda app file metadata").
			Params(buildFileGetParams(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("GET", appFileGetPath(appID), buildFileGetParams(rctx), nil)
		if err != nil {
			return err
		}
		info := projectFileInfo(data)
		rctx.OutFormat(info, nil, func(w io.Writer) {
			renderFileGetPretty(w, info)
		})
		return nil
	},
}

// buildFileGetParams 组装 file_get 查询参数：按 path 精确寻址单文件。
func buildFileGetParams(rctx *common.RuntimeContext) map[string]interface{} {
	path, _ := requireFilePath(rctx.Str("path"))
	return map[string]interface{}{"path": path}
}

// renderFileGetPretty 输出对齐 key/value；uploaded_by 只展示 name（id 仅 json 保留）。
func renderFileGetPretty(w io.Writer, info fileInfo) {
	pairs := [][2]string{
		{"file_name", dashIfEmpty(info.FileName)},
		{"path", info.Path},
		{"size", fileSizeDetail(info.SizeBytes)},
		{"type", dashIfEmpty(info.Type)},
	}
	if info.UploadedBy != nil {
		pairs = append(pairs, [2]string{"uploaded_by", info.UploadedBy.Name})
	}
	pairs = append(pairs, [2]string{"uploaded_at", dashIfEmpty(info.UploadedAt)})
	if info.DownloadURL != "" {
		pairs = append(pairs, [2]string{"download_url", info.DownloadURL})
	}
	renderKeyValuePairs(w, pairs)
}
