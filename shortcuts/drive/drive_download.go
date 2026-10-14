// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	extdownload "github.com/larksuite/cli/extension/download"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/downloadtransport"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// driveDownloadPartSize bounds each ranged request of a drive download. 64 MiB
// keeps per-request overhead low for multi-GB files while staying far below the
// stream lifetime observed on the drive download endpoint, so a failed part is
// cheap to replay.
const driveDownloadPartSize int64 = 64 << 20

// driveDownloadProgressEvery reports transfer progress every 64 MiB.
const driveDownloadProgressEvery int64 = 64 << 20

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
		{Name: "continue", Type: "bool", Desc: "resume an interrupted download from <output>.partial; the partial file is kept on failure for a later --continue"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := normalizeDriveFileSource(runtime.Str("file-token"), runtime.Str("url"), runtime.Str("wiki-token"))
		if err != nil {
			return err
		}
		outputPath := runtime.Str("output")
		if outputPath == "" {
			if runtime.Bool("continue") {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--continue requires an explicit --output path (the partial file lives next to it)").WithParam("--continue")
			}
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

		common.AddDriveFileViewPermissionDryRun(
			plan,
			fileToken,
			fmt.Sprintf("[%d] Check whether the current identity can view the Drive file", step),
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

		outputPath := runtime.Str("output")
		overwrite := runtime.Bool("overwrite")

		// Early path validation + overwrite check
		if outputPath != "" {
			if err := driveDownloadEnsureOutputAbsent(runtime, outputPath, overwrite); err != nil {
				return err
			}
		}

		fileToken, wikiResolution, err := resolveDriveFileSource(ctx, runtime, source)
		if err != nil {
			return err
		}
		allowed, err := common.CheckDriveFileViewPermission(runtime, fileToken)
		if err != nil {
			if driveDownloadIsPermissionAuthScopeError(err) {
				fmt.Fprintf(runtime.IO().ErrOut, "warning: view permission check failed; continuing with download: %v\n", err)
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

		// Resumable, chunked download. The drive download API honors HTTP
		// Range (206), so long transfers are split into bounded parts with
		// per-part retries, and --continue can resume from an existing
		// <output>.partial instead of restarting from byte 0.
		transport := downloadtransport.NewOAPI(runtime.DoAPIStream).Get(
			"/open-apis/drive/v1/files/:file_token/download",
			downloadtransport.PathParam("file_token", fileToken),
		)
		dlSource := extdownload.MutableSource(transport)

		cont := runtime.Bool("continue")
		var resumeIO fileio.ResumableFileIO
		if cont {
			var ok bool
			resumeIO, ok = runtime.FileIO().(fileio.ResumableFileIO)
			if !ok {
				return errs.NewInternalError(errs.SubtypeFileIO, "file backend does not support --continue downloads").
					WithHint("configure a resumable file backend or omit --continue")
			}
		}

		var startOffset int64
		var resumeSize int64
		var resumeETag string
		var partialPath string
		var checkpointPath string
		if cont {
			var resolveErr error
			_, partialPath, checkpointPath, resolveErr = driveDownloadPaths(runtime, outputPath)
			if resolveErr != nil {
				return resolveErr
			}
		}

		if cont {
			fi, statErr := runtime.FileIO().Stat(partialPath)
			if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
				return errs.NewInternalError(errs.SubtypeFileIO, "inspect resume partial: %s", statErr).WithCause(statErr)
			}
			if statErr == nil {
				localSize := fi.Size()
				if localSize > 0 {
					checkpoint, checkpointErr := driveDownloadReadCheckpoint(resumeIO, checkpointPath)
					checkpointETag := ""
					if checkpoint != nil {
						checkpointETag = driveDownloadStrongETag(checkpoint.ETag)
					}
					if checkpointErr != nil || checkpoint == nil || checkpoint.Size <= 0 || checkpointETag == "" {
						if cleanupErr := driveDownloadDiscardResume(resumeIO, partialPath, checkpointPath); cleanupErr != nil {
							return driveDownloadResumeArtifactError("discard unverifiable partial", cleanupErr)
						}
					} else {
						probe, probeErr := extdownload.ProbeRange(ctx, dlSource, checkpointETag)
						if probeErr != nil && driveDownloadRangeProbeRejected(probeErr) {
							if cleanupErr := driveDownloadDiscardResume(resumeIO, partialPath, checkpointPath); cleanupErr != nil {
								return driveDownloadResumeArtifactError("discard partial after unsupported Range", cleanupErr)
							}
						} else if probeErr != nil {
							return withDriveDownloadRecoveryHint(wrapDriveNetworkErr(probeErr, "resume probe failed: %s", probeErr), fileToken)
						} else {
							probeTotal, probeETag := probe.TotalSize, probe.ETag
							checkpointValid := checkpoint.Size == probeTotal && probeETag != "" && probeETag == checkpointETag
							switch {
							case !checkpointValid || localSize > probeTotal:
								if cleanupErr := driveDownloadDiscardResume(resumeIO, partialPath, checkpointPath); cleanupErr != nil {
									return driveDownloadResumeArtifactError("discard stale partial", cleanupErr)
								}
							case localSize == probeTotal:
								if err := driveDownloadEnsureOutputAbsent(runtime, outputPath, overwrite); err != nil {
									return err
								}
								if err := resumeIO.CommitResumeArtifact(partialPath, outputPath, overwrite); err != nil {
									return driveDownloadCommitError(err, outputPath, overwrite)
								}
								if cleanupErr := driveDownloadDiscardResume(resumeIO, "", checkpointPath); cleanupErr != nil {
									fmt.Fprintf(runtime.IO().ErrOut, "warning: committed download but could not remove resume checkpoint: %v\n", cleanupErr)
								}
								savedPath, _ := runtime.ResolveSavePath(outputPath)
								runtime.Out(annotateDriveFileWikiOutput(map[string]interface{}{
									"saved_path": savedPath,
									"size_bytes": localSize,
									"resumed":    true,
								}, wikiResolution), nil)
								return nil
							default:
								startOffset = localSize
								resumeSize = checkpoint.Size
								resumeETag = checkpointETag
							}
						}
					}
				} else if cleanupErr := driveDownloadDiscardResume(resumeIO, partialPath, checkpointPath); cleanupErr != nil {
					return driveDownloadResumeArtifactError("discard empty partial", cleanupErr)
				}
			}
		}

		openStream := func(offset int64, etag string) (*extdownload.Stream, error) {
			return extdownload.Open(ctx, dlSource, extdownload.Options{
				PartSize:     driveDownloadPartSize,
				StartOffset:  offset,
				ExpectedETag: etag,
			})
		}
		stream, err := openStream(startOffset, resumeETag)
		if err != nil && cont && startOffset > 0 && driveDownloadShouldRestartResume(err) {
			if cleanupErr := driveDownloadDiscardResume(resumeIO, partialPath, checkpointPath); cleanupErr != nil {
				return driveDownloadResumeArtifactError("discard partial before full restart", cleanupErr)
			}
			startOffset, resumeSize, resumeETag = 0, 0, ""
			stream, err = openStream(0, "")
		}
		if err != nil {
			return withDriveDownloadRecoveryHint(wrapDriveNetworkErr(err, "download failed: %s", err), fileToken)
		}
		if cont && startOffset > 0 && stream.ContentLength != resumeSize {
			stream.Body.Close()
			if cleanupErr := driveDownloadDiscardResume(resumeIO, partialPath, checkpointPath); cleanupErr != nil {
				return driveDownloadResumeArtifactError("discard partial after size change", cleanupErr)
			}
			startOffset, resumeSize, resumeETag = 0, 0, ""
			stream, err = openStream(0, "")
			if err != nil {
				return withDriveDownloadRecoveryHint(wrapDriveNetworkErr(err, "download failed: %s", err), fileToken)
			}
		}
		defer stream.Body.Close()

		if outputPath == "" {
			var resolveErr error
			outputPath, resolveErr = driveDownloadDefaultOutputPath(stream.Header, metadataTitle, fileToken, func(path string) error {
				_, err := runtime.ResolveSavePath(path)
				return err
			})
			if resolveErr != nil {
				return errs.NewInternalError(errs.SubtypeFileIO, "cannot derive a safe default output path: %s", resolveErr).WithCause(resolveErr)
			}
		}
		if err := driveDownloadEnsureOutputAbsent(runtime, outputPath, overwrite); err != nil {
			return err
		}

		progress := &driveDownloadProgressReader{
			r:       stream.Body,
			total:   stream.ContentLength,
			written: startOffset,
			out:     runtime.IO().ErrOut,
		}

		if !cont {
			result, saveErr := runtime.FileIO().Save(outputPath, fileio.SaveOptions{
				ContentType:   stream.Header.Get("Content-Type"),
				ContentLength: stream.ContentLength,
			}, progress)
			if saveErr != nil {
				return driveSaveError(saveErr)
			}
			written := result.Size()
			if stream.ContentLength > 0 && written != stream.ContentLength {
				// Save already published outputPath. Remove the truncated artifact
				// so a retry without --overwrite is not blocked by a corrupt file.
				driveDownloadRemovePublishedOutput(runtime, outputPath)
				return errs.NewNetworkError(errs.SubtypeNetworkProtocol,
					"download size mismatch: got %d of %d bytes", written, stream.ContentLength)
			}
			savedPath, _ := runtime.ResolveSavePath(outputPath)
			runtime.Out(annotateDriveFileWikiOutput(map[string]interface{}{
				"saved_path": savedPath,
				"size_bytes": written,
				"resumed":    false,
			}, wikiResolution), nil)
			return nil
		}

		// The checkpoint is written before the append so a failed stream leaves
		// enough remote identity for the next --continue attempt to validate the
		// bytes already kept in the partial file.
		if cpErr := driveDownloadWriteCheckpoint(resumeIO, checkpointPath, stream.ContentLength, driveDownloadStrongETag(stream.Header.Get("ETag"))); cpErr != nil {
			fmt.Fprintf(runtime.IO().ErrOut, "warning: could not write resume checkpoint: %v\n", cpErr)
		}
		appendLength := stream.ContentLength
		if appendLength >= 0 {
			appendLength -= startOffset
		}
		result, copyErr := resumeIO.AppendTo(partialPath, fileio.SaveOptions{
			ContentType:   stream.Header.Get("Content-Type"),
			ContentLength: appendLength,
		}, progress)
		if copyErr != nil {
			return withDriveDownloadRecoveryHint(driveAppendError(copyErr), fileToken)
		}
		written := result.Size()
		if stream.ContentLength > 0 && written != stream.ContentLength-startOffset {
			return errs.NewNetworkError(errs.SubtypeNetworkProtocol,
				"download size mismatch: got %d of %d bytes", written+startOffset, stream.ContentLength)
		}
		partialInfo, partialStatErr := runtime.FileIO().Stat(partialPath)
		if partialStatErr != nil {
			return errs.NewInternalError(errs.SubtypeFileIO, "inspect completed resume partial: %s", partialStatErr).
				WithCause(partialStatErr)
		}
		finalPartialSize := partialInfo.Size()
		if finalPartialSize != startOffset+written {
			return errs.NewInternalError(errs.SubtypeFileIO,
				"resume partial size changed during append: got %d, want %d", finalPartialSize, startOffset+written)
		}
		if stream.ContentLength > 0 && finalPartialSize != stream.ContentLength {
			return errs.NewNetworkError(errs.SubtypeNetworkProtocol,
				"download size mismatch after append: got %d of %d bytes", finalPartialSize, stream.ContentLength)
		}

		if err := resumeIO.CommitResumeArtifact(partialPath, outputPath, overwrite); err != nil {
			return driveDownloadCommitError(err, outputPath, overwrite)
		}
		if cleanupErr := driveDownloadDiscardResume(resumeIO, "", checkpointPath); cleanupErr != nil {
			fmt.Fprintf(runtime.IO().ErrOut, "warning: committed download but could not remove resume checkpoint: %v\n", cleanupErr)
		}
		savedPath, _ := runtime.ResolveSavePath(outputPath)
		runtime.Out(annotateDriveFileWikiOutput(map[string]interface{}{
			"saved_path": savedPath,
			"size_bytes": finalPartialSize,
			"resumed":    startOffset > 0,
		}, wikiResolution), nil)
		return nil
	},
}

// driveDownloadCheckpoint ties a <output>.partial file to the remote
// representation it was downloaded from. A resume is only allowed when both
// the size and strong ETag still match the current remote file.
type driveDownloadCheckpoint struct {
	Size int64  `json:"size"`
	ETag string `json:"etag,omitempty"`
}

// driveDownloadPaths resolves the output path and derives the sibling
// partial and checkpoint paths from it.
func driveDownloadPaths(runtime *common.RuntimeContext, outputPath string) (resolved, partial, checkpoint string, err error) {
	resolved, err = runtime.ResolveSavePath(outputPath)
	if err != nil {
		return "", "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", err).WithParam("--output")
	}
	// Keep the artifact names in the provider's logical namespace. A custom
	// backend may not use local absolute paths, while the suffix relationship
	// to the user-supplied output name remains meaningful to that backend.
	return resolved, outputPath + ".partial", outputPath + ".partial.meta", nil
}

// driveDownloadRemovePublishedOutput best-effort deletes a final output path
// that was published before a later integrity check failed.
func driveDownloadRemovePublishedOutput(runtime *common.RuntimeContext, outputPath string) {
	if workspace, ok := runtime.FileIO().(fileio.WorkspaceFileIO); ok {
		if err := workspace.RemoveWorkspaceEntry(outputPath); err != nil {
			fmt.Fprintf(runtime.IO().ErrOut, "warning: download size mismatch left an incomplete output that could not be removed: %v\n", err)
		}
		return
	}
	if resolved, err := runtime.FileIO().ResolvePath(outputPath); err == nil {
		if removeErr := os.Remove(resolved); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) { //nolint:forbidigo // FileIO has no generic delete; shortcuts cannot import internal/vfs.
			fmt.Fprintf(runtime.IO().ErrOut, "warning: download size mismatch left an incomplete output that could not be removed: %v\n", removeErr)
		}
	}
}

