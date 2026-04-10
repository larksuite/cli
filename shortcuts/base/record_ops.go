```go
// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

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

func recordBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	jsonVal := strings.TrimSpace(runtime.Str("json"))
	fieldsVal := strings.TrimSpace(runtime.Str("fields"))
	if jsonVal != "" && fieldsVal != "" {
		return nil, common.FlagErrorf("--json and --fields are mutually exclusive")
	}
	if jsonVal != "" {
		return parseJSONObject(jsonVal, "json")
	}
	if fieldsVal != "" {
		return parseJSONObject(fieldsVal, "fields")
	}
	return nil, common.FlagErrorf("provide --json or --fields")
}

func dryRunRecordUpsert(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	body, _ := recordBody(runtime)
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
	jsonVal := strings.TrimSpace(runtime.Str("json"))
	fieldsVal := strings.TrimSpace(runtime.Str("fields"))
	if jsonVal != "" && fieldsVal != "" {
		return common.FlagErrorf("--json and --fields are mutually exclusive")
	}
	if jsonVal == "" && fieldsVal == "" {
		return common.FlagErrorf("provide --json or --fields")
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
	body, err := recordBody(runtime)
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
```