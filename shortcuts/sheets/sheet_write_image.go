// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/shortcuts/common"
)

var SheetWriteImage = common.Shortcut{
	Service:     "sheets",
	Command:     "+write-image",
	Description: "Write an image into a spreadsheet cell",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "sheet-id", Desc: "sheet ID"},
		{Name: "range", Desc: "target cell (e.g. A1 or <sheetId>!A1). Start and end cell must be the same", Required: true},
		{Name: "image", Desc: "local image file path (supported formats: PNG, JPEG, JPG, GIF, BMP, JFIF, EXIF, TIFF, BPG, HEIC)", Required: true},
		{Name: "name", Desc: "image file name with extension (defaults to the basename of --image)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}
		if token == "" {
			return common.FlagErrorf("specify --url or --spreadsheet-token")
		}
		if err := validateSheetRangeInput(runtime.Str("sheet-id"), runtime.Str("range")); err != nil {
			return err
		}
		if err := validateSingleCellRange(runtime.Str("range")); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}
		pointRange := normalizePointRange(runtime.Str("sheet-id"), runtime.Str("range"))
		imageName := runtime.Str("name")
		if imageName == "" {
			imageName = filepath.Base(runtime.Str("image"))
		}
		return common.NewDryRunAPI().
			Desc("JSON upload with inline image bytes").
			POST("/open-apis/sheets/v2/spreadsheets/:token/values_image").
			Body(map[string]interface{}{
				"range": pointRange,
				"image": fmt.Sprintf("<binary: %s>", runtime.Str("image")),
				"name":  imageName,
			}).
			Set("token", token)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("spreadsheet-token")
		if runtime.Str("url") != "" {
			token = extractSpreadsheetToken(runtime.Str("url"))
		}

		// Resolve the target cell range (--range is required).
		pointRange := normalizePointRange(runtime.Str("sheet-id"), runtime.Str("range"))

		// Resolve image file.
		imagePath := runtime.Str("image")
		safePath, err := validate.SafeInputPath(imagePath)
		if err != nil {
			return output.ErrValidation("unsafe image path: %s", err)
		}
		stat, err := vfs.Stat(safePath)
		if err != nil {
			return output.ErrValidation("image file not found: %s", imagePath)
		}
		if !stat.Mode().IsRegular() {
			return output.ErrValidation("image must be a regular file: %s", imagePath)
		}
		const maxImageSize int64 = 20 * 1024 * 1024 // 20 MB
		if stat.Size() > maxImageSize {
			return output.ErrValidation("image %.1fMB exceeds 20MB limit", float64(stat.Size())/1024/1024)
		}

		imageBytes, err := vfs.ReadFile(safePath)
		if err != nil {
			return output.ErrValidation("cannot read image file: %s", err)
		}

		imageName := runtime.Str("name")
		if imageName == "" {
			imageName = filepath.Base(imagePath)
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Writing image: %s (%d bytes) → %s\n", imageName, stat.Size(), pointRange)

		// The sheets v2 values_image API expects a JSON body with the image
		// as an inline byte array, not multipart/form-data.
		data, err := runtime.CallAPI("POST", fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/values_image", validate.EncodePathSegment(token)), nil, map[string]interface{}{
			"range": pointRange,
			"image": imageBytes,
			"name":  imageName,
		})
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}
