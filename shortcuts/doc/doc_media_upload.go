// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/util"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/shortcuts/common"
)

var MediaUpload = common.Shortcut{
	Service:     "docs",
	Command:     "+media-upload",
	Description: "Upload media file (image/attachment) to a document block",
	Risk:        "write",
	Scopes:      []string{"docs:document.media:upload"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file", Desc: "local file path (max 20MB)", Required: true},
		{Name: "parent-type", Desc: "parent type: docx_image | docx_file", Required: true},
		{Name: "parent-node", Desc: "parent node ID (block_id)", Required: true},
		{Name: "doc-id", Desc: "document ID (for drive_route_token)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		filePath := runtime.Str("file")
		parentType := runtime.Str("parent-type")
		parentNode := runtime.Str("parent-node")
		docId := runtime.Str("doc-id")
		body := map[string]interface{}{
			"file_name":   filepath.Base(filePath),
			"parent_type": parentType,
			"parent_node": parentNode,
			"file":        "@" + filePath,
		}
		if docId != "" {
			body["extra"] = fmt.Sprintf(`{"drive_route_token":"%s"}`, docId)
		}
		return common.NewDryRunAPI().
			Desc("multipart/form-data upload").
			POST("/open-apis/drive/v1/medias/upload_all").
			Body(body)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		filePath := runtime.Str("file")
		parentType := runtime.Str("parent-type")
		parentNode := runtime.Str("parent-node")
		docId := runtime.Str("doc-id")

		safeFilePath, pathErr := validate.SafeInputPath(filePath)
		if pathErr != nil {
			return output.ErrValidation("unsafe file path: %s", pathErr)
		}
		filePath = safeFilePath

		// Validate file
		stat, err := vfs.Stat(filePath)
		if err != nil {
			return output.ErrValidation("file not found: %s", filePath)
		}
		if stat.Size() > maxFileSize {
			return output.ErrValidation("file %.1fMB exceeds 20MB limit", float64(stat.Size())/1024/1024)
		}

		fileName := filepath.Base(filePath)
		fmt.Fprintf(runtime.IO().ErrOut, "Uploading: %s (%d bytes)\n", fileName, stat.Size())

		f, err := vfs.Open(filePath)
		if err != nil {
			return output.ErrValidation("cannot open file: %v", err)
		}
		defer f.Close()

		// Build SDK Formdata
		fd := larkcore.NewFormdata()
		fd.AddField("file_name", fileName)
		fd.AddField("parent_type", parentType)
		fd.AddField("parent_node", parentNode)
		fd.AddField("size", fmt.Sprintf("%d", stat.Size()))
		if docId != "" {
			extra, err := buildDriveRouteExtra(docId)
			if err != nil {
				return err
			}
			fd.AddField("extra", extra)
		}
		fd.AddFile("file", f)

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod: http.MethodPost,
			ApiPath:    "/open-apis/drive/v1/medias/upload_all",
			Body:       fd,
		}, larkcore.WithFileUpload())
		if err != nil {
			var exitErr *output.ExitError
			if errors.As(err, &exitErr) {
				return err
			}
			return output.ErrNetwork("upload failed: %v", err)
		}

		var result map[string]interface{}
		if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
			return output.Errorf(output.ExitAPI, "api_error", "upload failed: invalid response JSON: %v", err)
		}

		code, _ := util.ToFloat64(result["code"])
		if code != 0 {
			msg, _ := result["msg"].(string)
			return output.ErrAPI(int(code), fmt.Sprintf("upload failed: [%d] %s", int(code), msg), result["error"])
		}

		data, _ := result["data"].(map[string]interface{})
		fileToken, _ := data["file_token"].(string)
		if fileToken == "" {
			return output.Errorf(output.ExitAPI, "api_error", "upload failed: no file_token returned")
		}

		runtime.Out(map[string]interface{}{
			"file_token": fileToken,
			"file_name":  fileName,
			"size":       stat.Size(),
		}, nil)
		return nil
	},
}
