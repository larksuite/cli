// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"net/http"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

func docMediaDownloadIsPermissionAuthScopeError(err error) bool {
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryAuthorization {
		return false
	}
	switch problem.Code {
	case output.LarkErrAppScopeNotEnabled,
		output.LarkErrTokenNoPermission,
		output.LarkErrUserScopeInsufficient:
		return true
	default:
		return false
	}
}

var DocMediaDownload = common.Shortcut{
	Service:           "docs",
	Command:           "+media-download",
	Description:       "Download document media or whiteboard thumbnail (auto-detects extension)",
	Risk:              "read",
	Scopes:            []string{"docs:document.media:download"},
	ConditionalScopes: []string{common.DrivePermissionMemberAuthScope},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "token", Desc: "resource token (file_token or whiteboard_id)", Required: true},
		{Name: "output", Desc: "local save path", Required: true},
		{Name: "type", Default: "media", Desc: "resource type: media (default) | whiteboard"},
		{Name: "overwrite", Type: "bool", Desc: "overwrite existing output file"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token := runtime.Str("token")
		outputPath := runtime.Str("output")
		mediaType := runtime.Str("type")
		if mediaType == "whiteboard" {
			return common.NewDryRunAPI().
				GET("/open-apis/board/v1/whiteboards/:token/download_as_image").
				Desc("(when --type=whiteboard) Download whiteboard as image").
				Set("token", token).Set("output", outputPath)
		}
		plan := common.AddDriveFileExportPermissionDryRun(
			common.NewDryRunAPI(),
			token,
			"[1] Check whether the current identity can export the document media",
		)
		return plan.
			GET("/open-apis/drive/v1/medias/:token/download").
			Desc("[2] (when --type=media) Download document media file").
			Set("token", token).Set("output", outputPath)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("token")
		outputPath := runtime.Str("output")
		mediaType := runtime.Str("type")
		overwrite := runtime.Bool("overwrite")

		if err := validate.ResourceName(token, "--token"); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--token")
		}
		if _, err := runtime.ResolveSavePath(outputPath); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", err).WithParam("--output").WithCause(err)
		}
		if mediaType != "whiteboard" {
			allowed, err := common.CheckDriveFileExportPermission(runtime, token)
			if err != nil {
				if docMediaDownloadIsPermissionAuthScopeError(err) {
					fmt.Fprintf(runtime.IO().ErrOut, "warning: export permission check failed; continuing with download: %v\n", err)
				} else {
					return withDocMediaDownloadRecoveryHint(err, mediaType)
				}
			} else if !allowed {
				return docMediaDownloadPermissionDeniedError()
			}
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Downloading: %s %s\n", mediaType, common.MaskToken(token))

		// Build API URL
		encodedToken := validate.EncodePathSegment(token)
		var apiPath string
		if mediaType == "whiteboard" {
			apiPath = fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/download_as_image", encodedToken)
		} else {
			apiPath = fmt.Sprintf("/open-apis/drive/v1/medias/%s/download", encodedToken)
		}

		resp, err := runtime.DoAPIStream(ctx, &larkcore.ApiReq{
			HttpMethod: http.MethodGet,
			ApiPath:    apiPath,
		})
		if err != nil {
			return withDocMediaDownloadRecoveryHint(wrapDocNetworkErr(err, "download failed: %v", err), mediaType)
		}
		defer resp.Body.Close()

		fallbackExt := ""
		if mediaType == "whiteboard" {
			fallbackExt = ".png"
		}
		finalPath, _ := autoAppendDocMediaExtension(outputPath, resp.Header, fallbackExt)

		// Validate final path after extension append
		if finalPath != outputPath {
			if _, err := runtime.ResolveSavePath(finalPath); err != nil {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", err).WithParam("--output").WithCause(err)
			}
		}

		// Overwrite check on final path (after extension detection)
		if !overwrite {
			if _, statErr := runtime.FileIO().Stat(finalPath); statErr == nil {
				return errs.NewValidationError(errs.SubtypeFailedPrecondition, "output file already exists: %s (use --overwrite to replace)", finalPath).WithParam("--output")
			}
		}

		result, err := runtime.FileIO().Save(finalPath, fileio.SaveOptions{
			ContentType:   resp.Header.Get("Content-Type"),
			ContentLength: resp.ContentLength,
		}, resp.Body)
		if err != nil {
			return common.WrapSaveErrorTyped(err)
		}

		savedPath, _ := runtime.ResolveSavePath(finalPath)
		if savedPath == "" {
			savedPath = finalPath
		}
		runtime.Out(map[string]interface{}{
			"saved_path":   savedPath,
			"size_bytes":   result.Size(),
			"content_type": resp.Header.Get("Content-Type"),
		}, nil)
		return nil
	},
}
