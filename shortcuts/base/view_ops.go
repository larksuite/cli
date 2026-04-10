// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"fmt"
	"net/url"

	"github.com/larksuite/cli/shortcuts/common"
)

func dryRunViewBase(runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		Set("view_id", runtime.Str("view-id"))
}

func dryRunViewList(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	limit := common.ParseIntBounded(runtime, "limit", 1, 200)
	return dryRunViewBase(runtime).
		GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/views").
		Params(map[string]interface{}{"offset": offset, "limit": limit})
}

func dryRunViewGet(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return dryRunViewBase(runtime).
		GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/views/:view_id")
}

func dryRunViewCreate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	pc := newParseCtx(runtime)
	api := dryRunViewBase(runtime)
	bodyList, err := parseObjectList(pc, runtime.Str("json"), "json")
	if err != nil || len(bodyList) == 0 {
		return api.POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/views")
	}
	for _, body := range bodyList {
		api.POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/views").Body(body)
	}
	return api
}

func dryRunViewDelete(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return dryRunViewBase(runtime).
		DELETE("/open-apis/base/v3/bases/:base_token/tables/:table_id/views/:view_id")
}

func dryRunViewGetProperty(runtime *common.RuntimeContext, segment string) *common.DryRunAPI {
	return dryRunViewBase(runtime).
		GET(fmt.Sprintf("/open-apis/base/v3/bases/:base_token/tables/:table_id/views/:view_id/%s", url.PathEscape(segment)))
}

func dryRunViewSetJSONObject(runtime *common.RuntimeContext, segment string) *common.DryRunAPI {
	pc := newParseCtx(runtime)
	body, _ := parseJSONObject(pc, runtime.Str("json"), "json")
	return dryRunViewBase(runtime).
		PUT(fmt.Sprintf("/open-apis/base/v3/bases/:base_token/tables/:table_id/views/:view_id/%s", url.PathEscape(segment))).
		Body(body)
}

func dryRunViewSetWrapped(runtime *common.RuntimeContext, segment string, wrapper string) *common.DryRunAPI {
	pc := newParseCtx(runtime)
	raw, err := parseJSONValue(pc, runtime.Str("json"), "json")
	if err != nil {
		raw = nil
	}
	return dryRunViewBase(runtime).
		PUT(fmt.Sprintf("/open-apis/base/v3/bases/:base_token/tables/:table_id/views/:view_id/%s", url.PathEscape(segment))).
		Body(wrapViewPropertyBody(raw, wrapper))
}

func dryRunViewGetFilter(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return dryRunViewGetProperty(runtime, "filter")
}

func dryRunViewGetVisibleFields(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return dryRunViewGetProperty(runtime, "visible_fields")
}

func dryRunViewSetFilter(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return dryRunViewSetJSONObject(runtime, "filter")
}

func dryRunViewSetVisibleFields(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return dryRunViewSetJSONObject(runtime, "visible_fields")
}

func dryRunViewGetGroup(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return dryRunViewGetProperty(runtime, "group")
}

func dryRunViewSetGroup(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return dryRunViewSetWrapped(runtime, "group", "group_config")
}

func dryRunViewGetSort(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return dryRunViewGetProperty(runtime, "sort")
}

func dryRunViewSetSort(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return dryRunViewSetWrapped(runtime, "sort", "sort_config")
}

func dryRunViewGetTimebar(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return dryRunViewGetProperty(runtime, "timebar")
}

func dryRunViewSetTimebar(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return dryRunViewSetJSONObject(runtime, "timebar")
}

func dryRunViewGetCard(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return dryRunViewGetProperty(runtime, "card")
}

func dryRunViewSetCard(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return dryRunViewSetJSONObject(runtime, "card")
}

func dryRunViewRename(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return dryRunViewBase(runtime).
		PATCH("/open-apis/base/v3/bases/:base_token/tables/:table_id/views/:view_id").
		Body(map[string]interface{}{"name": runtime.Str("name")})
}

func wrapViewPropertyBody(raw interface{}, key string) interface{} {
	if items, ok := raw.([]interface{}); ok {
		return map[string]interface{}{key: items}
	}
	return raw
}

func validateViewCreate(runtime *common.RuntimeContext) error {
	return nil
}

func validateViewJSONObject(runtime *common.RuntimeContext) error {
	return nil
}

func validateViewJSONValue(runtime *common.RuntimeContext) error {
	return nil
}

