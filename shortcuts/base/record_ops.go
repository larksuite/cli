// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"net/url"
	"strconv"

	"github.com/larksuite/cli/shortcuts/common"
)

func dryRunRecordList(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	limit := common.ParseIntBounded(runtime, "limit", 1, 200)
	params := url.Values{}
	params.Set("offset", strconv.Itoa(offset))
	params.Set("limit", strconv.Itoa(limit))
	for _, field := range recordListFields(runtime) {
		params.Add("field_id", field)
	}
	if viewID := runtime.Str("view-id"); viewID != "" {
		params.Set("view_id", viewID)
	}
	path := "/open-apis/base/v3/bases/:base_token/tables/:table_id/records?" + params.Encode()
	return common.NewDryRunAPI().
		GET(path).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime))
}

func dryRunRecordGet(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/records/:record_id").
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		Set("record_id", runtime.Str("record-id"))
}

func dryRunRecordSearch(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	pc := newParseCtx(runtime)
	body, _ := parseJSONObject(pc, runtime.Str("json"), "json")
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/records/search").
		Body(body).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime))
}

func dryRunRecordUpsert(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	pc := newParseCtx(runtime)
	body, _ := parseJSONObject(pc, runtime.Str("json"), "json")
	if recordID := runtime.Str("record-id"); recordID != "" {
		return common.NewDryRunAPI().
			PATCH("/open-apis/base/v3/bases/:base_token/tables/:table_id/records/:record_id").
			Body(body).
			Set("base_token", runtime.Str("base-token")).
			Set("table_id", baseTableID(runtime)).
			Set("record_id", recordID)
	}
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/records").
		Body(body).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime))
}

func dryRunRecordBatchCreate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	pc := newParseCtx(runtime)
	body, _ := parseJSONObject(pc, runtime.Str("json"), "json")
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/records/batch_create").
		Body(body).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime))
}

func dryRunRecordBatchUpdate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	pc := newParseCtx(runtime)
	body, _ := parseJSONObject(pc, runtime.Str("json"), "json")
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/records/batch_update").
		Body(body).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime))
}

func dryRunRecordDelete(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		DELETE("/open-apis/base/v3/bases/:base_token/tables/:table_id/records/:record_id").
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		Set("record_id", runtime.Str("record-id"))
}

func dryRunRecordHistoryList(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	params := map[string]interface{}{
		"table_id":  baseTableID(runtime),
		"record_id": runtime.Str("record-id"),
		"page_size": runtime.Int("page-size"),
	}
	if value := runtime.Int("max-version"); value > 0 {
		params["max_version"] = value
	}
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token/record_history").
		Params(params).
		Set("base_token", runtime.Str("base-token"))
}

func validateRecordJSON(runtime *common.RuntimeContext) error {
	return nil
}

func recordListFields(runtime *common.RuntimeContext) []string {
	return runtime.StrArray("field-id")
}

func executeRecordList(runtime *common.RuntimeContext) error {
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	limit := common.ParseIntBounded(runtime, "limit", 1, 200)
	params := map[string]interface{}{"offset": offset, "limit": limit}
	fields := recordListFields(runtime)
	if len(fields) > 0 {
		params["field_id"] = fields
	}
	if viewID := runtime.Str("view-id"); viewID != "" {
		params["view_id"] = viewID
	}
	data, err := baseV3Call(runtime, "GET", baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "records"), params, nil)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

func executeRecordGet(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "GET", baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "records", runtime.Str("record-id")), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

func executeRecordSearch(runtime *common.RuntimeContext) error {
	pc := newParseCtx(runtime)
	body, err := parseJSONObject(pc, runtime.Str("json"), "json")
	if err != nil {
		return err
	}
	data, err := baseV3Call(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "records", "search"), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

func executeRecordUpsert(runtime *common.RuntimeContext) error {
	pc := newParseCtx(runtime)
	body, err := parseJSONObject(pc, runtime.Str("json"), "json")
	if err != nil {
		return err
	}
	baseToken := runtime.Str("base-token")
	tableIDValue := baseTableID(runtime)
	if recordID := runtime.Str("record-id"); recordID != "" {
		data, err := baseV3Call(runtime, "PATCH", baseV3Path("bases", baseToken, "tables", tableIDValue, "records", recordID), nil, body)
		if err != nil {
			return err
		}
		runtime.Out(map[string]interface{}{"record": data, "updated": true}, nil)
		return nil
	}
	data, err := baseV3Call(runtime, "POST", baseV3Path("bases", baseToken, "tables", tableIDValue, "records"), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"record": data, "created": true}, nil)
	return nil
}

func executeRecordBatchCreate(runtime *common.RuntimeContext) error {
	pc := newParseCtx(runtime)
	body, err := parseJSONObject(pc, runtime.Str("json"), "json")
	if err != nil {
		return err
	}
	result, err := baseV3Raw(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "records", "batch_create"), nil, body)
	data, err := handleBaseAPIResult(result, err, "batch create records")
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

func executeRecordBatchUpdate(runtime *common.RuntimeContext) error {
	pc := newParseCtx(runtime)
	body, err := parseJSONObject(pc, runtime.Str("json"), "json")
	if err != nil {
		return err
	}
	result, err := baseV3Raw(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "records", "batch_update"), nil, body)
	data, err := handleBaseAPIResult(result, err, "batch update records")
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

func executeRecordDelete(runtime *common.RuntimeContext) error {
	_, err := baseV3Call(runtime, "DELETE", baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "records", runtime.Str("record-id")), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"deleted": true, "record_id": runtime.Str("record-id")}, nil)
	return nil
}
