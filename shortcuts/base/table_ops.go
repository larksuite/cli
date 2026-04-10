// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"fmt"

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
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables").
		Body(map[string]interface{}{"name": runtime.Str("name")}).
		Set("base_token", runtime.Str("base-token"))
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
	items := make([]interface{}, 0, len(tables))
	for _, table := range tables {
		items = append(items, map[string]interface{}{"table_id": tableID(table), "table_name": tableNameFromMap(table)})
	}
	runtime.Out(map[string]interface{}{"items": items, "offset": offset, "limit": limit, "count": len(items), "total": total}, nil)
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
	created, err := baseV3Call(runtime, "POST", baseV3Path("bases", baseToken, "tables"), nil, map[string]interface{}{"name": runtime.Str("name")})
	if err != nil {
		return err
	}
	result := map[string]interface{}{"table": created}
	tableIDValue := tableID(created)
	pc := newParseCtx(runtime)
	if tableIDValue != "" && runtime.Str("fields") != "" {
		fieldItems, err := parseJSONArray(pc, runtime.Str("fields"), "fields")
		if err != nil {
			return err
		}
		defaultFields, err := listEveryField(runtime, baseToken, tableIDValue)
		if err != nil {
			return err
		}
		createdFields := []interface{}{}
		for idx, item := range fieldItems {
			body, ok := item.(map[string]interface{})
			if !ok {
				return fmt.Errorf("--fields item %d must be an object", idx+1)
			}
			if idx == 0 && len(defaultFields) > 0 {
				fieldData, err := baseV3Call(runtime, "PUT", baseV3Path("bases", baseToken, "tables", tableIDValue, "fields", fieldID(defaultFields[0])), nil, body)
				if err != nil {
					return err
				}
				createdFields = append(createdFields, fieldData)
				continue
			}
			fieldData, err := baseV3Call(runtime, "POST", baseV3Path("bases", baseToken, "tables", tableIDValue, "fields"), nil, body)
			if err != nil {
				return err
			}
			createdFields = append(createdFields, fieldData)
		}
		result["fields"] = createdFields
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
