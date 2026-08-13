// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestWriteTypedErrorEnvelopeUsesEditionCompatibleCLIOrigin(t *testing.T) {
	var out bytes.Buffer
	err := errs.NewValidationError(errs.SubtypeInvalidArgument, "bad flag")
	if ok := WriteTypedErrorEnvelope(&out, err, "user"); !ok {
		t.Fatal("typed error was not written")
	}
	var envelope struct {
		Error struct {
			Origin string `json:"origin"`
		} `json:"error"`
	}
	if decodeErr := json.Unmarshal(out.Bytes(), &envelope); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if envelope.Error.Origin != defaultErrorOrigin() {
		t.Fatalf("origin = %q, want %q", envelope.Error.Origin, defaultErrorOrigin())
	}
	var rawEnvelope struct {
		Error map[string]json.RawMessage `json:"error"`
	}
	if decodeErr := json.Unmarshal(out.Bytes(), &rawEnvelope); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	_, originPresent := rawEnvelope.Error["origin"]
	if wantPresent := defaultErrorOrigin() != ""; originPresent != wantPresent {
		t.Fatalf("origin field present = %v, want %v; envelope = %s", originPresent, wantPresent, out.Bytes())
	}
	if metadata, ok := errs.DiagnosticMetadataOf(err); ok {
		t.Fatalf("rendering mutated the reusable typed error: %#v", metadata)
	}
}

func TestWriteTypedErrorEnvelopeDoesNotUseInnerProducerMetadata(t *testing.T) {
	inner := errs.WithDiagnosticMetadata(
		errs.NewNetworkError(errs.SubtypeUpstreamUnavailable, "proxy unavailable"),
		errs.DiagnosticMetadata{
			Origin:         "proxy",
			ProxyRequestID: "proxy_req_inner",
		},
	)
	outer := errs.NewInternalError(errs.SubtypeUnknown, "business reclassified failure").
		WithCause(inner)

	var out bytes.Buffer
	if ok := WriteTypedErrorEnvelope(&out, outer, "bot"); !ok {
		t.Fatal("typed outer error was not written")
	}
	var envelope struct {
		Error struct {
			Type           errs.Category `json:"type"`
			Subtype        errs.Subtype  `json:"subtype"`
			Message        string        `json:"message"`
			Origin         string        `json:"origin"`
			ProxyRequestID string        `json:"proxy_request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Type != errs.CategoryInternal ||
		envelope.Error.Subtype != errs.SubtypeUnknown ||
		envelope.Error.Message != "business reclassified failure" {
		t.Fatalf("wire error used the wrong typed producer: %#v", envelope.Error)
	}
	if envelope.Error.Origin != defaultErrorOrigin() {
		t.Fatalf("origin = %q, want outer producer default %q", envelope.Error.Origin, defaultErrorOrigin())
	}
	if envelope.Error.ProxyRequestID != "" {
		t.Fatalf("outer producer inherited inner proxy request id %q", envelope.Error.ProxyRequestID)
	}
}

// failingWriter writes up to limit bytes then returns io.ErrShortWrite on
// the write that would push past the limit. Used to simulate a stderr that
// dies mid-envelope.
type failingWriter struct {
	limit int
	n     int
}

func (f *failingWriter) Write(p []byte) (int, error) {
	if f.n+len(p) > f.limit {
		canWrite := f.limit - f.n
		if canWrite < 0 {
			canWrite = 0
		}
		f.n += canWrite
		return canWrite, io.ErrShortWrite
	}
	f.n += len(p)
	return len(p), nil
}

// TestWriteTypedErrorEnvelope_PartialWritePreservesSuccessStatus pins that
// when serialization succeeds but the underlying write fails mid-envelope,
// WriteTypedErrorEnvelope returns true so the dispatcher honors the typed
// exit code instead of reclassifying the error. Exit code is preserved
// separately by handleRootError computing ExitCodeOf(err) before the write.
func TestWriteTypedErrorEnvelope_PartialWritePreservesSuccessStatus(t *testing.T) {
	err := errs.NewAuthenticationError(errs.SubtypeTokenExpired, "token expired")
	w := &failingWriter{limit: 20} // dies mid-envelope
	if ok := WriteTypedErrorEnvelope(w, err, "user"); !ok {
		t.Error("partial write must return true; exit code is preserved separately")
	}
}

func TestGetNotice(t *testing.T) {
	// Nil PendingNotice → nil
	origNotice := PendingNotice
	PendingNotice = nil
	if got := GetNotice(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}

	// With PendingNotice → returns value
	PendingNotice = func() map[string]interface{} {
		return map[string]interface{}{"update": "test"}
	}
	got := GetNotice()
	if got == nil || got["update"] != "test" {
		t.Errorf("expected {update: test}, got %v", got)
	}

	// PendingNotice returns nil → nil
	PendingNotice = func() map[string]interface{} { return nil }
	if got := GetNotice(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}

	PendingNotice = origNotice
}
