// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package rules

import (
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

func TestCheckTransportRetrySourceRejectsRoundTripLoop(t *testing.T) {
	src := []byte(`package transport
func (t *retrying) RoundTrip(req *http.Request) (*http.Response, error) {
	for attempt := 0; attempt < 3; attempt++ {
		resp, err := t.base.RoundTrip(req)
		if err == nil { return resp, nil }
	}
	return nil, errFailed
}`)

	diagnostics, err := checkTransportRetrySource("transport.go", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Rule != "transport_no_automatic_retry" {
		t.Fatalf("diagnostics = %#v, want one transport retry rejection", diagnostics)
	}
}

func TestCheckTransportRetrySourceAllowsSingleRequest(t *testing.T) {
	src := []byte(`package transport
func (t *single) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.base.RoundTrip(req)
}`)

	diagnostics, err := checkTransportRetrySource("transport.go", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diagnostics)
	}
}

func TestCheckTransportRetrySourceRequiresWaiverReason(t *testing.T) {
	for _, test := range []struct {
		name            string
		doc             string
		wantDiagnostics int
	}{
		{name: "reason supplied", doc: "// RoundTrip handles a protocol exception.\n// qualitygate:allow-roundtrip-retry required by RFC 9999", wantDiagnostics: 0},
		{name: "reason missing", doc: "// qualitygate:allow-roundtrip-retry", wantDiagnostics: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			src := []byte("package transport\n" + test.doc + `
func (t *exception) RoundTrip(req *http.Request) (*http.Response, error) {
	for {
		return t.base.RoundTrip(req)
	}
}`)
			diagnostics, err := checkTransportRetrySource("transport.go", src)
			if err != nil {
				t.Fatal(err)
			}
			if len(diagnostics) != test.wantDiagnostics {
				t.Fatalf("diagnostics = %#v, want %d", diagnostics, test.wantDiagnostics)
			}
		})
	}
}

func TestCheckTransportRetryLoopsScansRepository(t *testing.T) {
	repo := t.TempDir()
	path := filepath.Join(repo, "internal", "transport", "retry.go")
	if err := vfs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	src := []byte(`package transport
func (t *retrying) RoundTrip(req *http.Request) (*http.Response, error) {
	for range 2 { _, _ = t.base.RoundTrip(req) }
	return nil, nil
}`)
	if err := vfs.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}

	diagnostics, err := CheckTransportRetryLoops(repo, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || diagnostics[0].File != "internal/transport/retry.go" {
		t.Fatalf("diagnostics = %#v, want retry.go rejection", diagnostics)
	}
}
