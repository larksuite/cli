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

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

var wbNodeUpdateScopes = []string{"board:whiteboard:node:update"}
var wbNodeUpdateAuthTypes = []string{"user", "bot"}
var wbNodeUpdateFlags = []common.Flag{
	{Name: "whiteboard-token", Desc: "whiteboard token of the whiteboard to update nodes in. You need edit permission on the whiteboard.", Required: true},
	{Name: "source", Desc: `JSON payload containing a non-empty "nodes" array. Each node must include non-empty string "id" and "type" fields; for tolerance, unnormalized raw export/query responses with "data.nodes" are accepted, envelope fields are dropped, and valid WhiteboardNode fields are kept before calling batch_update.`, Required: true, Input: []string{common.Stdin, common.File}},
	{Name: "idempotent-token", Desc: "idempotent token to make batch update requests retry-safe. Default is empty. Minimum length is 10.", Required: false},
}

func wbNodeUpdateValidate(_ context.Context, runtime *common.RuntimeContext) error {
	if err := common.RejectDangerousCharsTyped("--whiteboard-token", runtime.Str("whiteboard-token")); err != nil {
		return err
	}
	if err := validateOptionalWhiteboardNodeIdempotentToken(runtime.Str("idempotent-token")); err != nil {
		return err
	}
	_, err := parseWbNodeUpdatePayload([]byte(runtime.Str("source")))
	return err
}

func wbNodeUpdateDryRun(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	payload, err := parseWbNodeUpdatePayload([]byte(runtime.Str("source")))
	if err != nil {
		return common.NewDryRunAPI().Desc("parse input failed: " + err.Error())
	}

	dry := common.NewDryRunAPI().
		PUT(wbNodeBatchUpdateDryRunURL(runtime.Str("whiteboard-token"))).
		Body(wbNodeBatchUpdateBody(payload)).
		Desc("batch update nodes in the whiteboard.")
	if params := wbNodeUpdateParams(runtime); len(params) > 0 {
		dry.Params(params)
	}
	return dry
}

func wbNodeUpdateExecute(ctx context.Context, runtime *common.RuntimeContext) error {
	payload, err := parseWbNodeUpdatePayload([]byte(runtime.Str("source")))
	if err != nil {
		return err
	}

	data, err := callWhiteboardNodeUpdateAPI(
		ctx,
		runtime,
		http.MethodPut,
		wbNodeBatchUpdateURL(runtime.Str("whiteboard-token")),
		wbNodeUpdateParams(runtime),
		wbNodeBatchUpdateBody(payload),
	)
	if err != nil {
		return err
	}
	updatedNodeIDs, err := wbNodeUpdateIDs(data)
	if err != nil {
		return err
	}

	outData := map[string]interface{}{
		"ids":   strings.Join(updatedNodeIDs, ","),
		"count": len(updatedNodeIDs),
	}
	runtime.OutFormat(outData, nil, func(w io.Writer) {
		fmt.Fprintf(w, "%d nodes updated.\n", len(updatedNodeIDs))
		fmt.Fprintf(w, "Update whiteboard nodes success")
	})
	return nil
}

func callWhiteboardNodeUpdateAPI(ctx context.Context, runtime *common.RuntimeContext, method, apiPath string, params map[string]interface{}, body interface{}) (map[string]interface{}, error) {
	req := &larkcore.ApiReq{
		HttpMethod:  method,
		ApiPath:     apiPath,
		QueryParams: wbNodeQueryParams(params),
		Body:        body,
	}
	resp, err := runtime.DoAPIWithContext(ctx, req)
	if err != nil {
		return nil, err
	}
	data, classifyErr := runtime.ClassifyAPIResponse(resp)
	return data, enrichWhiteboardNodeUpdateError(classifyErr, resp)
}

func wbNodeQueryParams(params map[string]interface{}) larkcore.QueryParams {
	if len(params) == 0 {
		return nil
	}
	query := larkcore.QueryParams{}
	for key, value := range params {
		query.Set(key, fmt.Sprint(value))
	}
	return query
}

