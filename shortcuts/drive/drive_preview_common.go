// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	drivePreviewIfExistsError     = "error"
	drivePreviewIfExistsOverwrite = "overwrite"
	drivePreviewIfExistsRename    = "rename"
)

type drivePreviewCandidate struct {
	Type         string
	TypeCode     string
	TypeName     string
	Label        string
	Status       string
	StatusCode   string
	Downloadable bool
	Reason       string
}

type driveCoverSpec struct {
	Name        string
	Label       string
	Description string
	PreviewType string
	BusType     string
	Platform    string
	Width       int
	Height      int
	Policy      string
	FallbackExt string
}

type driveExtensionResolution struct {
	Ext    string
	Source string
	Detail string
}

type drivePreviewTypeMeta struct {
	Code    string
	Name    string
	Type    string
	Label   string
	Aliases []string
}

type drivePreviewStatusMeta struct {
	Code         string
	Name         string
	Reason       string
	Downloadable bool
}

var drivePreviewMimeToExt = map[string]string{
	"application/json":         ".json",
	"application/msword":       ".doc",
	"application/pdf":          ".pdf",
	"application/xml":          ".xml",
	"application/zip":          ".zip",
	"image/bmp":                ".bmp",
	"image/gif":                ".gif",
	"image/jpeg":               ".jpg",
	"image/png":                ".png",
	"image/svg+xml":            ".svg",
	"image/webp":               ".webp",
	"text/csv":                 ".csv",
	"text/html":                ".html",
	"text/plain":               ".txt",
	"text/xml":                 ".xml",
	"video/mp4":                ".mp4",
	"application/octet-stream": "",
}

var drivePreviewTypes = []drivePreviewTypeMeta{
	{Code: "0", Name: "PDF", Type: "pdf", Label: "PDF Preview"},
	{Code: "1", Name: "PNG", Type: "png", Label: "PNG Preview", Aliases: []string{"image"}},
	{Code: "2", Name: "PAGES", Type: "pages", Label: "Paged Preview"},
	{Code: "3", Name: "VIDEO", Type: "video", Label: "Video Preview"},
	{Code: "4", Name: "MP4_360P", Type: "mp4_360p", Label: "MP4 360P Preview"},
	{Code: "5", Name: "MP4_480P", Type: "mp4_480p", Label: "MP4 480P Preview"},
	{Code: "6", Name: "MP4_720P", Type: "mp4_720p", Label: "MP4 720P Preview"},
	{Code: "7", Name: "JPG", Type: "jpg", Label: "JPG Preview", Aliases: []string{"image"}},
	{Code: "8", Name: "HTML", Type: "html", Label: "HTML Preview"},
	{Code: "9", Name: "PDF_LIN", Type: "pdf_lin", Label: "Linearized PDF Preview"},
	{Code: "10", Name: "XOD", Type: "xod", Label: "XOD Preview"},
	{Code: "11", Name: "JPG_LIN", Type: "jpg_lin", Label: "Linearized JPG Preview", Aliases: []string{"image"}},
	{Code: "12", Name: "PNG_LIN", Type: "png_lin", Label: "Linearized PNG Preview", Aliases: []string{"image"}},
	{Code: "13", Name: "ARCHIVE", Type: "archive", Label: "Archive Preview"},
	{Code: "14", Name: "TEXT", Type: "text", Label: "Text Preview"},
	{Code: "15", Name: "PDF_PART", Type: "pdf_part", Label: "Partial PDF Preview"},
	{Code: "16", Name: "SOURCE_FILE", Type: "source_file", Label: "Source File", Aliases: []string{"source"}},
	{Code: "17", Name: "VIDEO_META", Type: "video_meta", Label: "Video Metadata"},
	{Code: "18", Name: "WPS", Type: "wps", Label: "WPS Preview"},
	{Code: "19", Name: "SPLIT_PNG", Type: "split_png", Label: "Split PNG Preview", Aliases: []string{"image"}},
	{Code: "20", Name: "MEDIA_RESULT", Type: "media_result", Label: "Media Result"},
	{Code: "21", Name: "MIME", Type: "mime", Label: "MIME Type"},
	{Code: "22", Name: "SPILT_IMG_TXT", Type: "spilt_img_txt", Label: "Split Image Text"},
	{Code: "23", Name: "MP4_1080P", Type: "mp4_1080p", Label: "MP4 1080P Preview"},
	{Code: "24", Name: "IMAGE_META", Type: "image_meta", Label: "Image Metadata"},
	{Code: "25", Name: "DOC_PART", Type: "doc_part", Label: "Document Part"},
	{Code: "26", Name: "WATERMARK_PDF", Type: "watermark_pdf", Label: "Watermarked PDF Preview"},
	{Code: "27", Name: "FILE_WATERMARK", Type: "file_watermark", Label: "File Watermark"},
}

