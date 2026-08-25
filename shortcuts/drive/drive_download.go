// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const driveMetadataReadScope = "drive:drive.metadata:readonly"

type driveDownloadOutputPathValidator func(string) error

func driveDownloadNormalizeFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)
	if name == "" || name == "." || name == ".." || strings.Trim(name, "/") == "" {
		return ""
	}
	return name
}

func driveDownloadFallbackFileName(title, fileToken string) string {
	if name := driveDownloadNormalizeFileName(title); name != "" {
		return name
	}
	return fileToken
}

func driveDownloadCandidateOutputPath(header http.Header, candidate string) (string, bool) {
	fileName := driveDownloadNormalizeFileName(candidate)
	if fileName == "" {
		return "", false
	}

	fileName = sanitizeExportFileName(fileName, "")
	if fileName == "" {
		return "", false
	}

	fileName, _ = common.AutoAppendDownloadExtension(fileName, header, "")
	if strings.TrimSpace(fileName) == "" || fileName == "." || fileName == ".." || strings.Trim(fileName, "/") == "" {
		return "", false
	}
	return fileName, true
}

func driveDownloadDefaultOutputPath(header http.Header, title, fileToken string, validatePath driveDownloadOutputPathValidator) (string, error) {
	candidates := []string{
		larkcore.FileNameByHeader(header),
		title,
		fileToken,
	}

	var lastErr error
	for _, candidate := range candidates {
		fileName, ok := driveDownloadCandidateOutputPath(header, candidate)
		if !ok {
			continue
		}
		if validatePath != nil {
			if err := validatePath(fileName); err != nil {
				lastErr = err
				continue
			}
		}
		return fileName, nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return fileToken, nil
}

func driveDownloadShouldFailOnMetadataTitleError(ctx context.Context, err error) bool {
	if ctx != nil {
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return true
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if problem, ok := errs.ProblemOf(err); ok {
		if problem.Category == errs.CategoryAuthorization {
			return true
		}
	}
	return false
}

func driveDownloadIsPermissionAuthScopeError(err error) bool {
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

var DriveDownload = common.Shortcut{
	Service:     "drive",
	Command:     "+download",
	Description: "Download a file from Drive to local",
	Risk:        "read",
	Scopes:      []string{"drive:file:download"},
	// Metadata is only required when --output is omitted and the CLI needs the
	// remote title as the pre-download fallback filename. The wiki scope is only
	// required when the caller passes a wiki node (--wiki-token or a /wiki/ URL)
	// that must be resolved to the underlying file token. Permission auth scope
	// failures are non-blocking so they do not prevent the download API call.
	ConditionalScopes: []string{common.DrivePermissionMemberAuthScope, driveMetadataReadScope, driveWikiNodeRetrieveScope},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file-token", Desc: "Drive file token"},
		{Name: "url", Desc: "Drive file URL, or a wiki node URL that wraps an uploaded file (resolved to the underlying file token)"},
		{Name: "wiki-token", Desc: "wiki node token wrapping an uploaded file (resolved to the underlying file token)"},
		{Name: "output", Desc: "local save path"},
		{Name: "overwrite", Type: "bool", Desc: "overwrite existing output file"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		source, err := normalizeDriveFileSource(runtime.Str("file-token"), runtime.Str("url"), runtime.Str("wiki-token"))
		if err != nil {
			return err
		}
		if source.NeedsWikiResolution() {
			if err := runtime.EnsureScopes([]string{driveWikiNodeRetrieveScope}); err != nil {
				return err
			}
		}

		outputPath := runtime.Str("output")
		if outputPath == "" {
			return runtime.EnsureScopes([]string{driveMetadataReadScope})
		}
		if _, resolveErr := runtime.ResolveSavePath(outputPath); resolveErr != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", resolveErr).WithParam("--output")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		source, err := normalizeDriveFileSource(runtime.Str("file-token"), runtime.Str("url"), runtime.Str("wiki-token"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}

		outputPath := runtime.Str("output")
		plan := common.NewDryRunAPI()
		step := 1

		fileToken := source.FileToken
		if source.NeedsWikiResolution() {
			plan.GET("/open-apis/wiki/v2/spaces/get_node").
				Desc(fmt.Sprintf("[%d] Resolve wiki node to the underlying Drive file token (obj_type must be file)", step)).
				Params(map[string]interface{}{"token": source.WikiToken})
			plan.Set("wiki_token", source.WikiToken)
			fileToken = "obj_token_from_wiki_node"
			step++
		}

		common.AddDriveFileExportPermissionDryRun(
			plan,
			fileToken,
			fmt.Sprintf("[%d] Check whether the current identity can export the Drive file", step),
		)
		step++

		downloadDesc := fmt.Sprintf("[%d] Download file bytes to the explicit output path", step)
		if outputPath == "" {
			outputPath = "<Content-Disposition filename | metadata title | token>"
			plan.
				POST("/open-apis/drive/v1/metas/batch_query").
				Desc(fmt.Sprintf("[%d] Resolve metadata title before downloading; fails before the download request if metadata scope is missing", step)).
				Body(map[string]interface{}{
					"request_docs": []map[string]interface{}{
						{
							"doc_token": fileToken,
							"doc_type":  "file",
						},
					},
				})
			step++
			downloadDesc = fmt.Sprintf("[%d] Download file bytes; Content-Disposition filename wins over metadata title when present", step)
		}
		return plan.
			GET("/open-apis/drive/v1/files/:file_token/download").
			Desc(downloadDesc).
			Set("file_token", fileToken).
			Set("output", outputPath)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		source, err := normalizeDriveFileSource(runtime.Str("file-token"), runtime.Str("url"), runtime.Str("wiki-token"))
		if err != nil {
			return err
		}

		fileToken := source.FileToken
		var wikiResolution driveFileWikiResolution
		if source.NeedsWikiResolution() {
			resolvedToken, resolution, resolveErr := resolveDriveFileWikiSource(ctx, runtime, source)
			if resolveErr != nil {
				return resolveErr
			}
			fileToken = resolvedToken
			wikiResolution = resolution
		}

		outputPath := runtime.Str("output")
		overwrite := runtime.Bool("overwrite")

		// Early path validation + overwrite check
		if outputPath != "" {
			if _, resolveErr := runtime.ResolveSavePath(outputPath); resolveErr != nil {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", resolveErr).WithParam("--output")
			}
			if _, statErr := runtime.FileIO().Stat(outputPath); statErr == nil && !overwrite {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "output file already exists: %s (use --overwrite to replace)", outputPath).WithParam("--output")
			}
		}

		allowed, err := common.CheckDriveFileExportPermission(runtime, fileToken)
		if err != nil {
			if driveDownloadIsPermissionAuthScopeError(err) {
				fmt.Fprintf(runtime.IO().ErrOut, "warning: export permission check failed; continuing with download: %v\n", err)
			} else {
				return withDriveDownloadRecoveryHint(err, fileToken)
			}
		} else if !allowed {
			return driveDownloadPermissionDeniedError()
		}

		var metadataTitle string
		if outputPath == "" {
			title, err := common.FetchDriveMetaTitle(runtime, fileToken, "file")
			if err != nil {
				if driveDownloadShouldFailOnMetadataTitleError(ctx, err) {
					if ctxErr := ctx.Err(); ctxErr != nil {
						return ctxErr
					}
					return err
				}
				fmt.Fprintf(runtime.IO().ErrOut, "warning: metadata title lookup failed; continuing with Content-Disposition or token filename: %v\n", err)
			} else {
				metadataTitle = title
			}
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Downloading: %s\n", common.MaskToken(fileToken))

		resp, err := runtime.DoAPIStream(ctx, &larkcore.ApiReq{
			HttpMethod: http.MethodGet,
			ApiPath:    fmt.Sprintf("/open-apis/drive/v1/files/%s/download", validate.EncodePathSegment(fileToken)),
		})
		if err != nil {
			return withDriveDownloadRecoveryHint(wrapDriveNetworkErr(err, "download failed: %s", err), fileToken)
		}
		defer resp.Body.Close()

		if outputPath == "" {
			var resolveErr error
			outputPath, resolveErr = driveDownloadDefaultOutputPath(resp.Header, metadataTitle, fileToken, func(path string) error {
				_, err := runtime.ResolveSavePath(path)
				return err
			})
			if resolveErr != nil {
				return errs.NewInternalError(errs.SubtypeFileIO, "cannot derive a safe default output path: %s", resolveErr).WithCause(resolveErr)
			}
		}
		if _, statErr := runtime.FileIO().Stat(outputPath); statErr == nil && !overwrite {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "output file already exists: %s (use --overwrite to replace)", outputPath).WithParam("--output")
		}

		result, err := runtime.FileIO().Save(outputPath, fileio.SaveOptions{
			ContentType:   resp.Header.Get("Content-Type"),
			ContentLength: resp.ContentLength,
		}, resp.Body)
		if err != nil {
			return driveSaveError(err)
		}

		savedPath, _ := runtime.ResolveSavePath(outputPath)
		if savedPath == "" {
			savedPath = outputPath
		}
		runtime.Out(annotateDriveFileWikiOutput(map[string]interface{}{
			"saved_path": savedPath,
			"size_bytes": result.Size(),
		}, wikiResolution), nil)
		return nil
	},
}
