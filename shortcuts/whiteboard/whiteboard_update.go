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
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	// FormatRaw sends raw whiteboard node JSON to the create-nodes API.
	FormatRaw = "raw"
	// FormatPlantUML sends PlantUML source through the diagram import API.
	FormatPlantUML = "plantuml"
	// FormatMermaid sends Mermaid source through the diagram import API.
	FormatMermaid = "mermaid"
	// FormatSVG sends SVG source through the diagram import API.
	FormatSVG = "svg"
)

var formatCodeMap = map[string]int{
	FormatRaw:      0,
	FormatPlantUML: 1,
	FormatMermaid:  2,
	FormatSVG:      3,
}

var wbUpdateScopes = []string{"board:whiteboard:node:create"}
var wbUpdateAuthTypes = []string{"user", "bot"}
var wbUpdateFlags = []common.Flag{
	{Name: "idempotent-token", Desc: "idempotent token to ensure the update is idempotent. Default is empty. min length is 10.", Required: false},
	{Name: "whiteboard-token", Desc: "whiteboard token of the whiteboard to update. You will need edit permission to update the whiteboard.", Required: true},
	{Name: "overwrite", Desc: "overwrite the whiteboard content, delete all existing content before update. Default is false.", Required: false, Type: "bool"},
	{Name: "source", Desc: "Input whiteboard data.", Required: true, Input: []string{common.Stdin, common.File}},
	{Name: "input_format", Desc: "format of input data: raw | plantuml | mermaid | svg. Default is raw.", Required: false},
}

func wbUpdateValidate(ctx context.Context, runtime *common.RuntimeContext) error {
	// 检查 token 是否包含控制字符（空字符串下自动跳过了）
	if err := common.RejectDangerousCharsTyped("--whiteboard-token", runtime.Str("whiteboard-token")); err != nil {
		return err
	}
	itoken := runtime.Str("idempotent-token")
	if err := common.RejectDangerousCharsTyped("--idempotent-token", itoken); err != nil {
		return err
	}
	if itoken != "" && len(itoken) < 10 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--idempotent-token must be at least 10 characters long.").WithParam("--idempotent-token")
	}

	// 检查 --input_format 标志
	format := getFormat(runtime)
	if format != FormatRaw && format != FormatPlantUML && format != FormatMermaid && format != FormatSVG {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--input_format must be one of: raw | plantuml | mermaid | svg").WithParam("--input_format")
	}
	return nil
}

// getFormat 获取 format，默认返回 raw
func getFormat(runtime *common.RuntimeContext) string {
	format := runtime.Str("input_format")
	if format == "" {
		return FormatRaw
	}
	return format
}

func wbUpdateDryRun(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	// 读取输入内容
	input := runtime.Str("source")
	if input == "" {
		return common.NewDryRunAPI().Desc("read input failed: source is required")
	}
	format := getFormat(runtime)
	token := runtime.Str("whiteboard-token")
	overwrite := runtime.Bool("overwrite")
	descStr := "will call whiteboard open api to update content."
	desc := common.NewDryRunAPI().Desc(descStr)

	switch format {
	case FormatRaw:
		nodes, err, _ := parseWBcliNodes([]byte(input))
		if err != nil {
			return common.NewDryRunAPI().Desc("parse input failed: " + err.Error())
		}
		reqBody := rawNodesCreateReq{
			Nodes:     nodes,
			Overwrite: overwrite,
		}
		desc.POST(fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", common.MaskToken(url.PathEscape(token)))).Body(reqBody).Desc("create all nodes of the whiteboard.")
	case FormatPlantUML, FormatMermaid, FormatSVG:
		syntaxType := formatCodeMap[format]
		reqBody := plantumlCreateReq{
			PlantUmlCode: input,
			SyntaxType:   syntaxType,
			ParseMode:    1,
			DiagramType:  0,
			Overwrite:    overwrite,
		}
		desc.POST(fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/plantuml", common.MaskToken(url.PathEscape(token)))).Body(reqBody).Desc(fmt.Sprintf("create %s node on the whiteboard.", format))
	}

	return desc
}

func wbUpdateExecute(ctx context.Context, runtime *common.RuntimeContext) error {
	token := runtime.Str("whiteboard-token")
	overwrite := runtime.Bool("overwrite")
	idempotentToken := runtime.Str("idempotent-token")
	format := getFormat(runtime)

	input := runtime.Str("source")
	if input == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "read input failed: source is required").WithParam("--source")
	}

	switch format {
	case FormatRaw:
		return updateWhiteboardByRawNodes(ctx, runtime, token, []byte(input), overwrite, idempotentToken)
	case FormatPlantUML, FormatMermaid, FormatSVG:
		return updateWhiteboardByCode(ctx, runtime, token, []byte(input), format, overwrite, idempotentToken)
	default:
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsupported format: %s", format).WithParam("--input_format")
	}
}

// WhiteboardUpdateDescription describes the whiteboard update shortcut.
const WhiteboardUpdateDescription = "Update an existing whiteboard in lark document with mermaid, plantuml or whiteboard dsl. refer to lark-whiteboard skill for more details."

// WhiteboardUpdate registers the `whiteboard +update` shortcut.
var WhiteboardUpdate = common.Shortcut{
	Service:     "whiteboard",
	Command:     "+update",
	Description: WhiteboardUpdateDescription,
	Risk:        "write",
	Scopes:      wbUpdateScopes,
	AuthTypes:   wbUpdateAuthTypes,
	Flags:       wbUpdateFlags,
	HasFormat:   false, // 不使用 lark 的 format flag（使用画板内部的格式）
	Validate:    wbUpdateValidate,
	DryRun:      wbUpdateDryRun,
	Execute:     wbUpdateExecute,
}