var drivePreviewStatuses = []drivePreviewStatusMeta{
	{Code: "0", Name: "READY", Downloadable: true},
	{Code: "1", Name: "PROCESSING", Reason: "Preview is still processing."},
	{Code: "2", Name: "FAILED", Reason: "Preview generation failed."},
	{Code: "3", Name: "FAILED_NOT_RETRY", Reason: "Preview generation failed and will not retry."},
	{Code: "4", Name: "INVALID_EXTENTION", Reason: "File extension is invalid for this preview type."},
	{Code: "5", Name: "FILE_TOO_LARGE", Reason: "File is too large for preview generation."},
	{Code: "6", Name: "EMPTY_FILE", Reason: "File is empty."},
	{Code: "7", Name: "NO_SUPPORT", Reason: "Preview is not supported for this file."},
	{Code: "8", Name: "INVALID_PREVIEW_TYPE", Reason: "Preview type is invalid."},
	{Code: "9", Name: "NEED_PASSWORD", Reason: "Preview requires a password."},
	{Code: "10", Name: "FILE_INVALID", Reason: "File is invalid."},
	{Code: "11", Name: "TOO_MANY_PAGES", Reason: "File has too many pages for preview."},
	{Code: "1001", Name: "ARCHIVE_INVALID_FORMAT", Reason: "Archive format is invalid."},
	{Code: "1002", Name: "ARCHIVE_TOO_MANY_NODES", Reason: "Archive contains too many nodes."},
	{Code: "1003", Name: "ARCHIVE_TOO_MANY_NODES_PER_DIR", Reason: "Archive directory contains too many nodes."},
	{Code: "1004", Name: "THIRD_ENC_NO_PERMISSION", Reason: "No permission for third-party encrypted file."},
	{Code: "1006", Name: "NOT_SUPPORT_DECRYPT_THIRD_ENC_FILE", Reason: "Third-party encrypted file cannot be decrypted for preview."},
}

var drivePreviewTypeByCode = func() map[string]drivePreviewTypeMeta {
	out := make(map[string]drivePreviewTypeMeta, len(drivePreviewTypes))
	for _, meta := range drivePreviewTypes {
		out[meta.Code] = meta
	}
	return out
}()

var drivePreviewStatusByCode = func() map[string]drivePreviewStatusMeta {
	out := make(map[string]drivePreviewStatusMeta, len(drivePreviewStatuses))
	for _, meta := range drivePreviewStatuses {
		out[meta.Code] = meta
	}
	return out
}()

