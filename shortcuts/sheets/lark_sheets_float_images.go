// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const sheetImageParentType = "sheet_image"

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
		_, _, err := validateSheetMediaUploadFile(runtime, runtime.Str("file"))
		return err
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

		safePath, stat, err := validateSheetMediaUploadFile(runtime, filePath)
		if err != nil {
			return err
		}

		fileName := filepath.Base(safePath)
		fmt.Fprintf(runtime.IO().ErrOut, "Uploading: %s (%s) -> spreadsheet %s\n",
			fileName, common.FormatSize(stat.Size()), common.MaskToken(parentNode))
		if stat.Size() > common.MaxDriveMediaUploadSinglePartSize {
			fmt.Fprintf(runtime.IO().ErrOut, "File exceeds 20MB, using multipart upload\n")
		}

		fileToken, err := uploadSheetMediaFile(runtime, safePath, fileName, stat.Size(), parentNode)
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

func validateSheetMediaUploadFile(runtime *common.RuntimeContext, filePath string) (string, fileio.FileInfo, error) {
	stat, err := runtime.FileIO().Stat(filePath)
	if err != nil {
		return "", nil, common.WrapInputStatError(err, "file not found")
	}
	if !stat.Mode().IsRegular() {
		return "", nil, output.ErrValidation("file must be a regular file: %s", filePath)
	}
	return filePath, stat, nil
}

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

func sheetMediaShouldUseMultipart(fio fileio.FileIO, filePath string) bool {
	info, err := fio.Stat(filePath)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular() && info.Size() > common.MaxDriveMediaUploadSinglePartSize
}

func floatImageBasePath(token, sheetID string) string {
	return fmt.Sprintf("/open-apis/sheets/v3/spreadsheets/%s/sheets/%s/float_images",
		validate.EncodePathSegment(token), validate.EncodePathSegment(sheetID))
}

func floatImageItemPath(token, sheetID, floatImageID string) string {
	return fmt.Sprintf("%s/%s", floatImageBasePath(token, sheetID), validate.EncodePathSegment(floatImageID))
}

func validateFloatImageToken(runtime *common.RuntimeContext) (string, error) {
	token := runtime.Str("spreadsheet-token")
	if u := runtime.Str("url"); u != "" {
		if parsed := extractSpreadsheetToken(u); parsed != u {
			token = parsed
		}
	}
	if token == "" {
		return "", common.FlagErrorf("specify --url or --spreadsheet-token")
	}
	return token, nil
}

func validateFloatImageRange(sheetID, rangeVal string) error {
	if rangeVal == "" {
		return nil
	}
	if err := validateSingleCellRange(rangeVal); err != nil {
		return err
	}
	if prefix, _, ok := splitSheetRange(rangeVal); ok && sheetID != "" && prefix != sheetID {
		return common.FlagErrorf("--range prefix %q does not match --sheet-id %q", prefix, sheetID)
	}
	return nil
}

func validateFloatImageUpdatePayload(runtime *common.RuntimeContext) error {
	hasField := runtime.Str("range") != "" ||
		runtime.Cmd.Flags().Changed("width") ||
		runtime.Cmd.Flags().Changed("height") ||
		runtime.Cmd.Flags().Changed("offset-x") ||
		runtime.Cmd.Flags().Changed("offset-y")
	if !hasField {
		return common.FlagErrorf("specify at least one of --range, --width, --height, --offset-x, --offset-y to update")
	}
	return nil
}

func validateFloatImageDims(runtime *common.RuntimeContext) error {
	if runtime.Cmd.Flags().Changed("width") {
		if v := runtime.Int("width"); v < 20 {
			return common.FlagErrorf("--width must be >= 20 pixels, got %d", v)
		}
	}
	if runtime.Cmd.Flags().Changed("height") {
		if v := runtime.Int("height"); v < 20 {
			return common.FlagErrorf("--height must be >= 20 pixels, got %d", v)
		}
	}
	if runtime.Cmd.Flags().Changed("offset-x") {
		if v := runtime.Int("offset-x"); v < 0 {
			return common.FlagErrorf("--offset-x must be >= 0, got %d", v)
		}
	}
	if runtime.Cmd.Flags().Changed("offset-y") {
		if v := runtime.Int("offset-y"); v < 0 {
			return common.FlagErrorf("--offset-y must be >= 0, got %d", v)
		}
	}
	return nil
}

func buildFloatImageBody(runtime *common.RuntimeContext, includeToken bool) map[string]interface{} {
	body := map[string]interface{}{}
	if includeToken {
		if s := runtime.Str("float-image-token"); s != "" {
			body["float_image_token"] = s
		}
	}
	if s := runtime.Str("range"); s != "" {
		body["range"] = s
	}
	if runtime.Cmd.Flags().Changed("width") {
		body["width"] = runtime.Int("width")
	}
	if runtime.Cmd.Flags().Changed("height") {
		body["height"] = runtime.Int("height")
	}
	if runtime.Cmd.Flags().Changed("offset-x") {
		body["offset_x"] = runtime.Int("offset-x")
	}
	if runtime.Cmd.Flags().Changed("offset-y") {
		body["offset_y"] = runtime.Int("offset-y")
	}
	return body
}

