// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

func dryRunTableList(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	limit := runtime.Int("limit")
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
	viewItems, err := parseObjectList(newParseCtx(runtime), runtime.Str("view"), "view")
	if err != nil {
		return common.NewDryRunAPI().Desc(fmt.Sprintf("dry-run validation failed: %v", err))
	}
	body := dryRunTableCreateBody(runtime, runtime.Str("name"))
	d := common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables").
		Body(body).
		Set("base_token", runtime.Str("base-token"))
	if len(viewItems) > 0 {
		d.Set("created_table_id", "<created_table_id>")
		for _, view := range viewItems {
			d.POST("/open-apis/base/v3/bases/:base_token/tables/:created_table_id/views").Body(view)
		}
	}
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
	_, err := parseObjectList(newParseCtx(runtime), runtime.Str("view"), "view")
	return err
}

func executeTableList(runtime *common.RuntimeContext) error {
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	limit := runtime.Int("limit")
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
	viewItems, err := parseObjectList(pc, runtime.Str("view"), "view")
	if err != nil {
		return err
	}
	created, err := baseV3Call(runtime, "POST", baseV3Path("bases", baseToken, "tables"), nil, body)
	if err != nil {
		return err
	}
	result := map[string]interface{}{"table": created}
	tableIDValue := tableID(created)
	if tableIDValue == "" {
		result["message"] = "table create response omitted the new table ID; creation state is unknown"
		result["hint"] = "The table may have been created. Do not retry it blindly; use +table-list to reconcile the result before retrying or creating views."
		return runtime.OutPartialFailure(result, nil)
	}
	if runtime.Str("fields") != "" {
		if fields, ok := created["fields"]; ok {
			result["fields"] = fields
		}
	}
	if len(viewItems) > 0 {
		createdViews, failedIndex, err := createViews(runtime, baseToken, tableIDValue, viewItems)
		result["views"] = createdViews
		if err != nil {
			payload := viewCreateProgressPayload(err, createdViews, failedIndex)
			for key, value := range result {
				payload[key] = value
			}
			payload["message"] = fmt.Sprintf("table was created, but view creation failed at item %d after %d view(s) succeeded: %v", failedIndex, len(createdViews), err)
			payload["hint"] = "The table and any earlier views were created. Do not retry the table or successful views; inspect the failed view item before deciding whether to retry it."
			return runtime.OutPartialFailure(payload, nil)
		}
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