var driveCoverSpecs = []driveCoverSpec{
	{
		Name:        "default",
		Label:       "Default Cover",
		Description: "Standard large cover (1280x1280).",
		PreviewType: "1",
		BusType:     "cover",
		Platform:    "pc",
		FallbackExt: ".png",
	},
	{
		Name:        "icon",
		Label:       "Icon",
		Description: "Small list icon (120x120).",
		PreviewType: "1",
		BusType:     "icon",
		FallbackExt: ".png",
	},
	{
		Name:        "grid",
		Label:       "Grid Cover",
		Description: "Grid/card stream cover (360x360).",
		PreviewType: "1",
		BusType:     "grid",
		FallbackExt: ".png",
	},
	{
		Name:        "small",
		Label:       "Small Graph",
		Description: "PC small graph cover (480x480).",
		PreviewType: "1",
		BusType:     "small_graph",
		Platform:    "pc",
		FallbackExt: ".png",
	},
	{
		Name:        "middle",
		Label:       "Middle Cover",
		Description: "Medium-sized cover (720x720).",
		PreviewType: "1",
		BusType:     "middle",
		FallbackExt: ".png",
	},
	{
		Name:        "big",
		Label:       "Big Cover",
		Description: "Large mobile-oriented cover (850x850).",
		PreviewType: "1",
		BusType:     "big",
		Platform:    "mobile",
		FallbackExt: ".png",
	},
	{
		Name:        "square",
		Label:       "Square Cover",
		Description: "Square-cropped grid cover (360x360).",
		PreviewType: "1",
		Width:       360,
		Height:      360,
		Policy:      "near",
		FallbackExt: ".png",
	},
}

// validateDrivePreviewMode checks the required flag combinations for list and
// download modes.
func validateDrivePreviewMode(selected string, listOnly bool, outputPath, flagName string) error {
	selected = strings.TrimSpace(selected)
	outputPath = strings.TrimSpace(outputPath)
	selectedFlag := "--" + flagName
	if listOnly {
		if selected != "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s cannot be combined with --list-only", selectedFlag).WithParam(selectedFlag)
		}
		if outputPath != "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output cannot be combined with --list-only").WithParam("--output")
		}
		return nil
	}
	if selected == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "either --list-only or %s is required", selectedFlag).WithParam(selectedFlag)
	}
	if outputPath == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output is required when %s is set", selectedFlag).WithParam("--output")
	}
	return nil
}

// validateDrivePreviewIfExists validates the accepted overwrite policy values.
func validateDrivePreviewIfExists(policy string) error {
	switch strings.TrimSpace(policy) {
	case "", drivePreviewIfExistsError, drivePreviewIfExistsOverwrite, drivePreviewIfExistsRename:
		return nil
	default:
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --if-exists %q: allowed values are error, overwrite, rename", policy).WithParam("--if-exists")
	}
}

// fetchDrivePreviewCandidates loads preview_result data and normalizes the
// returned candidate list.
func fetchDrivePreviewCandidates(runtime *common.RuntimeContext, fileToken string, body map[string]interface{}) (map[string]interface{}, []drivePreviewCandidate, error) {
	data, err := runtime.CallAPITyped(
		"POST",
		fmt.Sprintf("/open-apis/drive/v1/medias/%s/preview_result", validate.EncodePathSegment(fileToken)),
		nil,
		body,
	)
	if err != nil {
		return nil, nil, err
	}
	return data, normalizeDrivePreviewCandidates(data), nil
}

// normalizeDrivePreviewCandidates converts preview_result items into internal
// candidate records with stable type and status metadata.
func normalizeDrivePreviewCandidates(data map[string]interface{}) []drivePreviewCandidate {
	items := common.GetSlice(data, "preview_results")
	candidates := make([]drivePreviewCandidate, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		typeCode := firstString(raw, "preview_type", "type_code", "type")
		statusCode := firstString(raw, "preview_status", "status_code", "status")
		candidate := drivePreviewCandidate{
			TypeCode:   typeCode,
			StatusCode: statusCode,
			Reason:     strings.TrimSpace(firstString(raw, "reason", "status_msg", "message", "msg", "detail")),
		}
		applyDrivePreviewTypeMeta(&candidate)
		applyDrivePreviewStatusMeta(&candidate)
		candidates = append(candidates, candidate)
	}
	return candidates
}

