// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
package whiteboard

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	"code.byted.org/lark/larksuite-cli/internal/output"
	"code.byted.org/lark/larksuite-cli/internal/validate"
	"code.byted.org/lark/larksuite-cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

var WhiteboardPreview = common.Shortcut{
	Service:     "whiteboard",
	Command:     "+export",
	Description: "Export a existing whiteboard.",
	Risk:        "read",
	Scopes:      []string{"board:whiteboard:node:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "whiteboard-token", Desc: "whiteboard token of the whiteboard. You will need read permission to download preview image.", Required: true},
		{Name: "output", Desc: "output directory. If not specified, preview will be saved on local directory. Preview image will be named as \"whiteboard-preview-{whiteboard-token}.png\"", Required: false},
	},
	HasFormat: true,
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		// 检查 token 是否包含控制字符
		token := runtime.Str("whiteboard-token")
		if err := validate.RejectControlChars(token, "whiteboard-token"); err != nil {
			return err
		}
		output := runtime.Str("output")
		if output == "" {
			_, err := validate.SafeOutputPath(fmt.Sprintf("whiteboard-preview-%s.png", token))
			if err != nil {
				return err
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			GET(fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/download_as_image", runtime.Str("whiteboard-token"))).
			Desc("GetPreview image of given whiteboard")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		// 构建 API 请求
		token := runtime.Str("whiteboard-token")
		outDir := runtime.Str("output")
		outDir = filepath.Join(outDir, fmt.Sprintf("whiteboard-preview-%s.png", token))
		req := &larkcore.ApiReq{
			HttpMethod: http.MethodGet,
			ApiPath:    fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/download_as_image", token),
		}
		// 执行 API 请求
		resp, err := runtime.DoAPI(req)
		if err != nil {
			return output.ErrNetwork(fmt.Sprintf("get whiteboard preview failed: %v", err))
		}
		// 检查响应状态码
		if resp.StatusCode != http.StatusOK {
			return output.ErrAPI(resp.StatusCode, string(resp.RawBody), nil)
		}
		if err := validate.AtomicWrite(outDir, resp.RawBody, 0644); err != nil {
			return output.Errorf(output.ExitInternal, "api_error", "cannot save preview image: %s", err)
		}
		runtime.OutFormat(map[string]interface{}{
			"saved_path": outDir,
			"size_bytes": len(resp.RawBody),
		}, nil, nil)
		return nil
	},
}
