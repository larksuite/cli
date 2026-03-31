// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) {
	return 0, errors.New("boom")
}

func TestPrintJson_UsesInjectableErrorSink(t *testing.T) {
	var sink bytes.Buffer
	prev := SetErrorSink(&sink)
	t.Cleanup(func() { SetErrorSink(prev) })

	var out bytes.Buffer
	PrintJson(&out, map[string]interface{}{"bad": make(chan int)})

	if out.Len() != 0 {
		t.Fatalf("expected no stdout output on marshal error, got %q", out.String())
	}
	if !strings.Contains(sink.String(), "json marshal error:") {
		t.Fatalf("expected marshal failure in redirected sink, got %q", sink.String())
	}
}

func TestPrintNdjson_UsesInjectableErrorSink(t *testing.T) {
	var sink bytes.Buffer
	prev := SetErrorSink(&sink)
	t.Cleanup(func() { SetErrorSink(prev) })

	var out bytes.Buffer
	PrintNdjson(&out, []interface{}{map[string]interface{}{"bad": make(chan int)}})

	if out.Len() != 0 {
		t.Fatalf("expected no stdout output on marshal error, got %q", out.String())
	}
	if !strings.Contains(sink.String(), "ndjson marshal error:") {
		t.Fatalf("expected ndjson failure in redirected sink, got %q", sink.String())
	}
}

func TestFormatAsCSV_UsesInjectableErrorSink(t *testing.T) {
	var sink bytes.Buffer
	prev := SetErrorSink(&sink)
	t.Cleanup(func() { SetErrorSink(prev) })

	data := []interface{}{
		map[string]interface{}{"name": "Alice"},
	}

	FormatAsCSV(failingWriter{}, data)

	if !strings.Contains(sink.String(), "csv write error:") {
		t.Fatalf("expected csv failure in redirected sink, got %q", sink.String())
	}
}