// selectDrivePreviewCandidate matches a requested preview type or alias against
// the available candidates.
func selectDrivePreviewCandidate(candidates []drivePreviewCandidate, requested string) (drivePreviewCandidate, bool) {
	requested = normalizeDrivePreviewRequest(requested)
	if requested == "" {
		return drivePreviewCandidate{}, false
	}

	for _, candidate := range candidates {
		if requested == candidate.Type || requested == strings.ToLower(candidate.TypeName) || requested == strings.ToLower(strings.TrimSpace(candidate.TypeCode)) {
			return candidate, true
		}
	}

	var firstAliasMatch drivePreviewCandidate
	hasAliasMatch := false
	for _, candidate := range candidates {
		if !slices.Contains(previewAliasesForCandidate(candidate), requested) {
			continue
		}
		if candidate.Downloadable {
			return candidate, true
		}
		if !hasAliasMatch {
			firstAliasMatch = candidate
			hasAliasMatch = true
		}
	}
	if hasAliasMatch {
		return firstAliasMatch, true
	}
	return drivePreviewCandidate{}, false
}

// buildDrivePreviewListOutput formats preview candidates for --list-only
// responses.
func buildDrivePreviewListOutput(fileToken string, candidates []drivePreviewCandidate) map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(candidates))
	for _, candidate := range candidates {
		item := map[string]interface{}{
			"type":         candidate.Type,
			"type_code":    candidate.TypeCode,
			"label":        candidate.Label,
			"status":       candidate.Status,
			"status_code":  candidate.StatusCode,
			"downloadable": candidate.Downloadable,
		}
		if candidate.Reason != "" {
			item["reason"] = candidate.Reason
		}
		items = append(items, item)
	}
	out := map[string]interface{}{
		"mode":       "list",
		"file_token": fileToken,
		"candidates": items,
	}
	if len(items) > 0 {
		out["next_action"] = "select one candidate and rerun with --type plus --output"
	}
	return out
}

// buildDriveCoverListOutput formats the built-in cover specs for --list-only
// responses.
func buildDriveCoverListOutput(fileToken string) map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(driveCoverSpecs))
	for _, spec := range driveCoverSpecs {
		item := map[string]interface{}{
			"spec":  spec.Name,
			"label": spec.Label,
		}
		if spec.Description != "" {
			item["description"] = spec.Description
		}
		items = append(items, item)
	}
	return map[string]interface{}{
		"mode":        "list",
		"file_token":  fileToken,
		"candidates":  items,
		"next_action": "select one spec and rerun with --spec plus --output",
	}
}

// findDriveCoverSpec resolves a cover spec by its user-facing name.
func findDriveCoverSpec(name string) (driveCoverSpec, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, spec := range driveCoverSpecs {
		if spec.Name == name {
			return spec, true
		}
	}
	return driveCoverSpec{}, false
}

// buildDriveCoverDownloadParams translates a cover spec into preview_download
// query parameters.
func buildDriveCoverDownloadParams(version string, spec driveCoverSpec) map[string]interface{} {
	params := map[string]interface{}{
		"preview_type": spec.PreviewType,
	}
	if strings.TrimSpace(spec.BusType) != "" {
		params["bus_type"] = spec.BusType
	}
	if strings.TrimSpace(spec.Platform) != "" {
		params["platform"] = spec.Platform
	}
	if spec.Width > 0 {
		params["width"] = spec.Width
	}
	if spec.Height > 0 {
		params["height"] = spec.Height
	}
	if strings.TrimSpace(spec.Policy) != "" {
		params["policy"] = spec.Policy
	}
	if strings.TrimSpace(version) != "" {
		params["version"] = version
	}
	return params
}

// downloadDrivePreviewArtifact downloads a preview artifact for a single
// preview_type value.
func downloadDrivePreviewArtifact(ctx context.Context, runtime *common.RuntimeContext, fileToken, previewType, version, outputPath, ifExists, fallbackExt string) (map[string]interface{}, error) {
	query := map[string]interface{}{
		"preview_type": previewType,
	}
	if strings.TrimSpace(version) != "" {
		query["version"] = version
	}
	return downloadDrivePreviewArtifactWithParams(ctx, runtime, fileToken, query, outputPath, ifExists, fallbackExt)
}

