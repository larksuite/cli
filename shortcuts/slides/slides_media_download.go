// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	defaultSlidesMediaDownloadDir = ".lark-slides/media"
	slidesMediaPreviewTypeSource  = "16"
)

// SlidesMediaDownload downloads a Slides media file token to a local image
// file. It retries through the Drive source-file preview artifact when direct
// export is denied, which is the path available for media embedded in Slides.
var SlidesMediaDownload = common.Shortcut{
	Service:           "slides",
	Command:           "+media-download",
	Description:       "Download a Slides media file_token to a local image file",
	Risk:              "read",
	Scopes:            []string{"docs:document.media:download"},
	ConditionalScopes: []string{"drive:file:download"},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file-token", Desc: "Slides media file_token", Required: true},
		{Name: "output", Desc: "preferred relative output path for one image (extension optional; .png, .jpg, or .jpeg)"},
		{Name: "output-dir", Default: defaultSlidesMediaDownloadDir, Desc: "relative directory for saved media"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateSlidesMediaDownloadFileToken(runtime.Str("file-token")); err != nil {
			return err
		}
		if runtime.Changed("output") {
			if runtime.Changed("output-dir") {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output cannot be combined with --output-dir").WithParam("--output")
			}
			return validateScreenshotOutputPath(runtime, runtime.Str("output"))
		}
		_, err := validateScreenshotOutputDir(runtime, runtime.Str("output-dir"))
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		fileToken := strings.TrimSpace(runtime.Str("file-token"))
		if err := validateSlidesMediaDownloadFileToken(fileToken); err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}

		output := runtime.Str("output")
		if output == "" {
			output = filepath.Join(runtime.Str("output-dir"), fileToken)
		}
		return common.NewDryRunAPI().
			Desc("Try direct Drive media download; on permission denied, fetch the source-file preview artifact").
			GET(fmt.Sprintf("/open-apis/drive/v1/medias/%s/download", validate.EncodePathSegment(fileToken))).
			Set("file_token", fileToken).
			Set("output", output).
			GET(fmt.Sprintf("/open-apis/drive/v1/medias/%s/preview_download", validate.EncodePathSegment(fileToken))).
			Desc("Fallback: download the Drive source-file preview artifact").
			Params(map[string]interface{}{"preview_type": slidesMediaPreviewTypeSource}).
			Set("file_token", fileToken)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		fileToken := strings.TrimSpace(runtime.Str("file-token"))
		if err := validateSlidesMediaDownloadFileToken(fileToken); err != nil {
			return err
		}

		outputTarget, err := resolveSlidesScreenshotOutputTarget(runtime)
		if err != nil {
			return err
		}

		resp, err := runtime.DoAPIStream(ctx, &larkcore.ApiReq{
			HttpMethod: http.MethodGet,
			ApiPath:    fmt.Sprintf("/open-apis/drive/v1/medias/%s/download", validate.EncodePathSegment(fileToken)),
		})
		if err == nil {
			return saveSlidesMediaDownloadResponse(runtime, resp, outputTarget, fileToken, "download")
		}

		if !isSlidesMediaDownloadPermissionError(err) {
			return wrapSlidesMediaDownloadError(err, "media download failed: %s")
		}

		if scopeErr := runtime.EnsureScopes([]string{"drive:file:download"}); scopeErr != nil {
			return scopeErr
		}

		previewResp, previewErr := runtime.DoAPIStream(ctx, &larkcore.ApiReq{
			HttpMethod: http.MethodGet,
			ApiPath:    fmt.Sprintf("/open-apis/drive/v1/medias/%s/preview_download", validate.EncodePathSegment(fileToken)),
			QueryParams: larkcore.QueryParams{
				"preview_type": []string{slidesMediaPreviewTypeSource},
			},
		})
		if previewErr != nil {
			return wrapSlidesMediaDownloadError(previewErr, "source-file preview download failed: %s")
		}
		return saveSlidesMediaDownloadResponse(runtime, previewResp, outputTarget, fileToken, "preview")
	},
}

func validateSlidesMediaDownloadFileToken(fileToken string) error {
	if err := validate.ResourceName(fileToken, "--file-token"); err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--file-token")
	}
	return nil
}

func wrapSlidesMediaDownloadError(err error, format string) error {
	if err == nil {
		return nil
	}
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	return errs.NewNetworkError(errs.SubtypeNetworkTransport, format, err).WithCause(err)
}

