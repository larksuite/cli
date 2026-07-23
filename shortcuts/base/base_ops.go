// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const baseCreateHint = "Tip: Base created the platform default first-table schema. To configure the initial table schema during +base-create, pass both --table-name and --fields."

var baseCreateDefaultTableDeleteDelay = time.Second

func dryRunBaseGet(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token").
		Set("base_token", runtime.Str("base-token"))
}

func dryRunBaseCopy(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	d := common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/copy").
		Body(buildBaseCopyBody(runtime)).
		Set("base_token", runtime.Str("base-token"))
	if runtime.IsBot() {
		d.Desc("After Base copy succeeds in bot mode, the CLI will also try to grant the current CLI user full_access on the new Base.")
	}
	return d
}

func dryRunBaseCreate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	d := common.NewDryRunAPI()
	if runtime.IsBot() {
		d.Desc("After Base creation succeeds in bot mode, the CLI will also try to grant the current CLI user full_access on the new Base.")
	}
	d.
		POST("/open-apis/base/v3/bases").
		Body(buildBaseCreateBody(runtime))
	hasFields := strings.TrimSpace(runtime.Str("fields")) != ""
	hasTableName := strings.TrimSpace(runtime.Str("table-name")) != ""
	if hasFields {
		d.GET("/open-apis/base/v3/bases/:created_base_token/tables").
			Params(map[string]interface{}{"offset": 0, "limit": 100}).
			Desc("If the create response does not include the default table ID, the CLI lists tables to find it.").
			Set("created_base_token", "<created_base_token>")
		d.POST("/open-apis/base/v3/bases/:created_base_token/tables").
			Body(dryRunTableCreateBody(runtime, baseCreateFirstTableName(runtime))).
			Desc("Create a replacement first table with --fields in the create-table body.")
		d.DELETE("/open-apis/base/v3/bases/:created_base_token/tables/:default_table_id").
			Desc("After a 1s wait, delete the default table created with the Base.").
			Set("default_table_id", "<default_table_id>")
	} else if hasTableName {
		d.GET("/open-apis/base/v3/bases/:created_base_token/tables").
			Params(map[string]interface{}{"offset": 0, "limit": 100}).
			Desc("If the create response does not include the default table ID, the CLI lists tables to find it.").
			Set("created_base_token", "<created_base_token>")
		d.PATCH("/open-apis/base/v3/bases/:created_base_token/tables/:default_table_id").
			Body(map[string]interface{}{"name": baseCreateFirstTableName(runtime)}).
			Desc("Rename the default first table when only --table-name is provided.").
			Set("default_table_id", "<default_table_id>")
	}
	return d
}

func validateBaseCreate(runtime *common.RuntimeContext) error {
	return nil
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
	data, err := baseV3Call(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "copy"), nil, buildBaseCopyBody(runtime))
	if err != nil {
		return err
	}
	out := map[string]interface{}{"base": data, "copied": true}
	augmentBasePermissionGrant(runtime, out, data)
	runtime.Out(out, nil)
	return nil
}

func executeBaseCreate(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "POST", baseV3Path("bases"), nil, buildBaseCreateBody(runtime))
	if err != nil {
		return err
	}
	out := map[string]interface{}{"base": data, "created": true}
	if strings.TrimSpace(runtime.Str("fields")) != "" {
		customTable, createdFields, defaultTableID, err := replaceBaseDefaultTable(runtime, data)
		if err != nil {
			return err
		}
		out["table"] = customTable
		out["fields"] = createdFields
		out["default_table_deleted"] = true
		out["deleted_default_table_id"] = defaultTableID
	} else if strings.TrimSpace(runtime.Str("table-name")) != "" {
		renamedTable, defaultTableID, err := renameBaseDefaultTable(runtime, data)
		if err != nil {
			return err
		}
		out["table"] = renamedTable
		out["default_table_renamed"] = true
		out["renamed_default_table_id"] = defaultTableID
	} else {
		fmt.Fprintln(runtime.IO().ErrOut, baseCreateHint)
	}
	augmentBasePermissionGrant(runtime, out, data)
	runtime.Out(out, nil)
	return nil
}

func buildBaseCopyBody(runtime *common.RuntimeContext) map[string]interface{} {
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
	return body
}

func buildBaseCreateBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{"name": runtime.Str("name")}
	if folderToken := strings.TrimSpace(runtime.Str("folder-token")); folderToken != "" {
		body["folder_token"] = folderToken
	}
	if timeZone := strings.TrimSpace(runtime.Str("time-zone")); timeZone != "" {
		body["time_zone"] = timeZone
	}
	return body
}

