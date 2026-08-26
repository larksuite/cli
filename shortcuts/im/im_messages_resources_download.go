// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/download"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/downloadtransport"
	"github.com/larksuite/cli/shortcuts/common"
)

var ImMessagesResourcesDownload = common.Shortcut{
	Service:     "im",
	Command:     "+messages-resources-download",
	Description: "Download images/files from a message; user/bot; downloads image/file resources by message-id and file-key to a safe relative output path",
	Risk:        "write",
	Scopes:      []string{"im:message:readonly"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "message-id", Desc: "message ID (om_xxx)", Required: true},
		{Name: "file-key", Desc: "resource key (img_xxx or file_xxx)", Required: true},
		{Name: "type", Desc: "resource type (image or file)", Required: true, Enum: []string{"image", "file"}},
		{Name: "output", Desc: "local save path (relative only, no .. traversal); when omitted, uses the server's Content-Disposition filename if available, otherwise file_key; extension is inferred from Content-Disposition or Content-Type if not provided"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		fileKey := runtime.Str("file-key")
		outputPath := runtime.Str("output")
		if outputPath == "" {
			outputPath = fileKey
		}
		return common.NewDryRunAPI().
			GET("/open-apis/im/v1/messages/:message_id/resources/:file_key").
			Params(map[string]interface{}{"type": runtime.Str("type")}).
			Set("message_id", runtime.Str("message-id")).Set("file_key", fileKey).
			Set("type", runtime.Str("type")).Set("output", outputPath)
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if messageId := runtime.Str("message-id"); messageId == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--message-id is required (om_xxx)").WithParam("--message-id")
		} else if _, err := validateMessageID(messageId); err != nil {
			return err
		}
		relPath, err := normalizeDownloadOutputPath(runtime.Str("file-key"), runtime.Str("output"))
		if err != nil {
			return err
		}
		if _, err := runtime.ResolveSavePath(relPath); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", err).WithParam("--output").WithCause(err)
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		messageId := runtime.Str("message-id")
		fileKey := runtime.Str("file-key")
		fileType := runtime.Str("type")
		relPath, err := normalizeDownloadOutputPath(fileKey, runtime.Str("output"))
		if err != nil {
			return err
		}
		if _, err := runtime.ResolveSavePath(relPath); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", err).WithParam("--output").WithCause(err)
		}

		// With an explicit --output, keep that basename (append only an
		// extension); without it, adopt the server's original filename.
		preserveBasename := runtime.Str("output") != ""
		finalPath, sizeBytes, err := downloadIMResourceToPath(ctx, runtime, messageId, fileKey, fileType, relPath, preserveBasename)
		if err != nil {
			return err
		}

		runtime.Out(map[string]interface{}{"saved_path": finalPath, "size_bytes": sizeBytes}, nil)
		return nil
	},
}

func normalizeDownloadOutputPath(fileKey, outputPath string) (string, error) {
	fileKey = strings.TrimSpace(fileKey)
	if fileKey == "" {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "file-key cannot be empty").WithParam("--file-key")
	}
	if strings.ContainsAny(fileKey, "/\\") {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "file-key cannot contain path separators").WithParam("--file-key")
	}
	if outputPath == "" {
		return fileKey, nil
	}
	outputPath = filepath.Clean(strings.TrimSpace(outputPath))
	if outputPath == "." {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "path cannot be empty").WithParam("--output")
	}
	if filepath.IsAbs(outputPath) {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "absolute paths are not allowed").WithParam("--output")
	}
	if outputPath == ".." || strings.HasPrefix(outputPath, ".."+string(filepath.Separator)) {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "path cannot escape the current working directory").WithParam("--output")
	}
	return outputPath, nil
}

const (
	imDownloadPartSize   = 32 * 1024 * 1024
	imPartRetries        = download.DefaultPartRetries
	imDownloadRetryDelay = 300 * time.Millisecond
)