// downloadDrivePreviewArtifactWithParams downloads a preview artifact using the
// provided preview_download query parameters and writes it to the local path.
func downloadDrivePreviewArtifactWithParams(ctx context.Context, runtime *common.RuntimeContext, fileToken string, query map[string]interface{}, outputPath, ifExists, fallbackExt string) (map[string]interface{}, error) {
	if err := validate.ResourceName(fileToken, "--file-token"); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--file-token")
	}
	if _, err := runtime.ResolveSavePath(outputPath); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", err).WithParam("--output")
	}

	queryParams := make(larkcore.QueryParams, len(query))
	for key, value := range query {
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			continue
		}
		queryParams[key] = []string{text}
	}

	apiReq := &larkcore.ApiReq{
		HttpMethod:  http.MethodGet,
		ApiPath:     fmt.Sprintf("/open-apis/drive/v1/medias/%s/preview_download", validate.EncodePathSegment(fileToken)),
		QueryParams: queryParams,
	}

	resp, err := runtime.DoAPIStream(ctx, apiReq)
	if err != nil {
		return nil, wrapDriveNetworkErr(err, "preview download failed: %s", err)
	}
	defer resp.Body.Close()

	finalPath, _, err := resolveDrivePreviewOutputPath(runtime, outputPath, resp.Header, fallbackExt, ifExists)
	if err != nil {
		return nil, err
	}

	result, err := runtime.FileIO().Save(finalPath, fileio.SaveOptions{
		ContentType:   resp.Header.Get("Content-Type"),
		ContentLength: resp.ContentLength,
	}, resp.Body)
	if err != nil {
		return nil, driveSaveError(err)
	}

	savedPath, _ := runtime.ResolveSavePath(finalPath)
	if savedPath == "" {
		savedPath = finalPath
	}

	return map[string]interface{}{
		"output_path":  savedPath,
		"size_bytes":   result.Size(),
		"content_type": resp.Header.Get("Content-Type"),
		"status":       "READY",
	}, nil
}

// resolveDrivePreviewOutputPath finalizes the save path, applying extension
// inference and the selected collision policy.
func resolveDrivePreviewOutputPath(runtime *common.RuntimeContext, outputPath string, header http.Header, fallbackExt, ifExists string) (string, *driveExtensionResolution, error) {
	finalPath, resolution := autoAppendDrivePreviewExtension(outputPath, header, fallbackExt)
	if _, err := runtime.ResolveSavePath(finalPath); err != nil {
		return "", nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", err).WithParam("--output")
	}

	switch ifExists {
	case "", drivePreviewIfExistsError:
		if _, statErr := runtime.FileIO().Stat(finalPath); statErr == nil {
			return "", nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "output file already exists: %s (use --if-exists overwrite or rename)", finalPath).WithParam("--output")
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return "", nil, errs.NewInternalError(errs.SubtypeFileIO, "cannot access output path %s: %s", finalPath, statErr).WithCause(statErr)
		}
		return finalPath, resolution, nil
	case drivePreviewIfExistsOverwrite:
		return finalPath, resolution, nil
	case drivePreviewIfExistsRename:
		renamed, err := nextAvailableDrivePreviewPath(runtime.FileIO(), finalPath)
		if err != nil {
			return "", nil, err
		}
		if _, err := runtime.ResolveSavePath(renamed); err != nil {
			return "", nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", err).WithParam("--output")
		}
		return renamed, resolution, nil
	default:
		return "", nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --if-exists %q: allowed values are error, overwrite, rename", ifExists).WithParam("--if-exists")
	}
}

// nextAvailableDrivePreviewPath finds the first unused "name (n)" variant for a
// target output path.
func nextAvailableDrivePreviewPath(fio fileio.FileIO, path string) (string, error) {
	if _, err := fio.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return path, nil
		}
		return "", errs.NewInternalError(errs.SubtypeFileIO, "cannot access output path %s: %s", path, err).WithCause(err)
	}
	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)
	for i := 1; i < 10000; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
		if _, err := fio.Stat(candidate); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return candidate, nil
			}
			return "", errs.NewInternalError(errs.SubtypeFileIO, "cannot access candidate output path %s: %s", candidate, err).WithCause(err)
		}
	}
	return "", errs.NewInternalError(errs.SubtypeFileIO, "cannot allocate a unique output path for %s", path)
}

