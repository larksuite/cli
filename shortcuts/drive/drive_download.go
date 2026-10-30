// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/download"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/downloadtransport"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

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

// driveDownloadPartSize bounds each ranged response. open.feishu.cn resets a
// download stream about five minutes after it starts, so one full response
// cannot carry a large file. Each part request costs ~0.5-1 s before the
// first byte; 128 MiB parts keep that overhead small while a part still
// finishes well inside the window on links down to ~0.5 MB/s.
var driveDownloadPartSize = int64(128 * 1024 * 1024)

// openDriveFileDownload streams the file in validated ranged parts.
//
// The file token does not pin the bytes: +upload --file-token replaces the
// content in place, and the download response carries no ETag. Parts may be
// combined only when the caller holds the latest_modify_time read before the
// first part and re-checks it after the last (driveRevisionCheckReader).
// Without it, MutableSource makes the engine fall back to one full response.
//
// Ceiling: latest_modify_time has one-second resolution, so a replacement that
// lands in the same second as the revision read is not detected. A server
// validator (ETag or a readable version) would close that window.
func openDriveFileDownload(ctx context.Context, runtime *common.RuntimeContext, fileToken, revision string) (*download.Stream, error) {
	transport := downloadtransport.NewOAPI(runtime.DoAPIStream).Get(
		"/open-apis/drive/v1/files/:file_token/download",
		downloadtransport.PathParam("file_token", fileToken),
	)
	source := download.MutableSource(transport)
	if revision != "" {
		source = download.ImmutableSource(transport)
	}
	return download.Open(ctx, source, download.Options{PartSize: driveDownloadPartSize})
}

// driveRevisionCheckReader runs verify before reporting EOF, so a file that
// changed during a multipart download fails before the output is committed.
type driveRevisionCheckReader struct {
	reader io.Reader
	verify func() error
	err    error
}

func (r *driveRevisionCheckReader) Read(p []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	n, err := r.reader.Read(p)
	if errors.Is(err, io.EOF) {
		if verifyErr := r.verify(); verifyErr != nil {
			r.err = verifyErr
			return n, verifyErr
		}
	}
	return n, err
}

func verifyDriveDownloadRevision(runtime *common.RuntimeContext, fileToken, revision string) error {
	meta, err := common.FetchDriveMeta(runtime, fileToken, "file", false)
	if err != nil {
		return wrapDriveNetworkErr(err, "cannot confirm the file was unchanged during a multipart download: %s", err)
	}
	if meta.LatestModifyTime != revision {
		return errs.NewNetworkError(errs.SubtypeNetworkRepresentationChanged,
			"file changed while downloading: latest_modify_time is %q, want %q", meta.LatestModifyTime, revision).
			WithRetryable().
			WithHint("run the command again to download the current version")
	}
	return nil
}

