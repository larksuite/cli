// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

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
		return validate.RejectControlChars(runtime.Str("whiteboard-token"), "whiteboard-token")
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
			outputPath = token + ".png"
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

		// 确保目录存在
		dir := filepath.Dir(outputPath)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return output.Errorf(output.ExitInternal, "filesystem", fmt.Sprintf("create directory failed: %v", err))
			}
		}

		if err := os.WriteFile(outputPath, resp.RawBody, 0644); err != nil {
			return output.Errorf(output.ExitInternal, "filesystem", fmt.Sprintf("write file failed: %v", err))
		}

		fmt.Fprintf(os.Stdout, "Image saved to %s (%d bytes)\n", outputPath, len(resp.RawBody))
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

		// 直接输出 JSON 响应
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
