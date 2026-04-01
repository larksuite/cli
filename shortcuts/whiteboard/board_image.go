// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

var BoardImage = common.Shortcut{
	Service:     "board",
	Command:     "+image",
	Description: "Download whiteboard as PNG image.",
	Risk:        "read",
	Scopes:      []string{"board:whiteboard:node:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "whiteboard-token", Desc: "Whiteboard token (required)", Required: true},
		{Name: "output", Desc: "Output file path (default: <whiteboard-token>.png)"},
	},
	HasFormat: false,
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validate.RejectControlChars(runtime.Str("whiteboard-token"), "whiteboard-token"); err != nil {
			return err
		}
		// 校验输出路径安全性，防止路径遍历攻击
		if out := runtime.Str("output"); out != "" {
			if _, err := validate.SafeOutputPath(out); err != nil {
				return err
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token := runtime.Str("whiteboard-token")
		return common.NewDryRunAPI().
			Desc("Download whiteboard as PNG image").
			GET(fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/download_as_image", common.MaskToken(url.PathEscape(token))))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("whiteboard-token")
		outputPath := runtime.Str("output")
		if outputPath == "" {
			// 对 token 进行 sanitize，防止路径分隔符导致写入非预期目录
			safeToken := strings.ReplaceAll(strings.ReplaceAll(token, "/", "_"), "\\", "_")
			if safeToken == "" {
				safeToken = "whiteboard"
			}
			outputPath = safeToken + ".png"
		}

		// 使用 SafeOutputPath 校验并解析为安全的绝对路径
		safePath, err := validate.SafeOutputPath(outputPath)
		if err != nil {
			return err
		}

		resp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod: http.MethodGet,
			ApiPath:    fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/download_as_image", url.PathEscape(token)),
		})
		if err != nil {
			return output.ErrNetwork(fmt.Sprintf("download whiteboard image failed: %v", err))
		}
		if resp.StatusCode != http.StatusOK {
			return output.ErrAPI(resp.StatusCode, string(resp.RawBody), nil)
		}

		// 校验响应是否为有效 PNG，防止 API 返回 JSON 错误时写入损坏文件
		pngMagic := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
		if len(resp.RawBody) < len(pngMagic) || !bytes.Equal(resp.RawBody[:len(pngMagic)], pngMagic) {
			// 尝试解析为 API JSON 错误响应
			var apiErr struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
			}
			if json.Unmarshal(resp.RawBody, &apiErr) == nil && apiErr.Code != 0 {
				return output.ErrAPI(apiErr.Code, apiErr.Msg, nil)
			}
			return output.Errorf(output.ExitInternal, "response", "expected PNG image but received invalid data")
		}

		// 确保目录存在
		dir := filepath.Dir(safePath)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return output.Errorf(output.ExitInternal, "filesystem", fmt.Sprintf("create directory failed: %v", err))
			}
		}

		if err := os.WriteFile(safePath, resp.RawBody, 0644); err != nil {
			return output.Errorf(output.ExitInternal, "filesystem", fmt.Sprintf("write file failed: %v", err))
		}

		fmt.Fprintf(runtime.IO().Out, "Image saved to %s (%d bytes)\n", outputPath, len(resp.RawBody))
		return nil
	},
}

// BoardNodes 获取画板所有节点
var BoardNodes = common.Shortcut{
	Service:     "board",
	Command:     "+nodes",
	Description: "List all nodes in a whiteboard.",
	Risk:        "read",
	Scopes:      []string{"board:whiteboard:node:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "whiteboard-token", Desc: "Whiteboard token (required)", Required: true},
	},
	HasFormat: true,
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validate.RejectControlChars(runtime.Str("whiteboard-token"), "whiteboard-token")
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token := runtime.Str("whiteboard-token")
		return common.NewDryRunAPI().
			Desc("List all nodes in whiteboard").
			GET(fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", common.MaskToken(url.PathEscape(token))))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("whiteboard-token")

		resp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod: http.MethodGet,
			ApiPath:    fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", url.PathEscape(token)),
		})
		if err != nil {
			return output.ErrNetwork(fmt.Sprintf("get whiteboard nodes failed: %v", err))
		}
		if resp.StatusCode != http.StatusOK {
			return output.ErrAPI(resp.StatusCode, string(resp.RawBody), nil)
		}

		// 检查 API 层面错误码
		var envelope struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if err := json.Unmarshal(resp.RawBody, &envelope); err != nil {
			return output.Errorf(output.ExitInternal, "parsing", fmt.Sprintf("parse response failed: %v", err))
		}
		if envelope.Code != 0 {
			return output.ErrAPI(envelope.Code, envelope.Msg, nil)
		}

		var raw map[string]any
		if err := json.Unmarshal(resp.RawBody, &raw); err != nil {
			return output.Errorf(output.ExitInternal, "parsing", fmt.Sprintf("parse response failed: %v", err))
		}

		runtime.OutFormat(raw, nil, func(w io.Writer) {
			// pretty 模式：统计节点数
			data, ok := raw["data"].(map[string]any)
			if !ok {
				fmt.Fprintf(w, "No data in response\n")
				return
			}
			nodes, ok := data["nodes"].([]any)
			if !ok {
				fmt.Fprintf(w, "No nodes found\n")
				return
			}
			fmt.Fprintf(w, "Whiteboard: %s\n", token)
			fmt.Fprintf(w, "Total nodes: %d\n", len(nodes))
			for i, n := range nodes {
				if node, ok := n.(map[string]any); ok {
					id, _ := node["id"].(string)
					nodeType, _ := node["type"].(string)
					fmt.Fprintf(w, "  [%d] id=%s type=%s\n", i+1, id, nodeType)
				}
			}
		})

		return nil
	},
}
