// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// sheetImageParentType is the parent_type accepted by the drive media upload
// endpoint for media that will be anchored via +create-float-image.
const sheetImageParentType = "sheet_image"

// SheetMediaUpload uploads a local image to the drive media endpoint against
// a spreadsheet and returns the file_token. The token is usable as the
// --float-image-token argument to +create-float-image.
//
// Files up to 20 MB go through /drive/v1/medias/upload_all; larger files are
// streamed via upload_prepare / upload_part / upload_finish. This matches the
// pattern used by docs +media-upload and drive +import.
var SheetMediaUpload = common.Shortcut{
	Service:     "sheets",
	Command:     "+media-upload",
	Description: "Upload a local image for use as a floating image and return the file_token",
	Risk:        "write",
	Scopes:      []string{"docs:document.media:upload"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "file", Desc: "local image path (files > 20MB use multipart upload automatically)", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSheetMediaUploadParent(runtime); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		parentNode, err := resolveSheetMediaUploadParent(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		filePath := runtime.Str("file")
		fileName := filepath.Base(filePath)

		dry := common.NewDryRunAPI()
		if sheetMediaShouldUseMultipart(runtime.FileIO(), filePath) {
			dry.Desc("chunked media upload (files > 20MB)").
				POST("/open-apis/drive/v1/medias/upload_prepare").
				Body(map[string]interface{}{
					"file_name":   fileName,
					"parent_type": sheetImageParentType,
					"parent_node": parentNode,
					"size":        "<file_size>",
				}).
				POST("/open-apis/drive/v1/medias/upload_part").
				Body(map[string]interface{}{
					"upload_id": "<upload_id>",
					"seq":       "<chunk_index>",
					"size":      "<chunk_size>",
					"file":      "<chunk_binary>",
				}).
				POST("/open-apis/drive/v1/medias/upload_finish").
				Body(map[string]interface{}{
					"upload_id": "<upload_id>",
					"block_num": "<block_num>",
				})
			return dry.Set("spreadsheet_token", parentNode)
		}
		return dry.Desc("multipart/form-data upload").
			POST("/open-apis/drive/v1/medias/upload_all").
			Body(map[string]interface{}{
				"file_name":   fileName,
				"parent_type": sheetImageParentType,
				"parent_node": parentNode,
				"size":        "<file_size>",
				"file":        "@" + filePath,
			}).
			Set("spreadsheet_token", parentNode)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		parentNode, err := resolveSheetMediaUploadParent(runtime)
		if err != nil {
			return err
		}
		filePath := runtime.Str("file")

		stat, err := runtime.FileIO().Stat(filePath)
		if err != nil {
			return common.WrapInputStatError(err, "file not found")
		}
		if !stat.Mode().IsRegular() {
			return output.ErrValidation("file must be a regular file: %s", filePath)
		}

		fileName := filepath.Base(filePath)
		fmt.Fprintf(runtime.IO().ErrOut, "Uploading: %s (%s) -> spreadsheet %s\n",
			fileName, common.FormatSize(stat.Size()), common.MaskToken(parentNode))
		if stat.Size() > common.MaxDriveMediaUploadSinglePartSize {
			fmt.Fprintf(runtime.IO().ErrOut, "File exceeds 20MB, using multipart upload\n")
		}

		fileToken, err := uploadSheetMediaFile(runtime, filePath, fileName, stat.Size(), parentNode)
		if err != nil {
			return err
		}

		runtime.Out(map[string]interface{}{
			"file_token":        fileToken,
			"file_name":         fileName,
			"size":              stat.Size(),
			"spreadsheet_token": parentNode,
		}, nil)
		return nil
	},
}

// resolveSheetMediaUploadParent returns the spreadsheet token to use as parent_node,
// accepting either --url or --spreadsheet-token.
func resolveSheetMediaUploadParent(runtime *common.RuntimeContext) (string, error) {
	token := runtime.Str("spreadsheet-token")
	if u := runtime.Str("url"); u != "" {
		if parsed := extractSpreadsheetToken(u); parsed != "" {
			token = parsed
		}
	}
	if token == "" {
		return "", common.FlagErrorf("specify --url or --spreadsheet-token")
	}
	return token, nil
}

// uploadSheetMediaFile routes to the single-part or multipart upload path based
// on file size. Always uses parent_type=sheet_image so the returned token can
// be consumed by +create-float-image.
func uploadSheetMediaFile(runtime *common.RuntimeContext, filePath, fileName string, fileSize int64, parentNode string) (string, error) {
	if fileSize <= common.MaxDriveMediaUploadSinglePartSize {
		pn := parentNode
		return common.UploadDriveMediaAll(runtime, common.DriveMediaUploadAllConfig{
			FilePath:   filePath,
			FileName:   fileName,
			FileSize:   fileSize,
			ParentType: sheetImageParentType,
			ParentNode: &pn,
		})
	}
	return common.UploadDriveMediaMultipart(runtime, common.DriveMediaMultipartUploadConfig{
		FilePath:   filePath,
		FileName:   fileName,
		FileSize:   fileSize,
		ParentType: sheetImageParentType,
		ParentNode: parentNode,
	})
}

// sheetMediaShouldUseMultipart mirrors docMediaShouldUseMultipart: dry-run uses
// local stat as a best-effort planning hint. Execute re-validates before
// choosing the actual upload path.
func sheetMediaShouldUseMultipart(fio fileio.FileIO, filePath string) bool {
	info, err := fio.Stat(filePath)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular() && info.Size() > common.MaxDriveMediaUploadSinglePartSize
}
