// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	htmlcharset "golang.org/x/net/html/charset"
	"golang.org/x/text/transform"
)

func isTextualMediaType(mediaType string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(mediaType)), "text/")
}

func normalizeCharsetLabel(label string) string {
	label = strings.TrimSpace(label)
	label = strings.Trim(label, `"`)
	return label
}

func isUTF8LikeCharset(label string) bool {
	switch strings.ToLower(normalizeCharsetLabel(label)) {
	case "", "utf-8", "utf8", "us-ascii", "ascii":
		return true
	default:
		return false
	}
}

func decodeTextCharset(body []byte, label string) ([]byte, error) {
	if isUTF8LikeCharset(label) {
		return body, nil
	}
	reader, err := htmlcharset.NewReaderLabel(label, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(reader)
}

func encodeTextCharset(body []byte, label string) ([]byte, error) {
	if isUTF8LikeCharset(label) {
		return body, nil
	}
	enc, _ := htmlcharset.Lookup(label)
	if enc == nil {
		return nil, fmt.Errorf("unsupported charset %q", label) //nolint:forbidigo // intermediate draft charset error; mail command layer wraps into typed ValidationError.
	}
	var buf bytes.Buffer
	writer := transform.NewWriter(&buf, enc.NewEncoder())
	if _, err := writer.Write(body); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
