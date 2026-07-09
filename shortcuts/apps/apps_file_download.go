// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/shortcuts/common"
)

// AppsFileDownload downloads a file to a local path via a signed URL。
//
// 两步：POST /apps/{app_id}/storage/file_sign 拿 signed_url（presigned，直连对象存储），
// 再客户端 GET signed_url 落盘到 --output（默认远端 basename）。不单设 download 接口。
var AppsFileDownload = common.Shortcut{
	Service:     appsService,
	Command:     "+file-download",
	Description: "Download a file to a local path (via a signed URL)",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +file-download --app-id <app_id> --path /1858537546760216.png --output ./logo.png",
		"Example (omit --output): lark-cli apps +file-download --app-id <app_id> --path /1858537546760216.png   # saves to ./1858537546760216.png",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "path", Desc: "remote file path", Required: true},
		{Name: "output", Desc: "local output path (default: remote file basename in cwd)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if err := rejectOutputTraversal(rctx.Str("output")); err != nil {
			return err
		}
		_, err := requireFilePath(rctx.Str("path"))
		return err
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		remotePath, _ := requireFilePath(rctx.Str("path"))
		return common.NewDryRunAPI().
			POST(appFileSignPath(appID)).
			Desc("Sign a download URL, then GET it to --output").
			Body(map[string]interface{}{"path": remotePath})
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		remotePath, err := requireFilePath(rctx.Str("path"))
		if err != nil {
			return err
		}

		// 1. 签名拿 presigned signed_url。
		signData, err := rctx.CallAPITyped("POST", appFileSignPath(appID), nil, map[string]interface{}{"path": remotePath})
		if err != nil {
			return err
		}
		signedURL := common.GetString(signData, "signed_url")
		if signedURL == "" {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "sign returned no signed_url")
		}

		// 2. 直连 GET signed_url 落盘。
		out := strings.TrimSpace(rctx.Str("output"))
		if out == "" {
			out = path.Base(strings.TrimPrefix(remotePath, "/"))
			if out == "" || out == "." || out == "/" {
				out = "download"
			}
		}
		req, err := http.NewRequestWithContext(rctx.Ctx(), http.MethodGet, signedURL, nil) //nolint:forbidigo // GET from a presigned object-storage URL bypasses the Lark gateway; raw HTTP required (not a Lark API call).
		if err != nil {
			return errs.NewNetworkError(errs.SubtypeNetworkTransport, "build download request").WithCause(err)
		}
		resp, err := newFileTransferClient().Do(req) //nolint:forbidigo // see above: direct presigned-URL download, RuntimeContext.DoAPI does not apply.
		if err != nil {
			// dial/transport 失败是典型可重试场景。
			return errs.NewNetworkError(errs.SubtypeNetworkTransport, "download failed").WithCause(err).WithRetryable()
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			// 5xx 是上游瞬时故障，标 retryable；4xx（如签名过期）需重新签名而非盲重试，不标。
			if resp.StatusCode >= 500 {
				return errs.NewNetworkError(errs.SubtypeNetworkServer, "download failed: HTTP %d", resp.StatusCode).WithRetryable()
			}
			return errs.NewNetworkError(errs.SubtypeNetworkTransport, "download failed: HTTP %d", resp.StatusCode)
		}
		saved, err := rctx.FileIO().Save(out, fileio.SaveOptions{
			ContentType:   resp.Header.Get("Content-Type"),
			ContentLength: resp.ContentLength,
		}, resp.Body)
		if err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output: %v", err).WithParam("--output").WithCause(err)
		}
		resolved, perr := rctx.FileIO().ResolvePath(out)
		if perr != nil || resolved == "" {
			resolved = out
		}
		result := map[string]interface{}{
			"path":       remotePath,
			"output":     resolved,
			"size_bytes": saved.Size(),
		}
		rctx.OutFormat(result, nil, func(w io.Writer) {
			fmt.Fprintf(w, "✓ Downloaded %s → %s (%s)\n", remotePath, resolved, humanBytes(saved.Size()))
		})
		return nil
	},
}