func isSlidesMediaDownloadPermissionError(err error) bool {
	problem, ok := errs.ProblemOf(err)
	if !ok || problem == nil {
		return false
	}
	if problem.Category == errs.CategoryAuthorization && problem.Subtype == errs.SubtypePermissionDenied {
		return true
	}
	return problem.Category == errs.CategoryNetwork && problem.Code == http.StatusForbidden
}

func saveSlidesMediaDownloadResponse(runtime *common.RuntimeContext, resp *http.Response, target slidesScreenshotOutputTarget, fileToken, source string) error {
	if resp == nil || resp.Body == nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "media download returned an empty response")
	}
	defer resp.Body.Close()

	outputPath, err := resolveSlidesMediaDownloadOutputPath(runtime, target, resp.Header, fileToken)
	if err != nil {
		return err
	}
	result, err := runtime.FileIO().Save(outputPath, fileio.SaveOptions{
		ContentType:   resp.Header.Get("Content-Type"),
		ContentLength: resp.ContentLength,
	}, resp.Body)
	if err != nil {
		return common.WrapSaveErrorTypedForFlag(err, "--output")
	}

	savedPath, _ := runtime.ResolveSavePath(outputPath)
	if savedPath == "" {
		savedPath = outputPath
	}
	resultData := map[string]interface{}{
		"file_token":   fileToken,
		"path":         savedPath,
		"size":         result.Size(),
		"content_type": resp.Header.Get("Content-Type"),
		"source":       source,
	}
	if target.requested != "" {
		resultData["output"] = savedPath
		if filepath.Clean(target.requestedResolved) != filepath.Clean(savedPath) {
			resultData["requested_output"] = target.requested
			resultData["output_adjusted"] = true
		}
	} else {
		resultData["output_dir"] = target.outputDir
	}
	runtime.Out(resultData, nil)
	return nil
}

func resolveSlidesMediaDownloadOutputPath(runtime *common.RuntimeContext, target slidesScreenshotOutputTarget, header http.Header, fileToken string) (string, error) {
	var outputPath string
	if target.requested != "" {
		outputPath, _ = common.AutoAppendDownloadExtension(target.requested, header, ".jpg")
	} else {
		fileName := common.ResolveDownloadFileName(header, fileToken)
		fileName = safeScreenshotFileBase(fileName)
		fileName, _ = common.AutoAppendDownloadExtension(fileName, header, ".jpg")
		outputPath = filepath.Join(target.safeOutputDir, fileName)
	}
	outputPath = slidesMediaDownloadCorrectOutputExt(outputPath, header)
	outputPath, err := nextSlidesMediaDownloadPath(runtime, outputPath)
	if err != nil {
		return "", err
	}
	if _, err := runtime.ResolveSavePath(outputPath); err != nil {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--output invalid: %v", err).
			WithParam("--output").
			WithCause(err)
	}
	return outputPath, nil
}

func slidesMediaDownloadExtMatches(outputExt string, responseExt string) bool {
	outputExt = strings.ToLower(outputExt)
	if responseExt == ".jpg" {
		return outputExt == ".jpg" || outputExt == ".jpeg"
	}
	return outputExt == responseExt
}

func slidesMediaDownloadCorrectOutputExt(outputPath string, header http.Header) string {
	resolution := common.ExtensionByContentType(header.Get("Content-Type"))
	if resolution == nil {
		return outputPath
	}
	outputExt := filepath.Ext(outputPath)
	if outputExt == "" {
		return outputPath + resolution.Ext
	}
	if slidesMediaDownloadExtMatches(outputExt, resolution.Ext) {
		return outputPath
	}
	return strings.TrimSuffix(outputPath, outputExt) + resolution.Ext
}

func nextSlidesMediaDownloadPath(runtime *common.RuntimeContext, outputPath string) (string, error) {
	ext := filepath.Ext(outputPath)
	base := strings.TrimSuffix(outputPath, ext)
	for i := 0; i < 1000; i++ {
		candidate := outputPath
		if i > 0 {
			candidate = fmt.Sprintf("%s_%d%s", base, i+1, ext)
		}
		if _, err := runtime.FileIO().Stat(candidate); err == nil {
			continue
		} else if !isScreenshotFileNotExist(err) {
			return "", errs.NewInternalError(errs.SubtypeFileIO, "inspect media output path %s: %v", candidate, err).WithCause(err)
		}
		return candidate, nil
	}
	return "", errs.NewInternalError(errs.SubtypeFileIO, "write media output %s: too many duplicate file names", outputPath)
}
