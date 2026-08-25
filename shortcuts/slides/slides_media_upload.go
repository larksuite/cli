// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// Drive media parent_type values for uploading an image into a presentation.
//
// Native (API-created) presentations use "slide_file", verified empirically:
// `slide_image` returns 1061001 unknown error, `slides_image` / `slides_file`
// return 1061002 params error, but `slide_file` returns a valid file_token
// that can be used as <img src="..."> in slide XML.
//
// Imported "office" presentations carry either a legacy synthetic-token prefix
// or a 28-character token whose interleaved product/region marker is "OFL0X",
// and the drive backend requires "office_slide_file" for those — the
// presentation counterpart of the office_sheet_file rule the sheets domain
// already applies (shortcuts/sheets/helpers.go). The token shapes are the same
// there: an imported office file is an imported office file whether it backs a
// spreadsheet or a deck.
//
// NOTE: neither value is accepted by the multipart upload_prepare endpoint
// (99992402 field validation failed), so slides image uploads stay capped at
// 20 MB regardless of which one applies.
const (
	slideFileParentType       = "slide_file"
	officeSlideFileParentType = "office_slide_file"
	fakeOfficePrefix          = "fake_office_"
	localOfficePrefix         = "local_office_"
)

// officePrefixes are the legacy synthetic token prefixes an imported "office"
// presentation may carry.
var officePrefixes = []string{fakeOfficePrefix, localOfficePrefix}

func isOfficePresentation(presentationToken string) bool {
	for _, prefix := range officePrefixes {
		if strings.HasPrefix(presentationToken, prefix) {
			return true
		}
	}
	if len(presentationToken) != 28 {
		return false
	}
	// The five-character marker occupies positions 5, 10, 15, 20, and 25
	// (1-based) in the interleaved token.
	marker := []byte{
		presentationToken[4],
		presentationToken[9],
		presentationToken[14],
		presentationToken[19],
		presentationToken[24],
	}
	return string(marker) == "OFL0X"
}

// slidesMediaParentType returns the drive media parent_type to use when
// uploading an image whose parent_node is presentationToken. It is the single
// place that maps a presentation token to its parent_type so every image-upload
// entry point (+media-upload, and the @path placeholder pipeline behind
// +create / +add-slide / +update-slide) and its dry-run preview stay
// consistent.
func slidesMediaParentType(presentationToken string) string {
	if isOfficePresentation(presentationToken) {
		return officeSlideFileParentType
	}
	return slideFileParentType
}

// unresolvedSlidesTokenPlaceholder is what a dry-run shows in place of a
// presentation token it cannot know: the caller passed a wiki reference, and
// resolving it needs the get_node call a preview must not make.
const unresolvedSlidesTokenPlaceholder = "<resolved_slides_token>"

// slidesDryRunParentType returns the parent_type a dry-run should preview for
// ref, without resolving anything.
//
// It exists so the placeholder token never reaches slidesMediaParentType. Doing
// that happens to yield the right answer — a placeholder matches no office token
// shape, so it falls through to slideFileParentType — but by accident rather
// than on purpose, which makes the preview hostage to the placeholder's spelling
// and to every future rule added to isOfficePresentation.
//
// A wiki ref is native by construction, not by default: resolvePresentationID
// rejects any wiki node whose obj_type is not "slides" (helpers.go), and an
// imported office deck sits in drive as a "file" node, so it never survives that
// gate to reach an upload. That is why this can assert slideFileParentType for a
// token it has not seen.
func slidesDryRunParentType(ref presentationRef) string {
	if ref.Kind == "wiki" {
		return slideFileParentType
	}
	return slidesMediaParentType(ref.Token)
}

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
		requiredPresentationRefFlag(),
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
		uploadNode := ref.Token
		stepBase := 1
		if ref.Kind == "wiki" {
			uploadNode = unresolvedSlidesTokenPlaceholder
			stepBase = 2
			dry.Desc("2-step orchestration: resolve wiki → upload media").
				GET("/open-apis/wiki/v2/spaces/get_node").
				Desc("[1] Resolve wiki node to slides presentation").
				Params(map[string]interface{}{"token": ref.Token})
		} else {
			dry.Desc("Upload local file to slides presentation")
		}
		appendSlidesUploadDryRun(dry, filePath, uploadNode, slidesDryRunParentType(ref), stepBase)
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
			return slidesInputStatError(err, "--file", filePath)
		}
		if !stat.Mode().IsRegular() {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "file must be a regular file: %s", filePath).WithParam("--file")
		}

		if stat.Size() > common.MaxDriveMediaUploadSinglePartSize {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "file %s is %s, exceeds 20 MB limit for slides image upload",
				filepath.Base(filePath), common.FormatSize(stat.Size())).WithParam("--file")
		}

		fileName := filepath.Base(filePath)

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
// the +create placeholder pipeline. The presentation_id is the parent_node and
// picks the parent_type via slidesMediaParentType, so an imported office deck
// gets office_slide_file rather than the native slide_file.
//
// Callers must ensure fileSize ≤ MaxDriveMediaUploadSinglePartSize (20 MB)
// because the multipart upload API accepts neither parent_type.
func uploadSlidesMedia(runtime *common.RuntimeContext, filePath, fileName string, fileSize int64, presentationID string) (string, error) {
	if fileSize > common.MaxDriveMediaUploadSinglePartSize {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "file %s is %s, exceeds 20 MB limit for slides image upload",
			fileName, common.FormatSize(fileSize))
	}
	parent := presentationID
	return common.UploadDriveMediaAllTyped(runtime, common.DriveMediaUploadAllConfig{
		FilePath:   filePath,
		FileName:   fileName,
		FileSize:   fileSize,
		ParentType: slidesMediaParentType(presentationID),
		ParentNode: &parent,
	})
}

// appendSlidesUploadDryRun renders the upload_all step for a single file. It is
// pure rendering: parentType is passed in rather than derived from parentNode,
// because parentNode may be a placeholder and a placeholder cannot be classified.
func appendSlidesUploadDryRun(d *common.DryRunAPI, filePath, parentNode, parentType string, step int) {
	d.POST("/open-apis/drive/v1/medias/upload_all").
		Desc(fmt.Sprintf("[%d] Upload local file (max 20 MB)", step)).
		Body(map[string]interface{}{
			"file_name":   filepath.Base(filePath),
			"parent_type": parentType,
			"parent_node": parentNode,
			"size":        "<file_size>",
			"file":        "@" + filePath,
		})
}