var DriveDownload = common.Shortcut{
	Service:     "drive",
	Command:     "+download",
	Description: "Download a file from Drive to local",
	Risk:        "read",
	Scopes:      []string{"drive:file:download"},
	// Entity lookup uses metadata permission best-effort. Metadata is required
	// only for default naming; wiki permission is required only if an explicit
	// wiki input falls back to get_node. Permission auth scope failures are also
	// non-blocking so they do not prevent the download API call.
	ConditionalScopes: []string{common.DrivePermissionMemberAuthScope, driveMetadataReadScope, driveWikiNodeRetrieveScope},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file-token", Desc: "Drive file token"},
		{Name: "url", Desc: "Drive file URL or Wiki node URL wrapping an uploaded file"},
		{Name: "wiki-token", Desc: "Wiki node token wrapping an uploaded file"},
		{Name: "output", Desc: "local save path"},
		{Name: "overwrite", Type: "bool", Desc: "overwrite existing output file"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := normalizeDriveFileSource(runtime.Str("file-token"), runtime.Str("url"), runtime.Str("wiki-token"))
		if err != nil {
			return err
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
		fileToken, step := addDriveFileSourceDryRun(plan, source)

		common.AddDriveFileExportPermissionDryRun(
			plan,
			fileToken,
			fmt.Sprintf("[%d] Check whether the current identity can export the Drive file", step),
		)
		step++

		metadataDesc := fmt.Sprintf("[%d] Read the file revision (latest_modify_time) so ranged parts can be combined; on failure the download uses one full response", step)
		downloadDesc := fmt.Sprintf("[%d] Download file bytes to the explicit output path", step+1)
		if outputPath == "" {
			outputPath = "<Content-Disposition filename | metadata title | token>"
			metadataDesc = fmt.Sprintf("[%d] Resolve metadata title and file revision before downloading; fails before the download request if metadata scope is missing", step)
			downloadDesc = fmt.Sprintf("[%d] Download file bytes; Content-Disposition filename wins over metadata title when present", step+1)
		}
		plan.
			POST("/open-apis/drive/v1/metas/batch_query").
			Desc(metadataDesc).
			Body(map[string]interface{}{
				"request_docs": []map[string]interface{}{
					{
						"doc_token": fileToken,
						"doc_type":  "file",
					},
				},
			})
		downloadDesc += fmt.Sprintf(" in %d MiB Range requests", driveDownloadPartSize>>20)
		return plan.
			GET("/open-apis/drive/v1/files/:file_token/download").
			Desc(downloadDesc).
			POST("/open-apis/drive/v1/metas/batch_query").
			Desc(fmt.Sprintf("[%d] Only when the file spans more than one part: re-read latest_modify_time and fail before committing the output if it changed", step+2)).
			Body(map[string]interface{}{
				"request_docs": []map[string]interface{}{
					{
						"doc_token": fileToken,
						"doc_type":  "file",
					},
				},
			}).
			Set("file_token", fileToken).
			Set("output", outputPath)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		source, err := normalizeDriveFileSource(runtime.Str("file-token"), runtime.Str("url"), runtime.Str("wiki-token"))
		if err != nil {
			return err
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

		fileToken, wikiResolution, err := resolveDriveFileSource(ctx, runtime, source)
		if err != nil {
			return err
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

		// Metadata supplies the default file name and the revision that lets
		// ranged parts be combined. With --output it is best-effort: without a
		// revision the download keeps using one full response.
		meta, metaErr := common.FetchDriveMeta(runtime, fileToken, "file", false)
		if metaErr != nil {
			if outputPath == "" && driveDownloadShouldFailOnMetadataTitleError(ctx, metaErr) {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				return metaErr
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if outputPath == "" {
				fmt.Fprintf(runtime.IO().ErrOut, "warning: metadata title lookup failed; continuing with Content-Disposition or token filename: %v\n", metaErr)
			}
			meta = common.DriveMeta{}
		}

		stream, err := openDriveFileDownload(ctx, runtime, fileToken, meta.LatestModifyTime)
		if err != nil {
			return withDriveDownloadRecoveryHint(wrapDriveNetworkErr(err, "download failed: %s", err), fileToken)
		}
		defer stream.Body.Close()
		if meta.LatestModifyTime == "" && stream.ContentLength > driveDownloadPartSize {
			reason := "metadata has no latest_modify_time"
			if metaErr != nil {
				reason = metaErr.Error()
			}
			fmt.Fprintf(runtime.IO().ErrOut, "warning: file revision unavailable, so %d bytes download in one request, which the server may reset for large files: %s\n", stream.ContentLength, reason)
		}

		if outputPath == "" {
			var resolveErr error
			outputPath, resolveErr = driveDownloadDefaultOutputPath(stream.Header, meta.Title, fileToken, func(path string) error {
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

		body := io.Reader(stream.Body)
		if meta.LatestModifyTime != "" && stream.ContentLength > driveDownloadPartSize {
			body = &driveRevisionCheckReader{
				reader: stream.Body,
				verify: func() error {
					return verifyDriveDownloadRevision(runtime, fileToken, meta.LatestModifyTime)
				},
			}
		}
		result, err := runtime.FileIO().Save(outputPath, fileio.SaveOptions{
			ContentType:   stream.Header.Get("Content-Type"),
			ContentLength: stream.ContentLength,
		}, body)
		if err != nil {
			if _, ok := errs.ProblemOf(err); ok {
				return withDriveDownloadRecoveryHint(err, fileToken)
			}
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
