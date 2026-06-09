// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
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
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--file is required").WithParam("--file")
		}
		ext := strings.ToLower(filepath.Ext(filePath))
		if !allowedImageExts[ext] {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--file must be an image (supported: JPG, JPEG, PNG, GIF, BMP), got %q", ext).WithParam("--file")
		}

		targetID := runtime.Str("target-id")
		if targetID == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--target-id is required").WithParam("--target-id")
		}
		if id, err := strconv.ParseInt(targetID, 10, 64); err != nil || id <= 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--target-id must be a positive int64").WithParam("--target-id")
		}

		targetType := runtime.Str("target-type")
		if _, ok := targetTypeAllowed[targetType]; !ok {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--target-type must be one of: objective | key_result").WithParam("--target-type")
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
			return okrInputStatError(err)
		}

		f, err := runtime.FileIO().Open(filePath)
		if err != nil {
			return okrInputStatError(err)
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
			// The DoAPI boundary already returns typed errs.* (auth →
			// AuthenticationError, transport → NetworkError, etc.); wrapOkrNetworkErr
			// passes those through via ProblemOf and only wraps a still-untyped error.
			return wrapOkrNetworkErr(err, "upload failed: %v", err)
		}

		data, err := runtime.ClassifyAPIResponse(apiResp)
		if err != nil {
			return err
		}

		fileToken := common.GetString(data, "file_token")
		if fileToken == "" {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "upload failed: no file_token returned")
		}
		url := common.GetString(data, "url")

		runtime.Out(map[string]interface{}{
			"file_token": fileToken,
			"url":        url,
			"file_name":  fileName,
			"size":       info.Size(),
		}, nil)
		return nil
	},
}
