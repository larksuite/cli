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
	"slices"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

var wbUpdateScopes = []string{"board:whiteboard:node:read", "board:whiteboard:node:create", "board:whiteboard:node:delete"}
var wbUpdateAuthTypes = []string{"user", "bot"}
var wbUpdateFlags = []common.Flag{
	{Name: "idempotent-token", Desc: "idempotent token to ensure the update is idempotent. Default is empty. min length is 10.", Required: false},
	{Name: "whiteboard-token", Desc: "whiteboard token of the whiteboard to update. You will need edit permission to update the whiteboard.", Required: true},
	{Name: "overwrite", Desc: "overwrite the whiteboard content, delete all existing content before update. Default is false.", Required: false, Type: "bool"},
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
	stat, err := os.Stdin.Stat()
	if err != nil || (stat.Mode()&os.ModeCharDevice) != 0 {
		return output.ErrValidation("read stdin failed, please follow lark-whiteboard skill to pipe in input data")
	}
	return nil
}

func wbUpdateDryRun(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	// 读取 stdin 内容，解析为 OAPI 参数
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return common.NewDryRunAPI().Desc("read stdin failed: " + err.Error())
	}
	nodes, err, _ := parseWBcliNodes(input)
	if err != nil {
		return common.NewDryRunAPI().Desc("parse stdin failed: " + err.Error())
	}
	token := runtime.Str("whiteboard-token")
	overwrite := runtime.Bool("overwrite")
	descStr := "will call whiteboard open api to draw such DSL content."
	var delNum int
	if overwrite {
		// 还是会读取一下 whiteboard nodes，确认是否有节点要删除
		delNum, _, err = clearWhiteboardContent(ctx, runtime, token, []string{}, true)
		if err != nil {
			return common.NewDryRunAPI().Desc("read whiteboard nodes failed: " + err.Error())
		}
		if delNum > 0 {
			descStr += fmt.Sprintf("%d existing nodes deleted before update.", delNum)
		}
	}
	desc := common.NewDryRunAPI().Desc(descStr)
	desc.POST(fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", common.MaskToken(url.PathEscape(token)))).Body(nodes).Desc("create all nodes of the whiteboard.")
	if overwrite && delNum > 0 {
		// 在 DryRun 中只记录意图，不实际拉取和计算节点
		desc.GET(fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", common.MaskToken(url.PathEscape(token)))).Desc("get all nodes of the whiteboard to delete, then filter out newly created ones.")
		desc.DELETE(fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/batch_delete", common.MaskToken(url.PathEscape(token)))).Body("{\"ids\":[\"...\"]}").
			Desc(fmt.Sprintf("delete all old nodes of the whiteboard 100 nodes at a time. This API may be called multiple times and is not reversible. %d whiteboard nodes will be deleted while update.", delNum))
	}
	return desc
}

func wbUpdateExecute(ctx context.Context, runtime *common.RuntimeContext) error {
	// 检查 token
	token := runtime.Str("whiteboard-token")
	overwrite := runtime.Bool("overwrite")
	idempotentToken := runtime.Str("idempotent-token")
	// 读取 stdin 内容，解析为 OAPI 参数
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return output.ErrValidation("read stdin failed: " + err.Error())
	}
	nodes, err, isRaw := parseWBcliNodes(input)
	if err != nil {
		return err
	}
	outData := make(map[string]string)
	// 写入画板节点
	req := &larkcore.ApiReq{
		HttpMethod:  http.MethodPost,
		ApiPath:     fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", url.PathEscape(token)),
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
			detail = fmt.Sprintf("It is not advised to edit openapi format json directly. Please follow instruction in lark-whiteboard skill," +
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
			detail += fmt.Sprintf("\n It is not advised to edit openapi format json directly. Please follow instruction in lark-whiteboard skill," +
				"using whiteboard-cli to transcript Whiteboard DSL pattern instead.")
		}
		return output.ErrAPI(createResp.Code, "update whiteboard failed", detail)
	}
	outData["created_node_ids"] = strings.Join(createResp.Data.NodeIDs, ",")
	// 清空画板节点，先写后删，起码新的能写进去
	if overwrite {
		numNodes, _, err := clearWhiteboardContent(ctx, runtime, token, createResp.Data.NodeIDs, false)
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
		fmt.Fprintf(w, "update whiteboard success")
	})
	return nil
}

var WhiteboardUpdate = common.Shortcut{
	Service:     "whiteboard",
	Command:     "+update",
	Description: "Update an existing whiteboard in lark document with whiteboard dsl. Such DSL input from stdin. refer to lark-whiteboard skill for more details.",
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
	Description: "Update an existing whiteboard in lark document with whiteboard dsl. Such DSL input from stdin. refer to lark-whiteboard skill for more details.",
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
			Id string `json:"id"`
		} `json:"nodes"`
	} `json:"data"`
}

type deleteNodeReqBody struct {
	Ids []string `json:"ids"`
}

func parseWBcliNodes(rawjson []byte) (wbNodes interface{}, err error, isRaw bool) {
	var wbOutput WbCliOutput
	if err := json.Unmarshal(rawjson, &wbOutput); err != nil {
		return nil, output.Errorf(output.ExitInternal, "parsing", fmt.Sprintf("unmarshal stdin json failed: %v", err)), false
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
	nodeIds := make([]string, 0, len(nodes.Data.Nodes))
	if nodes.Data.Nodes != nil {
		for _, node := range nodes.Data.Nodes {
			nodeIds = append(nodeIds, node.Id)
		}
	}
	delIds := make([]string, 0, len(nodeIds))
	for _, nodeId := range nodeIds {
		if !slices.Contains(newNodeIDs, nodeId) {
			delIds = append(delIds, nodeId)
		}
	}
	if dryRun {
		return len(delIds), delIds, nil
	}
	// 实际删除节点，按每批最多100个进行切分
	for i := 0; i < len(delIds); i += 100 {
		time.Sleep(time.Millisecond * 1000) // 画板内删除大量节点时，内部会有大量写操作，需要稍等一下，避免被限流
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