var imMimeToExt = map[string]string{
	"image/png":                    ".png",
	"image/jpeg":                   ".jpg",
	"image/gif":                    ".gif",
	"image/webp":                   ".webp",
	"image/svg+xml":                ".svg",
	"application/pdf":              ".pdf",
	"video/mp4":                    ".mp4",
	"video/3gpp":                   ".3gp",
	"video/x-msvideo":              ".avi",
	"audio/mpeg":                   ".mp3",
	"audio/ogg":                    ".ogg",
	"audio/wav":                    ".wav",
	"text/plain":                   ".txt",
	"text/html":                    ".html",
	"text/css":                     ".css",
	"text/csv":                     ".csv",
	"application/zip":              ".zip",
	"application/x-zip-compressed": ".zip",
	"application/x-rar-compressed": ".rar",
	"application/json":             ".json",
	"application/xml":              ".xml",
	"application/octet-stream":     ".bin",
	"application/msword":           ".doc",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": ".docx",
	"application/vnd.ms-excel": ".xls",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         ".xlsx",
	"application/vnd.ms-powerpoint":                                             ".ppt",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": ".pptx",
}

func downloadIMResourceToPath(ctx context.Context, runtime *common.RuntimeContext, messageID, fileKey, fileType, outputPath string, preserveBasename bool) (string, int64, error) {
	stream, err := openIMResourceDownload(ctx, runtime, messageID, fileKey, fileType)
	if err != nil {
		return "", 0, err
	}
	defer stream.Body.Close()

	finalPath := resolveIMResourceDownloadPath(outputPath, stream.Header.Get("Content-Type"), stream.Header.Get("Content-Disposition"), preserveBasename)
	sizeBytes := stream.ContentLength

	result, err := runtime.FileIO().Save(finalPath, fileio.SaveOptions{
		ContentType:   stream.Header.Get("Content-Type"),
		ContentLength: sizeBytes,
	}, stream.Body)
	if err != nil {
		return "", 0, common.WrapSaveErrorTyped(err)
	}
	if sizeBytes >= 0 && result.Size() != sizeBytes {
		return "", 0, errs.NewNetworkError(errs.SubtypeNetworkTransport, "file size mismatch: expected %d, got %d", sizeBytes, result.Size())
	}
	savedPath, resolveErr := runtime.ResolveSavePath(finalPath)
	if resolveErr != nil || savedPath == "" {
		savedPath = finalPath
	}
	return savedPath, result.Size(), nil
}

func openIMResourceDownload(ctx context.Context, runtime *common.RuntimeContext, messageID, fileKey, fileType string) (*download.Stream, error) {
	source := imResourceDownloadSource(runtime, messageID, fileKey, fileType)
	return download.Open(ctx, source, download.Options{
		PartSize:         imDownloadPartSize,
		MaxPartRetries:   imPartRetries,
		RetryDelay:       imDownloadRetryDelay,
		DisableMultipart: fileType != "file",
	})
}

// preserveBasename keeps explicit and batch output names collision-safe.
func resolveIMResourceDownloadPath(safePath, contentType, contentDisposition string, preserveBasename bool) string {
	if filepath.Ext(safePath) != "" {
		return safePath
	}
	if cdFilename := parseContentDispositionFilename(contentDisposition); cdFilename != "" {
		if !preserveBasename {
			dir := filepath.Dir(safePath)
			if dir == "." {
				return cdFilename
			}
			return filepath.Join(dir, cdFilename)
		}
		if ext := filepath.Ext(cdFilename); ext != "" {
			return safePath + ext
		}
	}
	mimeType := strings.TrimSpace(strings.Split(contentType, ";")[0])
	if ext, ok := imMimeToExt[mimeType]; ok {
		return safePath + ext
	}
	return safePath
}

// parseContentDispositionFilename returns a safe server filename.
func parseContentDispositionFilename(header string) string {
	if header == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(header)
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(params["filename"])
	if name == "" {
		return ""
	}
	if i := strings.LastIndexAny(name, "/\\"); i >= 0 {
		name = name[i+1:]
	}
	if name == "" || name == "." || name == ".." {
		return ""
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	return name
}

func imResourceDownloadSource(runtime *common.RuntimeContext, messageID, fileKey, fileType string) download.Source {
	oapi := downloadtransport.NewOAPI(runtime.DoAPIStream)
	transport := oapi.Get(
		"/open-apis/im/v1/messages/:message_id/resources/:file_key",
		downloadtransport.PathParam("message_id", messageID),
		downloadtransport.PathParam("file_key", fileKey),
		downloadtransport.Query("type", fileType),
	)
	// A message resource key pins the attachment bytes.
	return download.ImmutableSource(transport)
}
