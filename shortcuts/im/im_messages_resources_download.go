// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
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
			return output.ErrValidation("--message-id is required (om_xxx)")
		} else if _, err := validateMessageID(messageId); err != nil {
			return err
		}
		relPath, err := normalizeDownloadOutputPath(runtime.Str("file-key"), runtime.Str("output"))
		if err != nil {
			return output.ErrValidation("%s", err)
		}
		if _, err := runtime.ResolveSavePath(relPath); err != nil {
			return output.ErrValidation("unsafe output path: %s", err)
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		messageId := runtime.Str("message-id")
		fileKey := runtime.Str("file-key")
		fileType := runtime.Str("type")
		relPath, err := normalizeDownloadOutputPath(fileKey, runtime.Str("output"))
		if err != nil {
			return output.ErrValidation("invalid output path: %s", err)
		}
		if _, err := runtime.ResolveSavePath(relPath); err != nil {
			return output.ErrValidation("unsafe output path: %s", err)
		}

		userSpecifiedOutput := runtime.Str("output") != ""
		finalPath, sizeBytes, err := downloadIMResourceToPath(ctx, runtime, messageId, fileKey, fileType, relPath, userSpecifiedOutput)
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
		return "", fmt.Errorf("file-key cannot be empty")
	}
	if strings.ContainsAny(fileKey, "/\\") {
		return "", fmt.Errorf("file-key cannot contain path separators")
	}
	if outputPath == "" {
		return fileKey, nil
	}
	outputPath = filepath.Clean(strings.TrimSpace(outputPath))
	if outputPath == "." {
		return "", fmt.Errorf("path cannot be empty")
	}
	if filepath.IsAbs(outputPath) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}
	if outputPath == ".." || strings.HasPrefix(outputPath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path cannot escape the current working directory")
	}
	return outputPath, nil
}

