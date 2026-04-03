// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/shortcuts/common"
)

const maxDriveUploadFileSize = 20 * 1024 * 1024 // 20MB

var DriveUpload = common.Shortcut{
	Service:     "drive",
	Command:     "+upload",
	Description: "Upload a local file to Drive",
	Risk:        "write",
	Scopes:      []string{"drive:file:upload"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file", Desc: "local file path (max 20MB)", Required: true},
		{Name: "folder-token", Desc: "target folder token (default: root)"},
		{Name: "name", Desc: "uploaded file name (default: local file name)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		filePath := runtime.Str("file")
		folderToken := runtime.Str("folder-token")
		name := runtime.Str("name")
		fileName := name
		if fileName == "" {
			fileName = filepath.Base(filePath)
		}
		return common.NewDryRunAPI().
			Desc("multipart/form-data upload").
			POST("/open-apis/drive/v1/files/upload_all").
			Body(map[string]interface{}{
				"file_name":   fileName,
				"parent_type": "explorer",
				"parent_node": folderToken,
				"file":        "@" + filePath,
			})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		filePath := runtime.Str("file")
		folderToken := runtime.Str("folder-token")
		name := runtime.Str("name")

		safeFilePath, err := validate.SafeInputPath(filePath)
		if err != nil {
			return output.ErrValidation("unsafe file path: %s", err)
		}
		filePath = safeFilePath

		fileName := name
		if fileName == "" {
			fileName = filepath.Base(filePath)
		}

		info, err := vfs.Stat(filePath)
		if err != nil {
			return output.ErrValidation("cannot read file: %s", err)
		}
		fileSize := info.Size()
		if fileSize > maxDriveUploadFileSize {
			return output.ErrValidation("file %.1fMB exceeds 20MB limit", float64(fileSize)/1024/1024)
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Uploading: %s (%s)\n", fileName, common.FormatSize(fileSize))

		// Use SDK multipart upload
		fileToken, err := uploadFileToDrive(ctx, runtime, filePath, fileName, folderToken, fileSize)
		if err != nil {
			return err
		}

		runtime.Out(map[string]interface{}{
			"file_token": fileToken,
			"file_name":  fileName,
			"size":       fileSize,
		}, nil)
		return nil
	},
}

func uploadFileToDrive(ctx context.Context, runtime *common.RuntimeContext, filePath, fileName, folderToken string, fileSize int64) (string, error) {
	f, err := vfs.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Build SDK Formdata
	fd := larkcore.NewFormdata()
	fd.AddField("file_name", fileName)
	fd.AddField("parent_type", "explorer")
	fd.AddField("parent_node", folderToken)
	fd.AddField("size", fmt.Sprintf("%d", fileSize))
	fd.AddFile("file", f)

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    "/open-apis/drive/v1/files/upload_all",
		Body:       fd,
	}, larkcore.WithFileUpload())
	if err != nil {
		var exitErr *output.ExitError
		if errors.As(err, &exitErr) {
			return "", err
		}
		return "", output.ErrNetwork("upload failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
		return "", output.Errorf(output.ExitAPI, "api_error", "upload failed: invalid response JSON: %v", err)
	}

	if larkCode := int(common.GetFloat(result, "code")); larkCode != 0 {
		msg, _ := result["msg"].(string)
		return "", output.ErrAPI(larkCode, fmt.Sprintf("upload failed: [%d] %s", larkCode, msg), result["error"])
	}

	data, _ := result["data"].(map[string]interface{})
	fileToken, _ := data["file_token"].(string)
	if fileToken == "" {
		return "", output.Errorf(output.ExitAPI, "api_error", "upload failed: no file_token returned")
	}
	return fileToken, nil
}
