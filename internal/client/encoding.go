// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
)

// autoDecodeBody attempts to detect and convert non-UTF-8 response bodies to
// valid UTF-8. Some Lark APIs (e.g. whiteboard) occasionally return JSON with
// Chinese characters in encodings other than UTF-8 despite the Content-Type
// header claiming otherwise. See https://github.com/larksuite/cli/issues/1155
func autoDecodeBody(body []byte) []byte {
	// Fast path: already valid UTF-8 (common case).
	if utf8.Valid(body) {
		return body
	}

	// Strip UTF-8 BOM if present.
	body = bytes.TrimPrefix(body, unicode.UTF8BOM)

	// Try encodings in order of likelihood for Lark/Feishu APIs.
	candidates := []struct {
		enc encoding.Encoding
		name string
	}{
		{simplifiedchinese.GBK, "GBK"},
		{simplifiedchinese.GB18030, "GB18030"},
		{simplifiedchinese.HZGB2312, "HZ-GB2312"},
	}

	for _, c := range candidates {
		if decoded, err := c.enc.NewDecoder().Bytes(body); err == nil && utf8.Valid(decoded) {
			return decoded
		}
	}

	// Return original unchanged if no conversion succeeded.
	return body
}
