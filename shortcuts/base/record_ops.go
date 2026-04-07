// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

func dryRunRecordList(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	limit := common.ParseIntBounded(runtime, "limit", 1, 200)
	params := map[string]interface{}{"offset": offset, "limit": limit}
	if viewID := runtime.Str("view-id"); viewID != "" {
		params["view_id"] = viewID
	}
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/records").
		Params(params).
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

func dryRunRecordUpsert(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	body, _ := parseJSONObject(runtime.Str("json"), "json")
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
	body, err := parseJSONObject(runtime.Str("json"), "json")
	if err != nil {
		// Keep invalid JSON handling on the execution path unchanged; only
		// intercept the common top-level shape mistake here.
		return nil
	}
	if len(body) == 1 {
		if fields, ok := body["fields"].(map[string]interface{}); ok && len(fields) > 0 {
			return common.FlagErrorf("--json for +record-upsert must be a direct record object, not a top-level \"fields\" wrapper; use '{\"Name\":\"Alice\"}' instead of '{\"fields\":{\"Name\":\"Alice\"}}'. If your real field name is literally \"fields\", use the field ID as the key.")
		}
	}
	return nil
}

func executeRecordList(runtime *common.RuntimeContext) error {
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	limit := common.ParseIntBounded(runtime, "limit", 1, 200)
	params := map[string]interface{}{"offset": offset, "limit": limit}
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

func executeRecordUpsert(runtime *common.RuntimeContext) error {
	body, err := parseJSONObject(runtime.Str("json"), "json")
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

func executeRecordDelete(runtime *common.RuntimeContext) error {
	_, err := baseV3Call(runtime, "DELETE", baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "records", runtime.Str("record-id")), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"deleted": true, "record_id": runtime.Str("record-id")}, nil)
	return nil
}
