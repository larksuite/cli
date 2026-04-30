// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// allowedImageExts lists the file extensions supported by the OKR image upload API.
var allowedImageExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".bmp":  true,
}

// OKRUploadImage uploads an image for use in OKR progress rich text.
var OKRUploadImage = common.Shortcut{
	Service:     "okr",
	Command:     "+upload-image",
	Description: "Upload an image for use in OKR progress rich text",
	Risk:        "write",
	Scopes:      []string{"okr:okr.progress.file:upload"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file", Desc: "local image path (supports JPG, JPEG, PNG, GIF, BMP)", Required: true},
		{Name: "target-id", Desc: "target ID (objective or key result ID) for the progress", Required: true},
		{Name: "target-type", Desc: "target type: objective | key_result", Required: true, Enum: []string{"objective", "key_result"}},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		filePath := runtime.Str("file")
		if filePath == "" {
			return common.FlagErrorf("--file is required")
		}
		ext := strings.ToLower(filepath.Ext(filePath))
		if !allowedImageExts[ext] {
			return common.FlagErrorf("--file must be an image (supported: JPG, JPEG, PNG, GIF, BMP), got %q", ext)
		}

		targetID := runtime.Str("target-id")
		if targetID == "" {
			return common.FlagErrorf("--target-id is required")
		}
		if id, err := strconv.ParseInt(targetID, 10, 64); err != nil || id <= 0 {
			return common.FlagErrorf("--target-id must be a positive int64")
		}

		targetType := runtime.Str("target-type")
		if _, ok := targetTypeAllowed[targetType]; !ok {
			return common.FlagErrorf("--target-type must be one of: objective | key_result")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		filePath := runtime.Str("file")
		targetID := runtime.Str("target-id")
		targetType := runtime.Str("target-type")
		targetTypeVal := targetTypeAllowed[targetType]

		return common.NewDryRunAPI().
			POST("/open-apis/okr/v1/images/upload").
			Body(map[string]interface{}{
				"file":        "@" + filePath,
				"target_id":   targetID,
				"target_type": targetTypeVal,
			}).
			Desc(fmt.Sprintf("Upload image for OKR %s %s", targetType, targetID))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		filePath := runtime.Str("file")
		targetID := runtime.Str("target-id")
		targetType := runtime.Str("target-type")
		targetTypeVal := targetTypeAllowed[targetType]

		info, err := runtime.FileIO().Stat(filePath)
		if err != nil {
			return common.WrapInputStatError(err)
		}

		f, err := runtime.FileIO().Open(filePath)
		if err != nil {
			return common.WrapInputStatError(err)
		}
		defer f.Close()

		fileName := filepath.Base(filePath)
		fmt.Fprintf(runtime.IO().ErrOut, "Uploading: %s (%s)\n", fileName, common.FormatSize(info.Size()))

		fd := larkcore.NewFormdata()
		fd.AddField("target_id", targetID)
		fd.AddField("target_type", fmt.Sprintf("%d", targetTypeVal))
		fd.AddFile("data", f)

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod: "POST",
			ApiPath:    "/open-apis/okr/v1/images/upload",
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

		if larkCode := int(common.GetFloat(result, "code")); larkCode != 0 {
			msg, _ := result["msg"].(string)
			return output.ErrAPI(larkCode, fmt.Sprintf("upload failed: [%d] %s", larkCode, msg), result["error"])
		}

		data, _ := result["data"].(map[string]interface{})
		fileToken, _ := data["file_token"].(string)
		url, _ := data["url"].(string)

		if fileToken == "" {
			return output.Errorf(output.ExitAPI, "api_error", "upload failed: no file_token returned")
		}

		runtime.Out(map[string]interface{}{
			"file_token": fileToken,
			"url":        url,
			"file_name":  fileName,
			"size":       info.Size(),
		}, nil)
		return nil
	},
}
