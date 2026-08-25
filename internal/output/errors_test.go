// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

// failingWriter writes up to limit bytes then returns io.ErrShortWrite on
// the write that would push past the limit. Used to simulate a stderr that
// dies mid-envelope.
type failingWriter struct {
	limit int
	n     int
}

func TestWriteTypedErrorEnvelopeWithNoticeProvider_NilDoesNotReadGlobal(t *testing.T) {
	original := PendingNotice
	called := false
	PendingNotice = func() map[string]interface{} {
		called = true
		return map[string]interface{}{"source": "global"}
	}
	t.Cleanup(func() { PendingNotice = original })

	var out strings.Builder
	cause := errors.New("invalid input sentinel")
	err := errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid input").
		WithParam("--meeting-id").
		WithCause(cause)
	if ok := WriteTypedErrorEnvelopeWithNoticeProvider(&out, err, "user", nil); !ok {
		t.Fatal("typed error was not handled")
	}
	if called {
		t.Fatal("nil invocation provider consulted PendingNotice")
	}

	var raw map[string]json.RawMessage
	if decodeErr := json.Unmarshal([]byte(out.String()), &raw); decodeErr != nil {
		t.Fatalf("decode typed envelope: %v; output=%s", decodeErr, out.String())
	}
	if _, ok := raw["_notice"]; ok {
		t.Fatalf("nil invocation provider emitted notice: %s", out.String())
	}

	var envelope struct {
		OK       bool   `json:"ok"`
		Identity string `json:"identity"`
		Error    struct {
			Type    errs.Category `json:"type"`
			Subtype errs.Subtype  `json:"subtype"`
			Message string        `json:"message"`
			Param   string        `json:"param"`
		} `json:"error"`
	}
	if decodeErr := json.Unmarshal([]byte(out.String()), &envelope); decodeErr != nil {
		t.Fatalf("decode typed envelope fields: %v; output=%s", decodeErr, out.String())
	}
	if envelope.OK || envelope.Identity != "user" {
		t.Errorf("envelope ok=%v identity=%q, want false/user", envelope.OK, envelope.Identity)
	}
	if envelope.Error.Type != errs.CategoryValidation || envelope.Error.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("typed error = %s/%s, want %s/%s", envelope.Error.Type, envelope.Error.Subtype, errs.CategoryValidation, errs.SubtypeInvalidArgument)
	}
	if envelope.Error.Message != "invalid input" || envelope.Error.Param != "--meeting-id" {
		t.Errorf("typed error message=%q param=%q, want invalid input/--meeting-id", envelope.Error.Message, envelope.Error.Param)
	}

	// ValidationError causes are an in-process errors.Is/errors.Unwrap contract
	// and are intentionally excluded from the JSON envelope.
	if !errors.Is(err, cause) {
		t.Fatal("typed error did not preserve its cause")
	}
	var errorFields map[string]json.RawMessage
	if decodeErr := json.Unmarshal(raw["error"], &errorFields); decodeErr != nil {
		t.Fatalf("decode typed error fields: %v; output=%s", decodeErr, out.String())
	}
	wantErrorFields := map[string]bool{
		"type": true, "subtype": true, "message": true, "param": true,
	}
	for field := range errorFields {
		if !wantErrorFields[field] {
			t.Fatalf("typed envelope exposed unexpected error field %q: %s", field, out.String())
		}
	}
	if strings.Contains(out.String(), cause.Error()) {
		t.Fatalf("typed envelope exposed in-process cause value: %s", out.String())
	}
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
