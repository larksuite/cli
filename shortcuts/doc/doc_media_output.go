// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

var docMediaMimeToExt = map[string]string{
	"image/png":       ".png",
	"image/jpeg":      ".jpg",
	"image/gif":       ".gif",
	"image/webp":      ".webp",
	"image/svg+xml":   ".svg",
	"application/pdf": ".pdf",
	"video/mp4":       ".mp4",
	"text/plain":      ".txt",
	"text/csv":        ".csv",
	"text/html":       ".html",
	"application/zip": ".zip",

	"application/msword":            ".doc",
	"application/vnd.ms-excel":      ".xls",
	"application/vnd.ms-powerpoint": ".ppt",

	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   ".docx",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         ".xlsx",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": ".pptx",
}

func resolveDocMediaOutputPath(outputPath, token string, headers http.Header, defaultExt string) string {
	if isDirectoryOutputTarget(outputPath) {
		name := docMediaFilenameFromHeader(headers)
		if name == "" {
			name = token + docMediaExtFromHeaders(headers, defaultExt)
		}
		return filepath.Join(outputPath, name)
	}

	if filepath.Ext(outputPath) != "" {
		return outputPath
	}

	if ext := filepath.Ext(docMediaFilenameFromHeader(headers)); ext != "" {
		return outputPath + ext
	}

	if ext := docMediaExtFromHeaders(headers, defaultExt); ext != "" {
		return outputPath + ext
	}
	return outputPath
}

func docMediaFilenameFromHeader(headers http.Header) string {
	name := larkcore.FileNameByHeader(headers)
	if name == "" {
		return ""
	}
	return sanitizeDocMediaFilename(name)
}

func docMediaExtFromHeaders(headers http.Header, defaultExt string) string {
	contentType := headers.Get("Content-Type")
	if contentType == "" {
		return defaultExt
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return defaultExt
	}
	if ext, ok := docMediaMimeToExt[mediaType]; ok {
		return ext
	}
	return defaultExt
}

func isDirectoryOutputTarget(outputPath string) bool {
	trimmed := strings.TrimSpace(outputPath)
	if trimmed == "." {
		return true
	}
	if strings.HasSuffix(trimmed, "/") || strings.HasSuffix(trimmed, string(filepath.Separator)) {
		return true
	}

	safePath, err := validate.SafeOutputPath(outputPath)
	if err != nil {
		return false
	}

	info, err := vfs.Stat(safePath)
	return err == nil && info.IsDir()
}

func sanitizeDocMediaFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimSpace(filepath.Base(name))
	if name == "" || name == "." || name == "/" {
		return ""
	}

	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_",
		"\n", "_", "\r", "_", "\t", "_", "\x00", "_",
	)
	name = replacer.Replace(name)
	name = strings.Trim(name, ". ")
	return name
}
