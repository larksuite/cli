// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

func dryRunTableList(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	limit := common.ParseIntBounded(runtime, "limit", 1, 100)
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token/tables").
		Params(map[string]interface{}{"offset": offset, "limit": limit}).
		Set("base_token", runtime.Str("base-token"))
}

func dryRunTableGet(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token/tables/:table_id").
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", runtime.Str("table-id"))
}

func dryRunTableCreate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	body := dryRunTableCreateBody(runtime, runtime.Str("name"))
	d := common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables").
		Body(body).
		Set("base_token", runtime.Str("base-token"))
	return d
}

func dryRunTableUpdate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		PATCH("/open-apis/base/v3/bases/:base_token/tables/:table_id").
		Body(map[string]interface{}{"name": runtime.Str("name")}).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", runtime.Str("table-id"))
}

func dryRunTableDelete(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		DELETE("/open-apis/base/v3/bases/:base_token/tables/:table_id").
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", runtime.Str("table-id"))
}

func validateTableCreate(runtime *common.RuntimeContext) error {
	return nil
}

func executeTableList(runtime *common.RuntimeContext) error {
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	limit := common.ParseIntBounded(runtime, "limit", 1, 100)
	tables, total, err := listAllTables(runtime, runtime.Str("base-token"), offset, limit)
	if err != nil {
		return err
	}
	if total == 0 {
		total = len(tables)
	}
	runtime.Out(map[string]interface{}{"tables": tables, "total": total}, nil)
	return nil
}

func executeTableGet(runtime *common.RuntimeContext) error {
	baseToken := runtime.Str("base-token")
	tableIDValue := runtime.Str("table-id")
	table, err := baseV3Call(runtime, "GET", baseV3Path("bases", baseToken, "tables", tableIDValue), nil, nil)
	if err != nil {
		return err
	}
	fields, err := listEveryField(runtime, baseToken, tableIDValue)
	if err != nil {
		return err
	}
	views, err := listEveryView(runtime, baseToken, tableIDValue)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{
		"table":  table,
		"fields": fields,
		"views":  views,
	}, nil)
	return nil
}

func executeTableCreate(runtime *common.RuntimeContext) error {
	baseToken := runtime.Str("base-token")
	pc := newParseCtx(runtime)
	body, err := buildTableCreateBody(runtime, pc, runtime.Str("name"))
	if err != nil {
		return err
	}
	created, err := baseV3Call(runtime, "POST", baseV3Path("bases", baseToken, "tables"), nil, body)
	if err != nil {
		return err
	}
	result := map[string]interface{}{"table": created}
	tableIDValue := tableID(created)
	if tableIDValue != "" && runtime.Str("fields") != "" {
		if fields, ok := created["fields"]; ok {
			result["fields"] = fields
		}
	}
	if tableIDValue != "" && runtime.Str("view") != "" {
		viewItems, err := parseObjectList(pc, runtime.Str("view"), "view")
		if err != nil {
			return err
		}
		createdViews := []interface{}{}
		for _, body := range viewItems {
			viewData, err := baseV3Call(runtime, "POST", baseV3Path("bases", baseToken, "tables", tableIDValue, "views"), nil, body)
			if err != nil {
				return err
			}
			createdViews = append(createdViews, viewData)
		}
		result["views"] = createdViews
	}
	runtime.Out(result, nil)
	return nil
}

func buildTableCreateBody(runtime *common.RuntimeContext, pc *parseCtx, tableName string) (map[string]interface{}, error) {
	body := map[string]interface{}{"name": tableName}
	if strings.TrimSpace(runtime.Str("fields")) == "" {
		return body, nil
	}
	fieldItems, err := parseJSONArray(pc, runtime.Str("fields"), "fields")
	if err != nil {
		return nil, err
	}
	for idx, item := range fieldItems {
		if _, ok := item.(map[string]interface{}); !ok {
			return nil, baseValidationErrorf("--fields item %d must be an object", idx+1)
		}
	}
	if len(fieldItems) > 0 {
		body["fields"] = fieldItems
	}
	return body, nil
}

func dryRunTableCreateBody(runtime *common.RuntimeContext, tableName string) map[string]interface{} {
	body := map[string]interface{}{"name": tableName}
	if strings.TrimSpace(runtime.Str("fields")) == "" {
		return body
	}
	fieldItems, err := parseJSONArray(newParseCtx(runtime), runtime.Str("fields"), "fields")
	if err != nil {
		body["fields"] = "<invalid_fields_json>"
		return body
	}
	body["fields"] = fieldItems
	return body
}

func listEveryField(runtime *common.RuntimeContext, baseToken, tableID string) ([]map[string]interface{}, error) {
	const pageLimit = 100
	offset := 0
	items := []map[string]interface{}{}
	for {
		batch, total, err := listAllFields(runtime, baseToken, tableID, offset, pageLimit)
		if err != nil {
			return nil, err
		}
		items = append(items, batch...)
		if len(batch) == 0 || len(batch) < pageLimit || (total > 0 && len(items) >= total) {
			break
		}
		offset += len(batch)
	}
	return items, nil
}

func listEveryView(runtime *common.RuntimeContext, baseToken, tableID string) ([]map[string]interface{}, error) {
	const pageLimit = 100
	offset := 0
	items := []map[string]interface{}{}
	for {
		batch, total, err := listAllViews(runtime, baseToken, tableID, offset, pageLimit)
		if err != nil {
			return nil, err
		}
		items = append(items, batch...)
		if len(batch) == 0 || len(batch) < pageLimit || (total > 0 && len(items) >= total) {
			break
		}
		offset += len(batch)
	}
	return items, nil
}

func executeTableUpdate(runtime *common.RuntimeContext) error {
	baseToken := runtime.Str("base-token")
	tableIDValue := runtime.Str("table-id")
	data, err := baseV3Call(runtime, "PATCH", baseV3Path("bases", baseToken, "tables", tableIDValue), nil, map[string]interface{}{"name": runtime.Str("name")})
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"table": data, "updated": true}, nil)
	return nil
}

func executeTableDelete(runtime *common.RuntimeContext) error {
	baseToken := runtime.Str("base-token")
	tableIDValue := runtime.Str("table-id")
	_, err := baseV3Call(runtime, "DELETE", baseV3Path("bases", baseToken, "tables", tableIDValue), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"deleted": true, "table_id": tableIDValue, "table_name": tableIDValue}, nil)
	return nil
}