func executeViewList(runtime *common.RuntimeContext) error {
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	limit := common.ParseIntBounded(runtime, "limit", 1, 200)
	views, total, err := listAllViews(runtime, runtime.Str("base-token"), baseTableID(runtime), offset, limit)
	if err != nil {
		return err
	}
	if total == 0 {
		total = len(views)
	}
	runtime.Out(map[string]interface{}{"views": views, "total": total}, nil)
	return nil
}

func executeViewGet(runtime *common.RuntimeContext) error {
	baseToken := runtime.Str("base-token")
	tableIDValue := baseTableID(runtime)
	viewRef := runtime.Str("view-id")
	data, err := baseV3Call(runtime, "GET", baseV3Path("bases", baseToken, "tables", tableIDValue, "views", viewRef), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"view": data}, nil)
	return nil
}

func executeViewCreate(runtime *common.RuntimeContext) error {
	pc := newParseCtx(runtime)
	baseToken := runtime.Str("base-token")
	tableIDValue := baseTableID(runtime)
	viewItems, err := parseObjectList(pc, runtime.Str("json"), "json")
	if err != nil {
		return err
	}
	created := []interface{}{}
	for _, body := range viewItems {
		data, err := baseV3Call(runtime, "POST", baseV3Path("bases", baseToken, "tables", tableIDValue, "views"), nil, body)
		if err != nil {
			return err
		}
		created = append(created, data)
	}
	runtime.Out(map[string]interface{}{"views": created}, nil)
	return nil
}

func executeViewDelete(runtime *common.RuntimeContext) error {
	baseToken := runtime.Str("base-token")
	tableIDValue := baseTableID(runtime)
	viewRef := runtime.Str("view-id")
	_, err := baseV3Call(runtime, "DELETE", baseV3Path("bases", baseToken, "tables", tableIDValue, "views", viewRef), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"deleted": true, "view_id": viewRef, "view_name": viewRef}, nil)
	return nil
}

func executeViewGetProperty(runtime *common.RuntimeContext, segment string, key string) error {
	baseToken := runtime.Str("base-token")
	tableIDValue := baseTableID(runtime)
	viewRef := runtime.Str("view-id")
	data, err := baseV3CallAny(runtime, "GET", baseV3Path("bases", baseToken, "tables", tableIDValue, "views", viewRef, segment), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{key: data}, nil)
	return nil
}

func executeViewSetJSONObject(runtime *common.RuntimeContext, segment string, key string) error {
	pc := newParseCtx(runtime)
	baseToken := runtime.Str("base-token")
	tableIDValue := baseTableID(runtime)
	viewRef := runtime.Str("view-id")
	body, err := parseJSONObject(pc, runtime.Str("json"), "json")
	if err != nil {
		return err
	}
	data, err := baseV3Call(runtime, "PUT", baseV3Path("bases", baseToken, "tables", tableIDValue, "views", viewRef, segment), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{key: data}, nil)
	return nil
}

func executeViewSetWrapped(runtime *common.RuntimeContext, segment string, wrapper string, key string) error {
	pc := newParseCtx(runtime)
	baseToken := runtime.Str("base-token")
	tableIDValue := baseTableID(runtime)
	viewRef := runtime.Str("view-id")
	raw, err := parseJSONValue(pc, runtime.Str("json"), "json")
	if err != nil {
		return err
	}
	payload := wrapViewPropertyBody(raw, wrapper)
	data, err := baseV3CallAny(runtime, "PUT", baseV3Path("bases", baseToken, "tables", tableIDValue, "views", viewRef, segment), nil, payload)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{key: data}, nil)
	return nil
}

func executeViewSetVisibleFields(runtime *common.RuntimeContext) error {
	pc := newParseCtx(runtime)
	baseToken := runtime.Str("base-token")
	tableIDValue := baseTableID(runtime)
	viewRef := runtime.Str("view-id")
	body, err := parseJSONObject(pc, runtime.Str("json"), "json")
	if err != nil {
		return err
	}
	data, err := baseV3CallAny(runtime, "PUT", baseV3Path("bases", baseToken, "tables", tableIDValue, "views", viewRef, "visible_fields"), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"visible_fields": data}, nil)
	return nil
}

func executeViewRename(runtime *common.RuntimeContext) error {
	baseToken := runtime.Str("base-token")
	tableIDValue := baseTableID(runtime)
	viewRef := runtime.Str("view-id")
	data, err := baseV3Call(runtime, "PATCH", baseV3Path("bases", baseToken, "tables", tableIDValue, "views", viewRef), nil, map[string]interface{}{"name": runtime.Str("name")})
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"view": data}, nil)
	return nil
}
