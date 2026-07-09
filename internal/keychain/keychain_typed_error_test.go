// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keychain

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
)

// TestWrapErrorEmitsTypedAPIError pins the wrapError contract after the typed
// errs migration: keychain failures surface as *errs.APIError with subtype
// "unknown", exit code 1 (ExitAPI, unchanged from the legacy behavior), a
// non-empty troubleshooting hint, and the underlying error reachable via
// errors.Unwrap.
func TestWrapErrorEmitsTypedAPIError(t *testing.T) {
	underlying := errors.New("keyring backend exploded")
	err := wrapError("Set", underlying)

	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("wrapError returned %T (%v); expected *errs.APIError", err, err)
	}
	if apiErr.Subtype != errs.SubtypeUnknown {
		t.Errorf("subtype = %q, want %q", apiErr.Subtype, errs.SubtypeUnknown)
	}
	if got := output.ExitCodeOf(err); got != output.ExitAPI {
		t.Errorf("exit code = %d, want %d (ExitAPI, legacy parity)", got, output.ExitAPI)
	}
	if !strings.Contains(apiErr.Message, "keychain Set failed") {
		t.Errorf("message = %q, want it to contain %q", apiErr.Message, "keychain Set failed")
	}
	if apiErr.Hint == "" {
		t.Error("hint is empty; wrapError must carry a troubleshooting hint")
	}
	if !errors.Is(err, underlying) {
		t.Error("underlying error not reachable via errors.Is; WithCause missing")
	}
}

// TestWrapErrorPassthrough pins the non-wrapping paths: nil stays nil and
// ErrNotFound is forwarded untouched so callers can keep using errors.Is.
func TestWrapErrorPassthrough(t *testing.T) {
	if err := wrapError("Get", nil); err != nil {
		t.Errorf("wrapError(nil) = %v, want nil", err)
	}
	if err := wrapError("Get", ErrNotFound); !errors.Is(err, ErrNotFound) {
		t.Errorf("wrapError(ErrNotFound) = %v, want ErrNotFound passthrough", err)
	}
	var apiErr *errs.APIError
	if err := wrapError("Get", ErrNotFound); errors.As(err, &apiErr) {
		t.Errorf("wrapError(ErrNotFound) wrapped into %T; want passthrough", apiErr)
	}
}