// autoAppendDrivePreviewExtension appends an inferred extension when the user
// did not provide one explicitly.
func autoAppendDrivePreviewExtension(outputPath string, header http.Header, fallbackExt string) (string, *driveExtensionResolution) {
	if drivePreviewHasExplicitExtension(outputPath) {
		return outputPath, nil
	}
	normalizedPath := outputPath
	if filepath.Ext(outputPath) == "." {
		normalizedPath = strings.TrimSuffix(outputPath, ".")
	}
	if resolution := drivePreviewExtensionByContentType(header.Get("Content-Type")); resolution != nil {
		return normalizedPath + resolution.Ext, resolution
	}
	if resolution := drivePreviewExtensionByContentDisposition(header); resolution != nil {
		return normalizedPath + resolution.Ext, resolution
	}
	if fallbackExt != "" {
		return normalizedPath + fallbackExt, &driveExtensionResolution{
			Ext:    fallbackExt,
			Source: "fallback",
			Detail: "default fallback",
		}
	}
	return outputPath, nil
}

// drivePreviewHasExplicitExtension reports whether the path already ends with a
// usable filename extension.
func drivePreviewHasExplicitExtension(path string) bool {
	ext := filepath.Ext(path)
	return ext != "" && ext != "."
}

// drivePreviewExtensionByContentType maps a response Content-Type header to a
// file extension when possible.
func drivePreviewExtensionByContentType(contentType string) *driveExtensionResolution {
	if contentType == "" {
		return nil
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	}
	if ext, ok := drivePreviewMimeToExt[strings.ToLower(mediaType)]; ok && ext != "" {
		return &driveExtensionResolution{
			Ext:    ext,
			Source: "Content-Type",
			Detail: contentType,
		}
	}
	return nil
}

// drivePreviewExtensionByContentDisposition extracts an extension from the
// response filename metadata.
func drivePreviewExtensionByContentDisposition(header http.Header) *driveExtensionResolution {
	filename := strings.TrimSpace(larkcore.FileNameByHeader(header))
	if filename == "" {
		return nil
	}
	ext := filepath.Ext(filename)
	if ext == "" || ext == "." {
		return nil
	}
	return &driveExtensionResolution{
		Ext:    ext,
		Source: "Content-Disposition",
		Detail: filename,
	}
}

// drivePreviewFallbackExt returns the default extension for known preview type
// aliases when headers do not provide one.
func drivePreviewFallbackExt(alias string) string {
	switch normalizeDrivePreviewRequest(alias) {
	case "pdf":
		return ".pdf"
	case "html":
		return ".html"
	case "text":
		return ".txt"
	case "png", "png_lin", "split_png":
		return ".png"
	case "jpg", "jpg_lin":
		return ".jpg"
	case "source", "source_file":
		return ""
	default:
		return ""
	}
}

// applyDrivePreviewTypeMeta fills normalized type metadata from the preview
// type code.
func applyDrivePreviewTypeMeta(candidate *drivePreviewCandidate) {
	if candidate == nil {
		return
	}
	if meta, ok := drivePreviewTypeByCode[candidate.TypeCode]; ok {
		candidate.Type = meta.Type
		candidate.TypeName = meta.Name
		candidate.Label = meta.Label
		return
	}
	code := strings.TrimSpace(candidate.TypeCode)
	if code == "" {
		candidate.Type = "unknown"
		candidate.TypeName = "UNKNOWN"
		candidate.Label = "Unknown Preview Type"
		return
	}
	candidate.Type = "unknown_" + code
	candidate.TypeName = "UNKNOWN"
	candidate.Label = fmt.Sprintf("Unknown Preview Type %s", code)
}

