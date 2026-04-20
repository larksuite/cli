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

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const (
	FormatRaw      = "raw"
	FormatPlantUML = "plantuml"
	FormatMermaid  = "mermaid"
)

var formatCodeMap = map[string]int{
	FormatRaw:      0,
	FormatPlantUML: 1,
	FormatMermaid:  2,
}

var wbUpdateScopes = []string{"board:whiteboard:node:create"}
var wbUpdateAuthTypes = []string{"user", "bot"}
var wbUpdateFlags = []common.Flag{
	{Name: "idempotent-token", Desc: "idempotent token to ensure the update is idempotent. Default is empty. min length is 10.", Required: false},
	{Name: "whiteboard-token", Desc: "whiteboard token of the whiteboard to update. You will need edit permission to update the whiteboard.", Required: true},
	{Name: "overwrite", Desc: "overwrite the whiteboard content, delete all existing content before update. Default is false.", Required: false, Type: "bool"},
	{Name: "source", Desc: "Input whiteboard data.", Required: true, Input: []string{common.Stdin, common.File}},
	{Name: "input_format", Desc: "format of input data: raw | plantuml | mermaid. Default is raw.", Required: false},
}

func wbUpdateValidate(ctx context.Context, runtime *common.RuntimeContext) error {
	// 检查 token 是否包含控制字符（空字符串下自动跳过了）
	if err := validate.RejectControlChars(runtime.Str("whiteboard-token"), "whiteboard-token"); err != nil {
		return err
	}
	itoken := runtime.Str("idempotent-token")
	if err := validate.RejectControlChars(itoken, "idempotent-token"); err != nil {
		return err
	}
	if itoken != "" && len(itoken) < 10 {
		return common.FlagErrorf("--idempotent-token must be at least 10 characters long.")
	}

	// 检查 --input_format 标志
	format := getFormat(runtime)
	if format != FormatRaw && format != FormatPlantUML && format != FormatMermaid {
		return common.FlagErrorf("--input_format must be one of: raw | plantuml | mermaid")
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
	case FormatPlantUML, FormatMermaid:
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
		return output.ErrValidation("read input failed: source is required")
	}

	switch format {
	case FormatRaw:
		return updateWhiteboardByRawNodes(ctx, runtime, token, []byte(input), overwrite, idempotentToken)
	case FormatPlantUML, FormatMermaid:
		return updateWhiteboardByCode(ctx, runtime, token, []byte(input), format, overwrite, idempotentToken)
	default:
		return output.ErrValidation(fmt.Sprintf("unsupported format: %s", format))
	}
}

const WhiteboardUpdateDescription = "Update an existing whiteboard in lark document with mermaid, plantuml or whiteboard dsl. refer to lark-whiteboard skill for more details."

var WhiteboardUpdate = common.Shortcut{
	Service:     "whiteboard",
	Command:     "+update",
	Description: WhiteboardUpdateDescription,
	Risk:        "high-risk-write",
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
	Risk:        "high-risk-write",
	Scopes:      wbUpdateScopes,
	AuthTypes:   wbUpdateAuthTypes,
	Flags:       wbUpdateFlags,
	HasFormat:   false, // 不使用 lark 的 format flag（使用画板内部的格式）
	Validate:    wbUpdateValidate,
	DryRun:      wbUpdateDryRun,
	Execute:     wbUpdateExecute,
}

type createResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		NodeIDs         []string `json:"ids"`
		IdempotentToken string   `json:"client_token"`
	} `json:"data"`
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

type plantumlCreateResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		NodeID string `json:"node_id"`
	} `json:"data"`
}

