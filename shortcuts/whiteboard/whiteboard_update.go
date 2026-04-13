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
	"time"

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

var wbUpdateScopes = []string{"board:whiteboard:node:read", "board:whiteboard:node:create", "board:whiteboard:node:delete"}
var wbUpdateAuthTypes = []string{"user", "bot"}
var skipDeleteNodesBatchSleep = false // for accelerate UT testing only
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
	var delNum int
	var err error
	if overwrite {
		// 还是会读取一下 whiteboard nodes，确认是否有节点要删除
		delNum, _, err = clearWhiteboardContent(ctx, runtime, token, []string{}, true)
		if err != nil {
			return common.NewDryRunAPI().Desc("read whiteboard nodes failed: " + err.Error())
		}
		if delNum > 0 {
			descStr += fmt.Sprintf(" %d existing nodes deleted before update.", delNum)
		}
	}

	desc := common.NewDryRunAPI().Desc(descStr)

	switch format {
	case FormatRaw:
		nodes, err, _ := parseWBcliNodes([]byte(input))
		if err != nil {
			return common.NewDryRunAPI().Desc("parse input failed: " + err.Error())
		}
		desc.POST(fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", common.MaskToken(url.PathEscape(token)))).Body(nodes).Desc("create all nodes of the whiteboard.")
	case FormatPlantUML, FormatMermaid:
		syntaxType := formatCodeMap[format]
		reqBody := plantumlCreateReq{
			PlantUmlCode: input,
			SyntaxType:   syntaxType,
			ParseMode:    1,
			DiagramType:  0,
		}
		desc.POST(fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/plantuml", common.MaskToken(url.PathEscape(token)))).Body(reqBody).Desc(fmt.Sprintf("create %s node on the whiteboard.", format))
	}

	if overwrite && delNum > 0 {
		// 在 DryRun 中只记录意图，不实际拉取和计算节点
		desc.GET(fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", common.MaskToken(url.PathEscape(token)))).Desc("get all nodes of the whiteboard to delete, then filter out newly created ones.")
		desc.DELETE(fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/batch_delete", common.MaskToken(url.PathEscape(token)))).Body("{\"ids\":[\"...\"]}").
			Desc(fmt.Sprintf("delete all old nodes of the whiteboard 100 nodes at a time. This API may be called multiple times and is not reversible. %d whiteboard nodes will be deleted while update.", delNum))
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

type deleteResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type simpleNodeResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Nodes []struct {
			Id       string   `json:"id"`
			Children []string `json:"children"`
		} `json:"nodes"`
	} `json:"data"`
}

type deleteNodeReqBody struct {
	Ids []string `json:"ids"`
}

type plantumlCreateReq struct {
	PlantUmlCode string `json:"plant_uml_code"`
	SyntaxType   int    `json:"syntax_type"`
	DiagramType  int    `json:"diagram_type,omitempty"`
	ParseMode    int    `json:"parse_mode,omitempty"`
}

type plantumlCreateResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		NodeID string `json:"node_id"`
	} `json:"data"`
}

func parseWBcliNodes(rawjson []byte) (wbNodes interface{}, err error, isRaw bool) {
	var wbOutput WbCliOutput
	if err := json.Unmarshal(rawjson, &wbOutput); err != nil {
		return nil, output.Errorf(output.ExitValidation, "parsing", fmt.Sprintf("unmarshal input json failed: %v", err)), false
	}
	if (wbOutput.Code != 0 || wbOutput.Data.To != "openapi") && wbOutput.RawNodes == nil {
		return nil, output.Errorf(output.ExitValidation, "whiteboard-cli", "whiteboard-cli failed. please check previous log."), false
	}
	if wbOutput.RawNodes != nil {
		wbNodes = struct {
			Nodes []interface{} `json:"nodes"`
		}{
			Nodes: wbOutput.RawNodes,
		}
		isRaw = true
	} else {
		wbNodes = wbOutput.Data.Result
	}
	return wbNodes, nil, isRaw
}

func clearWhiteboardContent(ctx context.Context, runtime *common.RuntimeContext, wbToken string, newNodeIDs []string, dryRun bool) (int, []string, error) {
	resp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", url.PathEscape(wbToken)),
	})
	if err != nil {
		return 0, nil, output.ErrNetwork(fmt.Sprintf("get whiteboard nodes failed: %v", err))
	}
	if resp.StatusCode != http.StatusOK {
		return 0, nil, output.ErrAPI(resp.StatusCode, string(resp.RawBody), nil)
	}
	var nodes simpleNodeResp
	err = json.Unmarshal(resp.RawBody, &nodes)
	if err != nil {
		return 0, nil, output.Errorf(output.ExitInternal, "parsing", fmt.Sprintf("parse whiteboard nodes failed: %v", err))
	}
	if nodes.Code != 0 {
		return 0, nil, output.ErrAPI(nodes.Code, "get whiteboard nodes failed", fmt.Sprintf("get whiteboard nodes failed: %s", nodes.Msg))
	}

	// 收集所有新节点及其 children 的 ID，递归处理
	protectedIDs := make(map[string]bool)
	for _, id := range newNodeIDs {
		protectedIDs[id] = true
	}
	// 构建 node map 以便快速查找
	nodeMap := make(map[string][]string)
	if nodes.Data.Nodes != nil {
		for _, node := range nodes.Data.Nodes {
			nodeMap[node.Id] = node.Children
		}
	}
	// 递归收集所有 children
	visited := make(map[string]bool)
	var collectChildren func(id string)
	collectChildren = func(id string) {
		if visited[id] {
			return
		}
		visited[id] = true
		if children, ok := nodeMap[id]; ok {
			for _, child := range children {
				protectedIDs[child] = true
				collectChildren(child)
			}
		}
	}
	for _, id := range newNodeIDs {
		collectChildren(id)
	}

	// 确定要删除的节点
	nodeIds := make([]string, 0, len(nodes.Data.Nodes))
	if nodes.Data.Nodes != nil {
		for _, node := range nodes.Data.Nodes {
			nodeIds = append(nodeIds, node.Id)
		}
	}
	delIds := make([]string, 0, len(nodeIds))
	for _, nodeId := range nodeIds {
		if !protectedIDs[nodeId] {
			delIds = append(delIds, nodeId)
		}
	}
	if dryRun {
		return len(delIds), delIds, nil
	}
	// 实际删除节点，按每批最多100个进行切分
	for i := 0; i < len(delIds); i += 100 {
		if !skipDeleteNodesBatchSleep {
			time.Sleep(time.Millisecond * 1000) // 画板内删除大量节点时，内部会有大量写操作，需要稍等一下，避免被限流
		}
		end := i + 100
		if end > len(delIds) {
			end = len(delIds)
		}
		batchIds := delIds[i:end]
		delReq := deleteNodeReqBody{
			Ids: batchIds,
		}
		resp, err = runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod: http.MethodDelete,
			ApiPath:    fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/batch_delete", url.PathEscape(wbToken)),
			Body:       delReq,
		})
		if err != nil {
			return 0, nil, output.ErrNetwork(fmt.Sprintf("delete whiteboard nodes failed: %v", err))
		}
		if resp.StatusCode != http.StatusOK {
			return 0, nil, output.ErrAPI(resp.StatusCode, string(resp.RawBody), nil)
		}
		var delResp deleteResponse
		err = json.Unmarshal(resp.RawBody, &delResp)
		if err != nil {
			return 0, nil, output.Errorf(output.ExitInternal, "parsing", fmt.Sprintf("parse whiteboard delete response failed: %v", err))
		}
		if delResp.Code != 0 {
			return 0, nil, output.ErrAPI(delResp.Code, "delete whiteboard nodes failed", fmt.Sprintf("delete whiteboard nodes failed: %s", delResp.Msg))
		}
	}
	return len(delIds), delIds, nil
}

