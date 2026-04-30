// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestJqFilterRaw_PreservesXMLInComplexValue(t *testing.T) {
	data := map[string]interface{}{
		"data": map[string]interface{}{
			"document": map[string]interface{}{
				"title":   "<title>hello & welcome</title>",
				"content": "<p>a < b & c > d</p>",
			},
		},
	}

	var raw bytes.Buffer
	if err := JqFilterRaw(&raw, data, ".data.document"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Raw path must keep <, >, & as literal characters, not Go json-encoder's
	// default < / > / & unicode escapes.
	for _, unicodeEsc := range []string{"\\u003c", "\\u003e", "\\u0026"} {
		if strings.Contains(raw.String(), unicodeEsc) {
			t.Errorf("JqFilterRaw unexpectedly HTML-escaped %s: %s", unicodeEsc, raw.String())
		}
	}
	if !strings.Contains(raw.String(), "<title>") {
		t.Errorf("JqFilterRaw dropped raw <title>: %s", raw.String())
	}

	var escaped bytes.Buffer
	if err := JqFilter(&escaped, data, ".data.document"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// JqFilter keeps Go's default HTML escaping for back-compat.
	if !strings.Contains(escaped.String(), "\\u003c") {
		t.Errorf("JqFilter should HTML-escape < for back-compat: %s", escaped.String())
	}
}

func TestJqFilterRaw_ScalarMatchesJqFilter(t *testing.T) {
	data := map[string]interface{}{"content": "<title>hello</title>"}

	var raw, plain bytes.Buffer
	if err := JqFilterRaw(&raw, data, ".content"); err != nil {
		t.Fatalf("raw: %v", err)
	}
	if err := JqFilter(&plain, data, ".content"); err != nil {
		t.Fatalf("plain: %v", err)
	}
	// Scalar string path is raw in both (matches jq -r), so output is identical.
	if raw.String() != plain.String() {
		t.Errorf("scalar output diverged: raw=%q plain=%q", raw.String(), plain.String())
	}
	if !strings.Contains(raw.String(), "<title>") {
		t.Errorf("scalar output dropped <title>: %q", raw.String())
	}
}
