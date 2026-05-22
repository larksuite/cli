// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"encoding/json"
	"io"
	"mime/multipart"
)

// MultipartWriter wraps multipart.Writer for file uploads. CreateFormFile is
// promoted from the embedded *multipart.Writer so that special characters in
// the filename (`"`, `\`, CR, LF) are properly escaped per the stdlib's
// quoteEscaper — otherwise filenames like `report "draft".pdf` would produce
// a malformed Content-Disposition header.
type MultipartWriter struct {
	*multipart.Writer
}

// NewMultipartWriter creates a new MultipartWriter.
func NewMultipartWriter(w io.Writer) *MultipartWriter {
	return &MultipartWriter{multipart.NewWriter(w)}
}

// ParseJSON unmarshals JSON data into v.
func ParseJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
