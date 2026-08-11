// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package download

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestExactLengthReaderAcceptsExactBody(t *testing.T) {
	body := newExactLengthReader(io.NopCloser(strings.NewReader("abcd")), 4)
	got, err := io.ReadAll(body)
	if err != nil || string(got) != "abcd" {
		t.Fatalf("ReadAll() = %q, %v", got, err)
	}
}

func TestExactLengthReaderRejectsShortBody(t *testing.T) {
	body := newExactLengthReader(io.NopCloser(strings.NewReader("abc")), 4)
	got, err := io.ReadAll(body)
	if string(got) != "abc" {
		t.Fatalf("payload = %q, want abc", got)
	}
	requireProblem(t, err, errs.SubtypeNetworkProtocol, false, "ended after 3 of 4 bytes")
}

func TestExactLengthReaderRejectsLongBodyWithoutExposingExtraBytes(t *testing.T) {
	body := newExactLengthReader(io.NopCloser(bytes.NewReader([]byte("abcde"))), 4)
	got, err := io.ReadAll(body)
	if string(got) != "abcd" {
		t.Fatalf("payload = %q, want only the declared bytes", got)
	}
	requireProblem(t, err, errs.SubtypeNetworkProtocol, false, "exceeded its declared length")
}

func TestExactLengthReaderAllowsZeroLengthReads(t *testing.T) {
	body := newExactLengthReader(io.NopCloser(strings.NewReader("a")), 1)
	if n, err := body.Read(nil); n != 0 || err != nil {
		t.Fatalf("Read(nil) = %d, %v", n, err)
	}
}

func TestExactLengthReaderRejectsNoProgress(t *testing.T) {
	body := newExactLengthReader(nopReadCloser{Reader: noProgressReader{}}, 1)
	_, err := body.Read(make([]byte, 1))
	requireProblem(t, err, errs.SubtypeNetworkProtocol, false, "made no progress")
}

func TestExactLengthReaderRejectsInvalidReadCount(t *testing.T) {
	body := newExactLengthReader(nopReadCloser{Reader: invalidCountReader{}}, 1)
	_, err := body.Read(make([]byte, 1))
	requireProblem(t, err, errs.SubtypeNetworkProtocol, false, "invalid read count")
}

type noProgressReader struct{}

func (noProgressReader) Read([]byte) (int, error) { return 0, nil }

type invalidCountReader struct{}

func (invalidCountReader) Read(p []byte) (int, error) { return len(p) + 1, nil }

type nopReadCloser struct{ io.Reader }

func (nopReadCloser) Close() error { return nil }
