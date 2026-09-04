// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"archive/zip"
	"bytes"
	"io"

	"github.com/larksuite/cli/extension/fileio"
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
	return &appDevZipball{Body: buf.Bytes(), Size: size, FileCount: len(entries)}, nil
}
