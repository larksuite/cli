// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
)

// DoTypedAPIJSON executes and classifies one JSON API request through the
// restricted CommandContext. It preserves the legacy RuntimeContext typed API
// classification while keeping hooks independent of RuntimeContext flags and
// output methods. It also carries a header-only log_id in returned data so a
// hook that detects a malformed success payload can attach the request ID to
// its own typed invalid-response error.
func DoTypedAPIJSON(ctx context.Context, command CommandContext, method, apiPath string, query larkcore.QueryParams, body any) (map[string]any, error) {
	return DoTypedAPIJSONWithOptions(ctx, command, method, apiPath, query, body)
}

// DoTypedAPIJSONWithOptions is DoTypedAPIJSON with SDK request options, used by
// multipart/form-data callers that must opt into file upload handling.
func DoTypedAPIJSONWithOptions(ctx context.Context, command CommandContext, method, apiPath string, query larkcore.QueryParams, body any, requestOptions ...larkcore.RequestOptionFunc) (map[string]any, error) {
	apiClient, err := command.APIClient()
	if err != nil {
		return nil, typedOrInternal(err)
	}
	req := &larkcore.ApiReq{HttpMethod: method, ApiPath: apiPath, QueryParams: query}
	if body != nil {
		req.Body = body
	}
	opts := append([]larkcore.RequestOptionFunc(nil), requestOptions...)
	if option := cmdutil.ShortcutHeaderOpts(ctx); option != nil {
		opts = append(opts, option)
	}
	response, err := apiClient.DoSDKRequest(ctx, req, core.Identity(command.Identity()), opts...)
	if err != nil {
		return nil, typedOrInternal(err)
	}
	data, err := ClassifyAPIResponseWith(response, typedClassifyContext(command))
	if data == nil {
		data = map[string]any{}
	}
	if logID := response.Header.Get("x-tt-logid"); logID != "" {
		data["log_id"] = logID
	}
	return data, err
}

// CallTypedAPI preserves RuntimeContext.CallAPITyped's raw request semantics
// for Typed hooks whose request params are represented as loose query maps.
func CallTypedAPI(ctx context.Context, command CommandContext, method, apiPath string, params map[string]interface{}, data any) (map[string]interface{}, error) {
	apiClient, err := command.APIClient()
	if err != nil {
		return nil, typedOrInternal(err)
	}
	request := client.RawApiRequest{Method: method, URL: apiPath, Params: params, Data: data, As: core.Identity(command.Identity())}
	if option := cmdutil.ShortcutHeaderOpts(ctx); option != nil {
		request.ExtraOpts = append(request.ExtraOpts, option)
	}
	response, err := apiClient.DoAPI(ctx, request)
	if err != nil {
		return nil, typedOrInternal(err)
	}
	return ClassifyAPIResponseWith(response, typedClassifyContext(command))
}

func typedClassifyContext(command CommandContext) errclass.ClassifyContext {
	config := command.Config()
	classify := errclass.ClassifyContext{Brand: string(config.Brand), AppID: config.AppID, Identity: string(command.Identity())}
	if provider, ok := command.(interface{ typedCommandPath() string }); ok {
		classify.LarkCmd = provider.typedCommandPath()
	}
	return classify
}
