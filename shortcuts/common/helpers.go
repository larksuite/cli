// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"encoding/json"
	"io"
	"mime/multipart"
)

// MultipartWriter wraps multipart.Writer for file uploads. CreateFormFile is
// promoted from the embedded *multipart.Writer, which escapes special
// characters in the field name and filename — a filename like
// `report "draft".pdf` therefore round-trips through the Content-Disposition
// header instead of being truncated at the first unescaped quote.
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