// driveDownloadEnsureOutputAbsent fails when the final output already exists
// and --overwrite was not given.
func driveDownloadEnsureOutputAbsent(runtime *common.RuntimeContext, outputPath string, overwrite bool) error {
	if !overwrite {
		if _, statErr := runtime.FileIO().Stat(outputPath); statErr == nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "output file already exists: %s (use --overwrite to replace)", outputPath).WithParam("--output")
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			if errors.Is(statErr, fileio.ErrPathValidation) {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", statErr).WithParam("--output").WithCause(statErr)
			}
			return errs.NewInternalError(errs.SubtypeFileIO, "inspect output path: %s", statErr).WithCause(statErr)
		}
	}
	return nil
}

// driveDownloadDiscardResume removes a partial file and its checkpoint.
// Passing an empty path skips that artifact.
func driveDownloadDiscardResume(resumeIO fileio.ResumableFileIO, partialPath, checkpointPath string) error {
	var errsToJoin []error
	for _, artifactPath := range []string{partialPath, checkpointPath} {
		if artifactPath == "" {
			continue
		}
		if err := resumeIO.RemoveResumeArtifact(artifactPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errsToJoin = append(errsToJoin, err)
		}
	}
	return errors.Join(errsToJoin...)
}

// driveDownloadWriteCheckpoint persists the checkpoint next to the partial
// file. A failure is non-fatal for the current download; without a checkpoint
// a later --continue conservatively restarts from byte 0.
func driveDownloadWriteCheckpoint(resumeIO fileio.ResumableFileIO, path string, size int64, etag string) error {
	data, err := json.Marshal(driveDownloadCheckpoint{Size: size, ETag: etag})
	if err != nil {
		return err
	}
	return resumeIO.WriteResumeArtifact(path, data)
}

