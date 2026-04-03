// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

const recordListViewResolvePageLimit = 200

func isViewIDRef(viewRef string) bool {
	return strings.HasPrefix(viewRef, "vew_") || strings.HasPrefix(viewRef, "viw_")
}

func dryRunRecordList(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	limit := common.ParseIntBounded(runtime, "limit", 1, 200)
	params := map[string]interface{}{"offset": offset, "limit": limit}
	if viewRef := runtime.Str("view-id"); viewRef != "" {
		if isViewIDRef(viewRef) {
			params["view_id"] = viewRef
		} else {
			params["view_id"] = fmt.Sprintf("<resolved from view name: %s>", viewRef)
		}
	}
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/records").
		Params(params).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime))
}

func resolveRecordListViewID(runtime *common.RuntimeContext, viewRef string) (string, error) {
	if viewRef == "" {
		return "", nil
	}
	if isViewIDRef(viewRef) {
		return viewRef, nil
	}

	offset := 0
	for {
		views, total, err := listAllViews(runtime, runtime.Str("base-token"), baseTableID(runtime), offset, recordListViewResolvePageLimit)
		if err != nil {
			return "", err
		}
		if view, err := resolveViewRef(views, viewRef); err == nil {
			return viewID(view), nil
		}
		if len(views) == 0 {
			if total > 0 && offset+recordListViewResolvePageLimit < total {
				offset += recordListViewResolvePageLimit
				continue
			}
			break
		}
		offset += len(views)
		if total > 0 && offset >= total {
			break
		}
	}

	return "", fmt.Errorf("view %q not found", viewRef)
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
	return nil
}

func executeRecordList(runtime *common.RuntimeContext) error {
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	limit := common.ParseIntBounded(runtime, "limit", 1, 200)
	params := map[string]interface{}{"offset": offset, "limit": limit}
	if viewRef := runtime.Str("view-id"); viewRef != "" {
		resolvedViewID, err := resolveRecordListViewID(runtime, viewRef)
		if err != nil {
			return err
		}
		params["view_id"] = resolvedViewID
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
