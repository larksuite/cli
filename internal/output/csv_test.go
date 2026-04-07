// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestFormatAsCSV_BasicArray(t *testing.T) {
	data := []interface{}{
		map[string]interface{}{"name": "Alice", "age": float64(30)},
		map[string]interface{}{"name": "Bob", "age": float64(25)},
	}

	var buf bytes.Buffer
	FormatAsCSV(&buf, data)
	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + 2 rows), got %d:\n%s", len(lines), out)
	}

	// Header should contain both column names
	header := lines[0]
	if !strings.Contains(header, "name") || !strings.Contains(header, "age") {
		t.Errorf("header should contain 'name' and 'age', got: %s", header)
	}
}

func TestFormatAsCSV_RFC4180Escaping(t *testing.T) {
	data := []interface{}{
		map[string]interface{}{
			"text": `hello, "world"`,
		},
	}

	var buf bytes.Buffer
	FormatAsCSV(&buf, data)
	out := buf.String()

	// RFC 4180: fields with commas/quotes are quoted, internal quotes are doubled
	if !strings.Contains(out, `"hello, ""world"""`) {
		t.Errorf("CSV should properly escape commas and quotes, got:\n%s", out)
	}
}

func TestFormatAsCSV_NewlineInValue(t *testing.T) {
	data := []interface{}{
		map[string]interface{}{
			"text": "line1\nline2",
		},
	}

	var buf bytes.Buffer
	FormatAsCSV(&buf, data)
	out := buf.String()

	// RFC 4180: fields with newlines should be quoted
	if !strings.Contains(out, `"line1`) {
		t.Errorf("CSV should quote fields containing newlines, got:\n%s", out)
	}
}

func TestFormatAsCSV_NestedObject(t *testing.T) {
	data := []interface{}{
		map[string]interface{}{
			"user": map[string]interface{}{
				"name": "Alice",
			},
			"id": float64(1),
		},
	}

	var buf bytes.Buffer
	FormatAsCSV(&buf, data)
	out := buf.String()

	if !strings.Contains(out, "user.name") {
		t.Errorf("CSV should contain flattened 'user.name' column, got:\n%s", out)
	}
}

func TestFormatAsCSV_EmptyArray(t *testing.T) {
	data := []interface{}{}

	var buf bytes.Buffer
	FormatAsCSV(&buf, data)
	out := strings.TrimSpace(buf.String())

	if out != "(empty)" {
		t.Errorf("empty array should output '(empty)', got:\n%s", out)
	}
}

func TestFormatAsCSVPaginated_FirstPage(t *testing.T) {
	data := []interface{}{
		map[string]interface{}{"name": "Alice"},
	}

	var buf bytes.Buffer
	FormatAsCSVPaginated(&buf, data, true)
	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	if len(lines) != 2 {
		t.Errorf("first page should have header + 1 data row, got %d lines:\n%s", len(lines), out)
	}
	if lines[0] != "name" {
		t.Errorf("first line should be header 'name', got: %s", lines[0])
	}
}

func TestFormatAsCSVPaginated_ContinuationPage(t *testing.T) {
	data := []interface{}{
		map[string]interface{}{"name": "Bob"},
	}

	var buf bytes.Buffer
	FormatAsCSVPaginated(&buf, data, false)
	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	if len(lines) != 1 {
		t.Errorf("continuation page should have 1 data row, got %d lines:\n%s", len(lines), out)
	}
	if lines[0] != "Bob" {
		t.Errorf("continuation page data should be 'Bob', got: %s", lines[0])
	}
}

func TestFormatAsCSV_SingleObject(t *testing.T) {
	data := map[string]interface{}{
		"name": "Alice",
		"age":  float64(30),
	}

	var buf bytes.Buffer
	FormatAsCSV(&buf, data)
	out := buf.String()

	// Single object should render as key,value format
	if !strings.Contains(out, "key,value") {
		t.Errorf("single object should have key,value header, got:\n%s", out)
	}
	if !strings.Contains(out, "Alice") {
		t.Errorf("output should contain 'Alice', got:\n%s", out)
	}
}

type failingCSVWriter struct{}

func (failingCSVWriter) Write(p []byte) (int, error) {
	return 0, errors.New("boom")
}

func TestFormatAsCSV_WriteError_UsesErrorSink(t *testing.T) {
	origSink := ErrorSink
	var errBuf bytes.Buffer
	ErrorSink = &errBuf
	defer func() { ErrorSink = origSink }()

	data := []interface{}{
		map[string]interface{}{"name": "Alice"},
	}

	FormatAsCSV(failingCSVWriter{}, data)

	if got := errBuf.String(); got == "" || !strings.Contains(got, "csv write error: boom") {
		t.Fatalf("expected csv write error in ErrorSink, got: %q", got)
	}
}