func enrichWhiteboardNodeUpdateError(err error, resp *larkcore.ApiResp) error {
	if err == nil || resp == nil || len(resp.RawBody) == 0 {
		return err
	}
	var envelope map[string]interface{}
	if decodeErr := json.Unmarshal(resp.RawBody, &envelope); decodeErr != nil {
		return err
	}
	if code, ok := envelope["code"].(float64); !ok || int(code) != 2890002 {
		return err
	}
	errBlock, ok := envelope["error"].(map[string]interface{})
	if !ok {
		return err
	}
	message, _ := errBlock["message"].(string)
	if strings.TrimSpace(message) == "" {
		return err
	}
	if problem, ok := errs.ProblemOf(err); ok {
		problem.Hint = message
	}
	return err
}

func parseWbNodeUpdatePayload(raw []byte) (wbNodeBatchPayload, error) {
	payload, err := parseWbNodeBatchPayload(raw, true)
	if err != nil {
		return wbNodeBatchPayload{}, err
	}
	for i, node := range payload.Nodes {
		nodeType, ok := node["type"].(string)
		if ok && strings.TrimSpace(nodeType) != "" {
			continue
		}
		name := fmt.Sprintf("nodes[%d].type", i)
		return wbNodeBatchPayload{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument, "%s must be a non-empty string", name,
		).WithParam("--source").WithParams(errs.InvalidParam{
			Name:   name,
			Reason: "required by the current whiteboard.node gateway schema",
		})
	}
	return payload, nil
}

func wbNodeBatchUpdateURL(token string) string {
	return fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/batch_update", url.PathEscape(token))
}

func wbNodeBatchUpdateDryRunURL(token string) string {
	return fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/batch_update", common.MaskToken(url.PathEscape(token)))
}

func wbNodeUpdateParams(runtime *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{}
	if token := runtime.Str("idempotent-token"); token != "" {
		params["client_token"] = token
	}
	return params
}

func wbNodeBatchUpdateBody(payload wbNodeBatchPayload) map[string]interface{} {
	return map[string]interface{}{"nodes": sanitizeWbNodeUpdateNodes(payload.Nodes)}
}

func wbNodeUpdateIDs(data map[string]interface{}) ([]string, error) {
	switch raw := data["ids"].(type) {
	case nil:
		return nil, wbInvalidResponse("update whiteboard nodes failed: data.ids must be a non-empty array of strings")
	case []interface{}:
		if len(raw) == 0 {
			return nil, wbInvalidResponse("update whiteboard nodes failed: data.ids must be a non-empty array of strings")
		}
		out := make([]string, 0, len(raw))
		for i, value := range raw {
			id, ok := value.(string)
			if !ok {
				return nil, wbInvalidResponse("update whiteboard nodes failed: data.ids[%d] must be a string", i)
			}
			out = append(out, id)
		}
		return out, nil
	case []string:
		if len(raw) == 0 {
			return nil, wbInvalidResponse("update whiteboard nodes failed: data.ids must be a non-empty array of strings")
		}
		return append([]string(nil), raw...), nil
	default:
		return nil, wbInvalidResponse("update whiteboard nodes failed: data.ids must be an array of strings")
	}
}

// WhiteboardNodeUpdate registers the `whiteboard +node-update` shortcut.
var WhiteboardNodeUpdate = common.Shortcut{
	Service:     "whiteboard",
	Command:     "+node-update",
	Description: "Update nodes in an existing whiteboard.",
	Risk:        "write",
	Scopes:      wbNodeUpdateScopes,
	AuthTypes:   wbNodeUpdateAuthTypes,
	Flags:       wbNodeUpdateFlags,
	Tips: []string{
		`Prefer --source JSON with a non-empty top-level "nodes" array; each node must include non-empty string "id" and "type" fields.`,
		`Execution sends one whiteboard.node batch_update request and preserves fields that belong to the WhiteboardNode contract.`,
		`Unnormalized raw export/query responses are tolerated for robustness; response-only or internal extra fields are omitted from the update body.`,
		`Use --idempotent-token for retry-safe batch_update requests; the token is sent as client_token only when provided.`,
	},
	Validate: wbNodeUpdateValidate,
	DryRun:   wbNodeUpdateDryRun,
	Execute:  wbNodeUpdateExecute,
}
