// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var SheetCreate = common.Shortcut{
	Service:     "sheets",
	Command:     "+create",
	Description: "Create a spreadsheet (optional header row and initial data)",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:create", "sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "title", Desc: "spreadsheet title", Required: true},
		{Name: "folder-token", Desc: "target folder token"},
		{Name: "headers", Desc: "header row JSON array"},
		{Name: "data", Desc: "initial data JSON 2D array"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if headersStr := runtime.Str("headers"); headersStr != "" {
			var headers []interface{}
			if err := json.Unmarshal([]byte(headersStr), &headers); err != nil {
				return common.FlagErrorf("--headers invalid JSON, must be a 1D array")
			}
		}
		if dataStr := runtime.Str("data"); dataStr != "" {
			var rows [][]interface{}
			if err := json.Unmarshal([]byte(dataStr), &rows); err != nil {
				return common.FlagErrorf("--data invalid JSON, must be a 2D array")
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		d := common.NewDryRunAPI().
			POST("/open-apis/sheets/v3/spreadsheets").
			Body(map[string]interface{}{"title": runtime.Str("title")})
		if runtime.IsBot() {
			d.Desc("After spreadsheet creation succeeds in bot mode, the CLI will also try to grant the current CLI user full_access (可管理权限) on the new spreadsheet.")
		}
		return d
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		title := runtime.Str("title")
		folderToken := runtime.Str("folder-token")
		headersStr := runtime.Str("headers")
		dataStr := runtime.Str("data")
		var allRows []interface{}

		if headersStr != "" {
			var headers []interface{}
			if err := json.Unmarshal([]byte(headersStr), &headers); err != nil {
				return common.FlagErrorf("--headers invalid JSON, must be a 1D array")
			}
			if len(headers) > 0 {
				allRows = append(allRows, headers)
			}
		}

		if dataStr != "" {
			var rows []interface{}
			if err := json.Unmarshal([]byte(dataStr), &rows); err != nil {
				return common.FlagErrorf("--data invalid JSON, must be a 2D array")
			}
			if len(rows) > 0 {
				allRows = append(allRows, rows...)
			}
		}

		createData := map[string]interface{}{"title": title}
		if folderToken != "" {
			createData["folder_token"] = folderToken
		}

		data, err := runtime.CallAPI("POST", "/open-apis/sheets/v3/spreadsheets", nil, createData)
		if err != nil {
			return err
		}

		spreadsheet, _ := data["spreadsheet"].(map[string]interface{})
		token, _ := spreadsheet["spreadsheet_token"].(string)

		// Write headers and data if provided
		if len(allRows) > 0 && token != "" {
			appendRange, err := getFirstSheetID(runtime, token)
			if err != nil {
				return err
			}
			if _, err := runtime.CallAPI("POST", fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/values_append", validate.EncodePathSegment(token)), nil, map[string]interface{}{
				"valueRange": map[string]interface{}{
					"range":  appendRange,
					"values": allRows,
				},
			}); err != nil {
				return err
			}
		}

		out := map[string]interface{}{
			"spreadsheet_token": token,
			"title":             title,
			"url":               spreadsheet["url"],
		}
		if grant := common.AutoGrantCurrentUserDrivePermission(runtime, token, "sheet"); grant != nil {
			out["permission_grant"] = grant
		}

		runtime.Out(out, nil)
		return nil
	},
}
