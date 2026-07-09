// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"io"
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
