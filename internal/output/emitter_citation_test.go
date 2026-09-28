// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	extcs "github.com/larksuite/cli/extension/contentsafety"
	"github.com/larksuite/cli/internal/citation"
)

func citationTestEmitter(out, errOut *bytes.Buffer) *Emitter {
	return NewEmitter(EmitterConfig{Out: out, ErrOut: errOut, CommandPath: "lark test +cmd", Identity: "user"})
}

func sampleCitations() []citation.Citation {
	return []citation.Citation{{SourceType: citation.SourceWiki, URL: "https://docs.example/wiki/tok", Title: "t"}}
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
	var items []string
	if err := json.Unmarshal(raw, &items); err != nil || len(items) != 1 {
		t.Fatalf("citations = %s", raw)
	}
	want := `<document reference_id="https://docs.example/wiki/tok"><title>t</title><source_type>1</source_type><url>https://docs.example/wiki/tok</url></document>`
	if items[0] != want {
		t.Fatalf("citations[0] = %q, want %q", items[0], want)
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

func TestEmitEnvelopeWithCitationsSkipsHTMLEscaping(t *testing.T) {
	var out, errOut bytes.Buffer
	e := citationTestEmitter(&out, &errOut)
	err := e.Success(map[string]any{"k": "<b>A&B</b>"}, EmitOptions{Citations: func() []citation.Citation { return sampleCitations() }})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `<document reference_id=`) || strings.Contains(out.String(), `\u003c`) {
		t.Fatalf("envelope with citations must carry raw XML bytes: %s", out.String())
	}
}

func TestEmitEnvelopeWithoutCitationsKeepsHTMLEscaping(t *testing.T) {
	var out, errOut bytes.Buffer
	e := citationTestEmitter(&out, &errOut)
	if err := e.Success(map[string]any{"k": "<b>A&B</b>"}, EmitOptions{Citations: func() []citation.Citation { return nil }}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `\u003cb\u003eA\u0026B\u003c/b\u003e`) {
		t.Fatalf("envelope without citations must keep default HTML escaping: %s", out.String())
	}
}

func TestUsesEnvelopeMatchesSuccessDispatch(t *testing.T) {
	for _, tc := range []struct {
		format, jq     string
		renderer, want bool
	}{
		{want: true}, {format: "json", want: true}, {format: "JSON", want: true},
		{format: "unknown", want: true}, {format: "PRETTY", renderer: true, want: true},
		{format: "pretty", want: true}, {format: "pretty", renderer: true},
		{format: "pretty", jq: ".", renderer: true, want: true},
		{format: "table"}, {format: "TABLE"}, {format: "csv"}, {format: "ndjson"},
		{format: "table", jq: ".", want: true},
	} {
		t.Run(tc.format+"/"+tc.jq, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
			if got := UsesEnvelope(tc.format, tc.jq, tc.renderer); got != tc.want {
				t.Fatalf("UsesEnvelope = %t, want %t", got, tc.want)
			}
			built := false
			opts := EmitOptions{Format: tc.format, JQ: tc.jq, Citations: func() []citation.Citation { built = true; return nil }}
			if tc.renderer {
				opts.Pretty = func(io.Writer, bool) error { return nil }
			}
			err := NewEmitter(EmitterConfig{Out: io.Discard}).Success([]struct {
				Title string `json:"title"`
			}{{"fixture"}}, opts)
			if err != nil || built != tc.want {
				t.Fatalf("emitter built=%t want=%t err=%v", built, tc.want, err)
			}
		})
	}
}

type routeSafetyProvider struct{ calls atomic.Int32 }

func (*routeSafetyProvider) Name() string { return "route-fixture" }
func (p *routeSafetyProvider) Scan(context.Context, extcs.ScanRequest) (*extcs.Alert, error) {
	p.calls.Add(1)
	return &extcs.Alert{Provider: "route-fixture", MatchedRules: []string{"fixture"}}, nil
}

func TestPrettyFallbackPreservesSafetyPath(t *testing.T) {
	for _, mode := range []string{"warn", "block"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", mode)
			provider := &routeSafetyProvider{}
			previous := extcs.GetProvider()
			extcs.Register(provider)
			t.Cleanup(func() { extcs.Register(previous) })
			var stdout, stderr bytes.Buffer
			built := false
			err := NewEmitter(EmitterConfig{Out: &stdout, ErrOut: &stderr, CommandPath: "lark-cli fixture +emit"}).Success("fixture", EmitOptions{
				Format: "pretty", Citations: func() []citation.Citation { built = true; return nil },
			})
			if mode == "block" {
				if err == nil || provider.calls.Load() != 1 || built || stdout.Len() != 0 {
					t.Fatalf("blocked output: scans=%d built=%t err=%v stdout=%s", provider.calls.Load(), built, err, stdout.String())
				}
			} else if err != nil || provider.calls.Load() != 2 || !built || strings.Count(stderr.String(), "warning: content safety alert") != 1 || !strings.Contains(stdout.String(), "content_safety_alert") {
				t.Fatalf("pretty fallback: scans=%d built=%t err=%v stdout=%s stderr=%s", provider.calls.Load(), built, err, stdout.String(), stderr.String())
			}
		})
	}
}
