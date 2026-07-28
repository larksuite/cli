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

var DriveDownload = common.Shortcut{
	Service:     "drive",
	Command:     "+download",
	Description: "Download a file from Drive to local",
	Risk:        "read",
	Scopes:      []string{"drive:file:download"},
	// Metadata is only required when --output is omitted and the CLI needs the
	// remote title as the pre-download fallback filename.
	ConditionalScopes: []string{driveMetadataReadScope},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file-token", Desc: "file token", Required: true},
		{Name: "output", Desc: "local save path"},
		{Name: "overwrite", Type: "bool", Desc: "overwrite existing output file"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		fileToken := runtime.Str("file-token")
		outputPath := runtime.Str("output")

		if err := validate.ResourceName(fileToken, "--file-token"); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--file-token")
		}
		if outputPath == "" {
			if err := runtime.EnsureScopes([]string{driveMetadataReadScope}); err != nil {
				return err
			}
			return nil
		}
		if _, resolveErr := runtime.ResolveSavePath(outputPath); resolveErr != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", resolveErr).WithParam("--output")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		fileToken := runtime.Str("file-token")
		outputPath := runtime.Str("output")
		plan := common.NewDryRunAPI()
		downloadDesc := "[1] Download file bytes to the explicit output path"
		if outputPath == "" {
			outputPath = "<Content-Disposition filename | metadata title | token>"
			downloadDesc = "[2] Download file bytes; Content-Disposition filename wins over metadata title when present"
			plan.
				POST("/open-apis/drive/v1/metas/batch_query").
				Desc("[1] Resolve metadata title before downloading; fails before the download request if metadata scope is missing").
				Body(map[string]interface{}{
					"request_docs": []map[string]interface{}{
						{
							"doc_token": fileToken,
							"doc_type":  "file",
						},
					},
				})
		}
		return plan.
			GET("/open-apis/drive/v1/files/:file_token/download").
			Desc(downloadDesc).
			Set("file_token", fileToken).
			Set("output", outputPath)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		fileToken := runtime.Str("file-token")
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
			return wrapDriveNetworkErr(err, "download failed: %s", err)
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
		runtime.Out(map[string]interface{}{
			"saved_path": savedPath,
			"size_bytes": result.Size(),
		}, nil)
		return nil
	},
}
