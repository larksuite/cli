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

// buildAppDevZip packs candidates (paths relative to the dist dir, e.g.
// "output/index.html") into an in-memory zip. Entry names keep the
// output/... layout and never include the dist directory itself.
func buildAppDevZip(fio fileio.FileIO, candidates []htmlPublishCandidate) (*appDevZipball, error) {
	var rawTotal int64
	for _, c := range candidates {
		rawTotal += c.Size
	}
	if rawTotal > maxAppDevPublishRawBytes {
		return nil, appsValidationError(
			"dist total raw bytes %d exceeds %d bytes limit (uncompressed pre-pack cap)",
			rawTotal, maxAppDevPublishRawBytes).
			WithHint("reduce dist contents before publishing")
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, c := range candidates {
		w, err := zw.Create(c.RelPath)
		if err != nil {
			return nil, appsFileIOError(err, "zip create %s failed: %v", c.RelPath, err)
		}
		f, err := fio.Open(c.AbsPath)
		if err != nil {
			return nil, appsInputPathEntryError(c.AbsPath, err)
		}
		_, err = io.Copy(w, f)
		f.Close()
		if err != nil {
			return nil, appsFileIOError(err, "zip write %s failed: %v", c.RelPath, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, appsFileIOError(err, "zip finalize failed: %v", err)
	}
	size := int64(buf.Len())
	if size > maxAppDevPublishZipBytes {
		return nil, appsValidationError(
			"packed zip size %d bytes exceeds %d bytes limit", size, maxAppDevPublishZipBytes).
			WithHint("reduce dist contents; large media should be served from external storage")
	}
	return &appDevZipball{Body: buf.Bytes(), Size: size, FileCount: len(candidates)}, nil
}
