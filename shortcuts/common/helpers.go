// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/textproto"
	"os"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/vfs"
)

// MultipartWriter wraps multipart.Writer for file uploads.
type MultipartWriter struct {
	*multipart.Writer
}

// NewMultipartWriter creates a new MultipartWriter.
func NewMultipartWriter(w io.Writer) *MultipartWriter {
	return &MultipartWriter{multipart.NewWriter(w)}
}

// CreateFormFile creates a form file with the given field name and file name.
func (mw *MultipartWriter) CreateFormFile(fieldname, filename string) (io.Writer, error) {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="`+fieldname+`"; filename="`+filename+`"`)
	h.Set("Content-Type", "application/octet-stream")
	return mw.Writer.CreatePart(h)
}

// ParseJSON unmarshals JSON data into v.
func ParseJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// EnsureWritableFile refuses to overwrite an existing file unless overwrite is true.
func EnsureWritableFile(path string, overwrite bool) error {
	if overwrite {
		return nil
	}
	if _, err := vfs.Stat(path); err == nil {
		return output.ErrValidation("output file already exists: %s (use --overwrite to replace)", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return output.Errorf(output.ExitInternal, "io", "cannot access output path %s: %v", path, err)
	}
	return nil
}
