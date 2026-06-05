// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package yaml

import "io"

// bytesReader avoids pulling in bytes.NewReader at the call site -- yaml.v3
// only needs an io.Reader. Plain wrapper, no allocation surprises.
type byteReader struct {
	data []byte
	pos  int
}

func bytesReader(data []byte) io.Reader { return &byteReader{data: data} }

func (b *byteReader) Read(p []byte) (int, error) {
	if b.pos >= len(b.data) {
		return 0, io.EOF
	}
	n := copy(p, b.data[b.pos:])
	b.pos += n
	return n, nil
}
