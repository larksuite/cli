// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// DownloadExtensionResolution describes how a file extension was inferred.
type DownloadExtensionResolution struct {
	Ext    string
	Source string
	Detail string
}

var downloadMimeToExt = map[string]string{
	"application/msword":            ".doc",
	"application/pdf":               ".pdf",
	"application/vnd.ms-excel":      ".xls",
	"application/vnd.ms-powerpoint": ".ppt",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": ".pptx",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         ".xlsx",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   ".docx",
	"application/xml": ".xml",
	"application/zip": ".zip",
	"image/bmp":       ".bmp",
	"image/gif":       ".gif",
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/svg+xml":   ".svg",
	"image/webp":      ".webp",
	"text/csv":        ".csv",
	"text/html":       ".html",
	"text/plain":      ".txt",
	"text/xml":        ".xml",
	"video/mp4":       ".mp4",
}

// ResolveDownloadFileName returns a sanitized filename from Content-Disposition,
// falling back to the caller-provided name when the header is absent or invalid.
func ResolveDownloadFileName(header http.Header, fallback string) string {
	name := strings.TrimSpace(larkcore.FileNameByHeader(header))
	if name == "" {
		name = fallback
	}
	name = strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	name = path.Base(name)
	if name == "" || name == "." || name == ".." {
		return fallback
	}
	return name
}

// AutoAppendDownloadExtension appends an inferred file extension when the
// target path has no explicit suffix. If no extension can be inferred, the
// original basename is preserved without adding a synthetic fallback suffix.
func AutoAppendDownloadExtension(outputPath string, header http.Header, fallbackExt string) (string, *DownloadExtensionResolution) {
	if hasExplicitDownloadExtension(outputPath) {
		return outputPath, nil
	}
	normalizedPath := outputPath
	if filepath.Ext(outputPath) == "." {
		normalizedPath = strings.TrimSuffix(outputPath, ".")
	}
	if resolution := downloadExtensionByContentType(header.Get("Content-Type")); resolution != nil {
		return normalizedPath + resolution.Ext, resolution
	}
	if resolution := downloadExtensionByContentDisposition(header); resolution != nil {
		return normalizedPath + resolution.Ext, resolution
	}
	if fallbackExt != "" {
		return normalizedPath + fallbackExt, &DownloadExtensionResolution{
			Ext:    fallbackExt,
			Source: "fallback",
			Detail: "default fallback",
		}
	}
	return normalizedPath, nil
}

func hasExplicitDownloadExtension(path string) bool {
	ext := filepath.Ext(path)
	return ext != "" && ext != "."
}

func downloadExtensionByContentType(contentType string) *DownloadExtensionResolution {
	if contentType == "" {
		return nil
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	}
	if ext, ok := downloadMimeToExt[strings.ToLower(mediaType)]; ok {
		return &DownloadExtensionResolution{
			Ext:    ext,
			Source: "Content-Type",
			Detail: contentType,
		}
	}
	return nil
}

func downloadExtensionByContentDisposition(header http.Header) *DownloadExtensionResolution {
	filename := strings.TrimSpace(larkcore.FileNameByHeader(header))
	if filename == "" {
		return nil
	}
	ext := filepath.Ext(filename)
	if ext == "" || ext == "." {
		return nil
	}
	return &DownloadExtensionResolution{
		Ext:    ext,
		Source: "Content-Disposition",
		Detail: filename,
	}
}
