// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/citation"
)

func citationTestEmitter(out, errOut *bytes.Buffer) *Emitter {
	return NewEmitter(EmitterConfig{Out: out, ErrOut: errOut, CommandPath: "lark test +cmd", Identity: "user"})
}

func sampleCitations() []citation.Citation {
	return []citation.Citation{{SourceType: citation.SourceWiki, URL: "https://docs.example.com/wiki/tok", Title: "t"}}
}

func TestEmitEnvelopeInjectsCitations(t *testing.T) {
	var out, errOut bytes.Buffer
	e := citationTestEmitter(&out, &errOut)
	err := e.Success(map[string]any{"k": "v"}, EmitOptions{Citations: func() []citation.Citation { return sampleCitations() }})
	if err != nil {
		t.Fatal(err)
	}
	var env map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	raw, ok := env["citations"]
	if !ok {
		t.Fatalf("citations key missing: %s", out.String())
	}
	var items []citation.Citation
	if err := json.Unmarshal(raw, &items); err != nil || len(items) != 1 || items[0].SourceType != citation.SourceWiki {
		t.Fatalf("citations = %s", raw)
	}
}

func TestEmitEnvelopeNilProviderOmitsKey(t *testing.T) {
	var out, errOut bytes.Buffer
	e := citationTestEmitter(&out, &errOut)
	if err := e.Success(map[string]any{"k": "v"}, EmitOptions{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "citations") {
		t.Fatalf("citations key must be absent: %s", out.String())
	}
}

func TestEmitEnvelopeEmptyResultOmitsKey(t *testing.T) {
	var out, errOut bytes.Buffer
	e := citationTestEmitter(&out, &errOut)
	if err := e.Success(map[string]any{"k": "v"}, EmitOptions{Citations: func() []citation.Citation { return nil }}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "citations") {
		t.Fatalf("citations key must be absent: %s", out.String())
	}
}

func TestNonEnvelopeFormatsNeverInvokeProvider(t *testing.T) {
	probe := func() []citation.Citation { t.Fatal("provider must not be called"); return nil }
	data := []map[string]any{{"a": "1"}}
	for _, format := range []string{"table", "csv", "ndjson"} {
		var out, errOut bytes.Buffer
		e := citationTestEmitter(&out, &errOut)
		if err := e.Success(data, EmitOptions{Format: format, Citations: probe}); err != nil {
			t.Fatalf("format %s: %v", format, err)
		}
	}
	// pretty with renderer 也不构造
	var out, errOut bytes.Buffer
	e := citationTestEmitter(&out, &errOut)
	err := e.Success(data, EmitOptions{Format: "pretty", Citations: probe,
		Pretty: func(w io.Writer, _ bool) error { _, err := w.Write([]byte("ok\n")); return err }})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStreamPageHasNoCitationPath(t *testing.T) {
	// StreamOptions 没有 Citations 字段——本测试只固化流式页不经 emitEnvelope
	var out, errOut bytes.Buffer
	e := citationTestEmitter(&out, &errOut)
	if err := e.StreamPage([]map[string]any{{"a": "1"}}, StreamOptions{Format: "ndjson"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "citations") {
		t.Fatal("stream page must not carry citations")
	}
}

func TestJQCanFilterCitations(t *testing.T) {
	var out, errOut bytes.Buffer
	e := citationTestEmitter(&out, &errOut)
	err := e.Success(map[string]any{"k": "v"}, EmitOptions{JQ: ".citations | length", Citations: func() []citation.Citation { return sampleCitations() }})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "1" {
		t.Fatalf("jq .citations|length = %q, want 1", out.String())
	}
}

func TestPartialFailureEnvelopeInjectsCitations(t *testing.T) {
	var out, errOut bytes.Buffer
	e := citationTestEmitter(&out, &errOut)
	err := e.PartialFailure(map[string]any{"k": "v"}, EmitOptions{Citations: func() []citation.Citation { return sampleCitations() }})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "\"citations\"") {
		t.Fatalf("partial failure envelope missing citations: %s", out.String())
	}
}
