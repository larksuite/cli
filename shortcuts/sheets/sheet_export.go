// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/shortcuts/common"
)

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
		{Name: "file-extension", Desc: "export format: xlsx | csv", Required: true},
		{Name: "output-path", Desc: "local save path"},
		{Name: "sheet-id", Desc: "sheet ID (required for CSV)"},
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
			POST("/open-apis/drive/v1/export_tasks").
			Body(map[string]interface{}{"token": token, "type": "sheet", "file_extension": runtime.Str("file-extension")}).
			Set("token", token).Set("ext", runtime.Str("file-extension"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}

		fileExt := runtime.Str("file-extension")
		outputPath := runtime.Str("output-path")
		sheetIdFlag := runtime.Str("sheet-id")

		// Early path validation before any API call
		if outputPath != "" {
			if _, err := validate.SafeOutputPath(outputPath); err != nil {
				return output.ErrValidation("unsafe output path: %s", err)
			}
		}

		// Create export task
		exportData := map[string]interface{}{
			"token":          token,
			"type":           "sheet",
			"file_extension": fileExt,
		}
		if sheetIdFlag != "" {
			exportData["sub_id"] = sheetIdFlag
		}

		data, err := runtime.CallAPI("POST", "/open-apis/drive/v1/export_tasks", nil, exportData)
		if err != nil {
			return err
		}
		ticket, _ := data["ticket"].(string)

		// Poll for completion
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
		}

		// Download
		resp, err := runtime.DoAPIStream(ctx, &larkcore.ApiReq{
			HttpMethod: http.MethodGet,
			ApiPath:    fmt.Sprintf("/open-apis/drive/v1/export_tasks/file/%s/download", validate.EncodePathSegment(fileToken)),
		})
		if err != nil {
			return output.ErrNetwork("download failed: %s", err)
		}
		defer resp.Body.Close()

		safePath, pathErr := validate.SafeOutputPath(outputPath)
		if pathErr != nil {
			return output.ErrValidation("unsafe output path: %s", pathErr)
		}
		if err := vfs.MkdirAll(filepath.Dir(safePath), 0700); err != nil {
			return output.Errorf(output.ExitInternal, "api_error", "cannot create parent directory: %s", err)
		}

		sizeBytes, err := validate.AtomicWriteFromReader(safePath, resp.Body, 0600)
		if err != nil {
			return output.Errorf(output.ExitInternal, "api_error", "cannot create file: %s", err)
		}

		runtime.Out(map[string]interface{}{
			"saved_path": safePath,
			"size_bytes": sizeBytes,
		}, nil)
		return nil
	},
}