const (
	defaultIMResourceDownloadTimeout = 120 * time.Second
	probeChunkSize                   = int64(128 * 1024)
	normalChunkSize                  = int64(8 * 1024 * 1024)
	imDownloadRequestRetries         = 2
	imDownloadRetryDelay             = 300 * time.Millisecond
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

type rangeChunkReader struct {
	ctx        context.Context
	runtime    *common.RuntimeContext
	messageID  string
	fileKey    string
	fileType   string
	totalSize  int64
	delivered  int64
	current    io.ReadCloser
	nextOffset int64
}

func newRangeChunkReader(
	ctx context.Context,
	runtime *common.RuntimeContext,
	messageID, fileKey, fileType string,
	probeBody io.ReadCloser,
	totalSize int64,
) *rangeChunkReader {
	return &rangeChunkReader{
		ctx:        ctx,
		runtime:    runtime,
		messageID:  messageID,
		fileKey:    fileKey,
		fileType:   fileType,
		totalSize:  totalSize,
		current:    probeBody,
		nextOffset: probeChunkSize,
	}
}

func (r *rangeChunkReader) Read(p []byte) (int, error) {
	for {
		if r.current != nil {
			n, err := r.current.Read(p)
			r.delivered += int64(n)

			if r.delivered > r.totalSize {
				if err == io.EOF {
					closeErr := r.current.Close()
					r.current = nil
					if closeErr != nil {
						return 0, closeErr
					}
				}
				return 0, output.ErrNetwork("chunk overflow: delivered %d, expected %d", r.delivered, r.totalSize)
			}

			switch err {
			case nil:
				return n, nil
			case io.EOF:
				closeErr := r.current.Close()
				r.current = nil
				if closeErr != nil {
					return n, closeErr
				}
				if r.delivered == r.totalSize {
					if n > 0 {
						return n, nil
					}
					return 0, io.EOF
				}
				if n > 0 {
					return n, nil
				}
			default:
				return n, err
			}
		}

		if r.nextOffset >= r.totalSize {
			if r.delivered == r.totalSize {
				return 0, io.EOF
			}
			return 0, output.ErrNetwork("file size mismatch: expected %d, got %d", r.totalSize, r.delivered)
		}

		end := min(r.nextOffset+normalChunkSize-1, r.totalSize-1)
		resp, err := doIMResourceDownloadRequest(r.ctx, r.runtime, r.messageID, r.fileKey, r.fileType, map[string]string{
			"Range": fmt.Sprintf("bytes=%d-%d", r.nextOffset, end),
		})
		if err != nil {
			return 0, err
		}
		if resp.StatusCode >= 400 {
			defer resp.Body.Close()
			return 0, downloadResponseError(resp)
		}
		if resp.StatusCode != http.StatusPartialContent {
			resp.Body.Close()
			return 0, output.ErrNetwork("unexpected status code: %d", resp.StatusCode)
		}

		r.current = resp.Body
		r.nextOffset = end + 1
	}
}

func (r *rangeChunkReader) Close() error {
	if r.current == nil {
		return nil
	}
	err := r.current.Close()
	r.current = nil
	return err
}

func initialIMResourceDownloadHeaders(fileType string) map[string]string {
	if fileType != "file" {
		return nil
	}
	return map[string]string{
		"Range": fmt.Sprintf("bytes=0-%d", probeChunkSize-1),
	}
}

func downloadIMResourceToPath(ctx context.Context, runtime *common.RuntimeContext, messageID, fileKey, fileType, outputPath string, userSpecifiedOutput bool) (string, int64, error) {
	downloadResp, err := doIMResourceDownloadRequest(ctx, runtime, messageID, fileKey, fileType, initialIMResourceDownloadHeaders(fileType))
	if err != nil {
		return "", 0, err
	}
	if downloadResp == nil {
		return "", 0, output.ErrNetwork("download failed: empty response")
	}

	if downloadResp.StatusCode >= 400 {
		defer downloadResp.Body.Close()
		return "", 0, downloadResponseError(downloadResp)
	}

	finalPath := resolveIMResourceDownloadPath(outputPath, downloadResp.Header.Get("Content-Type"), downloadResp.Header.Get("Content-Disposition"), userSpecifiedOutput)

	var (
		body      io.ReadCloser
		sizeBytes int64
	)
	switch downloadResp.StatusCode {
	case http.StatusPartialContent:
		totalSize, err := parseTotalSize(downloadResp.Header.Get("Content-Range"))
		if err != nil {
			downloadResp.Body.Close()
			return "", 0, output.ErrNetwork("invalid Content-Range header on range response: %s", err)
		}
		body = newRangeChunkReader(ctx, runtime, messageID, fileKey, fileType, downloadResp.Body, totalSize)
		sizeBytes = totalSize

	case http.StatusOK:
		body = downloadResp.Body
		sizeBytes = downloadResp.ContentLength

	default:
		downloadResp.Body.Close()
		return "", 0, output.ErrNetwork("unexpected status code: %d", downloadResp.StatusCode)
	}
	defer body.Close()

	result, err := runtime.FileIO().Save(finalPath, fileio.SaveOptions{
		ContentType:   downloadResp.Header.Get("Content-Type"),
		ContentLength: sizeBytes,
	}, body)
	if err != nil {
		return "", 0, common.WrapSaveErrorByCategory(err, "api_error")
	}
	if sizeBytes >= 0 && result.Size() != sizeBytes {
		return "", 0, output.ErrNetwork("file size mismatch: expected %d, got %d", sizeBytes, result.Size())
	}
	savedPath, resolveErr := runtime.ResolveSavePath(finalPath)
	if resolveErr != nil || savedPath == "" {
		savedPath = finalPath
	}
	return savedPath, result.Size(), nil
}

func resolveIMResourceDownloadPath(safePath, contentType, contentDisposition string, userSpecifiedOutput bool) string {
	if filepath.Ext(safePath) != "" {
		return safePath
	}
	if cdFilename := parseContentDispositionFilename(contentDisposition); cdFilename != "" {
		if !userSpecifiedOutput {
			// No --output flag: use the original filename from the server.
			dir := filepath.Dir(safePath)
			if dir == "." {
				return cdFilename
			}
			return filepath.Join(dir, cdFilename)
		}
		// User specified a path without extension: append the extension from the CD filename.
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

// parseContentDispositionFilename extracts and sanitizes the filename from a
// Content-Disposition header. It handles RFC 5987 encoded filenames (filename*)
// with priority over plain filename via the standard mime package.
// Returns an empty string if no valid filename can be extracted.
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
	// Strip any path component (Unix or Windows style) to prevent path traversal.
	if i := strings.LastIndexAny(name, "/\\"); i >= 0 {
		name = name[i+1:]
	}
	if name == "" || name == "." || name == ".." {
		return ""
	}
	// Reject control characters (including null bytes).
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	return name
}

func doIMResourceDownloadRequest(ctx context.Context, runtime *common.RuntimeContext, messageID, fileKey, fileType string, headers map[string]string) (*http.Response, error) {
	query := larkcore.QueryParams{}
	query.Set("type", fileType)

	headerValues := make(http.Header, len(headers))
	for key, value := range headers {
		headerValues.Set(key, value)
	}

	req := &larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    "/open-apis/im/v1/messages/:message_id/resources/:file_key",
		PathParams: larkcore.PathParams{
			"message_id": messageID,
			"file_key":   fileKey,
		},
		QueryParams: query,
	}

	var lastErr error
	for attempt := 0; attempt <= imDownloadRequestRetries; attempt++ {
		resp, err := runtime.DoAPIStream(ctx, req, client.WithTimeout(defaultIMResourceDownloadTimeout), client.WithHeaders(headerValues))
		if err == nil {
			return resp, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		lastErr = err
		if attempt == imDownloadRequestRetries {
			break
		}
		sleepIMDownloadRetry(ctx, attempt)
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, output.ErrNetwork("download request failed")
}

func sleepIMDownloadRetry(ctx context.Context, attempt int) {
	delay := imDownloadRetryDelay * (1 << uint(attempt))
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func downloadResponseError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if len(body) > 0 {
		return output.ErrNetwork("download failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return output.ErrNetwork("download failed: HTTP %d", resp.StatusCode)
}

func parseTotalSize(contentRange string) (int64, error) {
	contentRange = strings.TrimSpace(contentRange)
	if contentRange == "" {
		return 0, fmt.Errorf("content-range is empty")
	}
	if !strings.HasPrefix(contentRange, "bytes ") {
		return 0, fmt.Errorf("unsupported content-range: %q", contentRange)
	}

	parts := strings.SplitN(strings.TrimPrefix(contentRange, "bytes "), "/", 2)
	if len(parts) != 2 || parts[1] == "" {
		return 0, fmt.Errorf("unsupported content-range: %q", contentRange)
	}
	if parts[0] == "*" {
		return 0, fmt.Errorf("unsupported content-range: %q", contentRange)
	}
	if parts[1] == "*" {
		return 0, fmt.Errorf("unknown total size in content-range: %q", contentRange)
	}

	totalSize, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse total size: %w", err)
	}
	if totalSize <= 0 {
		return 0, fmt.Errorf("invalid total size: %d", totalSize)
	}
	return totalSize, nil
}
