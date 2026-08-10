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

var wbNodeDeleteScopes = []string{"board:whiteboard:node:delete"}
var wbNodeDeleteAuthTypes = []string{"user", "bot"}
var wbNodeDeleteFlags = []common.Flag{
	{Name: "whiteboard-token", Desc: "whiteboard token of the whiteboard to delete nodes from. You need edit permission on the whiteboard.", Required: true},
	{Name: "node-ids", Desc: "comma-separated whiteboard node IDs to delete.", Required: true},
	{Name: "idempotent-token", Desc: "idempotent token to make delete requests retry-safe. Default is empty. Minimum length is 10.", Required: false},
}

type whiteboardNodeDeleteReq struct {
	IDs []string `json:"ids"`
}

func wbNodeDeleteValidate(_ context.Context, runtime *common.RuntimeContext) error {
	if err := common.RejectDangerousCharsTyped("--whiteboard-token", runtime.Str("whiteboard-token")); err != nil {
		return err
	}
	if err := validateOptionalWhiteboardNodeIdempotentToken(runtime.Str("idempotent-token")); err != nil {
		return err
	}
	_, err := parseWhiteboardNodeIDs(runtime.Str("node-ids"))
	return err
}

func wbNodeDeleteDryRun(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	ids, err := parseWhiteboardNodeIDs(runtime.Str("node-ids"))
	if err != nil {
		return common.NewDryRunAPI().Desc("parse node ids failed: " + err.Error())
	}

	dry := common.NewDryRunAPI().
		DELETE(wbNodeDeleteDryRunURL(runtime.Str("whiteboard-token"))).
		Body(whiteboardNodeDeleteReq{IDs: ids}).
		Desc("delete nodes from the whiteboard.")
	if params := wbNodeDeleteParams(runtime); len(params) > 0 {
		dry.Params(params)
	}
	return dry
}

func wbNodeDeleteExecute(_ context.Context, runtime *common.RuntimeContext) error {
	ids, err := parseWhiteboardNodeIDs(runtime.Str("node-ids"))
	if err != nil {
		return err
	}

	if _, err := runtime.CallAPITyped(
		http.MethodDelete,
		wbNodeDeleteURL(runtime.Str("whiteboard-token")),
		wbNodeDeleteParams(runtime),
		whiteboardNodeDeleteReq{IDs: ids},
	); err != nil {
		return err
	}

	outData := map[string]interface{}{
		"ids":   strings.Join(ids, ","),
		"count": len(ids),
	}
	runtime.OutFormat(outData, nil, func(w io.Writer) {
		fmt.Fprintf(w, "%d nodes deleted.\n", len(ids))
		fmt.Fprintf(w, "Delete whiteboard nodes success")
	})
	return nil
}

func wbNodeDeleteURL(token string) string {
	return fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/batch_delete", url.PathEscape(token))
}

func wbNodeDeleteDryRunURL(token string) string {
	return fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/batch_delete", common.MaskToken(url.PathEscape(token)))
}

func wbNodeDeleteParams(runtime *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{}
	if token := runtime.Str("idempotent-token"); token != "" {
		params["client_token"] = token
	}
	return params
}

// WhiteboardNodeDelete registers the `whiteboard +node-delete` shortcut.
var WhiteboardNodeDelete = common.Shortcut{
	Service:     "whiteboard",
	Command:     "+node-delete",
	Description: "Delete nodes from an existing whiteboard.",
	Risk:        "high-risk-write",
	Scopes:      wbNodeDeleteScopes,
	AuthTypes:   wbNodeDeleteAuthTypes,
	Flags:       wbNodeDeleteFlags,
	Validate:    wbNodeDeleteValidate,
	DryRun:      wbNodeDeleteDryRun,
	Execute:     wbNodeDeleteExecute,
}