// WhiteboardUpdateOld 向前兼容历史版本 Doc 域下的更新命令
var WhiteboardUpdateOld = common.Shortcut{
	Service:     "docs",
	Command:     "+whiteboard-update",
	Description: WhiteboardUpdateDescription,
	Risk:        "write",
	Scopes:      wbUpdateScopes,
	AuthTypes:   wbUpdateAuthTypes,
	Flags:       wbUpdateFlags,
	HasFormat:   false, // 不使用 lark 的 format flag（使用画板内部的格式）
	Validate:    wbUpdateValidate,
	DryRun:      wbUpdateDryRun,
	Execute:     wbUpdateExecute,
}

type plantumlCreateReq struct {
	PlantUmlCode string `json:"plant_uml_code"`
	SyntaxType   int    `json:"syntax_type"`
	DiagramType  int    `json:"diagram_type,omitempty"`
	ParseMode    int    `json:"parse_mode,omitempty"`
	Overwrite    bool   `json:"overwrite,omitempty"`
}

type rawNodesCreateReq struct {
	Nodes     []interface{} `json:"nodes"`
	Overwrite bool          `json:"overwrite,omitempty"`
}

func parseWBcliNodes(rawjson []byte) (wbNodes []interface{}, err error, isRaw bool) {
	var wbOutput WbCliOutput
	if err := json.Unmarshal(rawjson, &wbOutput); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "unmarshal input json failed: %v", err).WithParam("--source").WithCause(err), false
	}
	if (wbOutput.Code != 0 || wbOutput.Data.To != "openapi") && wbOutput.RawNodes == nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "whiteboard-cli failed. please check previous log.").WithParam("--source"), false
	}
	if wbOutput.RawNodes != nil {
		wbNodes = wbOutput.RawNodes
		isRaw = true
	} else {
		if wbOutput.Data.Result.Nodes == nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "whiteboard-cli failed. please check previous log.").WithParam("--source"), false
		}
		wbNodes = wbOutput.Data.Result.Nodes
	}
	return wbNodes, nil, isRaw
}

// updateWhiteboardByCode 使用 plantuml/mermaid 代码更新画板
func updateWhiteboardByCode(ctx context.Context, runtime *common.RuntimeContext, wbToken string, input []byte, format string, overwrite bool, idempotentToken string) error {
	syntaxType := formatCodeMap[format]
	reqBody := plantumlCreateReq{
		PlantUmlCode: string(input),
		SyntaxType:   syntaxType,
		ParseMode:    1,
		DiagramType:  0, // 0 表示自动识别
		Overwrite:    overwrite,
	}

	params := map[string]interface{}{}
	if idempotentToken != "" {
		params["client_token"] = idempotentToken
	}

	data, err := runtime.CallAPITyped(http.MethodPost, fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/plantuml", url.PathEscape(wbToken)), params, reqBody)
	if err != nil {
		return err
	}

	nodeID := common.GetString(data, "node_id")
	if nodeID == "" {
		return wbInvalidResponse("update whiteboard by code failed: missing data.node_id")
	}
	outData := map[string]string{"created_node_id": nodeID}
	runtime.OutFormat(outData, nil, func(w io.Writer) {
		fmt.Fprintf(w, "New node created.\n")
		fmt.Fprintf(w, "Update whiteboard success")
	})

	return nil
}

// updateWhiteboardByRawNodes 使用原始 Open API 格式数据更新画板
func updateWhiteboardByRawNodes(ctx context.Context, runtime *common.RuntimeContext, wbToken string, input []byte, overwrite bool, idempotentToken string) error {
	nodes, err, isRaw := parseWBcliNodes(input)
	if err != nil {
		return err
	}
	reqBody := rawNodesCreateReq{
		Nodes:     nodes,
		Overwrite: overwrite,
	}

	params := map[string]interface{}{}
	if idempotentToken != "" {
		params["client_token"] = idempotentToken
	}

	data, err := runtime.CallAPITyped(http.MethodPost, fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", url.PathEscape(wbToken)), params, reqBody)
	if err != nil {
		// Raw open-api JSON is hand-edited far more often than the DSL path, so
		// steer the user back to the recommended workflow on any API failure.
		if isRaw {
			if p, ok := errs.ProblemOf(err); ok {
				rawHint := "It is not advised to edit openapi format json directly. " +
					"Please follow instruction in lark-whiteboard skill, using whiteboard-cli " +
					"to transcript Whiteboard DSL pattern instead."
				if strings.TrimSpace(p.Hint) != "" {
					p.Hint = p.Hint + "\n" + rawHint
				} else {
					p.Hint = rawHint
				}
			}
		}
		return err
	}

	nodeIDs, err := stringSlice(data["ids"])
	if err != nil {
		return err
	}
	outData := map[string]string{"created_node_ids": strings.Join(nodeIDs, ",")}
	runtime.OutFormat(outData, nil, func(w io.Writer) {
		if outData["created_node_ids"] != "" {
			fmt.Fprintf(w, "%d new nodes created.\n", len(nodeIDs))
		}
		fmt.Fprintf(w, "Update whiteboard success")
	})

	return nil
}

// stringSlice coerces the JSON ids array into []string. A missing or malformed
// ids field is a response-shape bug, not a successful update with no output.
func stringSlice(v interface{}) ([]string, error) {
	switch raw := v.(type) {
	case []interface{}:
		out := make([]string, 0, len(raw))
		for i, e := range raw {
			s, ok := e.(string)
			if !ok {
				return nil, wbInvalidResponse("update whiteboard failed: data.ids[%d] must be a string", i)
			}
			out = append(out, s)
		}
		return out, nil
	case []string:
		return append([]string(nil), raw...), nil
	default:
		return nil, wbInvalidResponse("update whiteboard failed: data.ids must be an array of strings")
	}
}