// applyDrivePreviewStatusMeta fills normalized status metadata from the preview
// status code.
func applyDrivePreviewStatusMeta(candidate *drivePreviewCandidate) {
	if candidate == nil {
		return
	}
	if meta, ok := drivePreviewStatusByCode[candidate.StatusCode]; ok {
		candidate.Status = meta.Name
		candidate.Downloadable = meta.Downloadable
		if candidate.Reason == "" && !meta.Downloadable {
			candidate.Reason = meta.Reason
		}
		if meta.Downloadable {
			candidate.Reason = ""
		}
		return
	}
	candidate.Status = "UNKNOWN"
	candidate.Downloadable = false
	if candidate.Reason == "" {
		if strings.TrimSpace(candidate.StatusCode) == "" {
			candidate.Reason = "Preview status is missing."
		} else {
			candidate.Reason = fmt.Sprintf("Unknown preview status %s.", candidate.StatusCode)
		}
	}
}

// normalizeDrivePreviewRequest canonicalizes user input for preview type
// matching.
func normalizeDrivePreviewRequest(requested string) string {
	requested = strings.ToLower(strings.TrimSpace(requested))
	requested = strings.ReplaceAll(requested, "-", "_")
	requested = strings.ReplaceAll(requested, " ", "_")
	return requested
}

// previewAliasesForCandidate returns configured aliases for a preview
// candidate's type code.
func previewAliasesForCandidate(candidate drivePreviewCandidate) []string {
	if meta, ok := drivePreviewTypeByCode[candidate.TypeCode]; ok {
		return meta.Aliases
	}
	return nil
}

// firstString returns the first non-empty string-like value from the provided
// keys.
func firstString(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			if strings.TrimSpace(t) != "" {
				return t
			}
		case fmt.Stringer:
			if s := strings.TrimSpace(t.String()); s != "" {
				return s
			}
		case float64:
			return strconv.FormatInt(int64(t), 10)
		case int:
			return strconv.Itoa(t)
		case int64:
			return strconv.FormatInt(t, 10)
		case bool:
			return strconv.FormatBool(t)
		}
	}
	return ""
}

// versionString normalizes version fields from heterogeneous API payload types.
func versionString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	default:
		return ""
	}
}

// availableDrivePreviewTypes lists unique normalized preview type names from
// the candidate set.
func availableDrivePreviewTypes(candidates []drivePreviewCandidate) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		name := strings.TrimSpace(candidate.Type)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// availableDriveCoverSpecs lists the supported built-in cover spec names.
func availableDriveCoverSpecs() []string {
	out := make([]string, 0, len(driveCoverSpecs))
	for _, spec := range driveCoverSpecs {
		out = append(out, spec.Name)
	}
	return out
}

// wrapDrivePreviewUnavailable builds a validation error for an unsupported
// preview selection.
func wrapDrivePreviewUnavailable(fileToken, requested string, candidates []drivePreviewCandidate, reason string) error {
	available := availableDrivePreviewTypes(candidates)
	if reason == "" {
		reason = fmt.Sprintf("requested preview type %q is not available for file %s", requested, fileToken)
	}
	hint := "rerun with --list-only to inspect available preview types"
	if len(available) > 0 {
		hint = fmt.Sprintf("available preview types: %s", strings.Join(available, ", "))
	}
	return errs.NewValidationError(errs.SubtypeFailedPrecondition, reason).WithHint(hint).WithParam("--type")
}

// wrapDrivePreviewNotReady builds an actionable error for a preview candidate
// that exists but is not yet downloadable.
func wrapDrivePreviewNotReady(fileToken, requested string, candidate drivePreviewCandidate) error {
	reason := candidate.Reason
	if reason == "" {
		reason = fmt.Sprintf("preview type %q is not downloadable yet (status=%s)", requested, candidate.Status)
	}
	hint := fmt.Sprintf("rerun `lark-cli drive +preview --file-token %s --list-only` to inspect current candidate status", fileToken)
	return errs.NewValidationError(errs.SubtypeFailedPrecondition, reason).WithHint(hint).WithParam("--type")
}

// wrapDriveCoverUnavailable builds a validation error for an unknown cover
// spec.
func wrapDriveCoverUnavailable(requested string) error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsupported --spec %q", requested).
		WithHint("available cover specs: %s", strings.Join(availableDriveCoverSpecs(), ", ")).
		WithParam("--spec")
}
