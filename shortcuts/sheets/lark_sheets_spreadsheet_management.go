// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var SheetInfo = common.Shortcut{
	Service:     "sheets",
	Command:     "+info",
	Description: "View spreadsheet and sheet information",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}
		if token == "" {
			return common.FlagErrorf("specify --url or --spreadsheet-token")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}
		return common.NewDryRunAPI().
			GET("/open-apis/sheets/v3/spreadsheets/:token").
			Set("token", token)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}

		spreadsheetData, err := runtime.CallAPI("GET", fmt.Sprintf("/open-apis/sheets/v3/spreadsheets/%s", validate.EncodePathSegment(token)), nil, nil)
		if err != nil {
			return err
		}

		var sheetsData interface{}
		sheetsResult, sheetsErr := runtime.RawAPI("GET", fmt.Sprintf("/open-apis/sheets/v3/spreadsheets/%s/sheets/query", validate.EncodePathSegment(token)), nil, nil)
		if sheetsErr == nil {
			if sheetsMap, ok := sheetsResult.(map[string]interface{}); ok {
				if d, ok := sheetsMap["data"].(map[string]interface{}); ok {
					sheetsData = d
				}
			}
		}

		runtime.Out(map[string]interface{}{
			"spreadsheet": spreadsheetData,
			"sheets":      sheetsData,
		}, nil)
		return nil
	},
}

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
		body := map[string]interface{}{"title": runtime.Str("title")}
		if folderToken := runtime.Str("folder-token"); folderToken != "" {
			body["folder_token"] = folderToken
		}
		d := common.NewDryRunAPI().
			POST("/open-apis/sheets/v3/spreadsheets").
			Body(body)
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
				allRows = append(allRows, any(headers))
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
		}
		url, _ := spreadsheet["url"].(string)
		if url = strings.TrimSpace(url); url != "" {
			out["url"] = url
		} else if u := common.BuildResourceURL(runtime.Config.Brand, "sheet", token); u != "" {
			out["url"] = u
		}
		if grant := common.AutoGrantCurrentUserDrivePermission(runtime, token, "sheet"); grant != nil {
			out["permission_grant"] = grant
		}

		runtime.Out(out, nil)
		return nil
	},
}

var SheetExport = common.Shortcut{
	Service:     "sheets",
	Command:     "+export",
	Description: "Export a spreadsheet (async task polling + optional download)",
	Risk:        "read",
	Scopes:      []string{"docs:document:export", "drive:file:download"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "file-extension", Desc: "export format: xlsx | csv", Required: true, Enum: []string{"xlsx", "csv"}},
		{Name: "output-path", Desc: "local save path"},
		{Name: "sheet-id", Desc: "sheet ID (required for CSV)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateSheetManageToken(runtime); err != nil {
			return err
		}
		if runtime.Str("file-extension") == "csv" && strings.TrimSpace(runtime.Str("sheet-id")) == "" {
			return common.FlagErrorf("--sheet-id is required when --file-extension is csv")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateSheetManageToken(runtime)
		body := map[string]interface{}{
			"token":          token,
			"type":           "sheet",
			"file_extension": runtime.Str("file-extension"),
		}
		if sheetID := strings.TrimSpace(runtime.Str("sheet-id")); sheetID != "" {
			body["sub_id"] = sheetID
		}
		return common.NewDryRunAPI().
			POST("/open-apis/drive/v1/export_tasks").
			Body(body).
			Set("token", token).Set("ext", runtime.Str("file-extension"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateSheetManageToken(runtime)

		fileExt := runtime.Str("file-extension")
		outputPath := runtime.Str("output-path")
		sheetID := runtime.Str("sheet-id")

		if outputPath != "" {
			if _, err := runtime.ResolveSavePath(outputPath); err != nil {
				return output.ErrValidation("unsafe output path: %s", err)
			}
		}

		exportData := map[string]interface{}{
			"token":          token,
			"type":           "sheet",
			"file_extension": fileExt,
		}
		if sheetID != "" {
			exportData["sub_id"] = sheetID
		}

		data, err := runtime.CallAPI("POST", "/open-apis/drive/v1/export_tasks", nil, exportData)
		if err != nil {
			return err
		}
		ticket, _ := data["ticket"].(string)

		fmt.Fprintf(runtime.IO().ErrOut, "Waiting for export task to complete...\n")
		var fileToken string
		for i := 0; i < 50; i++ {
			time.Sleep(600 * time.Millisecond)
			pollResult, err := runtime.RawAPI("GET", "/open-apis/drive/v1/export_tasks/"+ticket, map[string]interface{}{"token": token}, nil)
			if err != nil {
				continue
			}
			pollMap, _ := pollResult.(map[string]interface{})
			pollData, _ := pollMap["data"].(map[string]interface{})
			pollResult2, _ := pollData["result"].(map[string]interface{})
			if pollResult2 != nil {
				ft, _ := pollResult2["file_token"].(string)
				if ft != "" {
					fileToken = ft
					break
				}
			}
		}

		if fileToken == "" {
			return output.Errorf(output.ExitAPI, "api_error", "export task timed out")
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Export complete: file_token=%s\n", fileToken)

		if outputPath == "" {
			runtime.Out(map[string]interface{}{
				"file_token": fileToken,
				"ticket":     ticket,
			}, nil)
			return nil
		}

		resp, err := runtime.DoAPIStream(ctx, &larkcore.ApiReq{
			HttpMethod: http.MethodGet,
			ApiPath:    fmt.Sprintf("/open-apis/drive/v1/export_tasks/file/%s/download", validate.EncodePathSegment(fileToken)),
		})
		if err != nil {
			return output.ErrNetwork("download failed: %s", err)
		}
		defer resp.Body.Close()

		result, err := runtime.FileIO().Save(outputPath, fileio.SaveOptions{
			ContentType:   resp.Header.Get("Content-Type"),
			ContentLength: resp.ContentLength,
		}, resp.Body)
		if err != nil {
			return common.WrapSaveErrorByCategory(err, "io")
		}

		savedPath, _ := runtime.ResolveSavePath(outputPath)
		if savedPath == "" {
			savedPath = outputPath
		}
		runtime.Out(map[string]interface{}{
			"saved_path": savedPath,
			"size_bytes": result.Size(),
		}, nil)
		return nil
	},
}
