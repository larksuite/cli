// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
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
		{Name: "output", Desc: "local save path (relative only, no .. traversal; defaults to file_key)"},
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
		if _, err := validate.SafeOutputPath(relPath); err != nil {
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
		safePath, err := validate.SafeOutputPath(relPath)
		if err != nil {
			return output.ErrValidation("unsafe output path: %s", err)
		}

		finalPath, sizeBytes, err := downloadIMResourceToPath(ctx, runtime, messageId, fileKey, fileType, safePath)
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

const defaultIMResourceDownloadTimeout = 120 * time.Second

var imMimeToExt = map[string]string{
	"image/png":                   ".png",
	"image/jpeg":                  ".jpg",
	"image/gif":                   ".gif",
	"image/webp":                  ".webp",
	"image/svg+xml":               ".svg",
	"application/pdf":             ".pdf",
	"video/mp4":                   ".mp4",
	"video/3gpp":                  ".3gp",
	"video/x-msvideo":             ".avi",
	"audio/mpeg":                  ".mp3",
	"audio/ogg":                   ".ogg",
	"audio/wav":                   ".wav",
	"text/plain":                  ".txt",
	"text/html":                   ".html",
	"text/css":                    ".css",
	"text/csv":                    ".csv",
	"application/zip":             ".zip",
	"application/x-zip-compressed": ".zip",
	"application/x-rar-compressed": ".rar",
	"application/json":            ".json",
	"application/xml":             ".xml",
	"application/octet-stream":    ".bin",
	"application/msword":          ".doc",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": ".docx",
	"application/vnd.ms-excel":    ".xls",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": ".xlsx",
	"application/vnd.ms-powerpoint": ".ppt",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": ".pptx",
}

func downloadIMResourceToPath(ctx context.Context, runtime *common.RuntimeContext, messageID, fileKey, fileType, safePath string) (string, int64, error) {
	query := larkcore.QueryParams{}
	query.Set("type", fileType)
	downloadResp, err := runtime.DoAPIStream(ctx, &larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    "/open-apis/im/v1/messages/:message_id/resources/:file_key",
		PathParams: larkcore.PathParams{
			"message_id": messageID,
			"file_key":   fileKey,
		},
		QueryParams: query,
	}, client.WithTimeout(defaultIMResourceDownloadTimeout))
	if err != nil {
		return "", 0, err
	}
	defer downloadResp.Body.Close()

	if err := vfs.MkdirAll(filepath.Dir(safePath), 0700); err != nil {
		return "", 0, output.Errorf(output.ExitInternal, "api_error", "cannot create parent directory: %s", err)
	}

	// Auto-detect extension from Content-Type if missing
	finalPath := safePath
	if filepath.Ext(safePath) == "" {
		contentType := downloadResp.Header.Get("Content-Type")
		mimeType := strings.Split(contentType, ";")[0]
		mimeType = strings.TrimSpace(mimeType)
		if ext, ok := imMimeToExt[mimeType]; ok {
			finalPath = safePath + ext
		}
	}

	sizeBytes, err := validate.AtomicWriteFromReader(finalPath, downloadResp.Body, 0600)
	if err != nil {
		return "", 0, output.Errorf(output.ExitInternal, "api_error", "cannot create file: %s", err)
	}
	return finalPath, sizeBytes, nil
}