// updateWhiteboardByCode 使用 plantuml/mermaid 代码更新画板
func updateWhiteboardByCode(ctx context.Context, runtime *common.RuntimeContext, wbToken string, input []byte, format string, overwrite bool, idempotentToken string) error {
	syntaxType := formatCodeMap[format]
	reqBody := plantumlCreateReq{
		PlantUmlCode: string(input),
		SyntaxType:   syntaxType,
		ParseMode:    1,
		DiagramType:  0, // 0 表示自动识别
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
	newNodeIDs := []string{createResp.Data.NodeID}

	if overwrite {
		numNodes, _, err := clearWhiteboardContent(ctx, runtime, wbToken, newNodeIDs, false)
		if err != nil {
			return err
		}
		outData["deleted_nodes_num"] = fmt.Sprintf("%d", numNodes)
	}

	runtime.OutFormat(outData, nil, func(w io.Writer) {
		if outData["deleted_nodes_num"] != "" {
			fmt.Fprintf(w, "%s existing nodes deleted.\n", outData["deleted_nodes_num"])
		}
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

	req := &larkcore.ApiReq{
		HttpMethod:  http.MethodPost,
		ApiPath:     fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", url.PathEscape(wbToken)),
		Body:        nodes,
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

	if overwrite {
		numNodes, _, err := clearWhiteboardContent(ctx, runtime, wbToken, createResp.Data.NodeIDs, false)
		if err != nil {
			return err
		}
		outData["deleted_nodes_num"] = fmt.Sprintf("%d", numNodes)
	}

	runtime.OutFormat(outData, nil, func(w io.Writer) {
		if outData["deleted_nodes_num"] != "" {
			fmt.Fprintf(w, "%s existing nodes deleted.\n", outData["deleted_nodes_num"])
		}
		if outData["created_node_ids"] != "" {
			fmt.Fprintf(w, "%d new nodes created.\n", len(createResp.Data.NodeIDs))
		}
		fmt.Fprintf(w, "Update whiteboard success")
	})

	return nil
}
