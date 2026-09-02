// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package download

import (
	"io"
)

// exactLengthReader rejects truncated and overlong bodies.
type exactLengthReader struct {
	source    io.ReadCloser
	expected  int64
	delivered int64
	checked   bool
}

func newExactLengthReader(source io.ReadCloser, expected int64) io.ReadCloser {
	if expected < 0 {
		return source
	}
	return &exactLengthReader{source: source, expected: expected}
}

func (r *exactLengthReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	remaining := r.expected - r.delivered
	if remaining < 0 {
		return 0, protocolError("download exceeded its declared length of %d bytes", r.expected)
	}
	if remaining == 0 {
		if r.checked {
			return 0, io.EOF
		}
		r.checked = true
		var probe [1]byte
		n, err := r.source.Read(probe[:])
		if n > 0 {
			return 0, protocolError("download exceeded its declared length of %d bytes", r.expected)
		}
		if err == nil {
			return 0, protocolError("download source made no progress after %d bytes", r.expected)
		}
		if err != io.EOF {
			return 0, err
		}
		return 0, io.EOF
	}

	if int64(len(p)) > remaining {
		p = p[:remaining]
	}
	n, err := r.source.Read(p)
	if n < 0 || n > len(p) {
		return 0, protocolError("download source returned invalid read count %d for a %d-byte buffer", n, len(p))
	}
	r.delivered += int64(n)
	if n == 0 && err == nil {
		return 0, protocolError("download source made no progress after %d of %d bytes", r.delivered, r.expected)
	}
	if err == io.EOF && r.delivered < r.expected {
		return n, protocolError("download ended after %d of %d bytes", r.delivered, r.expected)
	}
	return n, err
}

func (r *exactLengthReader) Close() error {
	return r.source.Close()
}
