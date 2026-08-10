// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var wbNodeCreateScopes = []string{"board:whiteboard:node:create"}
var wbNodeCreateAuthTypes = []string{"user", "bot"}
var wbNodeCreateFlags = []common.Flag{
	{Name: "whiteboard-token", Desc: "whiteboard token of the whiteboard to create nodes in. You need edit permission on the whiteboard.", Required: true},
	{Name: "source", Desc: `JSON payload containing a non-empty "nodes" array.`, Required: true, Input: []string{common.Stdin, common.File}},
	{Name: "idempotent-token", Desc: "idempotent token to make create requests retry-safe. Default is empty. Minimum length is 10.", Required: false},
}

type whiteboardNodeCreateReq struct {
	Nodes []map[string]interface{} `json:"nodes"`
}

func wbNodeCreateValidate(_ context.Context, runtime *common.RuntimeContext) error {
	if err := common.RejectDangerousCharsTyped("--whiteboard-token", runtime.Str("whiteboard-token")); err != nil {
		return err
	}
	if err := validateOptionalWhiteboardNodeIdempotentToken(runtime.Str("idempotent-token")); err != nil {
		return err
	}
	_, err := parseWhiteboardNodeBatchPayload([]byte(runtime.Str("source")), false)
	return err
}

func wbNodeCreateDryRun(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	payload, err := parseWhiteboardNodeBatchPayload([]byte(runtime.Str("source")), false)
	if err != nil {
		return common.NewDryRunAPI().Desc("parse input failed: " + err.Error())
	}

	dry := common.NewDryRunAPI().
		POST(wbNodeCreateDryRunURL(runtime.Str("whiteboard-token"))).
		Body(whiteboardNodeCreateReq{Nodes: payload.Nodes}).
		Desc("create nodes in the whiteboard.")
	if params := wbNodeCreateParams(runtime); len(params) > 0 {
		dry.Params(params)
	}
	return dry
}

func wbNodeCreateExecute(_ context.Context, runtime *common.RuntimeContext) error {
	payload, err := parseWhiteboardNodeBatchPayload([]byte(runtime.Str("source")), false)
	if err != nil {
		return err
	}

	data, err := runtime.CallAPITyped(
		http.MethodPost,
		wbNodeCreateURL(runtime.Str("whiteboard-token")),
		wbNodeCreateParams(runtime),
		whiteboardNodeCreateReq{Nodes: payload.Nodes},
	)
	if err != nil {
		return err
	}

	nodeIDs, err := whiteboardNodeCreateIDs(data)
	if err != nil {
		return err
	}
	outData := map[string]string{}
	if nodeIDs != nil {
		outData["ids"] = strings.Join(nodeIDs, ",")
	}
	runtime.OutFormat(outData, nil, func(w io.Writer) {
		if outData["ids"] != "" {
			fmt.Fprintf(w, "%d new nodes created.\n", len(nodeIDs))
		}
		fmt.Fprintf(w, "Create whiteboard nodes success")
	})
	return nil
}

func wbNodeCreateURL(token string) string {
	return fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", url.PathEscape(token))
}

func wbNodeCreateDryRunURL(token string) string {
	return fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", common.MaskToken(url.PathEscape(token)))
}

func wbNodeCreateParams(runtime *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{}
	if token := runtime.Str("idempotent-token"); token != "" {
		params["client_token"] = token
	}
	return params
}

func whiteboardNodeCreateIDs(data map[string]interface{}) ([]string, error) {
	switch raw := data["ids"].(type) {
	case nil:
		return nil, wbInvalidResponse("create whiteboard nodes failed: data.ids must be a non-empty array of strings")
	case []interface{}:
		if len(raw) == 0 {
			return nil, wbInvalidResponse("create whiteboard nodes failed: data.ids must be a non-empty array of strings")
		}
		out := make([]string, 0, len(raw))
		for i, value := range raw {
			id, ok := value.(string)
			if !ok {
				return nil, wbInvalidResponse("create whiteboard nodes failed: data.ids[%d] must be a string", i)
			}
			out = append(out, id)
		}
		return out, nil
	case []string:
		if len(raw) == 0 {
			return nil, wbInvalidResponse("create whiteboard nodes failed: data.ids must be a non-empty array of strings")
		}
		return append([]string(nil), raw...), nil
	default:
		return nil, wbInvalidResponse("create whiteboard nodes failed: data.ids must be an array of strings")
	}
}

// WhiteboardNodeCreate registers the `whiteboard +node-create` shortcut.
var WhiteboardNodeCreate = common.Shortcut{
	Service:     "whiteboard",
	Command:     "+node-create",
	Description: "Create nodes in an existing whiteboard.",
	Risk:        "write",
	Scopes:      wbNodeCreateScopes,
	AuthTypes:   wbNodeCreateAuthTypes,
	Flags:       wbNodeCreateFlags,
	Validate:    wbNodeCreateValidate,
	DryRun:      wbNodeCreateDryRun,
	Execute:     wbNodeCreateExecute,
}
