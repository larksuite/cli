// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"archive/zip"
	"bytes"
	"io"

	"github.com/larksuite/cli/extension/fileio"
)

// Size caps for the app-dev publish payload. Defaults pending server-side
// confirmation; vars (not consts) so unit tests can shrink them to cover the
// rejection paths.
var (
	// maxAppDevPublishRawBytes caps total uncompressed input, defending
	// against decompression-bomb style inputs before they balloon memory.
	maxAppDevPublishRawBytes int64 = 200 * 1024 * 1024
	// maxAppDevPublishZipBytes caps the packed zip payload.
	maxAppDevPublishZipBytes int64 = 50 * 1024 * 1024
)

// appDevZipball is an in-memory zip payload ready for TOS upload.
type appDevZipball struct {
	Body      []byte
	Size      int64
	FileCount int
}

// appDevPackEntry is one file of the normalized upload payload. ZipPath is
// the fixed protocol layout inside the zip (output/... for same-origin
// artifacts, output_resource/... for CDN artifacts) regardless of the
// project's directory names. Data comes from AbsPath, or from Content for
// CLI-generated files (a buildless routes.json).
type appDevPackEntry struct {
	ZipPath string
	AbsPath string
	Content []byte
	Size    int64
}

// buildAppDevZip packs the normalized entries into an in-memory zip: entry
// names are the fixed output/... and output_resource/... layout the hosting
// pipeline expects.
func buildAppDevZip(fio fileio.FileIO, entries []appDevPackEntry) (*appDevZipball, error) {
	var rawTotal int64
	for _, e := range entries {
		rawTotal += e.Size
	}
	if rawTotal > maxAppDevPublishRawBytes {
		return nil, appsValidationError(
			"publish payload total raw bytes %d exceeds %d bytes limit (uncompressed pre-pack cap)",
			rawTotal, maxAppDevPublishRawBytes).
			WithHint("reduce the artifact directory contents before publishing")
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.ZipPath)
		if err != nil {
			return nil, appsFileIOError(err, "zip create %s failed: %v", e.ZipPath, err)
		}
		if e.AbsPath == "" {
			if _, err := w.Write(e.Content); err != nil {
				return nil, appsFileIOError(err, "zip write %s failed: %v", e.ZipPath, err)
			}
			continue
		}
		f, err := fio.Open(e.AbsPath)
		if err != nil {
			return nil, appsInputPathEntryError(e.AbsPath, err)
		}
		_, err = io.Copy(w, f)
		f.Close()
		if err != nil {
			return nil, appsFileIOError(err, "zip write %s failed: %v", e.ZipPath, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, appsFileIOError(err, "zip finalize failed: %v", err)
	}
	size := int64(buf.Len())
	if size > maxAppDevPublishZipBytes {
		return nil, appsValidationError(
			"packed zip size %d bytes exceeds %d bytes limit", size, maxAppDevPublishZipBytes).
			WithHint("reduce the artifact directory contents; large media should be served from external storage")
	}
	return &appDevZipball{Body: buf.Bytes(), Size: size, FileCount: len(entries)}, nil
}
