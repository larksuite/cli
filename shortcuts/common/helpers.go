// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"encoding/json"
	"io"
	"mime/multipart"
	"net/textproto"
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
