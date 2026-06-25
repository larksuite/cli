// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// fileSignMaxExpiresSeconds 是签名链接最长有效期（30 天）。超出 → 校验失败。
const fileSignMaxExpiresSeconds = 30 * 24 * 60 * 60

// AppsFileSign generates a temporary signed download URL for a file。
//
// POST /apps/{app_id}/storage/file_sign，body {path, expires_in}。
// pretty 模式只打 signed_url（便于直接管道 / curl）；json 返 {file_name,path,signed_url,expires_at}。
var AppsFileSign = common.Shortcut{
	Service:     appsService,
	Command:     "+file-sign",
	Description: "Generate a temporary signed download URL for a file",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +file-sign --app-id <app_id> --path /1858537546760216.png",
		"Tip: curl the signed_url directly to download.",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "path", Desc: "remote file path", Required: true},
		{Name: "expires-in", Type: "int", Default: "86400", Desc: "link validity in seconds (max 2592000 = 30d)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if _, err := requireFilePath(rctx.Str("path")); err != nil {
			return err
		}
		if rctx.Int("expires-in") > fileSignMaxExpiresSeconds {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--expires-in exceeds the maximum of %d seconds (30d)", fileSignMaxExpiresSeconds).WithParam("--expires-in")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			POST(appFileSignPath(appID)).
			Desc("Sign a temporary download URL").
			Body(buildFileSignBody(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("POST", appFileSignPath(appID), nil, buildFileSignBody(rctx))
		if err != nil {
			return err
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintln(w, common.GetString(data, "signed_url"))
		})
		return nil
	},
}

// buildFileSignBody 组装 file_sign 请求体：path 及可选 expires_in（秒）。
func buildFileSignBody(rctx *common.RuntimeContext) map[string]interface{} {
	path, _ := requireFilePath(rctx.Str("path"))
	body := map[string]interface{}{"path": path}
	if v := rctx.Int("expires-in"); v > 0 {
		body["expires_in"] = v
	}
	return body
}
