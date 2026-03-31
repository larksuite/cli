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
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// maxDiagramSize 限制图表文件最大 10MB，防止内存耗尽
const maxDiagramSize = 10 * 1024 * 1024

// syntaxType 映射：plantuml=1, mermaid=2
func parseSyntaxType(s string) int {
	switch strings.ToLower(s) {
	case "mermaid":
		return 2
	default:
		return 1 // plantuml
	}
}

// styleType 映射：board=1, classic=2
func parseStyleType(s string) int {
	switch strings.ToLower(s) {
	case "classic":
		return 2
	default:
		return 1 // board
	}
}

// diagramType 映射
func parseDiagramType(s string) int {
	switch strings.ToLower(s) {
	case "mindmap":
		return 1
	case "sequence":
		return 2
	case "activity":
		return 3
	case "class":
		return 4
	case "er":
		return 5
	case "flowchart":
		return 6
	case "state":
		return 7
	case "component":
		return 8
	default:
		return 0 // auto
	}
}

var BoardImport = common.Shortcut{
	Service:     "board",
	Command:     "+import",
	Description: "Import Mermaid or PlantUML diagram into a whiteboard. Reads diagram code from --file or stdin.",
	Risk:        "write",
	Scopes:      []string{"board:whiteboard:node:create"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "whiteboard-token", Desc: "Whiteboard token (required)", Required: true},
		{Name: "syntax", Desc: "Diagram syntax: plantuml | mermaid (default: plantuml)", Default: "plantuml", Enum: []string{"plantuml", "mermaid"}},
		{Name: "diagram-type", Desc: "Diagram type: auto|mindmap|sequence|activity|class|er|flowchart|state|component (default: auto)", Default: "auto", Enum: []string{"auto", "mindmap", "sequence", "activity", "class", "er", "flowchart", "state", "component"}},
		{Name: "style", Desc: "Style: board | classic (default: board)", Default: "board", Enum: []string{"board", "classic"}},
		{Name: "file", Desc: "Path to diagram file (reads from stdin if omitted)"},
		{Name: "content", Desc: "Inline diagram code (alternative to --file or stdin)"},
	},
	HasFormat: true,
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validate.RejectControlChars(runtime.Str("whiteboard-token"), "whiteboard-token"); err != nil {
			return err
		}
		// 校验文件路径安全性，防止路径遍历攻击
		if file := runtime.Str("file"); file != "" {
			if _, err := validate.SafeInputPath(file); err != nil {
				return err
			}
		}
		// 必须提供 --file、--content 或 stdin 之一
		file := runtime.Str("file")
		content := runtime.Str("content")
		if file == "" && content == "" {
			stat, err := os.Stdin.Stat()
			if err != nil || (stat.Mode()&os.ModeCharDevice) != 0 {
				return common.FlagErrorf("one of --file, --content, or stdin is required")
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token := runtime.Str("whiteboard-token")
		syntax := runtime.Str("syntax")
		return common.NewDryRunAPI().
			Desc(fmt.Sprintf("Import %s diagram to whiteboard", syntax)).
			POST(fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/plantuml", common.MaskToken(url.PathEscape(token))))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("whiteboard-token")
		syntax := runtime.Str("syntax")
		diagramType := runtime.Str("diagram-type")
		style := runtime.Str("style")
		file := runtime.Str("file")
		content := runtime.Str("content")

		// 获取图表代码
		var code string
		switch {
		case content != "":
			if len(content) > maxDiagramSize {
				return output.Errorf(output.ExitValidation, "content", "inline diagram code exceeds 10 MB limit")
			}
			code = content
		case file != "":
			// 使用 SafeInputPath 解析安全路径
			safePath, err := validate.SafeInputPath(file)
			if err != nil {
				return err
			}
			info, err := os.Stat(safePath)
			if err != nil {
				return output.Errorf(output.ExitValidation, "file", fmt.Sprintf("stat diagram file failed: %v", err))
			}
			if info.Size() > maxDiagramSize {
				return output.Errorf(output.ExitValidation, "file", "diagram file exceeds 10 MB limit")
			}
			data, err := os.ReadFile(safePath)
			if err != nil {
				return output.Errorf(output.ExitValidation, "file", fmt.Sprintf("read diagram file failed: %v", err))
			}
			code = string(data)
		default:
			data, err := io.ReadAll(io.LimitReader(os.Stdin, maxDiagramSize+1))
			if err != nil {
				return output.Errorf(output.ExitValidation, "stdin", fmt.Sprintf("read stdin failed: %v", err))
			}
			if int64(len(data)) > maxDiagramSize {
				return output.Errorf(output.ExitValidation, "stdin", "stdin input exceeds 10 MB limit")
			}
			code = string(data)
		}

		if strings.TrimSpace(code) == "" {
			return output.ErrValidation("diagram code is empty")
		}

		// 构建请求
		reqBody := map[string]any{
			"plant_uml_code": code,
			"syntax_type":    parseSyntaxType(syntax),
			"style_type":     parseStyleType(style),
			"diagram_type":   parseDiagramType(diagramType),
		}
		resp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod: http.MethodPost,
			ApiPath:    fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/plantuml", url.PathEscape(token)),
			Body:       reqBody,
		})
		if err != nil {
			return output.ErrNetwork(fmt.Sprintf("import diagram failed: %v", err))
		}
		if resp.StatusCode != http.StatusOK {
			return output.ErrAPI(resp.StatusCode, string(resp.RawBody), nil)
		}

		var apiResp struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				NodeID   string `json:"node_id"`
				TicketID string `json:"ticket_id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
			return output.Errorf(output.ExitInternal, "parsing", fmt.Sprintf("parse response failed: %v", err))
		}
		if apiResp.Code != 0 {
			return output.ErrAPI(apiResp.Code, apiResp.Msg, nil)
		}

		nodeID := apiResp.Data.NodeID
		if nodeID == "" {
			nodeID = apiResp.Data.TicketID
		}

		outData := map[string]string{
			"whiteboard_token": token,
			"node_id":          nodeID,
			"syntax":           syntax,
			"style":            style,
		}

		runtime.OutFormat(outData, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Diagram imported successfully!\n")
			fmt.Fprintf(w, "  Whiteboard: %s\n", token)
			if nodeID != "" {
				fmt.Fprintf(w, "  Node ID:    %s\n", nodeID)
			}
			fmt.Fprintf(w, "  Syntax:     %s\n", syntax)
			fmt.Fprintf(w, "  Style:      %s\n", style)
		})

		return nil
	},
}