func parseWBcliNodes(rawjson []byte) (wbNodes []interface{}, err error, isRaw bool) {
	var wbOutput WbCliOutput
	if err := json.Unmarshal(rawjson, &wbOutput); err != nil {
		return nil, output.Errorf(output.ExitValidation, "parsing", fmt.Sprintf("unmarshal input json failed: %v", err)), false
	}
	if (wbOutput.Code != 0 || wbOutput.Data.To != "openapi") && wbOutput.RawNodes == nil {
		return nil, output.Errorf(output.ExitValidation, "whiteboard-cli", "whiteboard-cli failed. please check previous log."), false
	}
	if wbOutput.RawNodes != nil {
		wbNodes = wbOutput.RawNodes
		isRaw = true
	} else {
		if wbOutput.Data.Result.Nodes == nil {
			return nil, output.Errorf(output.ExitValidation, "whiteboard-cli", "whiteboard-cli failed. please check previous log."), false
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

	req := &larkcore.ApiReq{
		HttpMethod:  http.MethodPost,
		ApiPath:     fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/plantuml", url.PathEscape(wbToken)),
		Body:        reqBody,
		QueryParams: map[string][]string{},
	}
	if idempotentToken != "" {
		req.QueryParams["client_token"] = []string{idempotentToken}
	}

	resp, err := runtime.DoAPI(req)
	if err != nil {
		return output.ErrNetwork(fmt.Sprintf("update whiteboard by code failed: %v", err))
	}
	if resp.StatusCode != http.StatusOK {
		return output.ErrAPI(resp.StatusCode, string(resp.RawBody), nil)
	}

	var createResp plantumlCreateResp
	err = json.Unmarshal(resp.RawBody, &createResp)
	if err != nil {
		return output.Errorf(output.ExitInternal, "parsing", fmt.Sprintf("parse whiteboard create response failed: %v", err))
	}
	if createResp.Code != 0 {
		return output.ErrAPI(createResp.Code, "update whiteboard by code failed", fmt.Sprintf("update whiteboard by code failed: %s", createResp.Msg))
	}

	outData := make(map[string]string)
	outData["created_node_id"] = createResp.Data.NodeID
	runtime.OutFormat(outData, nil, func(w io.Writer) {
		if outData["created_node_id"] != "" {
			fmt.Fprintf(w, "New node created.\n")
		}
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
	outData := make(map[string]string)
	reqBody := rawNodesCreateReq{
		Nodes:     nodes,
		Overwrite: overwrite,
	}

	req := &larkcore.ApiReq{
		HttpMethod:  http.MethodPost,
		ApiPath:     fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", url.PathEscape(wbToken)),
		Body:        reqBody,
		QueryParams: map[string][]string{},
	}
	if idempotentToken != "" {
		req.QueryParams["client_token"] = []string{idempotentToken}
	}

	resp, err := runtime.DoAPI(req)
	if err != nil {
		return output.ErrNetwork(fmt.Sprintf("update whiteboard failed: %v", err))
	}
	if resp.StatusCode != http.StatusOK {
		var detail string
		if isRaw {
			detail = fmt.Sprintf("It is not advised to edit openapi format json directly. Please follow instruction in lark-whiteboard skill, " +
				"using whiteboard-cli to transcript Whiteboard DSL pattern instead.")
		}
		return output.ErrAPI(resp.StatusCode, string(resp.RawBody), detail)
	}

	var createResp createResponse
	err = json.Unmarshal(resp.RawBody, &createResp)
	if err != nil {
		return output.Errorf(output.ExitInternal, "parsing", fmt.Sprintf("parse whiteboard create response failed: %v", err))
	}
	if createResp.Code != 0 {
		detail := fmt.Sprintf("update whiteboard failed: %s", createResp.Msg)
		if isRaw {
			detail += fmt.Sprintf("\n It is not advised to edit openapi format json directly. Please follow instruction in lark-whiteboard skill, " +
				"using whiteboard-cli to transcript Whiteboard DSL pattern instead.")
		}
		return output.ErrAPI(createResp.Code, "update whiteboard failed", detail)
	}

	outData["created_node_ids"] = strings.Join(createResp.Data.NodeIDs, ",")
	runtime.OutFormat(outData, nil, func(w io.Writer) {
		if outData["created_node_ids"] != "" {
			fmt.Fprintf(w, "%d new nodes created.\n", len(createResp.Data.NodeIDs))
		}
		fmt.Fprintf(w, "Update whiteboard success")
	})

	return nil
}
