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

func (failingWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("boom")
}

func TestPrintJson_UsesInjectedErrorWriter(t *testing.T) {
	var stderr bytes.Buffer
	restore := SetErrorWriter(&stderr)
	t.Cleanup(restore)

	var stdout bytes.Buffer
	PrintJson(&stdout, map[string]interface{}{"bad": make(chan int)})

	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout output on marshal error, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "json marshal error:") {
		t.Fatalf("expected marshal error on injected stderr, got %q", stderr.String())
	}
}

func TestPrintNdjson_UsesInjectedErrorWriter(t *testing.T) {
	var stderr bytes.Buffer
	restore := SetErrorWriter(&stderr)
	t.Cleanup(restore)

	var stdout bytes.Buffer
	PrintNdjson(&stdout, []interface{}{make(chan int)})

	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout output on marshal error, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "ndjson marshal error:") {
		t.Fatalf("expected ndjson marshal error on injected stderr, got %q", stderr.String())
	}
}

func TestFormatAsCSV_UsesInjectedErrorWriter(t *testing.T) {
	var stderr bytes.Buffer
	restore := SetErrorWriter(&stderr)
	t.Cleanup(restore)

	FormatAsCSV(failingWriter{}, []interface{}{
		map[string]interface{}{"name": "Alice"},
	})

	if !strings.Contains(stderr.String(), "csv write error:") {
		t.Fatalf("expected csv write error on injected stderr, got %q", stderr.String())
	}
}
