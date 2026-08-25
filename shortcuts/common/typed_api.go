// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/commandbridge"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
)

// DoHostedAPIJSON executes one request for the internal command host. The
// internal access parameter keeps this bridge out of the authoring surface.
func DoHostedAPIJSON(ctx context.Context, command typedRuntimeContext, method, apiPath string, query larkcore.QueryParams, body any, _ commandbridge.Access) (map[string]any, error) {
	return doHostedAPIJSONWithOptions(ctx, command, method, apiPath, query, body)
}

func doHostedAPIJSONWithOptions(ctx context.Context, command typedRuntimeContext, method, apiPath string, query larkcore.QueryParams, body any, requestOptions ...larkcore.RequestOptionFunc) (map[string]any, error) {
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
	return ClassifyAPIResponseWith(response, typedClassifyContext(command))
}

// CallHostedAPI preserves RuntimeContext.CallAPITyped's raw request semantics
// for the internal pagination adapter.
func CallHostedAPI(ctx context.Context, command typedRuntimeContext, method, apiPath string, params map[string]interface{}, data any, _ commandbridge.Access) (map[string]interface{}, error) {
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

func typedClassifyContext(command typedRuntimeContext) errclass.ClassifyContext {
	config := command.Config()
	classify := errclass.ClassifyContext{Brand: string(config.Brand), AppID: config.AppID, Identity: string(command.Identity())}
	if provider, ok := command.(interface{ typedCommandPath() string }); ok {
		classify.LarkCmd = provider.typedCommandPath()
	}
	return classify
}
