// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

func dryRunBaseGet(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token").
		Set("base_token", runtime.Str("base-token"))
}

func dryRunBaseCopy(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	body := map[string]interface{}{}
	if name := strings.TrimSpace(runtime.Str("name")); name != "" {
		body["name"] = name
	}
	if folderToken := strings.TrimSpace(runtime.Str("folder-token")); folderToken != "" {
		body["folder_token"] = folderToken
	}
	if runtime.Bool("without-content") {
		body["without_content"] = true
	}
	if timeZone := strings.TrimSpace(runtime.Str("time-zone")); timeZone != "" {
		body["time_zone"] = timeZone
	}
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/copy").
		Body(body).
		Set("base_token", runtime.Str("base-token"))
}

func dryRunBaseCreate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	body := map[string]interface{}{"name": runtime.Str("name")}
	if folderToken := strings.TrimSpace(runtime.Str("folder-token")); folderToken != "" {
		body["folder_token"] = folderToken
	}
	if timeZone := strings.TrimSpace(runtime.Str("time-zone")); timeZone != "" {
		body["time_zone"] = timeZone
	}
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases").
		Body(body)
}

func executeBaseGet(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "GET", baseV3Path("bases", runtime.Str("base-token")), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"base": data}, nil)
	return nil
}

func executeBaseCopy(runtime *common.RuntimeContext) error {
	body := map[string]interface{}{}
	if name := strings.TrimSpace(runtime.Str("name")); name != "" {
		body["name"] = name
	}
	if folderToken := strings.TrimSpace(runtime.Str("folder-token")); folderToken != "" {
		body["folder_token"] = folderToken
	}
	if runtime.Bool("without-content") {
		body["without_content"] = true
	}
	if timeZone := strings.TrimSpace(runtime.Str("time-zone")); timeZone != "" {
		body["time_zone"] = timeZone
	}
	data, err := baseV3Call(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "copy"), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"base": data, "copied": true}, nil)
	return nil
}

func executeBaseCreate(runtime *common.RuntimeContext) error {
	body := map[string]interface{}{"name": runtime.Str("name")}
	if folderToken := strings.TrimSpace(runtime.Str("folder-token")); folderToken != "" {
		body["folder_token"] = folderToken
	}
	if timeZone := strings.TrimSpace(runtime.Str("time-zone")); timeZone != "" {
		body["time_zone"] = timeZone
	}
	data, err := baseV3Call(runtime, "POST", baseV3Path("bases"), nil, body)
	if err != nil {
		return err
	}

	deletedTotal := 0
	if !runtime.Bool("keep-empty-rows") {
		var baseToken string
		if m, ok := data["base"].(map[string]interface{}); ok {
			baseToken, _ = m["base_token"].(string)
		}
		if baseToken == "" {
			baseToken, _ = data["base_token"].(string)
		}
		if baseToken != "" {
			tables, _, err := listAllTables(runtime, baseToken, 0, 10)
			if err == nil && len(tables) > 0 {
				defaultTableID := tableID(tables[0])
				recordsData, err := baseV3Call(runtime, "GET", baseV3Path("bases", baseToken, "tables", defaultTableID, "records"), map[string]interface{}{"limit": 10}, nil)
				if err == nil {
					if rawItems, ok := recordsData["items"].([]interface{}); ok {
						for _, item := range rawItems {
							if recMap, ok := item.(map[string]interface{}); ok {
								// Emptiness guard: only delete if 'fields' is strictly empty (no values filled yet)
								fieldsMap, _ := recMap["fields"].(map[string]interface{})
								if len(fieldsMap) == 0 {
									if recID, ok := recMap["record_id"].(string); ok {
										_, delErr := baseV3Call(runtime, "DELETE", baseV3Path("bases", baseToken, "tables", defaultTableID, "records", recID), nil, nil)
										if delErr == nil {
											deletedTotal++
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	result := map[string]interface{}{"base": data, "created": true}
	if deletedTotal > 0 {
		result["_pruned_records"] = deletedTotal
	}
	runtime.Out(result, nil)
	return nil
}
