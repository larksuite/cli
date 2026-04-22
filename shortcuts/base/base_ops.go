// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

const baseCreateHint = "Tip: New bases include a default empty table with 5-10 blank records. After finishing table/field setup on this base, ask whether to delete that default table. If yes, run +table-list first, then delete the default table."

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
		d.Desc("After Base copy succeeds in bot mode, the CLI will also try to grant the current CLI user full_access (可管理权限) on the new Base.")
	}
	return d
}

func dryRunBaseCreate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	d := common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases").
		Body(buildBaseCreateBody(runtime))
	if runtime.IsBot() {
		d.Desc("After Base creation succeeds in bot mode, the CLI will also try to grant the current CLI user full_access (可管理权限) on the new Base.")
	}
	return d
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
	augmentBasePermissionGrant(runtime, out, data)
	runtime.Out(out, nil)
	fmt.Fprintln(runtime.IO().ErrOut, baseCreateHint)
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