var SheetCreateFloatImage = common.Shortcut{
	Service:     "sheets",
	Command:     "+create-float-image",
	Description: "Create a floating image on a sheet",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "sheet-id", Desc: "sheet ID", Required: true},
		{Name: "float-image-token", Desc: "image file token (from upload API)", Required: true},
		{Name: "range", Desc: "anchor cell, must be a single cell (e.g. sheetId!A1:A1)", Required: true},
		{Name: "width", Type: "int", Desc: "width in pixels (>=20)"},
		{Name: "height", Type: "int", Desc: "height in pixels (>=20)"},
		{Name: "offset-x", Type: "int", Desc: "horizontal offset from anchor cell's top-left (pixels, >=0)"},
		{Name: "offset-y", Type: "int", Desc: "vertical offset from anchor cell's top-left (pixels, >=0)"},
		{Name: "float-image-id", Desc: "custom 10-char alphanumeric ID (auto-generated if omitted)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateFloatImageToken(runtime); err != nil {
			return err
		}
		if err := validateFloatImageRange(runtime.Str("sheet-id"), runtime.Str("range")); err != nil {
			return err
		}
		return validateFloatImageDims(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateFloatImageToken(runtime)
		body := buildFloatImageBody(runtime, true)
		if s := runtime.Str("float-image-id"); s != "" {
			body["float_image_id"] = s
		}
		return common.NewDryRunAPI().
			POST("/open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/float_images").
			Body(body).Set("token", token).Set("sheet_id", runtime.Str("sheet-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateFloatImageToken(runtime)
		body := buildFloatImageBody(runtime, true)
		if s := runtime.Str("float-image-id"); s != "" {
			body["float_image_id"] = s
		}
		data, err := runtime.CallAPI("POST", floatImageBasePath(token, runtime.Str("sheet-id")), nil, body)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var SheetUpdateFloatImage = common.Shortcut{
	Service:     "sheets",
	Command:     "+update-float-image",
	Description: "Update a floating image",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "sheet-id", Desc: "sheet ID", Required: true},
		{Name: "float-image-id", Desc: "float image ID", Required: true},
		{Name: "range", Desc: "new anchor cell, must be a single cell (e.g. sheetId!B2:B2)"},
		{Name: "width", Type: "int", Desc: "width in pixels (>=20)"},
		{Name: "height", Type: "int", Desc: "height in pixels (>=20)"},
		{Name: "offset-x", Type: "int", Desc: "horizontal offset from anchor cell's top-left (pixels, >=0)"},
		{Name: "offset-y", Type: "int", Desc: "vertical offset from anchor cell's top-left (pixels, >=0)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateFloatImageToken(runtime); err != nil {
			return err
		}
		if err := validateFloatImageUpdatePayload(runtime); err != nil {
			return err
		}
		if err := validateFloatImageRange(runtime.Str("sheet-id"), runtime.Str("range")); err != nil {
			return err
		}
		return validateFloatImageDims(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateFloatImageToken(runtime)
		body := buildFloatImageBody(runtime, false)
		return common.NewDryRunAPI().
			PATCH("/open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/float_images/:float_image_id").
			Body(body).Set("token", token).Set("sheet_id", runtime.Str("sheet-id")).Set("float_image_id", runtime.Str("float-image-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateFloatImageToken(runtime)
		body := buildFloatImageBody(runtime, false)
		data, err := runtime.CallAPI("PATCH", floatImageItemPath(token, runtime.Str("sheet-id"), runtime.Str("float-image-id")), nil, body)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var SheetGetFloatImage = common.Shortcut{
	Service:     "sheets",
	Command:     "+get-float-image",
	Description: "Get a floating image by ID",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "sheet-id", Desc: "sheet ID", Required: true},
		{Name: "float-image-id", Desc: "float image ID", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := validateFloatImageToken(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateFloatImageToken(runtime)
		return common.NewDryRunAPI().
			GET("/open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/float_images/:float_image_id").
			Set("token", token).Set("sheet_id", runtime.Str("sheet-id")).Set("float_image_id", runtime.Str("float-image-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateFloatImageToken(runtime)
		data, err := runtime.CallAPI("GET", floatImageItemPath(token, runtime.Str("sheet-id"), runtime.Str("float-image-id")), nil, nil)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var SheetListFloatImages = common.Shortcut{
	Service:     "sheets",
	Command:     "+list-float-images",
	Description: "List all floating images in a sheet",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "sheet-id", Desc: "sheet ID", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := validateFloatImageToken(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateFloatImageToken(runtime)
		return common.NewDryRunAPI().
			GET("/open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/float_images/query").
			Set("token", token).Set("sheet_id", runtime.Str("sheet-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateFloatImageToken(runtime)
		data, err := runtime.CallAPI("GET", floatImageBasePath(token, runtime.Str("sheet-id"))+"/query", nil, nil)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var SheetDeleteFloatImage = common.Shortcut{
	Service:     "sheets",
	Command:     "+delete-float-image",
	Description: "Delete a floating image",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "sheet-id", Desc: "sheet ID", Required: true},
		{Name: "float-image-id", Desc: "float image ID", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := validateFloatImageToken(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateFloatImageToken(runtime)
		return common.NewDryRunAPI().
			DELETE("/open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/float_images/:float_image_id").
			Set("token", token).Set("sheet_id", runtime.Str("sheet-id")).Set("float_image_id", runtime.Str("float-image-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateFloatImageToken(runtime)
		data, err := runtime.CallAPI("DELETE", floatImageItemPath(token, runtime.Str("sheet-id"), runtime.Str("float-image-id")), nil, nil)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}
