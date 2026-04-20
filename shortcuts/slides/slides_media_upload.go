// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// slidesMediaParentType is the only parent_type the slides backend accepts for
// media uploaded against an xml_presentation. Verified empirically:
// `slide_image` returns 1061001 unknown error, `slides_image` / `slides_file`
// return 1061002 params error, but `slide_file` returns a valid file_token
// that can be used as <img src="..."> in slide XML.
//
// NOTE: `slide_file` is only accepted by the single-part upload_all endpoint.
// The multipart upload_prepare endpoint rejects it (99992402 field validation
// failed), so slides image uploads are capped at 20 MB.
const slidesMediaParentType = "slide_file"

// SlidesMediaUpload uploads a local image to drive media against a slides
// presentation and returns the file_token. The token can be used as the value
// of <img src="..."> in slide XML.
//
// This is the atomic building block for getting a local image into a slides
// deck. Higher-level shortcuts (e.g. +create with @path placeholders) reuse
// the same upload helpers.
var SlidesMediaUpload = common.Shortcut{
	Service:     "slides",
	Command:     "+media-upload",
	Description: "Upload a local image to a slides presentation and return the file_token (use as <img src=...>)",
	Risk:        "write",
	// wiki:node:read is required by the wiki-URL resolution path. Declared
	// up-front (matching the convention used by other multi-API shortcuts) so
	// users without it get the standard auth login --scope hint at pre-flight.
	Scopes:    []string{"docs:document.media:upload", "wiki:node:read"},
	AuthTypes: []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file", Desc: "local image path (max 20 MB)", Required: true},
		{Name: "presentation", Desc: "xml_presentation_id, slides URL, or wiki URL that resolves to slides", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := parsePresentationRef(runtime.Str("presentation")); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		filePath := runtime.Str("file")
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}

		dry := common.NewDryRunAPI()
		parentNode := ref.Token
		stepBase := 1
		if ref.Kind == "wiki" {
			parentNode = "<resolved_slides_token>"
			stepBase = 2
			dry.Desc("2-step orchestration: resolve wiki → upload media").
				GET("/open-apis/wiki/v2/spaces/get_node").
				Desc("[1] Resolve wiki node to slides presentation").
				Params(map[string]interface{}{"token": ref.Token})
		} else {
			dry.Desc("Upload local file to slides presentation")
		}
		appendSlidesUploadDryRun(dry, filePath, parentNode, stepBase)
		return dry.Set("presentation_id", ref.Token)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		filePath := runtime.Str("file")
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return err
		}
		presentationID, err := resolvePresentationID(runtime, ref)
		if err != nil {
			return err
		}

		stat, err := runtime.FileIO().Stat(filePath)
		if err != nil {
			return common.WrapInputStatError(err, "file not found")
		}
		if !stat.Mode().IsRegular() {
			return output.ErrValidation("file must be a regular file: %s", filePath)
		}

		if stat.Size() > common.MaxDriveMediaUploadSinglePartSize {
			return output.ErrValidation("file %s is %s, exceeds 20 MB limit for slides image upload",
				filepath.Base(filePath), common.FormatSize(stat.Size()))
		}

		fileName := filepath.Base(filePath)
		fmt.Fprintf(runtime.IO().ErrOut, "Uploading: %s (%s) -> presentation %s\n",
			fileName, common.FormatSize(stat.Size()), common.MaskToken(presentationID))

		fileToken, err := uploadSlidesMedia(runtime, filePath, fileName, stat.Size(), presentationID)
		if err != nil {
			return err
		}

		runtime.Out(map[string]interface{}{
			"file_token":      fileToken,
			"file_name":       fileName,
			"size":            stat.Size(),
			"presentation_id": presentationID,
		}, nil)
		return nil
	},
}

// uploadSlidesMedia is the shared upload helper used by both +media-upload and
// the +create placeholder pipeline. Always uses parent_type=slide_file with the
// presentation_id as parent_node — verified to be the only working combo.
//
// Callers must ensure fileSize ≤ MaxDriveMediaUploadSinglePartSize (20 MB)
// because the multipart upload API does not accept parent_type=slide_file.
func uploadSlidesMedia(runtime *common.RuntimeContext, filePath, fileName string, fileSize int64, presentationID string) (string, error) {
	if fileSize > common.MaxDriveMediaUploadSinglePartSize {
		return "", output.ErrValidation("file %s is %s, exceeds 20 MB limit for slides image upload",
			fileName, common.FormatSize(fileSize))
	}
	parent := presentationID
	return common.UploadDriveMediaAll(runtime, common.DriveMediaUploadAllConfig{
		FilePath:   filePath,
		FileName:   fileName,
		FileSize:   fileSize,
		ParentType: slidesMediaParentType,
		ParentNode: &parent,
	})
}

// appendSlidesUploadDryRun renders the upload_all step for a single file.
func appendSlidesUploadDryRun(d *common.DryRunAPI, filePath, parentNode string, step int) {
	d.POST("/open-apis/drive/v1/medias/upload_all").
		Desc(fmt.Sprintf("[%d] Upload local file (max 20 MB)", step)).
		Body(map[string]interface{}{
			"file_name":   filepath.Base(filePath),
			"parent_type": slidesMediaParentType,
			"parent_node": parentNode,
			"size":        "<file_size>",
			"file":        "@" + filePath,
		})
}
