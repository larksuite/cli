// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"net/http"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const baseTokenQueryParam = "base_token"

const appTokenPersistHeader = "rpc-persist-x-base-apptoken"

var BaseAppBlockGetData = common.Shortcut{
	Service:     "base",
	Command:     "+app-block-get-data",
	Description: "Get computed data for a BaseApp page chart block",
	Risk:        "read",
	Scopes:      []string{"base:appmode_block:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		appTokenFlag(true),
		baseTokenFlag(true),
		{Name: "block-id", Desc: "chart_token (cht… prefix) returned by +app-block-create, +app-block-list, or +app-block-get", Required: true},
	},
	Tips: []string{
		"lark-cli base +app-block-get-data --app-token <app_token> --base-token <base_token> --block-id <chart_token>",
		"Do not call this command for a component whose +app-block-list result has type=unsupported; the API will return an error.",
		"--block-id must be a chart_token, not a widget_id.",
		"Read --base-token from the chart block data_config.base_token; do not choose an arbitrary +app-get ref key when the app references multiple Bases.",
		"The response uses the same computed chart data protocol as +dashboard-block-get-data.",
		"List and text blocks have no computed data; use +app-block-get for their metadata instead. For text specifically, +app-block-get returns the Markdown content in data_config.text.",
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return dryRunAppBlockGetData(ctx, runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeAppBlockGetData(runtime)
	},
}

func dryRunAppBlockGetData(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/base_apps/:app_token/blocks/:block_id/data").
		Set("app_token", runtime.Str("app-token")).
		Set("block_id", runtime.Str("block-id")).
		Params(map[string]interface{}{baseTokenQueryParam: strings.TrimSpace(runtime.Str("base-token"))}).
		Header(appTokenPersistHeader, strings.TrimSpace(runtime.Str("app-token")))
}

func executeAppBlockGetData(runtime *common.RuntimeContext) error {
	queryParams := larkcore.QueryParams{}
	queryParams.Set(baseTokenQueryParam, strings.TrimSpace(runtime.Str("base-token")))
	req := &larkcore.ApiReq{
		HttpMethod:  "GET",
		ApiPath:     baseV3Path("base_apps", runtime.Str("app-token"), "blocks", runtime.Str("block-id"), "data"),
		QueryParams: queryParams,
	}
	resp, err := runtime.DoAPI(req, larkcore.WithHeaders(http.Header{
		appTokenPersistHeader: []string{strings.TrimSpace(runtime.Str("app-token"))},
	}))
	if err != nil {
		return err
	}
	data, err := runtime.ClassifyAPIResponse(resp)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}