// driveDownloadReadCheckpoint loads a checkpoint written by
// driveDownloadWriteCheckpoint. A missing or malformed checkpoint is reported
// as an error so the caller treats the partial as unverifiable.
func driveDownloadReadCheckpoint(resumeIO fileio.ResumableFileIO, path string) (*driveDownloadCheckpoint, error) {
	data, err := resumeIO.ReadResumeArtifact(path)
	if err != nil {
		return nil, err
	}
	var cp driveDownloadCheckpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, err
	}
	return &cp, nil
}

// driveDownloadStrongETag accepts only a quoted, non-weak validator. It keeps
// checkpoint identity compatible with download.Options.ExpectedETag, which
// deliberately rejects weak validators for If-Range.
func driveDownloadStrongETag(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "W/") || len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return ""
	}
	for i := 1; i < len(value)-1; i++ {
		if c := value[i]; c != 0x21 && !(c >= 0x23 && c <= 0x7e) && c < 0x80 {
			return ""
		}
	}
	return value
}

func driveDownloadResumeArtifactError(action string, err error) error {
	return errs.NewInternalError(errs.SubtypeFileIO, "%s: %s", action, err).WithCause(err)
}

func driveDownloadCommitError(err error, outputPath string, overwrite bool) error {
	if !overwrite && errors.Is(err, fs.ErrExist) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"output file already exists: %s (use --overwrite to replace)", outputPath).
			WithParam("--output").WithCause(err)
	}
	return errs.NewInternalError(errs.SubtypeFileIO, "cannot commit downloaded file: %s", err).WithCause(err)
}

func driveDownloadRangeProbeRejected(err error) bool {
	problem, ok := errs.ProblemOf(err)
	return ok && (problem.Code == http.StatusBadRequest || problem.Code == http.StatusRequestedRangeNotSatisfiable)
}

func driveDownloadShouldRestartResume(err error) bool {
	if extdownload.IsResumeUnsupported(err) {
		return true
	}
	problem, ok := errs.ProblemOf(err)
	return ok && problem.Subtype == errs.SubtypeNetworkRepresentationChanged
}

// driveDownloadProgressReader wraps a download body reader and prints coarse
// progress to stderr for long transfers.
type driveDownloadProgressReader struct {
	r       io.Reader
	total   int64
	written int64
	last    int64
	out     io.Writer
}

func (p *driveDownloadProgressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.written += int64(n)
	if p.out != nil && p.written-p.last >= driveDownloadProgressEvery {
		p.last = p.written
		if p.total > 0 {
			fmt.Fprintf(p.out, "downloaded %d of %d bytes (%d%%)\n",
				p.written, p.total, p.written*100/p.total)
		} else {
			fmt.Fprintf(p.out, "downloaded %d bytes\n", p.written)
		}
	}
	return n, err
}