func augmentBasePermissionGrant(runtime *common.RuntimeContext, out, base map[string]interface{}) {
	if grant := common.AutoGrantCurrentUserDrivePermission(runtime, extractBasePermissionToken(base), "bitable"); grant != nil {
		out["permission_grant"] = grant
	}
}

func extractBasePermissionToken(base map[string]interface{}) string {
	for _, key := range []string{"base_token", "app_token"} {
		if token := strings.TrimSpace(common.GetString(base, key)); token != "" {
			return token
		}
	}
	return ""
}

func replaceBaseDefaultTable(runtime *common.RuntimeContext, base map[string]interface{}) (map[string]interface{}, []interface{}, string, error) {
	baseToken := extractBasePermissionToken(base)
	if baseToken == "" {
		return nil, nil, "", errs.NewInternalError(errs.SubtypeInvalidResponse, "+base-create --fields could not find base_token/app_token in create response")
	}
	defaultTableID, err := findCreatedBaseDefaultTableID(runtime, baseToken, base, "--fields")
	if err != nil {
		return nil, nil, "", err
	}
	tableBody, err := buildTableCreateBody(runtime, newParseCtx(runtime), baseCreateFirstTableName(runtime))
	if err != nil {
		return nil, nil, "", err
	}
	customTable, err := baseV3Call(runtime, "POST", baseV3Path("bases", baseToken, "tables"), nil, tableBody)
	if err != nil {
		return nil, nil, "", err
	}
	customTableID := tableID(customTable)
	if customTableID == "" {
		return nil, nil, "", errs.NewInternalError(errs.SubtypeInvalidResponse, "+base-create --fields could not find table_id/id in replacement table create response")
	}
	createdFields := []interface{}{}
	if fields, ok := customTable["fields"].([]interface{}); ok {
		createdFields = fields
	}
	time.Sleep(baseCreateDefaultTableDeleteDelay)
	if _, err := baseV3Call(runtime, "DELETE", baseV3Path("bases", baseToken, "tables", defaultTableID), nil, nil); err != nil {
		return nil, nil, "", err
	}
	return customTable, createdFields, defaultTableID, nil
}

func renameBaseDefaultTable(runtime *common.RuntimeContext, base map[string]interface{}) (map[string]interface{}, string, error) {
	baseToken := extractBasePermissionToken(base)
	if baseToken == "" {
		return nil, "", errs.NewInternalError(errs.SubtypeInvalidResponse, "+base-create --table-name could not find base_token/app_token in create response")
	}
	defaultTableID, err := findCreatedBaseDefaultTableID(runtime, baseToken, base, "--table-name")
	if err != nil {
		return nil, "", err
	}
	renamedTable, err := baseV3Call(
		runtime,
		"PATCH",
		baseV3Path("bases", baseToken, "tables", defaultTableID),
		nil,
		map[string]interface{}{"name": baseCreateFirstTableName(runtime)},
	)
	if err != nil {
		return nil, "", err
	}
	return renamedTable, defaultTableID, nil
}

func baseCreateFirstTableName(runtime *common.RuntimeContext) string {
	if name := strings.TrimSpace(runtime.Str("table-name")); name != "" {
		return name
	}
	return "Table 1"
}

func findCreatedBaseDefaultTableID(runtime *common.RuntimeContext, baseToken string, base map[string]interface{}, flag string) (string, error) {
	if tableIDValue := tableIDFromCreateBaseResponse(base); tableIDValue != "" {
		return tableIDValue, nil
	}
	tables, _, err := listAllTables(runtime, baseToken, 0, 100)
	if err != nil {
		return "", err
	}
	if len(tables) == 0 {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "+base-create "+flag+" could not find the default table created with the Base")
	}
	if id := tableID(tables[0]); id != "" {
		return id, nil
	}
	return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "+base-create "+flag+" could not find table_id/id in default table list response")
}

func tableIDFromCreateBaseResponse(base map[string]interface{}) string {
	for _, key := range []string{"table_id", "default_table_id"} {
		if id := strings.TrimSpace(common.GetString(base, key)); id != "" {
			return id
		}
	}
	for _, key := range []string{"table", "default_table"} {
		if table, ok := base[key].(map[string]interface{}); ok {
			if id := tableID(table); id != "" {
				return id
			}
		}
	}
	for _, key := range []string{"tables", "default_tables"} {
		if tables, ok := base[key].([]interface{}); ok && len(tables) > 0 {
			if table, ok := tables[0].(map[string]interface{}); ok {
				if id := tableID(table); id != "" {
					return id
				}
			}
		}
	}
	return ""
}
